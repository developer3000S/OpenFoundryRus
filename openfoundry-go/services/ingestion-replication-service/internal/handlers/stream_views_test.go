package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

func seedIngestStream(store *fakeStore, owner uuid.UUID) models.StreamDefinition {
	now := time.Now().UTC()
	s := models.StreamDefinition{
		ID:                   uuid.New(),
		Name:                 "orders",
		Status:               "active",
		Schema:               []byte(`{"fields":[]}`),
		SourceBinding:        []byte(`{"connector_type":"kafka"}`),
		RetentionHours:       72,
		Partitions:           3,
		ConsistencyGuarantee: "at-least-once",
		StreamType:           "STANDARD",
		IngestConsistency:    "AT_LEAST_ONCE",
		PipelineConsistency:  "AT_LEAST_ONCE",
		CheckpointIntervalMS: 2000,
		Kind:                 models.StreamKindIngest,
		OwnerID:              owner,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	store.streams[s.ID] = s
	return s
}

func writableClaims(sub uuid.UUID) *authmw.Claims {
	return &authmw.Claims{Sub: sub, Email: "ops@example.com", Roles: []string{"streaming_admin"}}
}

func resetReq(method, target, body string, claims *authmw.Claims) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if claims != nil {
		req = req.WithContext(authmw.ContextWithClaims(context.Background(), claims))
	}
	return req
}

func TestResetStreamRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{Repo: newFakeStore()}
	rec := httptest.NewRecorder()
	h.ResetStream(rec, httptest.NewRequest("POST", "/streams/x:reset", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestResetStreamForbidsCallerWithoutPermission(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{Repo: newFakeStore()}
	c := &authmw.Claims{Sub: uuid.New(), Roles: []string{"viewer"}}
	rec := httptest.NewRecorder()
	h.ResetStream(rec, resetReq("POST", "/streams/x:reset", "{}", c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestResetStreamRotatesViewAndCallsRuntime(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore()
	s := seedIngestStream(store, owner)
	rt := &fakeRuntime{}
	h := &handlers.Handlers{
		Repo:    store,
		Runtime: rt,
		PushURL: &handlers.PushURLBuilder{BaseURL: "https://stream.example.com"},
	}
	req := withRouteParam(resetReq("POST", "/streams/"+s.ID.String()+":reset", "{}", writableClaims(owner)), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.ResetStream(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body models.ResetStreamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, models.StreamRIDFor(s.ID), body.StreamRID)
	assert.Equal(t, body.NewViewRID, body.OldViewRID, "first reset has no previous view")
	assert.Equal(t, int32(1), body.Generation)
	assert.True(t, strings.HasPrefix(body.PushURL, "https://stream.example.com/streams-push/"))
	assert.Equal(t, 1, rt.resetCalls)
}

func TestResetStreamRejectsDerivedKind(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore()
	s := seedIngestStream(store, owner)
	s.Kind = models.StreamKindDerived
	store.streams[s.ID] = s
	h := &handlers.Handlers{Repo: store, Runtime: &fakeRuntime{}}
	req := withRouteParam(resetReq("POST", "/streams/"+s.ID.String()+":reset", "{}", writableClaims(owner)), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.ResetStream(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), handlers.ErrResetRequiresIngest)
}

func TestResetStreamConflictWhenDownstreamActiveAndNotForced(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore()
	s := seedIngestStream(store, owner)
	store.downstreamActive[s.ID] = true
	h := &handlers.Handlers{Repo: store, Runtime: &fakeRuntime{}}
	req := withRouteParam(resetReq("POST", "/streams/"+s.ID.String()+":reset", "{}", writableClaims(owner)), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.ResetStream(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), handlers.ErrResetDownstreamActive)
}

func TestResetStreamForceBypassesDownstreamGuard(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore()
	s := seedIngestStream(store, owner)
	store.downstreamActive[s.ID] = true
	h := &handlers.Handlers{Repo: store, Runtime: &fakeRuntime{}}
	req := withRouteParam(resetReq("POST", "/streams/"+s.ID.String()+":reset", `{"force":true}`, writableClaims(owner)), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.ResetStream(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body models.ResetStreamResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Forced)
}

func TestResetStreamRuntimeFailureIsBestEffort(t *testing.T) {
	// The metadata mutation already commits before the runtime call.
	// When Kafka is briefly unreachable the response still succeeds —
	// operators retry the topic reset out of band. Mirrors the Rust
	// hot_buffer warn-and-continue branch.
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore()
	s := seedIngestStream(store, owner)
	rt := &fakeRuntime{resetErr: errors.New("kafka unreachable")}
	h := &handlers.Handlers{Repo: store, Runtime: rt}
	req := withRouteParam(resetReq("POST", "/streams/"+s.ID.String()+":reset", "{}", writableClaims(owner)), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.ResetStream(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, rt.resetCalls)
}

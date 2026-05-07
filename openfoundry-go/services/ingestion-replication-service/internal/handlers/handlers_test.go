package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

func TestIngestJobJSONShape(t *testing.T) {
	t.Parallel()
	kafka := "kafka-connector-1"
	flink := "flink-deployment-1"
	j := models.IngestJob{
		ID: uuid.New(), Name: "sales-cdc", Namespace: "data",
		Spec: json.RawMessage(`{"source":"snowflake"}`), Status: "running",
		KafkaConnectorName:  &kafka,
		FlinkDeploymentName: &flink,
		CreatedAt:           time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(j)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "name", "namespace", "spec", "status",
		"kafka_connector_name", "flink_deployment_name", "error",
		"created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

func TestCreateIngestJobRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("POST", "/ingest-jobs",
		strings.NewReader(`{"name":"x","namespace":"y","spec":{}}`))
	rec := httptest.NewRecorder()
	h.CreateIngestJob(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestCreateIngestJobRejectsEmptyFields(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/ingest-jobs",
		strings.NewReader(`{"name":"","namespace":""}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateIngestJob(rec, req)
	assert.Equal(t, 400, rec.Code)
}

func TestCreateIngestJobRejectsMissingSpec(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/ingest-jobs",
		strings.NewReader(`{"name":"a","namespace":"b"}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateIngestJob(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "spec")
}

func TestListIngestJobsRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("GET", "/ingest-jobs", nil)
	rec := httptest.NewRecorder()
	h.ListIngestJobs(rec, req)
	assert.Equal(t, 401, rec.Code)
}

type fakeStore struct {
	streams     map[uuid.UUID]models.StreamDefinition
	cdcStreams  map[uuid.UUID]models.CdcStream
	checkpoints map[uuid.UUID]models.IncrementalCheckpoint
	resolutions map[uuid.UUID]models.ResolutionState
}

func newFakeStore() *fakeStore {
	return &fakeStore{streams: map[uuid.UUID]models.StreamDefinition{}, cdcStreams: map[uuid.UUID]models.CdcStream{}, checkpoints: map[uuid.UUID]models.IncrementalCheckpoint{}, resolutions: map[uuid.UUID]models.ResolutionState{}}
}

func (f *fakeStore) ListIngestJobs(context.Context, string, string) ([]models.IngestJob, error) {
	return nil, nil
}
func (f *fakeStore) GetIngestJob(context.Context, uuid.UUID) (*models.IngestJob, error) {
	return nil, nil
}
func (f *fakeStore) CreateIngestJob(context.Context, *models.CreateIngestJobRequest) (*models.IngestJob, error) {
	return nil, nil
}
func (f *fakeStore) UpdateIngestJob(context.Context, uuid.UUID, *models.UpdateIngestJobRequest) (*models.IngestJob, error) {
	return nil, nil
}
func (f *fakeStore) DeleteIngestJob(context.Context, uuid.UUID) (bool, error) { return false, nil }

func (f *fakeStore) ListStreams(_ context.Context, ownerID uuid.UUID, status string) ([]models.StreamDefinition, error) {
	out := []models.StreamDefinition{}
	for _, s := range f.streams {
		if s.OwnerID == ownerID && (status == "" || s.Status == status) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeStore) GetStream(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	s, ok := f.streams[id]
	if !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	return &s, nil
}
func (f *fakeStore) CreateStream(_ context.Context, body *models.CreateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	if body.Name == "" {
		return nil, assert.AnError
	}
	if body.IngestConsistency == "EXACTLY_ONCE" {
		return nil, assert.AnError
	}
	partitions := int32(3)
	if body.Partitions != nil {
		partitions = *body.Partitions
	}
	if partitions < 1 {
		return nil, assert.AnError
	}
	now := time.Now().UTC()
	s := models.StreamDefinition{ID: uuid.New(), Name: body.Name, Description: body.Description, Status: "active", Schema: []byte(`{"fields":[]}`), SourceBinding: []byte(`{"connector_type":"kafka"}`), RetentionHours: 72, Partitions: partitions, ConsistencyGuarantee: "at-least-once", StreamType: "STANDARD", IngestConsistency: "AT_LEAST_ONCE", PipelineConsistency: "AT_LEAST_ONCE", CheckpointIntervalMS: 2000, Kind: "INGEST", OwnerID: ownerID, CreatedAt: now, UpdatedAt: now}
	f.streams[s.ID] = s
	return &s, nil
}
func (f *fakeStore) UpdateStream(_ context.Context, id uuid.UUID, body *models.UpdateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	s, ok := f.streams[id]
	if !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	if body.Status != nil {
		s.Status = *body.Status
	}
	if body.Partitions != nil {
		if *body.Partitions < 1 {
			return nil, assert.AnError
		}
		s.Partitions = *body.Partitions
	}
	s.UpdatedAt = time.Now().UTC()
	f.streams[id] = s
	return &s, nil
}

func (f *fakeStore) ListCdcStreams(_ context.Context, ownerID uuid.UUID) ([]models.CdcStream, error) {
	out := []models.CdcStream{}
	for _, s := range f.cdcStreams {
		if s.OwnerID == ownerID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeStore) RegisterCdcStream(_ context.Context, body *models.RegisterCdcStreamRequest, ownerID uuid.UUID) (*models.CdcStream, *models.IncrementalCheckpoint, *models.ResolutionState, error) {
	if body.Slug == "" || body.SourceKind == "" || body.SourceRef == "" {
		return nil, nil, nil, assert.AnError
	}
	keys, _ := json.Marshal(body.PrimaryKeys)
	now := time.Now().UTC()
	s := models.CdcStream{ID: uuid.New(), Slug: body.Slug, SourceKind: body.SourceKind, SourceRef: body.SourceRef, PrimaryKeys: keys, IncrementalMode: "log_based", Status: "registered", OwnerID: ownerID, CreatedAt: now, UpdatedAt: now}
	cp := models.IncrementalCheckpoint{StreamID: s.ID, UpdatedAt: now}
	res := models.ResolutionState{StreamID: s.ID, Status: "lagging", UpdatedAt: now}
	f.cdcStreams[s.ID] = s
	f.checkpoints[s.ID] = cp
	f.resolutions[s.ID] = res
	return &s, &cp, &res, nil
}
func (f *fakeStore) GetCdcStream(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.CdcStream, error) {
	s, ok := f.cdcStreams[id]
	if !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	return &s, nil
}
func (f *fakeStore) GetCheckpoint(_ context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.IncrementalCheckpoint, error) {
	if s, ok := f.cdcStreams[streamID]; !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	cp := f.checkpoints[streamID]
	return &cp, nil
}
func (f *fakeStore) GetResolution(_ context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.ResolutionState, error) {
	if s, ok := f.cdcStreams[streamID]; !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	res := f.resolutions[streamID]
	return &res, nil
}

func (f *fakeStore) ApplyCheckpoint(_ context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.CheckpointUpdate) (*models.IncrementalCheckpoint, error) {
	if s, ok := f.cdcStreams[streamID]; !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	cp := f.checkpoints[streamID]
	if update != nil {
		if update.LastOffset != nil {
			cp.LastOffset = update.LastOffset
		}
		if update.LastLSN != nil {
			cp.LastLSN = update.LastLSN
		}
		if update.LastEventAt != nil {
			cp.LastEventAt = update.LastEventAt
		}
		if update.RecordsObserved != nil {
			cp.RecordsObserved = *update.RecordsObserved
		}
		if update.RecordsApplied != nil {
			cp.RecordsApplied = *update.RecordsApplied
		}
		cp.UpdatedAt = time.Now().UTC()
		f.checkpoints[streamID] = cp
	}
	return &cp, nil
}
func (f *fakeStore) ApplyResolution(_ context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.ResolutionUpdate) (*models.ResolutionState, error) {
	if s, ok := f.cdcStreams[streamID]; !ok || s.OwnerID != ownerID {
		return nil, nil
	}
	res := f.resolutions[streamID]
	if update != nil {
		if update.Status != nil {
			res.Status = *update.Status
		}
		if update.Watermark != nil {
			res.Watermark = update.Watermark
		}
		if update.ConflictCount != nil {
			res.ConflictCount = *update.ConflictCount
		}
		if update.PendingResolutions != nil {
			res.PendingResolutions = *update.PendingResolutions
		}
		if update.Notes != nil {
			res.Notes = update.Notes
		}
		res.UpdatedAt = time.Now().UTC()
		f.resolutions[streamID] = res
	}
	return &res, nil
}

type fakeRuntime struct {
	provisioned int
	updated     int
	registered  int
	err         error
	cdcResult   *handlers.CdcRegistrationResult
}

func (f *fakeRuntime) ProvisionStream(context.Context, *models.StreamDefinition) error {
	f.provisioned++
	return f.err
}
func (f *fakeRuntime) UpdateStream(context.Context, *models.StreamDefinition) error {
	f.updated++
	return f.err
}
func (f *fakeRuntime) RegisterCDC(context.Context, *models.CdcStream) (*handlers.CdcRegistrationResult, error) {
	f.registered++
	return f.cdcResult, f.err
}

func authedReq(method, target, body string, sub uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(authmw.ContextWithClaims(context.Background(), &authmw.Claims{Sub: sub}))
}
func withRouteParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.RouteContext(req.Context())
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestStreamCRUDValidationAndRuntimeInterface(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore()
	rt := &fakeRuntime{}
	h := &handlers.Handlers{Repo: store, Runtime: rt}
	req := authedReq("POST", "/streams", `{"name":"orders","partitions":4}`, owner)
	rec := httptest.NewRecorder()
	h.CreateStream(rec, req)
	require.Equal(t, 201, rec.Code)
	assert.Equal(t, 1, rt.provisioned)
	var created models.StreamDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, int32(4), created.Partitions)
	req = authedReq("GET", "/streams", "", owner)
	rec = httptest.NewRecorder()
	h.ListStreams(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "orders")
	req = withRouteParam(authedReq("GET", "/streams/"+created.ID.String(), "", owner), "id", created.ID.String())
	rec = httptest.NewRecorder()
	h.GetStream(rec, req)
	assert.Equal(t, 200, rec.Code)
	paused := "paused"
	req = withRouteParam(authedReq("PATCH", "/streams/"+created.ID.String(), `{"status":"`+paused+`"}`, owner), "id", created.ID.String())
	rec = httptest.NewRecorder()
	h.UpdateStream(rec, req)
	assert.Equal(t, 200, rec.Code)
	req = authedReq("POST", "/streams", `{"name":"bad","ingest_consistency":"EXACTLY_ONCE"}`, owner)
	rec = httptest.NewRecorder()
	h.CreateStream(rec, req)
	assert.Equal(t, 400, rec.Code)
}

func TestCdcRegisterSeedsInitialMetadata(t *testing.T) {
	owner := uuid.New()
	observed := int64(10)
	applied := int64(9)
	lastOffset := "42"
	resolved := "resolved"
	h := &handlers.Handlers{Repo: newFakeStore(), Runtime: &fakeRuntime{cdcResult: &handlers.CdcRegistrationResult{Checkpoint: &models.CheckpointUpdate{LastOffset: &lastOffset, RecordsObserved: &observed, RecordsApplied: &applied}, Resolution: &models.ResolutionUpdate{Status: &resolved}}}}
	req := authedReq("POST", "/cdc/streams", `{"slug":"orders-cdc","source_kind":"postgres","source_ref":"pg://orders","primary_keys":["id"]}`, owner)
	rec := httptest.NewRecorder()
	h.RegisterCdcStream(rec, req)
	require.Equal(t, 201, rec.Code)
	assert.Contains(t, rec.Body.String(), "resolved")
	var body struct {
		Stream models.CdcStream `json:"stream"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	req = withRouteParam(authedReq("GET", "/cdc/streams/"+body.Stream.ID.String()+"/checkpoint", "", owner), "id", body.Stream.ID.String())
	rec = httptest.NewRecorder()
	h.GetCdcCheckpoint(rec, req)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "42")
	req = withRouteParam(authedReq("GET", "/cdc/streams/"+body.Stream.ID.String()+"/resolution", "", owner), "id", body.Stream.ID.String())
	rec = httptest.NewRecorder()
	h.GetCdcResolution(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func TestStreamingCdcAuthAndTenantIsolation(t *testing.T) {
	owner := uuid.New()
	intruder := uuid.New()
	store := newFakeStore()
	h := &handlers.Handlers{Repo: store}
	s, _, _, err := store.RegisterCdcStream(context.Background(), &models.RegisterCdcStreamRequest{Slug: "x", SourceKind: "postgres", SourceRef: "pg"}, owner)
	require.NoError(t, err)
	req := withRouteParam(authedReq("GET", "/cdc/streams/"+s.ID.String(), "", intruder), "id", s.ID.String())
	rec := httptest.NewRecorder()
	h.GetCdcStream(rec, req)
	assert.Equal(t, 404, rec.Code)
	req = httptest.NewRequest("GET", "/streams", nil)
	rec = httptest.NewRecorder()
	h.ListStreams(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestStreamProvisionFailureMapsRuntimeError(t *testing.T) {
	owner := uuid.New()
	h := &handlers.Handlers{Repo: newFakeStore(), Runtime: &fakeRuntime{err: &handlers.RuntimeError{Kind: handlers.RuntimeUpstream, Msg: "kafka provision topic: boom"}}}
	req := authedReq("POST", "/streams", `{"name":"orders"}`, owner)
	rec := httptest.NewRecorder()
	h.CreateStream(rec, req)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "boom")
}

func TestStreamRuntimeUnavailable(t *testing.T) {
	owner := uuid.New()
	h := &handlers.Handlers{Repo: newFakeStore()}
	req := authedReq("POST", "/streams", `{"name":"orders"}`, owner)
	rec := httptest.NewRecorder()
	h.CreateStream(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

type fakeKafkaAdmin struct {
	provisioned []handlers.KafkaTopicSpec
	updated     []handlers.KafkaTopicSpec
	cdc         []handlers.CdcRegistrationSpec
	err         error
	result      *handlers.CdcRegistrationResult
}

func (f *fakeKafkaAdmin) ProvisionTopic(_ context.Context, spec handlers.KafkaTopicSpec) error {
	f.provisioned = append(f.provisioned, spec)
	return f.err
}
func (f *fakeKafkaAdmin) UpdateTopic(_ context.Context, spec handlers.KafkaTopicSpec) error {
	f.updated = append(f.updated, spec)
	return f.err
}
func (f *fakeKafkaAdmin) RegisterCDCSource(_ context.Context, spec handlers.CdcRegistrationSpec) (*handlers.CdcRegistrationResult, error) {
	f.cdc = append(f.cdc, spec)
	return f.result, f.err
}

type fakeFlinkDeployer struct {
	deployed []handlers.FlinkJobSpec
	updated  []handlers.FlinkJobSpec
	cdc      []handlers.CdcRegistrationSpec
	err      error
	result   *handlers.CdcRegistrationResult
}

func (f *fakeFlinkDeployer) DeployStream(_ context.Context, spec handlers.FlinkJobSpec) error {
	f.deployed = append(f.deployed, spec)
	return f.err
}
func (f *fakeFlinkDeployer) UpdateStream(_ context.Context, spec handlers.FlinkJobSpec) error {
	f.updated = append(f.updated, spec)
	return f.err
}
func (f *fakeFlinkDeployer) RegisterCDCJob(_ context.Context, spec handlers.CdcRegistrationSpec) (*handlers.CdcRegistrationResult, error) {
	f.cdc = append(f.cdc, spec)
	return f.result, f.err
}

func TestProductionStreamingRuntimeProvisionAndUpdate(t *testing.T) {
	kafka := &fakeKafkaAdmin{}
	flink := &fakeFlinkDeployer{}
	rt := handlers.NewProductionStreamingRuntime(kafka, flink)
	stream := &models.StreamDefinition{ID: uuid.New(), Name: "Orders Raw", Partitions: 6, RetentionHours: 24, Schema: []byte(`{"fields":[]}`), SourceBinding: []byte(`{"connector_type":"kafka"}`), CheckpointIntervalMS: 5000, PipelineConsistency: "AT_LEAST_ONCE"}
	require.NoError(t, rt.ProvisionStream(context.Background(), stream))
	require.Len(t, kafka.provisioned, 1)
	require.Len(t, flink.deployed, 1)
	assert.Equal(t, int32(6), kafka.provisioned[0].Partitions)
	assert.Contains(t, kafka.provisioned[0].Topic, "orders-raw")
	require.NoError(t, rt.UpdateStream(context.Background(), stream))
	require.Len(t, kafka.updated, 1)
	require.Len(t, flink.updated, 1)
}

func TestProductionStreamingRuntimeRegisterCDC(t *testing.T) {
	lsn := "0/16B6C50"
	status := "caught_up"
	kafka := &fakeKafkaAdmin{result: &handlers.CdcRegistrationResult{Checkpoint: &models.CheckpointUpdate{LastLSN: &lsn}}}
	flink := &fakeFlinkDeployer{result: &handlers.CdcRegistrationResult{Resolution: &models.ResolutionUpdate{Status: &status}}}
	rt := handlers.NewProductionStreamingRuntime(kafka, flink)
	stream := &models.CdcStream{ID: uuid.New(), Slug: "orders-cdc", SourceKind: "postgres", SourceRef: "pg://orders", PrimaryKeys: []byte(`["id"]`), IncrementalMode: "log_based"}
	result, err := rt.RegisterCDC(context.Background(), stream)
	require.NoError(t, err)
	require.Len(t, kafka.cdc, 1)
	require.Len(t, flink.cdc, 1)
	require.NotNil(t, result.Checkpoint)
	require.NotNil(t, result.Resolution)
	assert.Equal(t, lsn, *result.Checkpoint.LastLSN)
	assert.Equal(t, status, *result.Resolution.Status)
}

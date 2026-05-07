// Tests for the Phase 5D side-effect helpers — envelope splitting,
// notification recipient resolution, template rendering, recipient
// caps, audit issue_service_token round-trip.
package actions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestSplitActionConfigLegacyConfig(t *testing.T) {
	t.Parallel()
	op, notifs, err := splitActionConfig(json.RawMessage(`{"property_mappings":[]}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if string(op) != `{"property_mappings":[]}` {
		t.Errorf("legacy config must round-trip, got %s", op)
	}
	if len(notifs) != 0 {
		t.Errorf("legacy config has no notifs, got %d", len(notifs))
	}
}

func TestSplitActionConfigEnvelopeExtractsNotifs(t *testing.T) {
	t.Parallel()
	cfg := `{"operation":{"x":1},"notification_side_effects":[{"title":"hi","body":"world"}]}`
	op, notifs, err := splitActionConfig(json.RawMessage(cfg))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if string(op) != `{"x":1}` {
		t.Errorf("op extraction drift: %s", op)
	}
	if len(notifs) != 1 || notifs[0].Title != "hi" || notifs[0].Body != "world" {
		t.Errorf("notifs drift: %+v", notifs)
	}
}

func TestSplitWebhookConfigsExtractsBoth(t *testing.T) {
	t.Parallel()
	wb := uuid.New()
	se := uuid.New()
	cfg := `{"operation":{},"webhook_writeback":{"webhook_id":"` + wb.String() + `"},` +
		`"webhook_side_effects":[{"webhook_id":"` + se.String() + `"}]}`
	writeback, sideEffects, err := splitWebhookConfigs(json.RawMessage(cfg))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if writeback == nil || writeback.WebhookID != wb {
		t.Errorf("writeback drift: %+v", writeback)
	}
	if len(sideEffects) != 1 || sideEffects[0].WebhookID != se {
		t.Errorf("side-effects drift: %+v", sideEffects)
	}
}

func TestExtractUUIDValuesAcceptsStringAndArray(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	got, err := extractUUIDValues(json.RawMessage(`"`+id.String()+`"`), "field")
	if err != nil || len(got) != 1 || got[0] != id {
		t.Fatalf("string drift: %v %v", got, err)
	}
	id2 := uuid.New()
	arr := `["` + id.String() + `","` + id2.String() + `"]`
	got, err = extractUUIDValues(json.RawMessage(arr), "field")
	if err != nil || len(got) != 2 {
		t.Fatalf("array drift: %v %v", got, err)
	}
}

func TestExtractUUIDValuesRejectsNonUUID(t *testing.T) {
	t.Parallel()
	if _, err := extractUUIDValues(json.RawMessage(`"nope"`), "field"); err == nil {
		t.Fatal("expected error for non-UUID string")
	}
	if _, err := extractUUIDValues(json.RawMessage(`42`), "field"); err == nil {
		t.Fatal("expected error for non-string non-array")
	}
}

func TestResolveNotificationRecipientsCombinesSourcesAndDedups(t *testing.T) {
	t.Parallel()
	actor := uuid.New()
	creator := uuid.New()
	target := &domain.ObjectInstance{
		CreatedBy:  creator,
		Properties: json.RawMessage(`{}`),
	}
	cfg := notificationSideEffectConfig{
		UserIDs:             []uuid.UUID{actor},
		SendToActor:         true,
		SendToTargetCreator: true,
	}
	got, broadcast, err := resolveNotificationRecipients(cfg, nil, target, actor)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if broadcast {
		t.Errorf("broadcast must be false")
	}
	if len(got) != 2 {
		t.Errorf("expected dedup to 2 recipients, got %d", len(got))
	}
}

func TestResolveNotificationRecipientsEnforcesCap(t *testing.T) {
	t.Parallel()
	target := &domain.ObjectInstance{Properties: json.RawMessage(`{}`)}
	ids := make([]uuid.UUID, maxNotificationRecipients+1)
	for i := range ids {
		ids[i] = uuid.New()
	}
	cfg := notificationSideEffectConfig{UserIDs: ids}
	if _, _, err := resolveNotificationRecipients(cfg, nil, target, uuid.New()); err == nil {
		t.Fatal("expected scale-limit error")
	}
}

func TestResolveNotificationRecipientsFromFunctionTighterCap(t *testing.T) {
	t.Parallel()
	creator := uuid.New()
	ids := make([]uuid.UUID, maxNotificationRecipientsFromFunc+1)
	for i := range ids {
		ids[i] = uuid.New()
	}
	arr := make([]string, len(ids))
	for i, id := range ids {
		arr[i] = `"` + id.String() + `"`
	}
	props := `{"watchers":[` + strings.Join(arr, ",") + `]}`
	target := &domain.ObjectInstance{
		CreatedBy:  creator,
		Properties: json.RawMessage(props),
	}
	prop := "watchers"
	cfg := notificationSideEffectConfig{TargetUserPropertyName: &prop}
	if _, _, err := resolveNotificationRecipients(cfg, nil, target, uuid.New()); err == nil {
		t.Fatal("expected from-function scale-limit error")
	}
}

func TestRenderTemplateNestedContext(t *testing.T) {
	t.Parallel()
	ctx := map[string]any{
		"action": map[string]any{"name": "Approve"},
		"actor":  map[string]any{"email": "alice@example.com"},
	}
	got := renderTemplate("{{action.name}} by {{actor.email}}", ctx)
	if got != "Approve by alice@example.com" {
		t.Errorf("rendered drift: %q", got)
	}
}

func TestRenderTemplateUnknownTokenIsEmpty(t *testing.T) {
	t.Parallel()
	got := renderTemplate("hi {{missing.path}}!", map[string]any{})
	if got != "hi !" {
		t.Errorf("unknown token must drop, got %q", got)
	}
}

func TestIssueServiceTokenRoundTrips(t *testing.T) {
	t.Parallel()
	state := &ontologykernel.AppState{JWTConfig: authmw.NewJWTConfig("test-secret")}
	claims := &authmw.Claims{Sub: uuid.New()}
	token, err := issueServiceToken(state, claims)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(token, "Bearer ") {
		t.Errorf("expected Bearer prefix, got %q", token)
	}
}

func TestEmitActionAuditEventPostsToConfiguredURL(t *testing.T) {
	t.Parallel()
	var hits int32
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	state := &ontologykernel.AppState{
		JWTConfig:       authmw.NewJWTConfig("test-secret"),
		AuditServiceURL: srv.URL,
		HTTPClient:      srv.Client(),
	}
	claims := &authmw.Claims{Sub: uuid.New(), Email: "alice@example.com"}
	action := newTestAction()
	if err := emitActionAuditEvent(context.Background(), state, claims, action, nil,
		nil, "success", "low", "", nil, nil, nil, nil); err != nil {
		t.Fatalf("err: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 audit-service call, got %d", hits)
	}
	var asMap map[string]any
	if err := json.Unmarshal(lastBody, &asMap); err != nil {
		t.Fatalf("audit body not JSON: %v", err)
	}
	if asMap["action"] != "ontology.action.execute" {
		t.Errorf("action label drift: %v", asMap["action"])
	}
}

func TestEmitActionAuditEventNoopWhenNotConfigured(t *testing.T) {
	t.Parallel()
	state := &ontologykernel.AppState{JWTConfig: authmw.NewJWTConfig("test-secret")}
	claims := &authmw.Claims{Sub: uuid.New()}
	if err := emitActionAuditEvent(context.Background(), state, claims, newTestAction(), nil,
		nil, "success", "low", "", nil, nil, nil, nil); err != nil {
		t.Errorf("must noop when AuditServiceURL is unset, got %v", err)
	}
}

func newTestAction() models.ActionType {
	return models.ActionType{
		ID:            uuid.New(),
		Name:          "Approve",
		OperationKind: "update_object",
	}
}

func TestEmitActionAuditEventReturnsHTTPAndTimeoutErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "4xx", status: http.StatusBadRequest},
		{name: "5xx", status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`nope`))
			}))
			defer srv.Close()
			state := &ontologykernel.AppState{JWTConfig: authmw.NewJWTConfig("test-secret"), AuditServiceURL: srv.URL, HTTPClient: srv.Client()}
			err := emitActionAuditEvent(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, newTestAction(), nil, nil, "failure", "high", "bad", nil, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "audit service returned") {
				t.Fatalf("expected audit status error, got %v", err)
			}
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	state := &ontologykernel.AppState{JWTConfig: authmw.NewJWTConfig("test-secret"), AuditServiceURL: srv.URL, HTTPClient: &http.Client{Timeout: 5 * time.Millisecond}}
	err := emitActionAuditEvent(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, newTestAction(), nil, nil, "failure", "high", "slow", nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to send audit event") {
		t.Fatalf("expected audit timeout error, got %v", err)
	}
}

func TestSendNotificationRequestSuccessHTTPAndTimeoutErrors(t *testing.T) {
	var seenAuth string
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("authorization")
		if r.URL.Path != "/internal/notifications" {
			t.Fatalf("path drift: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer success.Close()
	state := &ontologykernel.AppState{NotificationServiceURL: success.URL, HTTPClient: success.Client()}
	if err := sendNotificationRequest(context.Background(), state, internalSendNotificationRequest{Title: "hi", Body: "there"}); err != nil {
		t.Fatalf("success notification returned error: %v", err)
	}
	if seenAuth != "" {
		t.Fatalf("Rust parity drift: internal notification requests must not set auth header, got %q", seenAuth)
	}

	statusSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`down`))
	}))
	defer statusSrv.Close()
	state = &ontologykernel.AppState{NotificationServiceURL: statusSrv.URL, HTTPClient: statusSrv.Client()}
	if err := sendNotificationRequest(context.Background(), state, internalSendNotificationRequest{Title: "hi", Body: "there"}); err == nil || !strings.Contains(err.Error(), "notification service returned") {
		t.Fatalf("expected notification status error, got %v", err)
	}

	timeoutSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer timeoutSrv.Close()
	state = &ontologykernel.AppState{NotificationServiceURL: timeoutSrv.URL, HTTPClient: &http.Client{Timeout: 5 * time.Millisecond}}
	if err := sendNotificationRequest(context.Background(), state, internalSendNotificationRequest{Title: "hi", Body: "there"}); err == nil || !strings.Contains(err.Error(), "failed to send action notification") {
		t.Fatalf("expected notification timeout error, got %v", err)
	}
}

func TestRunWebhookSideEffectsPersistsBestEffortRows(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t)
	claims := &authmw.Claims{Sub: uuid.New()}
	actionID := uuid.New()
	targetID := uuid.New()
	webhookID := uuid.New()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/api/v1/webhooks/"+webhookID.String()+"/invoke" {
			t.Fatalf("path drift: %s", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"output_parameters":{"ok":true}}`))
	}))
	defer srv.Close()
	state.ConnectorManagementServiceURL = srv.URL
	state.HTTPClient = srv.Client()

	runWebhookSideEffects(ctx, state, claims, claims.Sub, actionID, &targetID, []webhookCallConfig{{WebhookID: webhookID}}, json.RawMessage(`{"x":1}`))
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected one webhook hit, got %d", hits)
	}
	entries, err := state.Stores.Actions.ListRecent(ctx, "default", storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list side effects: %v", err)
	}
	if !hasSideEffect(entries.Items, actionID.String(), "success") {
		t.Fatalf("missing success side-effect row: %+v", entries.Items)
	}

	state.ConnectorManagementServiceURL = "http://127.0.0.1:1"
	runWebhookSideEffects(ctx, state, claims, claims.Sub, actionID, &targetID, []webhookCallConfig{{WebhookID: uuid.New()}}, json.RawMessage(`{"x":1}`))
	entries, err = state.Stores.Actions.ListRecent(ctx, "default", storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list side effects after failure: %v", err)
	}
	if !hasSideEffect(entries.Items, actionID.String(), "failure") {
		t.Fatalf("missing failure side-effect row: %+v", entries.Items)
	}
}

func hasSideEffect(entries []storage.ActionLogEntry, actionID, status string) bool {
	for _, entry := range entries {
		if entry.Kind != "side_effect" || entry.ActionID != actionID {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(entry.Payload, &payload)
		if payload["status"] == status {
			return true
		}
	}
	return false
}

func TestInvokeRegisteredWebhookHTTPAndTimeoutErrors(t *testing.T) {
	webhookID := uuid.New()
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "4xx", status: http.StatusBadRequest},
		{name: "5xx", status: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`bad webhook`))
			}))
			defer srv.Close()
			state := &ontologykernel.AppState{ConnectorManagementServiceURL: srv.URL, HTTPClient: srv.Client()}
			_, err := invokeRegisteredWebhook(context.Background(), state, &webhookCallConfig{WebhookID: webhookID}, json.RawMessage(`{"x":1}`))
			if err == nil || !strings.Contains(err.Error(), "webhook returned") {
				t.Fatalf("expected webhook status error, got %v", err)
			}
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	state := &ontologykernel.AppState{ConnectorManagementServiceURL: srv.URL, HTTPClient: &http.Client{Timeout: 5 * time.Millisecond}}
	_, err := invokeRegisteredWebhook(context.Background(), state, &webhookCallConfig{WebhookID: webhookID}, json.RawMessage(`{"x":1}`))
	if err == nil || !strings.Contains(err.Error(), "webhook invocation failed") {
		t.Fatalf("expected webhook timeout error, got %v", err)
	}
}

// Tests for the Phase 5C batch helpers — scale-limit math, list-cap
// validation, batched_execution flag extraction.
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestEstimateEditBytesIgnoresWhitespace(t *testing.T) {
	t.Parallel()
	loose := json.RawMessage(`{ "a"  :  1 ,  "b" :  2 }`)
	tight := json.RawMessage(`{"a":1,"b":2}`)
	got, want := estimateEditBytes(loose), len(tight)
	if got != want {
		t.Errorf("expected canonical-bytes %d, got %d", want, got)
	}
}

func TestEstimateEditBytesEmptyZero(t *testing.T) {
	t.Parallel()
	if got := estimateEditBytes(nil); got != 0 {
		t.Errorf("nil rawMessage must be zero, got %d", got)
	}
	if got := estimateEditBytes(json.RawMessage("")); got != 0 {
		t.Errorf("empty rawMessage must be zero, got %d", got)
	}
}

func TestValidateParameterListSizes(t *testing.T) {
	t.Parallel()
	schema := []models.ActionInputField{
		{Name: "owners", PropertyType: "object_reference_list"},
		{Name: "tags", PropertyType: "array"},
	}
	tooManyRefs := json.RawMessage(`{"owners":` + buildJSONList(maxObjectReferenceList+1) + `}`)
	if msg := validateParameterListSizes(schema, tooManyRefs); msg == "" {
		t.Fatal("expected scale-limit message for over-cap object_reference_list")
	}
	tooManyArr := json.RawMessage(`{"tags":` + buildJSONList(maxListPrimitive+1) + `}`)
	if msg := validateParameterListSizes(schema, tooManyArr); msg == "" {
		t.Fatal("expected scale-limit message for over-cap array")
	}
	withinCap := json.RawMessage(`{"owners":[1,2],"tags":["a","b"]}`)
	if msg := validateParameterListSizes(schema, withinCap); msg != "" {
		t.Errorf("unexpected scale-limit error: %s", msg)
	}
}

func TestExtractBatchedExecutionFlag(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		`{"batched_execution":true}`:                     true,
		`{"batched_execution":false}`:                    false,
		`{"operation":{"x":1},"batched_execution":true}`: true,
		`{}`:   false,
		`null`: false,
		`""`:   false,
	}
	for raw, want := range cases {
		got := extractBatchedExecutionFlag(json.RawMessage(raw))
		if got != want {
			t.Errorf("config %q: got %v, want %v", raw, got, want)
		}
	}
}

func buildJSONList(n int) string {
	out := []byte{'['}
	for i := 0; i < n; i++ {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '0')
	}
	out = append(out, ']')
	return string(out)
}

func TestExecuteActionBatchFunctionBackedBatchedInvocation(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t)
	actionID := uuid.New()
	objectTypeID := uuid.New()
	seenBatch := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("payload json: %v", err)
		}
		params := payload["parameters"].(map[string]any)
		batch := params["batch"].([]any)
		seenBatch = len(batch)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	action := models.ActionType{
		ID:            actionID,
		Name:          "batch_func",
		DisplayName:   "Batch Func",
		ObjectTypeID:  objectTypeID,
		OperationKind: "invoke_function",
		Config:        json.RawMessage(fmt.Sprintf(`{"operation":{"url":"%s","method":"POST"},"batched_execution":true}`, server.URL)),
		OwnerID:       uuid.New(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	rec, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("ActionToRecord: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(ctx, rec, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	id1, id2 := uuid.New(), uuid.New()
	body := []byte(fmt.Sprintf(`{"target_object_ids":["%s","%s"],"parameters":{"mode":"bulk"}}`, id1, id2))
	req := requestWithRoute(ctx, http.MethodPost, "/execute-batch", body, map[string]string{"id": actionID.String()})
	w := httptest.NewRecorder()
	ExecuteActionBatchHandler(state).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if seenBatch != 2 || !strings.Contains(w.Body.String(), `"batched":true`) {
		t.Fatalf("batched invocation drift seenBatch=%d body=%s", seenBatch, w.Body.String())
	}
	entries, err := state.Stores.Actions.ListRecent(ctx, "default", storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list action log: %v", err)
	}
	if !hasActionAttempt(entries.Items, actionID.String(), "success") {
		t.Fatalf("missing batched success attempt: %+v", entries.Items)
	}
}

func TestExecuteActionBatchFunctionSingleCallLimit(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t)
	actionID := uuid.New()
	action := models.ActionType{
		ID:            actionID,
		Name:          "func",
		DisplayName:   "Func",
		ObjectTypeID:  uuid.New(),
		OperationKind: "invoke_function",
		Config:        json.RawMessage(`{"url":"http://127.0.0.1:1","method":"POST"}`),
		OwnerID:       uuid.New(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	rec, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("ActionToRecord: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(ctx, rec, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	ids := make([]string, defaultBatchMaxTargets+1)
	for i := range ids {
		ids[i] = `"` + uuid.New().String() + `"`
	}
	body := []byte(`{"target_object_ids":[` + strings.Join(ids, ",") + `],"parameters":{}}`)
	req := requestWithRoute(ctx, http.MethodPost, "/execute-batch", body, map[string]string{"id": actionID.String()})
	w := httptest.NewRecorder()
	ExecuteActionBatchHandler(state).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "scale_limit") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

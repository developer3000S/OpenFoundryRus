package writer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
)

type fakeAICatalog struct {
	table IcebergTableAppender
	err   error
	seen  []TableSpec
}

func (f *fakeAICatalog) LoadTable(_ context.Context, spec TableSpec) (IcebergTableAppender, error) {
	f.seen = append(f.seen, spec)
	if f.err != nil {
		return nil, f.err
	}
	return f.table, nil
}

type fakeAITable struct {
	err     error
	batches []AppendBatch
}

func (f *fakeAITable) Append(_ context.Context, batch AppendBatch) error {
	f.batches = append(f.batches, batch)
	return f.err
}

func TestIcebergWriterAppendBuildsRustCompatibleAIBatches(t *testing.T) {
	runID := uuid.MustParse("00000000-0000-7000-8000-000000000123")
	traceID := "trace-1"
	table := &fakeAITable{}
	catalog := &fakeAICatalog{table: table}
	w := NewIcebergWriterWithCatalog("http://lakekeeper", "http://table-writer", "wh", "of_ai", catalog)

	byTable := map[string][]envelope.AiEventEnvelope{
		envelope.TableResponses: {{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000001"), At: 1700000000000000, Kind: envelope.KindResponse, RunID: &runID, TraceID: &traceID, Producer: "agent-runtime-service", SchemaVersion: 1, Payload: []byte(`{"tokens":42}`)}},
	}
	if err := w.Append(context.Background(), byTable); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(catalog.seen) != 1 || catalog.seen[0].Namespace != "of_ai" || catalog.seen[0].Table != envelope.TableResponses {
		t.Fatalf("LoadTable specs = %#v", catalog.seen)
	}
	if got := catalog.seen[0].PartitionTransform; got != "day(at)" {
		t.Fatalf("partition = %q", got)
	}
	if got := catalog.seen[0].SortOrder; got != "at ASC" {
		t.Fatalf("sort order = %q", got)
	}
	if !reflect.DeepEqual(catalog.seen[0].Schema, aiSchema()) {
		t.Fatalf("schema = %#v, want %#v", catalog.seen[0].Schema, aiSchema())
	}
	if len(table.batches) != 1 || len(table.batches[0].Rows) != 1 {
		t.Fatalf("batches = %#v", table.batches)
	}
	row := table.batches[0].Rows[0]
	if row["kind"] != "response" || row["run_id"] != runID.String() || row["trace_id"] != traceID {
		t.Fatalf("row = %#v", row)
	}
	if !reflect.DeepEqual(table.batches[0].Spec.Schema, aiSchema()) {
		t.Fatalf("batch schema = %#v, want %#v", table.batches[0].Spec.Schema, aiSchema())
	}
}

func TestIcebergWriterAppendsEachNonEmptyAITargetWithFakeAppender(t *testing.T) {
	table := &fakeAITable{}
	catalog := &fakeAICatalog{table: table}
	w := NewIcebergWriterWithCatalog("http://lakekeeper:8181", "http://table-writer:8080", "warehouse-1", "of_ai", catalog)

	batch := map[string][]envelope.AiEventEnvelope{
		envelope.TablePrompts: {
			{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000001"), At: 1, Kind: envelope.KindPrompt, Producer: "agent-runtime-service", SchemaVersion: 1, Payload: []byte(`{"prompt":"hi"}`)},
		},
		envelope.TableResponses: {
			{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000002"), At: 2, Kind: envelope.KindResponse, Producer: "agent-runtime-service", SchemaVersion: 1, Payload: []byte(`{"response":"hello"}`)},
		},
		envelope.TableEvaluations: {},
		envelope.TableTraces: {
			{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000003"), At: 3, Kind: envelope.KindTrace, Producer: "agent-runtime-service", SchemaVersion: 1, Payload: []byte(`{"span":"root"}`)},
		},
	}
	if err := w.Append(context.Background(), batch); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	wantTables := []string{envelope.TablePrompts, envelope.TableResponses, envelope.TableTraces}
	if len(catalog.seen) != len(wantTables) || len(table.batches) != len(wantTables) {
		t.Fatalf("LoadTable specs = %#v; batches = %#v", catalog.seen, table.batches)
	}
	for i, wantTable := range wantTables {
		if catalog.seen[i].CatalogURL != "http://lakekeeper:8181" {
			t.Fatalf("spec[%d].CatalogURL = %q", i, catalog.seen[i].CatalogURL)
		}
		if catalog.seen[i].Table != wantTable {
			t.Fatalf("spec[%d].Table = %q, want %q", i, catalog.seen[i].Table, wantTable)
		}
		if table.batches[i].Spec.Table != wantTable || len(table.batches[i].Rows) != 1 {
			t.Fatalf("batch[%d] = %#v", i, table.batches[i])
		}
	}
}

func TestIcebergWriterRejectsEmptyAIBatch(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "", "of_ai", &fakeAICatalog{table: &fakeAITable{}})
	if err := w.Append(context.Background(), map[string][]envelope.AiEventEnvelope{}); !errors.Is(err, ErrEmptyBatch) {
		t.Fatalf("Append(empty) error = %v", err)
	}
}

func TestIcebergWriterPropagatesAITableNotFound(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "", "of_ai", &fakeAICatalog{err: ErrTableNotFound})
	batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, Kind: envelope.KindPrompt, Payload: []byte(`{}`)}}}
	if err := w.Append(context.Background(), batch); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestIcebergWriterPropagatesAISchemaMismatch(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "", "of_ai", &fakeAICatalog{table: &fakeAITable{err: ErrSchemaMismatch}})
	batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, Kind: envelope.KindPrompt, Payload: []byte(`{}`)}}}
	if err := w.Append(context.Background(), batch); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestIcebergWriterPropagatesAICommitFailure(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "", "of_ai", &fakeAICatalog{table: &fakeAITable{err: ErrCommitFailed}})
	batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, Kind: envelope.KindPrompt, Payload: []byte(`{}`)}}}
	if err := w.Append(context.Background(), batch); !errors.Is(err, ErrCommitFailed) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAIContract(t *testing.T) {
	runID := uuid.MustParse("00000000-0000-7000-8000-000000000123")
	traceID := "trace-1"
	catalogURL := "http://lakekeeper:8181"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/openfoundry/iceberg/v1/append" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		wantPayload := `{"spec":{"catalog":"lakekeeper","catalog_url":"http://lakekeeper:8181","warehouse":"warehouse-1","namespace":"of_ai","table":"responses","partition_transform":"day(at)","sort_order":"at ASC","schema":[{"id":1,"name":"event_id","type":"uuid","required":true},{"id":2,"name":"at","type":"timestamptz","required":true},{"id":3,"name":"kind","type":"string","required":true},{"id":4,"name":"run_id","type":"uuid","required":false},{"id":5,"name":"trace_id","type":"string","required":false},{"id":6,"name":"producer","type":"string","required":true},{"id":7,"name":"schema_version","type":"uint32","required":true},{"id":8,"name":"payload","type":"string","required":true}]},"rows":[{"at":1700000000000000,"event_id":"00000000-0000-7000-8000-000000000001","kind":"response","payload":"{\"tokens\":42}","producer":"agent-runtime-service","run_id":"00000000-0000-7000-8000-000000000123","schema_version":1,"trace_id":"trace-1"}]}`
		if string(body) != wantPayload {
			t.Fatalf("request payload mismatch\nwant: %s\n got: %s", wantPayload, body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	writer := NewIcebergWriterWithAdapter(catalogURL, server.URL, "warehouse-1", "of_ai")
	batch := map[string][]envelope.AiEventEnvelope{
		envelope.TableResponses: {{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000001"), At: 1700000000000000, Kind: envelope.KindResponse, RunID: &runID, TraceID: &traceID, Producer: "agent-runtime-service", SchemaVersion: 1, Payload: []byte(`{"tokens":42}`)}},
	}
	if err := writer.Append(context.Background(), batch); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAIAcceptsAny2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer server.Close()

	writer := NewIcebergWriter(server.URL, "warehouse-1", "of_ai")
	batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, At: 1, Kind: envelope.KindPrompt, Producer: "producer", SchemaVersion: 1, Payload: []byte(`{}`)}}}
	if err := writer.Append(context.Background(), batch); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{name: "404 table not found", status: http.StatusNotFound, want: ErrTableNotFound},
		{name: "409 schema mismatch", status: http.StatusConflict, want: ErrSchemaMismatch},
		{name: "422 schema mismatch", status: http.StatusUnprocessableEntity, want: ErrSchemaMismatch},
		{name: "500 commit failed", status: http.StatusInternalServerError, want: ErrCommitFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/openfoundry/iceberg/v1/append" {
					t.Fatalf("path = %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("adapter detail"))
			}))
			defer server.Close()

			writer := NewIcebergWriter(server.URL, "warehouse-1", "of_ai")
			batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, At: 1, Kind: envelope.KindPrompt, Producer: "producer", SchemaVersion: 1, Payload: []byte(`{}`)}}}
			err := writer.Append(context.Background(), batch)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Append() error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), "adapter detail") {
				t.Fatalf("Append() error = %v, want table-writer detail", err)
			}
		})
	}
}

func TestHTTPTableWriterAdapterAINetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	writer := NewIcebergWriter(url, "warehouse-1", "of_ai")
	batch := map[string][]envelope.AiEventEnvelope{envelope.TablePrompts: {{EventID: uuid.Nil, At: 1, Kind: envelope.KindPrompt, Producer: "producer", SchemaVersion: 1, Payload: []byte(`{}`)}}}
	err := writer.Append(context.Background(), batch)
	if !errors.Is(err, ErrCommitFailed) {
		t.Fatalf("Append() error = %v, want %v", err, ErrCommitFailed)
	}
}

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
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
)

type fakeAuditCatalog struct {
	table IcebergTableAppender
	err   error
	seen  []TableSpec
}

func (f *fakeAuditCatalog) LoadTable(_ context.Context, spec TableSpec) (IcebergTableAppender, error) {
	f.seen = append(f.seen, spec)
	if f.err != nil {
		return nil, f.err
	}
	return f.table, nil
}

type fakeAuditTable struct {
	err     error
	batches []AppendBatch
}

func (f *fakeAuditTable) Append(_ context.Context, batch AppendBatch) error {
	f.batches = append(f.batches, batch)
	return f.err
}

func TestIcebergWriterAppendBuildsRustCompatibleAuditBatch(t *testing.T) {
	corr := "corr-1"
	table := &fakeAuditTable{}
	catalog := &fakeAuditCatalog{table: table}
	w := NewIcebergWriterWithCatalog("http://catalog", "wh", "of_audit", "events", catalog)

	err := w.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000001"), At: 1700000000000000, CorrelationID: &corr, Kind: "auth.login.ok", Payload: []byte(`{"ok":true}`)}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(catalog.seen) != 1 || catalog.seen[0].Namespace != "of_audit" || catalog.seen[0].Table != "events" {
		t.Fatalf("LoadTable specs = %#v", catalog.seen)
	}
	if got := catalog.seen[0].PartitionTransform; got != "day(at)" {
		t.Fatalf("partition = %q", got)
	}
	if got := catalog.seen[0].SortOrder; got != "at ASC" {
		t.Fatalf("sort order = %q", got)
	}
	if !reflect.DeepEqual(catalog.seen[0].Schema, auditSchema()) {
		t.Fatalf("schema = %#v, want %#v", catalog.seen[0].Schema, auditSchema())
	}
	if len(table.batches) != 1 || len(table.batches[0].Rows) != 1 {
		t.Fatalf("batches = %#v", table.batches)
	}
	if got := table.batches[0].Rows[0]["payload"]; got != `{"ok":true}` {
		t.Fatalf("payload = %#v", got)
	}
	if !reflect.DeepEqual(table.batches[0].Spec.Schema, auditSchema()) {
		t.Fatalf("batch schema = %#v, want %#v", table.batches[0].Spec.Schema, auditSchema())
	}
}

func TestIcebergWriterRejectsEmptyAuditBatch(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "of_audit", "events", &fakeAuditCatalog{table: &fakeAuditTable{}})
	if err := w.Append(context.Background(), nil); !errors.Is(err, ErrEmptyBatch) {
		t.Fatalf("Append(empty) error = %v", err)
	}
}

func TestIcebergWriterPropagatesTableNotFound(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "of_audit", "events", &fakeAuditCatalog{err: ErrTableNotFound})
	if err := w.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, Payload: []byte(`{}`)}}); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestIcebergWriterPropagatesSchemaMismatch(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "of_audit", "events", &fakeAuditCatalog{table: &fakeAuditTable{err: ErrSchemaMismatch}})
	if err := w.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, Payload: []byte(`{}`)}}); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestIcebergWriterPropagatesCommitFailure(t *testing.T) {
	w := NewIcebergWriterWithCatalog("", "", "of_audit", "events", &fakeAuditCatalog{table: &fakeAuditTable{err: ErrCommitFailed}})
	if err := w.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, Payload: []byte(`{}`)}}); !errors.Is(err, ErrCommitFailed) {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAuditContract(t *testing.T) {
	corr := "corr-1"
	serverURL := ""
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
		wantPayload := `{"spec":{"catalog":"lakekeeper","catalog_url":"` + serverURL + `","warehouse":"warehouse-1","namespace":"of_audit","table":"events","partition_transform":"day(at)","sort_order":"at ASC","schema":[{"id":1,"name":"event_id","type":"uuid","required":true},{"id":2,"name":"at","type":"timestamptz","required":true},{"id":3,"name":"correlation_id","type":"string","required":false},{"id":4,"name":"kind","type":"string","required":true},{"id":5,"name":"payload","type":"string","required":true}]},"rows":[{"at":1700000000000000,"correlation_id":"corr-1","event_id":"00000000-0000-7000-8000-000000000001","kind":"auth.login.ok","payload":"{\"ok\":true}"}]}`
		if string(body) != wantPayload {
			t.Fatalf("request payload mismatch\nwant: %s\n got: %s", wantPayload, body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	serverURL = server.URL
	defer server.Close()

	writer := NewIcebergWriter(server.URL, "warehouse-1", "of_audit", "events")
	err := writer.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.MustParse("00000000-0000-7000-8000-000000000001"), At: 1700000000000000, CorrelationID: &corr, Kind: "auth.login.ok", Payload: []byte(`{"ok":true}`)}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterIsProductionPathNotStub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	writer := NewIcebergWriter(server.URL, "warehouse-1", "of_audit", "events")
	err := writer.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, At: 1, Kind: "kind", Payload: []byte(`{}`)}})
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Append() returned legacy stub error %v", err)
	}
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAuditAcceptsAny2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer server.Close()

	writer := NewIcebergWriter(server.URL, "warehouse-1", "of_audit", "events")
	err := writer.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, At: 1, Kind: "kind", Payload: []byte(`{}`)}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
}

func TestHTTPTableWriterAdapterAuditErrors(t *testing.T) {
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
				_, _ = w.Write([]byte("catalog said no"))
			}))
			defer server.Close()

			writer := NewIcebergWriter(server.URL, "warehouse-1", "of_audit", "events")
			err := writer.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, At: 1, Kind: "kind", Payload: []byte(`{}`)}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Append() error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), "catalog said no") {
				t.Fatalf("Append() error = %v, want table-writer detail", err)
			}
		})
	}
}

func TestHTTPTableWriterAdapterAuditNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	writer := NewIcebergWriter(url, "warehouse-1", "of_audit", "events")
	err := writer.Append(context.Background(), []envelope.AuditEnvelope{{EventID: uuid.Nil, At: 1, Kind: "kind", Payload: []byte(`{}`)}})
	if !errors.Is(err, ErrCommitFailed) {
		t.Fatalf("Append() error = %v, want %v", err, ErrCommitFailed)
	}
}

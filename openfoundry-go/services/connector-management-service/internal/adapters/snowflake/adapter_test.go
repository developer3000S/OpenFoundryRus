// Tests for the Snowflake adapter. Mirror the Rust unit tests in
// `services/connector-management-service/src/connectors/snowflake.rs`
// plus a httptest-driven fake `/api/v2/statements` surface that
// exercises discovery, virtual-table preview, multi-partition Arrow
// streaming, and JWT signing.
package snowflake

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// Mirrors Rust `requires_account_database_schema_and_credential`.
func TestValidateConfigRequiresAccountDatabaseSchemaAndCredential(t *testing.T) {
	require.Error(t, ValidateConfig(json.RawMessage(`{}`)))
	require.NoError(t, ValidateConfig(json.RawMessage(`{"account":"a","database":"d","schema":"s","oauth_token":"t"}`)))
	require.Error(t, ValidateConfig(json.RawMessage(`{"account":"a","database":"d","schema":"s"}`)))
}

// Mirrors Rust `build_query_qualifies_table`.
func TestBuildQueryQualifiesTable(t *testing.T) {
	cfg := &sfConfig{Account: "a", Database: "DB", Schema: "PUBLIC", OAuthToken: "t"}
	q, err := buildQuery(cfg, "ORDERS", 100)
	require.NoError(t, err)
	require.Equal(t, "SELECT * FROM DB.PUBLIC.ORDERS LIMIT 100", q)
}

// Mirrors Rust `build_query_passes_through_qualified_or_select`.
func TestBuildQueryPassesThroughQualifiedOrSelect(t *testing.T) {
	cfg := &sfConfig{Account: "a", Database: "DB", Schema: "PUBLIC", OAuthToken: "t"}
	q, err := buildQuery(cfg, "DB.PUBLIC.X", 10)
	require.NoError(t, err)
	require.Equal(t, "SELECT * FROM DB.PUBLIC.X LIMIT 10", q)
	q, err = buildQuery(cfg, "select 1", 10)
	require.NoError(t, err)
	require.Equal(t, "select 1", q)
}

func TestBuildQueryFallsBackToConfigQuery(t *testing.T) {
	cfg := &sfConfig{Account: "a", Database: "DB", Schema: "PUBLIC", OAuthToken: "t", Query: "SELECT CURRENT_VERSION()"}
	q, err := buildQuery(cfg, "", 1)
	require.NoError(t, err)
	require.Equal(t, "SELECT CURRENT_VERSION()", q)
}

func TestBuildQueryMissingSelectorAndQueryErrors(t *testing.T) {
	cfg := &sfConfig{Account: "a", Database: "DB", Schema: "PUBLIC", OAuthToken: "t"}
	_, err := buildQuery(cfg, "", 1)
	require.Error(t, err)
}

// Mirrors Rust `extracts_rows_with_column_names`.
func TestExtractsRowsWithColumnNames(t *testing.T) {
	columns := []string{"A", "B"}
	body := map[string]any{
		"data": []any{
			[]any{"1", "x"},
			[]any{"2", nil},
		},
	}
	rows := extractRows(body["data"], columns)
	require.Len(t, rows, 2)
	require.Equal(t, "1", rows[0]["A"])
	require.Nil(t, rows[1]["B"])
}

func TestExtractColumnsFromMeta(t *testing.T) {
	meta := map[string]any{"rowType": []any{
		map[string]any{"name": "id"},
		map[string]any{"name": "name"},
	}}
	require.Equal(t, []string{"id", "name"}, extractColumns(meta))
}

// Mirrors Rust `base_url_uses_account_subdomain`.
func TestBaseURLUsesAccountSubdomain(t *testing.T) {
	a := New()
	cfg := &sfConfig{Account: "xy12345.eu-central-1", Database: "DB", Schema: "PUBLIC", OAuthToken: "t"}
	u, err := a.baseURL(cfg)
	require.NoError(t, err)
	require.Equal(t, "https://xy12345.eu-central-1.snowflakecomputing.com", u.String())
}

func TestNormalizeAccountTrimsTrailingDots(t *testing.T) {
	require.Equal(t, "abc-1", normalizeAccount("  abc-1.. "))
}

func TestPageSizeClamps(t *testing.T) {
	zero := int64(0)
	tooMany := int64(1_000_000)
	require.Equal(t, defaultPageSize, pageSize(&sfConfig{}))
	require.Equal(t, int64(1), pageSize(&sfConfig{PageSize: &zero}))
	require.Equal(t, maxPageSize, pageSize(&sfConfig{PageSize: &tooMany}))
}

func TestBuildStatementBodyOmitsEmptyOptionalFields(t *testing.T) {
	body := buildStatementBody(&sfConfig{Database: "D", Schema: "S"}, "SELECT 1", nil)
	require.Equal(t, "SELECT 1", body["statement"])
	require.Equal(t, 60, body["timeout"])
	require.Equal(t, "D", body["database"])
	require.Equal(t, "S", body["schema"])
	_, hasWarehouse := body["warehouse"]
	require.False(t, hasWarehouse)
	_, hasParams := body["parameters"]
	require.False(t, hasParams)
}

func TestBuildStatementBodyEmitsParametersWhenLimitSet(t *testing.T) {
	limit := int64(25)
	body := buildStatementBody(&sfConfig{Warehouse: "WH", Role: "R"}, "SELECT 1", &limit)
	require.Equal(t, "WH", body["warehouse"])
	require.Equal(t, "R", body["role"])
	params := body["parameters"].(map[string]any)
	require.Equal(t, "25", params["ROWS_PER_RESULTSET"])
}

func TestSplitSelectorAcceptsThreePartIdentifier(t *testing.T) {
	cfg := &sfConfig{}
	d, s, tbl, err := splitSelector(cfg, "DB.PUBLIC.ORDERS")
	require.NoError(t, err)
	require.Equal(t, "DB", d)
	require.Equal(t, "PUBLIC", s)
	require.Equal(t, "ORDERS", tbl)
}

func TestSplitSelectorFallsBackToConnectionDefaults(t *testing.T) {
	cfg := &sfConfig{Database: "DB", Schema: "PUBLIC"}
	d, s, tbl, err := splitSelector(cfg, "ORDERS")
	require.NoError(t, err)
	require.Equal(t, "DB", d)
	require.Equal(t, "PUBLIC", s)
	require.Equal(t, "ORDERS", tbl)
}

func TestSplitSelectorRejectsEmpty(t *testing.T) {
	_, _, _, err := splitSelector(&sfConfig{Database: "DB", Schema: "PUBLIC"}, "")
	require.Error(t, err)
}

func TestObtainTokenPassesThroughOAuth(t *testing.T) {
	a := New()
	tok, err := a.obtainToken(&sfConfig{Account: "a", OAuthToken: "abc"})
	require.NoError(t, err)
	require.Equal(t, "abc", tok.token)
	require.Equal(t, "OAUTH", tok.kind)
}

func TestObtainTokenSignsKeypairJWT(t *testing.T) {
	priv, pemBytes := newRSAPEM(t)
	fingerprint := "SHA256:abcdef"
	fixedNow := time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC)

	a := New()
	a.SetNowFunc(func() time.Time { return fixedNow })
	cfg := &sfConfig{
		Account:              "xy12345.eu-central-1",
		Database:             "DB",
		Schema:               "PUBLIC",
		User:                 "alice",
		PrivateKeyPEM:        string(pemBytes),
		PublicKeyFingerprint: fingerprint,
	}
	tok, err := a.obtainToken(cfg)
	require.NoError(t, err)
	require.Equal(t, "KEYPAIR_JWT", tok.kind)

	parsed, err := jwt.Parse(tok.token, func(*jwt.Token) (any, error) { return &priv.PublicKey, nil })
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	claims := parsed.Claims.(jwt.MapClaims)
	require.Equal(t, "xy12345.eu-central-1.ALICE."+fingerprint, claims["iss"])
	require.Equal(t, "xy12345.eu-central-1.ALICE", claims["sub"])
	require.Equal(t, float64(fixedNow.Unix()), claims["iat"])
	require.Equal(t, float64(fixedNow.Unix()+jwtTTLSeconds), claims["exp"])
}

func TestObtainTokenRejectsMissingKeypairFields(t *testing.T) {
	a := New()
	_, err := a.obtainToken(&sfConfig{Account: "a"})
	require.Error(t, err)
	_, err = a.obtainToken(&sfConfig{Account: "a", User: "alice"})
	require.Error(t, err)
	_, err = a.obtainToken(&sfConfig{Account: "a", User: "alice", PublicKeyFingerprint: "fp"})
	require.Error(t, err)
}

func TestBuildIngestSpecEmitsSnowflakeDescriptor(t *testing.T) {
	a := New()
	conn := &models.Connection{
		ID:            uuid.New(),
		Name:          "warehouse",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"xy12345.eu-central-1","database":"DB","schema":"PUBLIC","oauth_token":"t","warehouse":"WH","role":"R"}`),
	}
	src := &adapters.Source{Selector: "DB.PUBLIC.ORDERS"}
	spec, err := a.BuildIngestSpec(context.Background(), conn, src)
	require.NoError(t, err)
	require.Equal(t, "warehouse", spec.Name)
	require.Equal(t, ConnectorType, spec.Source)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(spec.Config, &cfg))
	require.Equal(t, "xy12345.eu-central-1", cfg["account"])
	require.Equal(t, "DB", cfg["database"])
	require.Equal(t, "PUBLIC", cfg["schema"])
	require.Equal(t, "ORDERS", cfg["table"])
	require.Equal(t, "WH", cfg["warehouse"])
	require.Equal(t, "R", cfg["role"])
}

func TestBuildIngestSpecRejectsBadSelector(t *testing.T) {
	a := New()
	conn := &models.Connection{
		Name:          "warehouse",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"a","database":"DB","schema":"PUBLIC","oauth_token":"t"}`),
	}
	_, err := a.BuildIngestSpec(context.Background(), conn, &adapters.Source{Selector: "a.b.c.d.e"})
	require.Error(t, err)
}

// fakeSnowflake is a minimal in-process replica of the subset of the
// Snowflake `/api/v2/statements` REST API the adapter exercises:
//
//   - POST /api/v2/statements                    → execute statement
//   - GET  /api/v2/statements/{handle}?partition → fetch partition page
type fakeSnowflake struct {
	t            *testing.T
	expectBearer string
	expectKind   string

	pages           []map[string]any
	statementHandle string
	lastBody        map[string]any
}

func (f *fakeSnowflake) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if f.expectBearer != "" {
			require.Equal(f.t, "Bearer "+f.expectBearer, r.Header.Get("Authorization"))
		}
		if f.expectKind != "" {
			require.Equal(f.t, f.expectKind, r.Header.Get("X-Snowflake-Authorization-Token-Type"))
		}
		require.Equal(f.t, "application/json", r.Header.Get("Accept"))

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/statements":
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			require.NoError(f.t, json.Unmarshal(body, &f.lastBody))
			require.NotEmpty(f.t, f.pages, "fakeSnowflake.pages not configured")
			page := f.pages[0]
			if f.statementHandle != "" {
				page["statementHandle"] = f.statementHandle
			}
			writeJSON(w, page)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/statements/"):
			require.NotEmpty(f.t, r.URL.Query().Get("partition"))
			idx, err := strconv.Atoi(r.URL.Query().Get("partition"))
			require.NoError(f.t, err)
			require.Less(f.t, idx, len(f.pages), "fakeSnowflake: partition out of range")
			writeJSON(w, f.pages[idx])
		default:
			http.Error(w, "unknown path: "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestDiscoverSourcesAgainstFakeREST(t *testing.T) {
	fake := &fakeSnowflake{
		t:            t,
		expectBearer: "live-token",
		expectKind:   "OAUTH",
		pages: []map[string]any{
			{
				"resultSetMetaData": map[string]any{
					"rowType": []any{
						map[string]any{"name": "name"},
						map[string]any{"name": "kind"},
					},
				},
				"data": []any{
					[]any{"orders", "TABLE"},
					[]any{"customers", "VIEW"},
				},
			},
		},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	a := New()
	a.SetBaseURL(srv.URL)
	a.SetHTTPClient(srv.Client())

	conn := &models.Connection{
		ID:            uuid.New(),
		Name:          "sf",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"acct","database":"DB","schema":"PUBLIC","oauth_token":"live-token"}`),
	}
	out, err := a.DiscoverSources(context.Background(), conn, "")
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "DB.PUBLIC.orders", out[0].Selector)
	require.Equal(t, "DB.PUBLIC.customers", out[1].Selector)
	require.Equal(t, defaultSourceKind, out[0].SourceKind)
	require.True(t, out[0].SupportsZeroCopy)
	require.Contains(t, fake.lastBody["statement"], "SHOW TABLES IN SCHEMA DB.PUBLIC")
}

func TestQueryVirtualTableAgainstFakeREST(t *testing.T) {
	fake := &fakeSnowflake{
		t:            t,
		expectBearer: "live-token",
		expectKind:   "OAUTH",
		pages: []map[string]any{
			{
				"resultSetMetaData": map[string]any{
					"rowType": []any{
						map[string]any{"name": "ID"},
						map[string]any{"name": "NAME"},
					},
				},
				"data": []any{
					[]any{"1", "Alice"},
				},
			},
		},
		statementHandle: "stmt-42",
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	a := New()
	a.SetBaseURL(srv.URL)
	a.SetHTTPClient(srv.Client())

	conn := &models.Connection{
		Name:          "sf",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"acct","database":"DB","schema":"PUBLIC","oauth_token":"live-token"}`),
	}
	limit := 5
	res, err := a.QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "ORDERS", Limit: &limit}, "")
	require.NoError(t, err)
	require.Equal(t, []string{"ID", "NAME"}, res.Columns)
	require.Equal(t, 1, res.RowCount)
	require.Contains(t, fake.lastBody["statement"], "SELECT * FROM DB.PUBLIC.ORDERS LIMIT 5")

	params := fake.lastBody["parameters"].(map[string]any)
	require.Equal(t, "5", params["ROWS_PER_RESULTSET"])

	var row map[string]any
	require.NoError(t, json.Unmarshal(res.Rows[0], &row))
	require.Equal(t, "Alice", row["NAME"])
}

func TestStreamArrowFanOutsAcrossPartitions(t *testing.T) {
	fake := &fakeSnowflake{
		t:            t,
		expectBearer: "live-token",
		expectKind:   "OAUTH",
		pages: []map[string]any{
			{
				"resultSetMetaData": map[string]any{
					"rowType": []any{
						map[string]any{"name": "ID"},
						map[string]any{"name": "NAME"},
					},
					"partitionInfo": []any{
						map[string]any{"rowCount": 1},
						map[string]any{"rowCount": 1},
						map[string]any{"rowCount": 1},
					},
				},
				"data": []any{[]any{"1", "Alice"}},
			},
			{"data": []any{[]any{"2", "Bob"}}},
			{"data": []any{[]any{"3", "Carol"}}},
		},
		statementHandle: "stmt-7",
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	a := New()
	a.SetBaseURL(srv.URL)
	a.SetHTTPClient(srv.Client())

	conn := &models.Connection{
		Name:          "sf",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"acct","database":"DB","schema":"PUBLIC","oauth_token":"live-token"}`),
	}
	stream, err := a.StreamArrow(context.Background(), conn, &adapters.Query{Selector: "ORDERS"}, "")
	require.NoError(t, err)
	defer stream.Close()

	frame, err := stream.Next(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, frame)
	_, err = stream.Next(context.Background())
	require.ErrorIs(t, err, io.EOF)

	rdr, err := ipc.NewReader(strings.NewReader(string(frame)), ipc.WithAllocator(memory.NewGoAllocator()))
	require.NoError(t, err)
	defer rdr.Release()

	require.True(t, rdr.Next(), "expected at least one record batch")
	rec := rdr.RecordBatch()
	require.Equal(t, int64(3), rec.NumRows())
	require.Equal(t, int64(2), rec.NumCols())
	require.Equal(t, "ID", rec.Schema().Field(0).Name)
}

func TestExecuteStatementSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"denied"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	a := New()
	a.SetBaseURL(srv.URL)
	a.SetHTTPClient(srv.Client())

	conn := &models.Connection{
		Name:          "sf",
		ConnectorType: ConnectorType,
		Config:        json.RawMessage(`{"account":"a","database":"DB","schema":"PUBLIC","oauth_token":"t"}`),
	}
	_, err := a.DiscoverSources(context.Background(), conn, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 403")
}

func newRSAPEM(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return priv, pemBytes
}

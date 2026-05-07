// Package snowflake is the Go port of the Rust Snowflake connector that
// lives in `services/connector-management-service/src/connectors/snowflake.rs`.
//
// The Rust connector is a productive REST implementation against
// Snowflake's `/api/v2/statements` endpoint. Both supported auth flavours
// are mirrored 1:1:
//
//   - Keypair JWT (preferred) — `private_key_pem` (PKCS#8 RSA) plus
//     `public_key_fingerprint` (`SHOW USERS` exposes it as
//     `RSA_PUBLIC_KEY_FP`). The adapter mints an RS256 JWT whose `iss`
//     and `sub` claims encode the qualified Snowflake user
//     (`{ACCOUNT}.{USER}`). The JWT itself is the bearer token (no
//     OAuth exchange).
//   - OAuth bearer — `oauth_token` is forwarded as
//     `Authorization: Bearer …` with
//     `X-Snowflake-Authorization-Token-Type: OAUTH`.
//
// Discovery runs `SHOW TABLES IN SCHEMA :database.:schema`; sync executes
// `SELECT * … LIMIT n` (or a user-supplied SQL) and paginates over the
// `partitionInfo[]` array via `?partition=N`. Result rows are
// materialised as Arrow IPC for the dataset-versioning-service to ingest
// as a new dataset version, mirroring the Rust pipeline exactly.
//
// The slice description for CMA-3 mentions "Arrow via gosnowflake"; the
// Rust source does not in fact use a Snowflake driver — it materialises
// JSON rows into Arrow IPC the same way BigQuery does. The 1:1 port
// keeps that REST-based path so wire shapes (HTTP envelopes, source
// signatures, partition fan-out cap) stay byte-identical.
package snowflake

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/golang-jwt/jwt/v5"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

const (
	// ConnectorType is the `connections.connector_type` value the registry
	// binds this adapter under. Mirrors the Rust `CONNECTOR_NAME`
	// constant in `src/connectors/snowflake.rs`.
	ConnectorType = "snowflake"

	defaultSourceKind = "snowflake_table"
	defaultPageSize   = int64(1_000)
	maxPageSize       = int64(100_000)
	// maxPartitions caps the number of `partitionInfo[]` slices the
	// adapter will fetch in a single sync. Mirrors Rust's
	// `MAX_PARTITIONS = 50`.
	maxPartitions = 50
	// jwtTTLSeconds matches Rust's `now + 3_600` exp claim.
	jwtTTLSeconds = 3_600
)

// Adapter is the Snowflake [adapters.ConnectorAdapter] implementation.
// It is safe for concurrent use; the embedded HTTP client is reused
// across calls.
type Adapter struct {
	httpClient *http.Client
	// baseURLOverride lets tests point the adapter at an httptest server
	// instead of `https://{account}.snowflakecomputing.com`. Production
	// path leaves it empty so [Adapter.baseURL] derives the host from
	// the connection config exactly as Rust does.
	baseURLOverride string
	now             func() time.Time
}

// New returns a ready-to-use [Adapter] backed by [http.DefaultClient]
// and the real Snowflake account-derived base URL.
func New() *Adapter {
	return &Adapter{
		httpClient: http.DefaultClient,
		now:        time.Now,
	}
}

// Factory returns an [adapters.Factory] that constructs fresh Snowflake
// adapters. Per-connection state (HTTP pool, base-URL overrides used by
// tests) is scoped to the constructed value rather than a package-level
// singleton.
func Factory() adapters.Factory {
	return adapters.FactoryFunc(func() adapters.ConnectorAdapter { return New() })
}

// SetHTTPClient overrides the embedded [http.Client]. Intended for
// tests that need to point the adapter at an httptest server.
func (a *Adapter) SetHTTPClient(client *http.Client) {
	if client != nil {
		a.httpClient = client
	}
}

// SetBaseURL overrides the base URL the adapter resolves
// `/api/v2/statements` against. Used by tests; production keeps the
// account-derived host.
func (a *Adapter) SetBaseURL(base string) {
	a.baseURLOverride = base
}

// SetNowFunc overrides the wallclock used for JWT iat/exp claims. Tests
// supply a fixed clock so the assertion produced by RS256 signing is
// deterministic; production uses [time.Now].
func (a *Adapter) SetNowFunc(fn func() time.Time) {
	if fn != nil {
		a.now = fn
	}
}

// sfConfig is the parsed shape of `connections.config` for Snowflake
// connections. Field names mirror the keys Rust pulls out of the
// `serde_json::Value` config in `src/connectors/snowflake.rs`.
type sfConfig struct {
	Account              string `json:"account"`
	Database             string `json:"database"`
	Schema               string `json:"schema"`
	Warehouse            string `json:"warehouse"`
	Role                 string `json:"role"`
	User                 string `json:"user"`
	PrivateKeyPEM        string `json:"private_key_pem"`
	PublicKeyFingerprint string `json:"public_key_fingerprint"`
	OAuthToken           string `json:"oauth_token"`
	Query                string `json:"query"`
	PageSize             *int64 `json:"page_size"`
}

func parseConfig(raw json.RawMessage) (*sfConfig, error) {
	cfg := &sfConfig{}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("snowflake: invalid config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig mirrors Rust's `validate_config`: account/database/
// schema must be non-empty, plus either keypair-JWT or OAuth credentials.
func ValidateConfig(raw json.RawMessage) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	for _, kv := range []struct {
		field string
		value string
	}{
		{"account", cfg.Account},
		{"database", cfg.Database},
		{"schema", cfg.Schema},
	} {
		if strings.TrimSpace(kv.value) == "" {
			return fmt.Errorf("%s connector requires '%s'", ConnectorType, kv.field)
		}
	}
	hasJWT := cfg.PrivateKeyPEM != "" && cfg.PublicKeyFingerprint != "" && cfg.User != ""
	hasOAuth := cfg.OAuthToken != ""
	if !hasJWT && !hasOAuth {
		return fmt.Errorf("%s connector requires either ('user' + 'private_key_pem' + 'public_key_fingerprint') or 'oauth_token'", ConnectorType)
	}
	return nil
}

// DiscoverSources runs `SHOW TABLES IN SCHEMA :database.:schema` and
// returns one [adapters.Source] per row. Mirrors Rust's
// `discover_sources`.
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	cfg, err := a.cfg(c)
	if err != nil {
		return nil, err
	}
	statement := fmt.Sprintf("SHOW TABLES IN SCHEMA %s.%s", cfg.Database, cfg.Schema)
	resp, err := a.executeStatement(ctx, cfg, statement, nil)
	if err != nil {
		return nil, err
	}
	cols := extractColumns(resp["resultSetMetaData"])
	rows := extractRows(resp["data"], cols)
	out := make([]adapters.Source, 0, len(rows))
	for _, row := range rows {
		name := stringFromRow(row, "name", "NAME")
		if name == "" {
			continue
		}
		selector := fmt.Sprintf("%s.%s.%s", cfg.Database, cfg.Schema, name)
		meta, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("snowflake: marshal row metadata: %w", err)
		}
		out = append(out, adapters.Source{
			Selector:         selector,
			DisplayName:      selector,
			SourceKind:       defaultSourceKind,
			SupportsSync:     true,
			SupportsZeroCopy: true,
			Metadata:         meta,
		})
	}
	return out, nil
}

// QueryVirtualTable runs a bounded `SELECT * … LIMIT n` against the
// Snowflake REST API and returns the rows as JSON. Mirrors Rust's
// `query_virtual_table`.
func (a *Adapter) QueryVirtualTable(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (*adapters.Result, error) {
	if q == nil {
		return nil, errors.New("snowflake: query request is nil")
	}
	cfg, err := a.cfg(c)
	if err != nil {
		return nil, err
	}
	limit := int64(50)
	if q.Limit != nil {
		limit = int64(*q.Limit)
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	statement, err := buildQuery(cfg, q.Selector, limit)
	if err != nil {
		return nil, err
	}
	resp, err := a.executeStatement(ctx, cfg, statement, &limit)
	if err != nil {
		return nil, err
	}
	cols := extractColumns(resp["resultSetMetaData"])
	rows := extractRows(resp["data"], cols)
	statementHandle, _ := resp["statementHandle"].(string)
	meta, _ := json.Marshal(map[string]any{
		"statement":        statement,
		"statement_handle": statementHandle,
	})
	rawRows := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		buf, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("snowflake: marshal preview row: %w", err)
		}
		rawRows = append(rawRows, buf)
	}
	return &adapters.Result{
		Selector: q.Selector,
		Mode:     "zero_copy",
		Columns:  cols,
		RowCount: len(rawRows),
		Rows:     rawRows,
		Metadata: meta,
	}, nil
}

// StreamArrow runs the same `SELECT * … LIMIT n` path as
// [Adapter.QueryVirtualTable] but materialises the full multi-partition
// result set into a single Arrow IPC frame, mirroring Rust's
// `fetch_dataset` + `arrow_payload_from_rows`. The first partition is
// served inline; partitions 1..min(N, maxPartitions) are fetched via
// `GET /api/v2/statements/{handle}?partition=N`.
func (a *Adapter) StreamArrow(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (adapters.ArrowStream, error) {
	if q == nil {
		return nil, errors.New("snowflake: query request is nil")
	}
	cfg, err := a.cfg(c)
	if err != nil {
		return nil, err
	}
	limit := pageSize(cfg)
	statement, err := buildQuery(cfg, q.Selector, limit)
	if err != nil {
		return nil, err
	}
	resp, err := a.executeStatement(ctx, cfg, statement, &limit)
	if err != nil {
		return nil, err
	}
	cols := extractColumns(resp["resultSetMetaData"])
	rows := extractRows(resp["data"], cols)
	statementHandle, _ := resp["statementHandle"].(string)

	partitions := partitionInfo(resp["resultSetMetaData"])
	if len(partitions) > 1 && statementHandle != "" {
		cap := len(partitions)
		if cap > maxPartitions {
			cap = maxPartitions
		}
		for partitionIndex := 1; partitionIndex < cap; partitionIndex++ {
			next, err := a.fetchPartition(ctx, cfg, statementHandle, partitionIndex)
			if err != nil {
				return nil, err
			}
			rows = append(rows, extractRows(next["data"], cols)...)
		}
	}
	frame, err := materializeArrowStream(cols, rows)
	if err != nil {
		return nil, err
	}
	return &singleFrameStream{frame: frame}, nil
}

// BuildIngestSpec emits an [adapters.IngestSpec] descriptor that the
// ingestion bridge uses to schedule a downstream sync. The Rust pipeline
// does not yet wire Snowflake into ingestion-replication-service, so the
// shape we emit is the structural placeholder agreed in CMA-0: source
// discriminator "snowflake" + a JSON config carrying account/database/
// schema/table (plus warehouse/role/query when set on the connection).
// Once the bridge gains a typed Snowflake variant, only the marshalling
// here needs to update.
func (a *Adapter) BuildIngestSpec(_ context.Context, c *models.Connection, src *adapters.Source) (*adapters.IngestSpec, error) {
	if c == nil {
		return nil, errors.New("snowflake: connection is nil")
	}
	if src == nil {
		return nil, errors.New("snowflake: source is nil")
	}
	cfg, err := parseConfig(c.Config)
	if err != nil {
		return nil, err
	}
	account := strings.TrimSpace(cfg.Account)
	if account == "" {
		return nil, errors.New("snowflake: connection config missing 'account'")
	}
	database, schema, table, err := splitSelector(cfg, src.Selector)
	if err != nil {
		return nil, err
	}
	specCfg := map[string]any{
		"account":  normalizeAccount(account),
		"database": database,
		"schema":   schema,
		"table":    table,
	}
	if cfg.Warehouse != "" {
		specCfg["warehouse"] = cfg.Warehouse
	}
	if cfg.Role != "" {
		specCfg["role"] = cfg.Role
	}
	if cfg.Query != "" {
		specCfg["query"] = cfg.Query
	}
	raw, err := json.Marshal(specCfg)
	if err != nil {
		return nil, fmt.Errorf("snowflake: marshal ingest spec: %w", err)
	}
	return &adapters.IngestSpec{
		Name:      c.Name,
		Namespace: "default",
		Source:    ConnectorType,
		Config:    raw,
	}, nil
}

func (a *Adapter) cfg(c *models.Connection) (*sfConfig, error) {
	if c == nil {
		return nil, errors.New("snowflake: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	return parseConfig(c.Config)
}

// executeStatement POSTs `statement` (with optional `ROWS_PER_RESULTSET`
// limit) to `/api/v2/statements`. Mirrors Rust's `execute_statement`.
func (a *Adapter) executeStatement(ctx context.Context, cfg *sfConfig, statement string, limit *int64) (map[string]any, error) {
	u, err := a.statementURL(cfg)
	if err != nil {
		return nil, err
	}
	token, err := a.obtainToken(cfg)
	if err != nil {
		return nil, err
	}
	body := buildStatementBody(cfg, statement, limit)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("snowflake: marshal statement body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("snowflake: build statement request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.token)
	req.Header.Set("X-Snowflake-Authorization-Token-Type", token.kind)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snowflake: statement transport error: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Snowflake statements returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("snowflake: decode statement response: %w", err)
	}
	return out, nil
}

// fetchPartition issues `GET /api/v2/statements/{handle}?partition=N`.
// Mirrors Rust's `fetch_partition`.
func (a *Adapter) fetchPartition(ctx context.Context, cfg *sfConfig, handle string, partition int) (map[string]any, error) {
	u, err := a.partitionURL(cfg, handle, partition)
	if err != nil {
		return nil, err
	}
	token, err := a.obtainToken(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("snowflake: build partition request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.token)
	req.Header.Set("X-Snowflake-Authorization-Token-Type", token.kind)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snowflake: partition transport error: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Snowflake partition fetch HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("snowflake: decode partition response: %w", err)
	}
	return out, nil
}

// authToken pairs a bearer token with the
// `X-Snowflake-Authorization-Token-Type` value the REST API expects.
type authToken struct {
	token string
	kind  string
}

// obtainToken builds the bearer token for a request. OAuth bearer is
// passed through; keypair JWT is minted on every call so the adapter
// stays stateless. Mirrors Rust's `obtain_token`.
func (a *Adapter) obtainToken(cfg *sfConfig) (*authToken, error) {
	if cfg.OAuthToken != "" {
		return &authToken{token: cfg.OAuthToken, kind: "OAUTH"}, nil
	}
	if cfg.User == "" {
		return nil, errors.New("snowflake connector requires 'user' for keypair JWT")
	}
	if cfg.PublicKeyFingerprint == "" {
		return nil, errors.New("snowflake connector requires 'public_key_fingerprint' for keypair JWT")
	}
	if cfg.PrivateKeyPEM == "" {
		return nil, errors.New("snowflake connector requires 'private_key_pem' for keypair JWT")
	}
	user := strings.ToUpper(cfg.User)
	account := normalizeAccount(cfg.Account)
	qualifiedUser := fmt.Sprintf("%s.%s", account, user)
	now := a.now().UTC().Unix()
	claims := jwt.MapClaims{
		"iss": fmt.Sprintf("%s.%s", qualifiedUser, cfg.PublicKeyFingerprint),
		"sub": qualifiedUser,
		"iat": now,
		"exp": now + jwtTTLSeconds,
	}
	key, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid snowflake private key: %w", err)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(key)
	if err != nil {
		return nil, fmt.Errorf("snowflake: sign jwt: %w", err)
	}
	return &authToken{token: signed, kind: "KEYPAIR_JWT"}, nil
}

func parsePrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("not PEM-encoded")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return k, nil
	case "PRIVATE KEY":
		anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := anyKey.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA key")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}

// buildStatementBody mirrors Rust's `build_statement_body`. Optional
// fields (warehouse/role) are only emitted when present, matching the
// shape Snowflake's REST API expects.
func buildStatementBody(cfg *sfConfig, statement string, limit *int64) map[string]any {
	body := map[string]any{
		"statement": statement,
		"timeout":   60,
	}
	if cfg.Database != "" {
		body["database"] = cfg.Database
	}
	if cfg.Schema != "" {
		body["schema"] = cfg.Schema
	}
	if cfg.Warehouse != "" {
		body["warehouse"] = cfg.Warehouse
	}
	if cfg.Role != "" {
		body["role"] = cfg.Role
	}
	if limit != nil {
		body["parameters"] = map[string]any{
			"ROWS_PER_RESULTSET": strconv.FormatInt(*limit, 10),
		}
	}
	return body
}

func (a *Adapter) statementURL(cfg *sfConfig) (*url.URL, error) {
	base, err := a.baseURL(cfg)
	if err != nil {
		return nil, err
	}
	rel, err := url.Parse("/api/v2/statements")
	if err != nil {
		return nil, fmt.Errorf("snowflake: parse statements path: %w", err)
	}
	return base.ResolveReference(rel), nil
}

func (a *Adapter) partitionURL(cfg *sfConfig, handle string, partition int) (*url.URL, error) {
	base, err := a.baseURL(cfg)
	if err != nil {
		return nil, err
	}
	rel, err := url.Parse(fmt.Sprintf("/api/v2/statements/%s", url.PathEscape(handle)))
	if err != nil {
		return nil, fmt.Errorf("snowflake: parse partition path: %w", err)
	}
	out := base.ResolveReference(rel)
	q := out.Query()
	q.Set("partition", strconv.Itoa(partition))
	out.RawQuery = q.Encode()
	return out, nil
}

// baseURL returns the Snowflake REST host. Production derives it from
// the account ID (`https://{account}.snowflakecomputing.com`); tests
// can override via [Adapter.SetBaseURL].
func (a *Adapter) baseURL(cfg *sfConfig) (*url.URL, error) {
	if a.baseURLOverride != "" {
		u, err := url.Parse(a.baseURLOverride)
		if err != nil {
			return nil, fmt.Errorf("snowflake: parse base url override: %w", err)
		}
		return u, nil
	}
	account := strings.TrimSpace(cfg.Account)
	if account == "" {
		return nil, errors.New("snowflake connector requires 'account'")
	}
	raw := fmt.Sprintf("https://%s.snowflakecomputing.com", normalizeAccount(account))
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("snowflake: parse base url: %w", err)
	}
	return u, nil
}

func normalizeAccount(account string) string {
	return strings.TrimRight(strings.TrimSpace(account), ".")
}

// buildQuery mirrors Rust's `build_query`: pass through SQL that already
// starts with SELECT, qualify a bare table identifier, or fall back to
// a `query` field on the connection config.
func buildQuery(cfg *sfConfig, selector string, limit int64) (string, error) {
	trimmed := strings.TrimSpace(selector)
	if strings.HasPrefix(strings.ToLower(trimmed), "select ") {
		return trimmed, nil
	}
	if trimmed != "" {
		qualified, err := qualifyTable(cfg, trimmed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("SELECT * FROM %s LIMIT %d", qualified, limit), nil
	}
	if q := strings.TrimSpace(cfg.Query); q != "" {
		return q, nil
	}
	return "", errors.New("snowflake sync requires a SQL query or table selector")
}

// qualifyTable mirrors Rust's `qualify_table`: pass through dotted
// identifiers, otherwise prefix with `{database}.{schema}`.
func qualifyTable(cfg *sfConfig, selector string) (string, error) {
	if strings.Contains(selector, ".") {
		return selector, nil
	}
	if cfg.Database == "" {
		return "", errors.New("snowflake connector requires 'database'")
	}
	if cfg.Schema == "" {
		return "", errors.New("snowflake connector requires 'schema'")
	}
	return fmt.Sprintf("%s.%s.%s", cfg.Database, cfg.Schema, selector), nil
}

// pageSize mirrors Rust's `page_size`: defaults to 1000, clamps into
// [1, 100_000].
func pageSize(cfg *sfConfig) int64 {
	if cfg.PageSize == nil {
		return defaultPageSize
	}
	v := *cfg.PageSize
	if v < 1 {
		return 1
	}
	if v > maxPageSize {
		return maxPageSize
	}
	return v
}

// extractColumns mirrors Rust's `extract_columns`: pulls the
// `resultSetMetaData.rowType[].name` ordered list of column names.
func extractColumns(meta any) []string {
	obj, ok := meta.(map[string]any)
	if !ok {
		return nil
	}
	fields, ok := obj["rowType"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		field, ok := f.(map[string]any)
		if !ok {
			continue
		}
		name, _ := field["name"].(string)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// extractRows mirrors Rust's `extract_rows`: each Snowflake row is an
// array of cells positionally aligned with `columns`. Missing cells map
// to nil.
func extractRows(data any, columns []string) []map[string]any {
	rowList, ok := data.([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(rowList))
	for _, item := range rowList {
		cells, _ := item.([]any)
		obj := make(map[string]any, len(columns))
		for idx, col := range columns {
			if idx < len(cells) {
				obj[col] = cells[idx]
			} else {
				obj[col] = nil
			}
		}
		out = append(out, obj)
	}
	return out
}

func partitionInfo(meta any) []any {
	obj, ok := meta.(map[string]any)
	if !ok {
		return nil
	}
	parts, _ := obj["partitionInfo"].([]any)
	return parts
}

func stringFromRow(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// splitSelector decomposes a "database.schema.table" selector for
// [Adapter.BuildIngestSpec]. Bare or two-part selectors fall back to the
// connection's database/schema.
func splitSelector(cfg *sfConfig, selector string) (database, schema, table string, err error) {
	parts := strings.Split(strings.TrimSpace(selector), ".")
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], nil
	case 2:
		if cfg.Database == "" {
			return "", "", "", fmt.Errorf("snowflake: selector %q needs a 'database' on the connection", selector)
		}
		return cfg.Database, parts[0], parts[1], nil
	case 1:
		if parts[0] == "" {
			return "", "", "", fmt.Errorf("snowflake: selector is empty")
		}
		if cfg.Database == "" || cfg.Schema == "" {
			return "", "", "", fmt.Errorf("snowflake: selector %q needs 'database' and 'schema' on the connection", selector)
		}
		return cfg.Database, cfg.Schema, parts[0], nil
	default:
		return "", "", "", fmt.Errorf("snowflake: selector %q must be 'database.schema.table'", selector)
	}
}

// materializeArrowStream encodes columns + rows as a single Arrow IPC
// stream. All columns are nullable Utf8, mirroring Rust's
// `materialize_arrow_stream` in `src/connectors/mod.rs`.
func materializeArrowStream(columns []string, rows []map[string]any) ([]byte, error) {
	mem := memory.NewGoAllocator()
	fields := make([]arrow.Field, 0, len(columns))
	arrays := make([]arrow.Array, 0, len(columns))
	for _, name := range columns {
		fields = append(fields, arrow.Field{Name: name, Type: arrow.BinaryTypes.String, Nullable: true})
		builder := array.NewStringBuilder(mem)
		for _, row := range rows {
			value, ok := row[name]
			if !ok || value == nil {
				builder.AppendNull()
				continue
			}
			switch v := value.(type) {
			case string:
				builder.Append(v)
			default:
				builder.Append(fmt.Sprint(v))
			}
		}
		arr := builder.NewArray()
		arrays = append(arrays, arr)
		builder.Release()
	}
	schema := arrow.NewSchema(fields, nil)
	rec := array.NewRecord(schema, arrays, int64(len(rows)))
	defer rec.Release()
	for _, arr := range arrays {
		arr.Release()
	}
	var buf bytes.Buffer
	writer := ipc.NewWriter(&buf, ipc.WithSchema(schema), ipc.WithAllocator(mem))
	if err := writer.Write(rec); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("snowflake: write arrow record: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("snowflake: close arrow stream: %w", err)
	}
	return buf.Bytes(), nil
}

type singleFrameStream struct {
	frame    []byte
	consumed bool
}

func (s *singleFrameStream) Next(_ context.Context) ([]byte, error) {
	if s.consumed {
		return nil, io.EOF
	}
	s.consumed = true
	return s.frame, nil
}

func (s *singleFrameStream) Close() error { return nil }

// PostgreSQL-backed repository boundary for residual ontology
// definitions.
//
// S1 keeps declarative ontology metadata in `pg-schemas` while runtime
// objects, links, action logs and materialized read models move to
// Cassandra and search-backed stores. HTTP handlers should not embed
// `pgx` calls directly; any remaining PG interaction is routed
// through this module or a typed repository built on top of it.
//
// Mirrors `libs/ontology-kernel/src/domain/pg_repository.rs`. The
// Rust source uses `sqlx::QueryBuilder<Postgres>` for dynamic WHERE
// clauses; Go uses [pgQueryBuilder] (declared at the bottom of this
// file) which keeps the same `$N` parameter discipline pgx requires.

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// definitionTable mirrors `struct DefinitionTable` — the metadata
// row used by [definitionTableFor] to map a logical
// [DefinitionKind] to its PG table + filter/search column set.
type definitionTable struct {
	table      string
	ownerCol   string
	parentCol  string
	searchCols []string
	filterCols []string
}

// definitionTableFor mirrors `fn definition_table`. Returns the
// metadata + ok=true on a known kind, the zero value + ok=false
// otherwise (Rust `Option::None`).
func definitionTableFor(kind storageabstraction.DefinitionKind) (definitionTable, bool) {
	switch string(kind) {
	case "object_type", "object_types":
		return definitionTable{
			table:      "object_types",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "display_name"},
			filterCols: []string{"name", "owner_id"},
		}, true
	case "property", "properties":
		return definitionTable{
			table:      "properties",
			parentCol:  "object_type_id",
			searchCols: []string{"name", "display_name", "property_type"},
			filterCols: []string{"object_type_id", "name", "property_type"},
		}, true
	case "shared_property_type", "shared_property_types":
		return definitionTable{
			table:      "shared_property_types",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "display_name", "property_type"},
			filterCols: []string{"name", "property_type", "owner_id"},
		}, true
	case "interface", "interfaces":
		return definitionTable{
			table:      "ontology_interfaces",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "display_name"},
			filterCols: []string{"name", "owner_id"},
		}, true
	case "interface_property", "interface_properties":
		return definitionTable{
			table:      "interface_properties",
			parentCol:  "interface_id",
			searchCols: []string{"name", "display_name", "property_type"},
			filterCols: []string{"interface_id", "name", "property_type"},
		}, true
	case "link_type", "link_types":
		return definitionTable{
			table:      "link_types",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "display_name", "cardinality"},
			filterCols: []string{"source_type_id", "target_type_id", "name", "cardinality", "owner_id"},
		}, true
	case "action_type", "action_types":
		return definitionTable{
			table:      "action_types",
			ownerCol:   "owner_id",
			parentCol:  "object_type_id",
			searchCols: []string{"name", "display_name", "operation_kind"},
			filterCols: []string{"object_type_id", "name", "operation_kind", "owner_id"},
		}, true
	case "rule", "rules":
		return definitionTable{
			table:      "ontology_rules",
			ownerCol:   "owner_id",
			parentCol:  "object_type_id",
			searchCols: []string{"name", "display_name", "evaluation_mode"},
			filterCols: []string{"object_type_id", "name", "evaluation_mode", "owner_id"},
		}, true
	case "function_package", "function_packages":
		return definitionTable{
			table:      "ontology_function_packages",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "display_name", "runtime"},
			filterCols: []string{"name", "runtime", "owner_id"},
		}, true
	case "object_set", "object_sets":
		return definitionTable{
			table:      "ontology_object_sets",
			ownerCol:   "owner_id",
			parentCol:  "base_object_type_id",
			searchCols: []string{"name", "description"},
			filterCols: []string{"base_object_type_id", "name", "owner_id"},
		}, true
	case "quiver_visual_function", "quiver_visual_functions":
		return definitionTable{
			table:      "ontology_quiver_visual_functions",
			ownerCol:   "owner_id",
			parentCol:  "primary_type_id",
			searchCols: []string{"name", "description", "chart_kind"},
			filterCols: []string{"primary_type_id", "secondary_type_id", "name", "chart_kind", "owner_id"},
		}, true
	case "project", "projects":
		return definitionTable{
			table:      "ontology_projects",
			ownerCol:   "owner_id",
			searchCols: []string{"slug", "display_name", "workspace_slug"},
			filterCols: []string{"slug", "workspace_slug", "owner_id"},
		}, true
	case "funnel_source", "funnel_sources":
		return definitionTable{
			table:      "ontology_funnel_sources",
			ownerCol:   "owner_id",
			searchCols: []string{"name", "description"},
			filterCols: []string{"name", "owner_id"},
		}, true
	}
	return definitionTable{}, false
}

func unsupportedKind(kind storageabstraction.DefinitionKind) error {
	return storageabstraction.Invalidf("unsupported ontology definition kind '%s'", string(kind))
}

// PostgresDefinitionStore implements [storageabstraction.DefinitionStore]
// against PostgreSQL via pgx. Mirrors `pub struct PostgresDefinitionStore`.
type PostgresDefinitionStore struct {
	Pool *pgxpool.Pool
}

// NewPostgresDefinitionStore mirrors `PostgresDefinitionStore::new(pool)`.
func NewPostgresDefinitionStore(pool *pgxpool.Pool) *PostgresDefinitionStore {
	return &PostgresDefinitionStore{Pool: pool}
}

// Compile-time pin.
var _ storageabstraction.DefinitionStore = (*PostgresDefinitionStore)(nil)

// Get mirrors the Rust impl. Projects the row as `to_jsonb(...)` so
// every PG column becomes a JSON field, then re-projects into the
// generic [DefinitionRecord] shape expected by callers.
func (s *PostgresDefinitionStore) Get(ctx context.Context, kind storageabstraction.DefinitionKind, id storageabstraction.DefinitionId, _ storageabstraction.ReadConsistency) (*storageabstraction.DefinitionRecord, error) {
	table, ok := definitionTableFor(kind)
	if !ok {
		return nil, unsupportedKind(kind)
	}
	sql := fmt.Sprintf(
		"SELECT to_jsonb(row) AS payload FROM (SELECT * FROM %s WHERE id::text = $1) row",
		table.table,
	)
	var payload json.RawMessage
	if err := s.Pool.QueryRow(ctx, sql, string(id)).Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, storageabstraction.Backend(err.Error())
	}
	rec, err := recordFromPayload(kind, table, payload)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// List mirrors the Rust impl. Builds a dynamic WHERE clause over the
// allowed filter columns, paginates via stringified offset tokens
// and returns a fresh `next_token` when the LIMIT+1 fetch sentinel
// proves another page exists.
func (s *PostgresDefinitionStore) List(ctx context.Context, query storageabstraction.DefinitionQuery, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.DefinitionRecord], error) {
	table, ok := definitionTableFor(query.Kind)
	if !ok {
		return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, unsupportedKind(query.Kind)
	}
	offset, err := pageOffset(query.Page)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, err
	}
	limit := query.Page.Size
	if limit < 1 {
		limit = 1
	}

	b := newPGQueryBuilder()
	b.WriteString("SELECT to_jsonb(row) AS payload FROM (SELECT * FROM ")
	b.WriteIdent(table.table)
	b.WriteString(" WHERE TRUE")
	pushDefinitionFilters(b, table, query)
	b.WriteString(" ORDER BY COALESCE(updated_at, created_at) DESC, id ASC LIMIT ")
	b.WriteParam(int64(limit) + 1)
	b.WriteString(" OFFSET ")
	b.WriteParam(int64(offset))
	b.WriteString(") row")

	rows, err := s.Pool.Query(ctx, b.SQL(), b.Args()...)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, storageabstraction.Backend(err.Error())
	}
	defer rows.Close()

	payloads := []json.RawMessage{}
	for rows.Next() {
		var payload json.RawMessage
		if err := rows.Scan(&payload); err != nil {
			return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, storageabstraction.Backend(err.Error())
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, storageabstraction.Backend(err.Error())
	}

	hasMore := uint32(len(payloads)) > limit
	if hasMore {
		payloads = payloads[:limit]
	}
	items := make([]storageabstraction.DefinitionRecord, 0, len(payloads))
	for _, payload := range payloads {
		rec, err := recordFromPayload(query.Kind, table, payload)
		if err != nil {
			return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{}, err
		}
		items = append(items, rec)
	}
	var nextToken *string
	if hasMore {
		token := strconv.FormatUint(uint64(offset+limit), 10)
		nextToken = &token
	}
	return storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{
		Items:     items,
		NextToken: nextToken,
	}, nil
}

// Put mirrors the Rust impl: only `action_type` and `object_set`
// kinds have hand-written upserts. Other kinds reject with the
// verbatim Rust error string.
func (s *PostgresDefinitionStore) Put(ctx context.Context, record storageabstraction.DefinitionRecord, _ *uint64) (storageabstraction.PutOutcome, error) {
	switch string(record.Kind) {
	case "action_type", "action_types":
		return s.putActionTypeDefinition(ctx, record)
	case "object_set", "object_sets":
		return s.putObjectSetDefinition(ctx, record)
	default:
		return storageabstraction.PutOutcome{}, storageabstraction.Backend(
			"PostgresDefinitionStore::put is intentionally not generic; use the typed definition repository for the owning model",
		)
	}
}

// Delete mirrors the Rust impl.
func (s *PostgresDefinitionStore) Delete(ctx context.Context, kind storageabstraction.DefinitionKind, id storageabstraction.DefinitionId) (bool, error) {
	table, ok := definitionTableFor(kind)
	if !ok {
		return false, unsupportedKind(kind)
	}
	sql := fmt.Sprintf("DELETE FROM %s WHERE id::text = $1", table.table)
	tag, err := s.Pool.Exec(ctx, sql, string(id))
	if err != nil {
		return false, storageabstraction.Backend(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

// Count mirrors the Rust impl. The Rust source rejects negative i64
// counts on a dedicated branch — pgx returns int64 from COUNT, so we
// guard the same way before casting to uint64.
func (s *PostgresDefinitionStore) Count(ctx context.Context, query storageabstraction.DefinitionQuery, _ storageabstraction.ReadConsistency) (uint64, error) {
	table, ok := definitionTableFor(query.Kind)
	if !ok {
		return 0, unsupportedKind(query.Kind)
	}
	b := newPGQueryBuilder()
	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteIdent(table.table)
	b.WriteString(" WHERE TRUE")
	pushDefinitionFilters(b, table, query)

	var count int64
	if err := s.Pool.QueryRow(ctx, b.SQL(), b.Args()...).Scan(&count); err != nil {
		return 0, storageabstraction.Backend(err.Error())
	}
	if count < 0 {
		return 0, storageabstraction.Backendf("negative count returned for %s", string(query.Kind))
	}
	return uint64(count), nil
}

// putActionTypeDefinition mirrors `put_action_type_definition`.
func (s *PostgresDefinitionStore) putActionTypeDefinition(ctx context.Context, record storageabstraction.DefinitionRecord) (storageabstraction.PutOutcome, error) {
	payload, err := payloadObject(record.Payload)
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	id, err := payloadUUID(payload, "id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	name, err := payloadString(payload, "name")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	displayName, err := payloadString(payload, "display_name")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	description := payloadOptString(payload, "description")
	objectTypeID, err := payloadUUID(payload, "object_type_id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	operationKind, err := payloadString(payload, "operation_kind")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	inputSchema := payloadJSONOrEmptyArray(payload, "input_schema")
	formSchema := payloadJSONOrEmptyObject(payload, "form_schema")
	config := payloadJSONOrNull(payload, "config")
	confirmationRequired := payloadBool(payload, "confirmation_required")
	permissionKey := payloadOptStringPtr(payload, "permission_key")
	authorizationPolicy := payloadJSONOrEmptyObject(payload, "authorization_policy")
	ownerID, err := payloadUUID(payload, "owner_id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}

	var inserted bool
	err = s.Pool.QueryRow(ctx, `INSERT INTO action_types (
               id, name, display_name, description, object_type_id, operation_kind,
               input_schema, form_schema, config, confirmation_required, permission_key,
               authorization_policy, owner_id
           ) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9::jsonb, $10, $11, $12::jsonb, $13)
           ON CONFLICT (id) DO UPDATE SET
               display_name = EXCLUDED.display_name,
               description = EXCLUDED.description,
               operation_kind = EXCLUDED.operation_kind,
               input_schema = EXCLUDED.input_schema,
               form_schema = EXCLUDED.form_schema,
               config = EXCLUDED.config,
               confirmation_required = EXCLUDED.confirmation_required,
               permission_key = EXCLUDED.permission_key,
               authorization_policy = EXCLUDED.authorization_policy,
               updated_at = now()
           RETURNING (xmax = 0) AS inserted`,
		id, name, displayName, description, objectTypeID, operationKind,
		inputSchema, formSchema, config, confirmationRequired, permissionKey,
		authorizationPolicy, ownerID,
	).Scan(&inserted)
	if err != nil {
		return storageabstraction.PutOutcome{}, storageabstraction.Backend(err.Error())
	}
	if inserted {
		return storageabstraction.Inserted(), nil
	}
	return storageabstraction.Updated(0, 0), nil
}

// putObjectSetDefinition mirrors `put_object_set_definition`.
func (s *PostgresDefinitionStore) putObjectSetDefinition(ctx context.Context, record storageabstraction.DefinitionRecord) (storageabstraction.PutOutcome, error) {
	payload, err := payloadObject(record.Payload)
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	id, err := payloadUUID(payload, "id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	name, err := payloadString(payload, "name")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	description := payloadOptString(payload, "description")
	baseObjectTypeID, err := payloadUUID(payload, "base_object_type_id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	filters := payloadJSONOrEmptyArray(payload, "filters")
	traversals := payloadJSONOrEmptyArray(payload, "traversals")
	// Rust accepts either `join` or `join_config`; first non-null wins.
	joinConfig := payloadFirstJSONNonNull(payload, "join", "join_config")
	projections := payloadJSONOrEmptyArray(payload, "projections")
	whatIfLabel := payloadOptStringPtr(payload, "what_if_label")
	policy := payloadJSONOrEmptyObject(payload, "policy")
	ownerID, err := payloadUUID(payload, "owner_id")
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}

	var inserted bool
	err = s.Pool.QueryRow(ctx, `INSERT INTO ontology_object_sets (
               id, name, description, base_object_type_id, filters, traversals, join_config,
               projections, what_if_label, policy, owner_id
           ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
           ON CONFLICT (id) DO UPDATE SET
               name = EXCLUDED.name,
               description = EXCLUDED.description,
               base_object_type_id = EXCLUDED.base_object_type_id,
               filters = EXCLUDED.filters,
               traversals = EXCLUDED.traversals,
               join_config = EXCLUDED.join_config,
               projections = EXCLUDED.projections,
               what_if_label = EXCLUDED.what_if_label,
               policy = EXCLUDED.policy,
               updated_at = now()
           RETURNING (xmax = 0) AS inserted`,
		id, name, description, baseObjectTypeID, filters, traversals, joinConfig,
		projections, whatIfLabel, policy, ownerID,
	).Scan(&inserted)
	if err != nil {
		return storageabstraction.PutOutcome{}, storageabstraction.Backend(err.Error())
	}
	if inserted {
		return storageabstraction.Inserted(), nil
	}
	return storageabstraction.Updated(0, 0), nil
}

// recordFromPayload mirrors `fn record_from_payload`. Lifts the row
// JSON into the generic [DefinitionRecord] shape used by callers.
func recordFromPayload(kind storageabstraction.DefinitionKind, table definitionTable, payload json.RawMessage) (storageabstraction.DefinitionRecord, error) {
	obj, err := payloadObject(payload)
	if err != nil {
		return storageabstraction.DefinitionRecord{}, storageabstraction.Backend("definition row did not project an id")
	}
	id, err := payloadString(obj, "id")
	if err != nil {
		return storageabstraction.DefinitionRecord{}, storageabstraction.Backend("definition row did not project an id")
	}
	rec := storageabstraction.DefinitionRecord{
		Kind:    kind,
		ID:      storageabstraction.DefinitionId(id),
		Payload: payload,
	}
	if table.ownerCol != "" {
		if v := payloadOptStringPtr(obj, table.ownerCol); v != nil {
			rec.OwnerID = v
		}
	}
	if table.parentCol != "" {
		if v := payloadOptStringPtr(obj, table.parentCol); v != nil {
			parentID := storageabstraction.DefinitionId(*v)
			rec.ParentID = &parentID
		}
	}
	if v := payloadOptU64(obj, "version"); v != nil {
		rec.Version = v
	} else if v := payloadOptU64(obj, "revision"); v != nil {
		rec.Version = v
	}
	return rec, nil
}

// pushDefinitionFilters mirrors `fn push_definition_filters`.
// Generates parameterised AND clauses over the table's allowed
// filter / search columns.
func pushDefinitionFilters(b *pgQueryBuilder, table definitionTable, query storageabstraction.DefinitionQuery) {
	if table.ownerCol != "" && query.OwnerID != nil {
		b.WriteString(" AND ")
		b.WriteIdent(table.ownerCol)
		b.WriteString("::text = ")
		b.WriteParam(*query.OwnerID)
	}
	if table.parentCol != "" && query.ParentID != nil {
		b.WriteString(" AND ")
		b.WriteIdent(table.parentCol)
		b.WriteString("::text = ")
		b.WriteParam(string(*query.ParentID))
	}
	for column, value := range query.Filters {
		if !columnAllowed(column, table.filterCols) {
			continue
		}
		b.WriteString(" AND ")
		b.WriteIdent(column)
		b.WriteString("::text = ")
		b.WriteParam(value)
	}
	if query.Search != nil {
		search := strings.TrimSpace(*query.Search)
		if search != "" {
			b.WriteString(" AND (")
			for i, column := range table.searchCols {
				if i > 0 {
					b.WriteString(" OR ")
				}
				b.WriteIdent(column)
				b.WriteString(" ILIKE ")
				b.WriteParam("%" + search + "%")
			}
			b.WriteString(")")
		}
	}
}

func columnAllowed(column string, allowed []string) bool {
	for _, a := range allowed {
		if a == column {
			return true
		}
	}
	return false
}

// pageOffset mirrors `fn page_offset`. Token is treated as a
// stringified u32; rejects with the verbatim Rust message.
func pageOffset(page storageabstraction.Page) (uint32, error) {
	if page.Token == nil {
		return 0, nil
	}
	n, err := strconv.ParseUint(*page.Token, 10, 32)
	if err != nil {
		return 0, storageabstraction.Invalid("definition page token is invalid")
	}
	return uint32(n), nil
}

// ---- payload helpers ------------------------------------------------------

func payloadObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, storageabstraction.Backend("definition payload is empty")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, storageabstraction.Backend("definition payload is not a JSON object")
	}
	return obj, nil
}

func payloadString(payload map[string]json.RawMessage, field string) (string, error) {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return "", storageabstraction.Invalidf("definition payload missing %s", field)
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", storageabstraction.Invalidf("definition payload missing %s", field)
	}
	return s, nil
}

func payloadOptString(payload map[string]json.RawMessage, field string) string {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

func payloadOptStringPtr(payload map[string]json.RawMessage, field string) *string {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil
	}
	return &s
}

func payloadUUID(payload map[string]json.RawMessage, field string) (uuid.UUID, error) {
	s, err := payloadString(payload, field)
	if err != nil {
		return uuid.Nil, err
	}
	id, parseErr := uuid.Parse(s)
	if parseErr != nil {
		return uuid.Nil, storageabstraction.Invalidf("%s is not a UUID: %s", field, parseErr)
	}
	return id, nil
}

func payloadBool(payload map[string]json.RawMessage, field string) bool {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return false
	}
	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return false
	}
	return b
}

func payloadOptU64(payload map[string]json.RawMessage, field string) *uint64 {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return nil
	}
	var n uint64
	if err := json.Unmarshal(v, &n); err != nil {
		return nil
	}
	return &n
}

// payloadJSONOrEmptyArray mirrors `fn payload_json` (which falls back
// to `Value::Array(vec![])`).
func payloadJSONOrEmptyArray(payload map[string]json.RawMessage, field string) json.RawMessage {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return json.RawMessage("[]")
	}
	return v
}

// payloadJSONOrEmptyObject mirrors `payload.get(field).cloned().unwrap_or_else(|| json!({}))`.
func payloadJSONOrEmptyObject(payload map[string]json.RawMessage, field string) json.RawMessage {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return json.RawMessage("{}")
	}
	return v
}

// payloadJSONOrNull mirrors `payload.get(field).cloned().unwrap_or(Value::Null)`.
func payloadJSONOrNull(payload map[string]json.RawMessage, field string) json.RawMessage {
	v, ok := payload[field]
	if !ok {
		return json.RawMessage("null")
	}
	return v
}

// payloadFirstJSONNonNull mirrors the
// `payload.get("join").or_else(|| payload.get("join_config")).filter(|v| !v.is_null())`
// chain in `put_object_set_definition`. Returns nil when none of the
// candidates resolve to a non-null value.
func payloadFirstJSONNonNull(payload map[string]json.RawMessage, fields ...string) json.RawMessage {
	for _, field := range fields {
		v, ok := payload[field]
		if !ok {
			continue
		}
		if string(v) == "null" {
			continue
		}
		return v
	}
	return nil
}

// ---- pgQueryBuilder -------------------------------------------------------

// pgQueryBuilder is the Go counterpart of `sqlx::QueryBuilder<Postgres>`.
// It accumulates the SQL text and the corresponding argument vector,
// renumbering `$N` placeholders as parameters are bound.
//
// Only literal SQL fragments + bound parameters cross the boundary;
// identifiers (table names, column names) are concatenated raw and
// callers MUST ensure they originate from the trusted
// [definitionTableFor] map.
type pgQueryBuilder struct {
	sql  strings.Builder
	args []any
}

func newPGQueryBuilder() *pgQueryBuilder { return &pgQueryBuilder{} }

func (b *pgQueryBuilder) WriteString(s string) { b.sql.WriteString(s) }

// WriteIdent writes a raw SQL identifier (table or column). The
// caller is responsible for trusting the input — see the type doc.
func (b *pgQueryBuilder) WriteIdent(ident string) { b.sql.WriteString(ident) }

// WriteParam binds a parameter and writes the next `$N` placeholder.
func (b *pgQueryBuilder) WriteParam(value any) {
	b.args = append(b.args, value)
	b.sql.WriteString("$")
	b.sql.WriteString(strconv.Itoa(len(b.args)))
}

func (b *pgQueryBuilder) SQL() string { return b.sql.String() }
func (b *pgQueryBuilder) Args() []any { return b.args }

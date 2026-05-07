// Package repo holds SQL queries + embedded migrations for
// iceberg-catalog-service.
package repo

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

type Repo struct{ Pool *pgxpool.Pool }

const namespaceSelect = `SELECT id, project_rid, name, parent_namespace_id,
	properties, created_at, created_by FROM iceberg_namespaces`

func (r *Repo) ListNamespaces(ctx context.Context, projectRID string) ([]models.IcebergNamespace, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if projectRID != "" {
		rows, err = r.Pool.Query(ctx, namespaceSelect+` WHERE project_rid = $1 ORDER BY name LIMIT 500`, projectRID)
	} else {
		rows, err = r.Pool.Query(ctx, namespaceSelect+` ORDER BY project_rid, name LIMIT 500`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.IcebergNamespace, 0)
	for rows.Next() {
		v, err := scanNamespace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetNamespace(ctx context.Context, id uuid.UUID) (*models.IcebergNamespace, error) {
	row := r.Pool.QueryRow(ctx, namespaceSelect+` WHERE id = $1`, id)
	v, err := scanNamespace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateNamespace(ctx context.Context, body *models.CreateNamespaceRequest, createdBy uuid.UUID) (*models.IcebergNamespace, error) {
	id := uuid.New()
	props := body.Properties
	if len(props) == 0 {
		props = []byte(`{}`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO iceberg_namespaces
		    (id, project_rid, name, parent_namespace_id, properties, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, project_rid, name, parent_namespace_id, properties,
		           created_at, created_by`,
		id, body.ProjectRID, body.Name, body.ParentNamespaceID, props, createdBy,
	)
	return scanNamespace(row)
}

func (r *Repo) UpdateNamespaceProperties(ctx context.Context, id uuid.UUID, properties []byte) (*models.IcebergNamespace, error) {
	current, err := r.GetNamespace(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	if len(properties) == 0 {
		return current, nil
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE iceberg_namespaces SET properties = $2 WHERE id = $1
		 RETURNING id, project_rid, name, parent_namespace_id, properties,
		           created_at, created_by`,
		id, properties)
	return scanNamespace(row)
}

func (r *Repo) DeleteNamespace(ctx context.Context, id uuid.UUID) (bool, error) {
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM iceberg_namespaces WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

type rowLikeT interface{ Scan(...any) error }

func scanNamespace(r rowLikeT) (*models.IcebergNamespace, error) {
	v := &models.IcebergNamespace{}
	if err := r.Scan(&v.ID, &v.ProjectRID, &v.Name, &v.ParentNamespaceID,
		&v.Properties, &v.CreatedAt, &v.CreatedBy); err != nil {
		return nil, err
	}
	return v, nil
}

const tableSelect = `SELECT t.id, t.rid, t.namespace_id, n.name AS namespace_name,
	t.name, t.table_uuid, t.format_version, t.location, t.current_snapshot_id,
	t.current_metadata_location, t.last_sequence_number, t.partition_spec,
	t.schema_json, t.sort_order, t.properties, t.markings, t.created_at, t.updated_at
	FROM iceberg_tables t JOIN iceberg_namespaces n ON n.id = t.namespace_id`

func (r *Repo) ListTables(ctx context.Context, projectRID string, namespace []string) ([]models.IcebergTable, error) {
	name := encodePath(namespace)
	rows, err := r.Pool.Query(ctx, tableSelect+` WHERE n.project_rid = $1 AND n.name = $2 ORDER BY t.name`, projectRID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.IcebergTable, 0)
	for rows.Next() {
		v, err := scanTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetTable(ctx context.Context, projectRID string, namespace []string, tableName string) (*models.IcebergTable, error) {
	row := r.Pool.QueryRow(ctx, tableSelect+` WHERE n.project_rid = $1 AND n.name = $2 AND t.name = $3`, projectRID, encodePath(namespace), tableName)
	v, err := scanTable(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateTable(ctx context.Context, projectRID string, namespace []string, body *models.CreateTableRequest, createdBy uuid.UUID) (*models.IcebergTable, string, error) {
	ns, err := r.GetNamespaceByProjectName(ctx, projectRID, encodePath(namespace))
	if err != nil || ns == nil {
		return nil, "", err
	}
	if strings.TrimSpace(body.Name) == "" {
		return nil, "", fmt.Errorf("table name is required")
	}
	if len(body.Schema) == 0 || !json.Valid(body.Schema) || string(body.Schema) == "null" {
		return nil, "", fmt.Errorf("schema is required")
	}
	formatVersion := int32(2)
	if body.FormatVersion != nil {
		formatVersion = *body.FormatVersion
	}
	if formatVersion < 1 || formatVersion > 3 {
		return nil, "", fmt.Errorf("invalid format-version %d; catalog accepts 1, 2, 3", formatVersion)
	}
	location := ""
	if body.Location != nil && strings.TrimSpace(*body.Location) != "" {
		location = strings.TrimRight(strings.TrimSpace(*body.Location), "/")
	} else {
		location = fmt.Sprintf("s3://openfoundry-warehouse/%s/%s", ns.Name, strings.TrimSpace(body.Name))
	}
	partitionSpec := body.PartitionSpec
	if len(partitionSpec) == 0 {
		partitionSpec = []byte(`{"spec-id":0,"fields":[]}`)
	}
	sortOrder := body.SortOrder
	if len(sortOrder) == 0 {
		sortOrder = []byte(`{"order-id":0,"fields":[]}`)
	}
	props, err := json.Marshal(body.Properties)
	if err != nil {
		return nil, "", err
	}
	markings := body.Markings
	if len(markings) == 0 {
		markings = []string{"public"}
	}
	id := uuid.New()
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO iceberg_tables (id, namespace_id, name, table_uuid, format_version, location,
		    partition_spec, schema_json, sort_order, properties, markings)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id, rid, namespace_id, $12::text AS namespace_name, name, table_uuid,
		           format_version, location, current_snapshot_id, current_metadata_location,
		           last_sequence_number, partition_spec, schema_json, sort_order, properties,
		           markings, created_at, updated_at`,
		id, ns.ID, strings.TrimSpace(body.Name), uuid.NewString(), formatVersion, location,
		partitionSpec, body.Schema, sortOrder, props, markings, ns.Name,
	)
	t, err := scanTable(row)
	if err != nil {
		return nil, "", err
	}
	metadataLocation := fmt.Sprintf("%s/metadata/v1.metadata.json", t.Location)
	_, err = r.Pool.Exec(ctx, `INSERT INTO iceberg_table_metadata_files (id, table_id, version, path) VALUES ($1,$2,1,$3) ON CONFLICT (table_id, version) DO NOTHING`, uuid.New(), t.ID, metadataLocation)
	return t, metadataLocation, err
}

func (r *Repo) CommitTable(ctx context.Context, projectRID string, namespace []string, tableName string, body *models.CommitTableRequest) (*models.IcebergTable, string, error) {
	cur, err := r.GetTable(ctx, projectRID, namespace, tableName)
	if err != nil || cur == nil {
		return cur, "", err
	}
	if err := enforceRequirements(cur, body.Requirements); err != nil {
		return nil, "", err
	}
	nextSchema := cur.SchemaJSON
	nextProps := cur.Properties
	nextPartition := cur.PartitionSpec
	nextSort := cur.SortOrder
	var lastSnapshotID *int64
	lastSeq := cur.LastSequenceNumber
	for _, raw := range body.Updates {
		var update map[string]json.RawMessage
		if err := json.Unmarshal(raw, &update); err != nil {
			return nil, "", err
		}
		action := jsonString(update["action"])
		switch action {
		case "add-schema":
			if schema, ok := update["schema"]; ok {
				nextSchema = schema
			}
		case "set-properties":
			nextProps = mergeProperties(nextProps, update["updates"])
		case "remove-properties":
			nextProps = removeProperties(nextProps, update["removals"])
		case "add-partition-spec":
			if spec, ok := update["spec"]; ok {
				nextPartition = spec
			}
		case "add-sort-order":
			if order, ok := update["sort-order"]; ok {
				nextSort = order
			}
		case "add-snapshot":
			snap, err := r.appendSnapshot(ctx, cur.ID, update["snapshot"])
			if err != nil {
				return nil, "", err
			}
			lastSnapshotID = &snap.SnapshotID
			if snap.SequenceNumber > lastSeq {
				lastSeq = snap.SequenceNumber
			}
		}
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE iceberg_tables SET schema_json=$2, properties=$3, partition_spec=$4, sort_order=$5,
		 current_snapshot_id=COALESCE($6, current_snapshot_id), last_sequence_number=GREATEST(last_sequence_number,$7), updated_at=NOW()
		 WHERE id=$1 RETURNING id, rid, namespace_id, $8::text AS namespace_name, name, table_uuid,
		 format_version, location, current_snapshot_id, current_metadata_location, last_sequence_number,
		 partition_spec, schema_json, sort_order, properties, markings, created_at, updated_at`,
		cur.ID, nextSchema, nextProps, nextPartition, nextSort, lastSnapshotID, lastSeq, encodePath(namespace),
	)
	updated, err := scanTable(row)
	if err != nil {
		return nil, "", err
	}
	version, err := r.nextMetadataVersion(ctx, updated.ID)
	if err != nil {
		return nil, "", err
	}
	metadataLocation := fmt.Sprintf("%s/metadata/v%d.metadata.json", updated.Location, version)
	_, err = r.Pool.Exec(ctx, `INSERT INTO iceberg_table_metadata_files (id, table_id, version, path) VALUES ($1,$2,$3,$4)`, uuid.New(), updated.ID, version, metadataLocation)
	if err != nil {
		return nil, "", err
	}
	_, err = r.Pool.Exec(ctx, `UPDATE iceberg_tables SET current_metadata_location=$2 WHERE id=$1`, updated.ID, metadataLocation)
	updated.CurrentMetadataLocation = &metadataLocation
	return updated, metadataLocation, err
}

func (r *Repo) ListSnapshots(ctx context.Context, tableID uuid.UUID) ([]models.Snapshot, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, table_id, snapshot_id, parent_snapshot_id, sequence_number, operation, manifest_list_location, summary, schema_id, timestamp_ms FROM iceberg_snapshots WHERE table_id=$1 ORDER BY timestamp_ms ASC, snapshot_id ASC`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Snapshot, 0)
	for rows.Next() {
		v, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetNamespaceByProjectName(ctx context.Context, projectRID, name string) (*models.IcebergNamespace, error) {
	row := r.Pool.QueryRow(ctx, namespaceSelect+` WHERE project_rid=$1 AND name=$2`, projectRID, name)
	v, err := scanNamespace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) appendSnapshot(ctx context.Context, tableID uuid.UUID, raw json.RawMessage) (*models.Snapshot, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("add-snapshot requires snapshot")
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	snapshotID := jsonInt64(snap["snapshot-id"], timeNowMillis())
	parentID := jsonInt64Ptr(snap["parent-snapshot-id"])
	seq := jsonInt64(snap["sequence-number"], 1)
	manifest := jsonString(snap["manifest-list"])
	summary := snap["summary"]
	if len(summary) == 0 {
		summary = []byte(`{}`)
	}
	operation := "append"
	var sm map[string]any
	if json.Unmarshal(summary, &sm) == nil {
		if op, ok := sm["operation"].(string); ok && op != "" {
			operation = op
		}
	}
	if !oneOf(operation, "append", "overwrite", "delete", "replace") {
		return nil, fmt.Errorf("invalid operation `%s`", operation)
	}
	schemaID := int32(jsonInt64(snap["schema-id"], 0))
	row := r.Pool.QueryRow(ctx, `INSERT INTO iceberg_snapshots (table_id, snapshot_id, parent_snapshot_id, sequence_number, operation, manifest_list_location, summary, schema_id, timestamp_ms) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (table_id, snapshot_id) DO UPDATE SET snapshot_id=EXCLUDED.snapshot_id RETURNING id, table_id, snapshot_id, parent_snapshot_id, sequence_number, operation, manifest_list_location, summary, schema_id, timestamp_ms`, tableID, snapshotID, parentID, seq, operation, manifest, summary, schemaID, timeNowMillis())
	return scanSnapshot(row)
}

func (r *Repo) nextMetadataVersion(ctx context.Context, tableID uuid.UUID) (int32, error) {
	var max *int32
	if err := r.Pool.QueryRow(ctx, `SELECT MAX(version) FROM iceberg_table_metadata_files WHERE table_id=$1`, tableID).Scan(&max); err != nil {
		return 0, err
	}
	if max == nil {
		return 1, nil
	}
	return *max + 1, nil
}

func scanTable(r rowLikeT) (*models.IcebergTable, error) {
	v := &models.IcebergTable{}
	var nsName string
	if err := r.Scan(&v.ID, &v.RID, &v.NamespaceID, &nsName, &v.Name, &v.TableUUID,
		&v.FormatVersion, &v.Location, &v.CurrentSnapshotID, &v.CurrentMetadataLocation,
		&v.LastSequenceNumber, &v.PartitionSpec, &v.SchemaJSON, &v.SortOrder, &v.Properties,
		&v.Markings, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Namespace = decodePath(nsName)
	if v.Markings == nil {
		v.Markings = []string{}
	}
	return v, nil
}

func scanSnapshot(r rowLikeT) (*models.Snapshot, error) {
	v := &models.Snapshot{}
	if err := r.Scan(&v.ID, &v.TableID, &v.SnapshotID, &v.ParentSnapshotID, &v.SequenceNumber, &v.Operation, &v.ManifestListLocation, &v.Summary, &v.SchemaID, &v.TimestampMS); err != nil {
		return nil, err
	}
	return v, nil
}

func encodePath(parts []string) string { return strings.Join(parts, ".") }
func decodePath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}
func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func jsonString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func jsonInt64(raw json.RawMessage, def int64) int64 {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	return def
}

func jsonInt64Ptr(raw json.RawMessage) *int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	n := jsonInt64(raw, 0)
	return &n
}

func timeNowMillis() int64 { return time.Now().UTC().UnixMilli() }

func mergeProperties(current, updates json.RawMessage) json.RawMessage {
	var dst map[string]any
	if json.Unmarshal(current, &dst) != nil || dst == nil {
		dst = map[string]any{}
	}
	var src map[string]any
	if json.Unmarshal(updates, &src) == nil {
		for k, v := range src {
			dst[k] = v
		}
	}
	out, _ := json.Marshal(dst)
	return out
}

func removeProperties(current, removals json.RawMessage) json.RawMessage {
	var dst map[string]any
	if json.Unmarshal(current, &dst) != nil || dst == nil {
		dst = map[string]any{}
	}
	var keys []string
	if json.Unmarshal(removals, &keys) == nil {
		for _, k := range keys {
			delete(dst, k)
		}
	}
	out, _ := json.Marshal(dst)
	return out
}

func enforceRequirements(table *models.IcebergTable, reqs []json.RawMessage) error {
	for _, raw := range reqs {
		var req map[string]json.RawMessage
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
		switch jsonString(req["type"]) {
		case "assert-uuid":
			if jsonString(req["uuid"]) != table.TableUUID {
				return fmt.Errorf("assert-uuid failed")
			}
		case "assert-ref-snapshot-id":
			expected := jsonInt64Ptr(req["snapshot-id"])
			if (expected == nil) != (table.CurrentSnapshotID == nil) || (expected != nil && table.CurrentSnapshotID != nil && *expected != *table.CurrentSnapshotID) {
				return fmt.Errorf("assert-ref-snapshot-id failed")
			}
		}
	}
	return nil
}

func (r *Repo) DropTable(ctx context.Context, projectRID string, namespace []string, tableName string, purge bool) (bool, error) {
	_ = purge
	cmd, err := r.Pool.Exec(ctx,
		`DELETE FROM iceberg_tables t USING iceberg_namespaces n
		 WHERE t.namespace_id=n.id AND n.project_rid=$1 AND n.name=$2 AND t.name=$3`,
		projectRID, encodePath(namespace), tableName)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (r *Repo) RenameTable(ctx context.Context, projectRID string, sourceNS []string, sourceName string, destNS []string, destName string) (*models.IcebergTable, error) {
	dest, err := r.GetNamespaceByProjectName(ctx, projectRID, encodePath(destNS))
	if err != nil || dest == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("destination namespace not found")
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE iceberg_tables t SET namespace_id=$4, name=$5, updated_at=NOW()
		 FROM iceberg_namespaces n
		 WHERE t.namespace_id=n.id AND n.project_rid=$1 AND n.name=$2 AND t.name=$3
		 RETURNING t.id, t.rid, t.namespace_id, $6::text AS namespace_name, t.name, t.table_uuid,
		          t.format_version, t.location, t.current_snapshot_id, t.current_metadata_location,
		          t.last_sequence_number, t.partition_spec, t.schema_json, t.sort_order, t.properties,
		          t.markings, t.created_at, t.updated_at`,
		projectRID, encodePath(sourceNS), sourceName, dest.ID, strings.TrimSpace(destName), encodePath(destNS))
	v, err := scanTable(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

const refSelect = `SELECT id, table_id, name, kind, snapshot_id, max_ref_age_ms,
	max_snapshot_age_ms, min_snapshots_to_keep, created_at FROM iceberg_table_branches`

func (r *Repo) ListRefs(ctx context.Context, tableID uuid.UUID) ([]models.TableRef, error) {
	rows, err := r.Pool.Query(ctx, refSelect+` WHERE table_id=$1 ORDER BY name`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TableRef{}
	for rows.Next() {
		v, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) UpsertRef(ctx context.Context, tableID uuid.UUID, name string, body *models.UpdateRefRequest) (*models.TableRef, error) {
	kind := body.Type
	if kind == "" {
		kind = "branch"
	}
	if !oneOf(kind, "branch", "tag") {
		return nil, fmt.Errorf("invalid ref type `%s`", kind)
	}
	canonical := name
	if canonical == "master" {
		canonical = "main"
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO iceberg_table_branches
		 (id, table_id, name, kind, snapshot_id, max_ref_age_ms, max_snapshot_age_ms, min_snapshots_to_keep)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (table_id, name) DO UPDATE SET kind=EXCLUDED.kind, snapshot_id=EXCLUDED.snapshot_id,
		 max_ref_age_ms=EXCLUDED.max_ref_age_ms, max_snapshot_age_ms=EXCLUDED.max_snapshot_age_ms,
		 min_snapshots_to_keep=EXCLUDED.min_snapshots_to_keep
		 RETURNING id, table_id, name, kind, snapshot_id, max_ref_age_ms,
		           max_snapshot_age_ms, min_snapshots_to_keep, created_at`,
		uuid.New(), tableID, canonical, kind, body.SnapshotID, body.MaxRefAgeMS, body.MaxSnapshotAgeMS, body.MinSnapshotsToKeep)
	return scanRef(row)
}

func (r *Repo) GetRef(ctx context.Context, tableID uuid.UUID, name string) (*models.TableRef, error) {
	if name == "master" {
		name = "main"
	}
	row := r.Pool.QueryRow(ctx, refSelect+` WHERE table_id=$1 AND name=$2`, tableID, name)
	v, err := scanRef(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) DeleteRef(ctx context.Context, tableID uuid.UUID, name string) (bool, error) {
	if name == "master" {
		name = "main"
	}
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM iceberg_table_branches WHERE table_id=$1 AND name=$2`, tableID, name)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (r *Repo) ListMetadataFiles(ctx context.Context, tableID uuid.UUID) ([]models.MetadataFile, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id, table_id, version, path, created_at FROM iceberg_table_metadata_files WHERE table_id=$1 ORDER BY version`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MetadataFile{}
	for rows.Next() {
		v, err := scanMetadataFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetMetadataFile(ctx context.Context, tableID uuid.UUID, version int32) (*models.MetadataFile, error) {
	row := r.Pool.QueryRow(ctx, `SELECT id, table_id, version, path, created_at FROM iceberg_table_metadata_files WHERE table_id=$1 AND version=$2`, tableID, version)
	v, err := scanMetadataFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) GetSnapshot(ctx context.Context, tableID uuid.UUID, snapshotID int64) (*models.Snapshot, error) {
	row := r.Pool.QueryRow(ctx, `SELECT id, table_id, snapshot_id, parent_snapshot_id, sequence_number, operation, manifest_list_location, summary, schema_id, timestamp_ms FROM iceberg_snapshots WHERE table_id=$1 AND snapshot_id=$2`, tableID, snapshotID)
	v, err := scanSnapshot(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func scanRef(r rowLikeT) (*models.TableRef, error) {
	v := &models.TableRef{}
	if err := r.Scan(&v.ID, &v.TableID, &v.Name, &v.Kind, &v.SnapshotID, &v.MaxRefAgeMS, &v.MaxSnapshotAgeMS, &v.MinSnapshotsToKeep, &v.CreatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func scanMetadataFile(r rowLikeT) (*models.MetadataFile, error) {
	v := &models.MetadataFile{}
	if err := r.Scan(&v.ID, &v.TableID, &v.Version, &v.Path, &v.CreatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

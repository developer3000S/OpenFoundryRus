package stores

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// PostgresObjectStore wraps a pgx pool and exposes [ObjectStore] over the
// storage-abstraction shaped PostgreSQL tables owned by ontology-kernel.
type PostgresObjectStore struct{ Pool *pgxpool.Pool }

func NewPostgresObjectStore(pool *pgxpool.Pool) *PostgresObjectStore {
	return &PostgresObjectStore{Pool: pool}
}

var _ storageabstraction.ObjectStore = (*PostgresObjectStore)(nil)

func pgBackend(err error) error {
	if err == nil {
		return nil
	}
	return storageabstraction.Backend(err.Error())
}

func ensurePool(pool *pgxpool.Pool) error {
	if pool == nil {
		return storageabstraction.Backend("postgres pool is nil")
	}
	return nil
}

func pageLimitOffset(page storageabstraction.Page) (uint32, uint64, error) {
	limit := page.Size
	if limit < 1 {
		limit = 1
	}
	if page.Token == nil || strings.TrimSpace(*page.Token) == "" {
		return limit, 0, nil
	}
	off, err := strconv.ParseUint(*page.Token, 10, 64)
	if err != nil {
		return 0, 0, storageabstraction.Invalid("invalid page token")
	}
	return limit, off, nil
}

func nextOffsetToken(offset uint64, returned int, limit uint32) *string {
	if returned < int(limit) {
		return nil
	}
	next := strconv.FormatUint(offset+uint64(returned), 10)
	return &next
}

func (s *PostgresObjectStore) Get(ctx context.Context, tenant storageabstraction.TenantId, id storageabstraction.ObjectId, _ storageabstraction.ReadConsistency) (*storageabstraction.Object, error) {
	if err := ensurePool(s.Pool); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `SELECT tenant, id, type_id, version, payload, organization_id, created_at_ms, updated_at_ms, owner, markings FROM storage_objects WHERE tenant=$1 AND id=$2`, tenant, id)
	obj, err := scanObject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, pgBackend(err)
	}
	return obj, nil
}

func (s *PostgresObjectStore) Put(ctx context.Context, obj storageabstraction.Object, expectedVersion *uint64) (storageabstraction.PutOutcome, error) {
	if err := ensurePool(s.Pool); err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	if expectedVersion == nil {
		var version uint64
		err := s.Pool.QueryRow(ctx, `INSERT INTO storage_objects (tenant,id,type_id,version,payload,organization_id,created_at_ms,updated_at_ms,owner,markings) VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8,$9) RETURNING version`, obj.Tenant, obj.ID, obj.TypeID, obj.Payload, obj.OrganizationID, obj.CreatedAtMs, obj.UpdatedAtMs, obj.Owner, obj.Markings).Scan(&version)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return storageabstraction.VersionConflict(0, 1), nil
			}
			return storageabstraction.PutOutcome{}, pgBackend(err)
		}
		return storageabstraction.PutOutcome{Kind: storageabstraction.PutInserted, NewVersion: version}, nil
	}
	newVersion := *expectedVersion + 1
	cmd, err := s.Pool.Exec(ctx, `UPDATE storage_objects SET type_id=$4, version=$5, payload=$6, organization_id=$7, created_at_ms=$8, updated_at_ms=$9, owner=$10, markings=$11 WHERE tenant=$1 AND id=$2 AND version=$3`, obj.Tenant, obj.ID, *expectedVersion, obj.TypeID, newVersion, obj.Payload, obj.OrganizationID, obj.CreatedAtMs, obj.UpdatedAtMs, obj.Owner, obj.Markings)
	if err != nil {
		return storageabstraction.PutOutcome{}, pgBackend(err)
	}
	if cmd.RowsAffected() == 1 {
		return storageabstraction.Updated(*expectedVersion, newVersion), nil
	}
	var actual uint64
	err = s.Pool.QueryRow(ctx, `SELECT version FROM storage_objects WHERE tenant=$1 AND id=$2`, obj.Tenant, obj.ID).Scan(&actual)
	if errors.Is(err, pgx.ErrNoRows) {
		actual = 0
	} else if err != nil {
		return storageabstraction.PutOutcome{}, pgBackend(err)
	}
	return storageabstraction.VersionConflict(*expectedVersion, actual), nil
}

func (s *PostgresObjectStore) Delete(ctx context.Context, tenant storageabstraction.TenantId, id storageabstraction.ObjectId) (bool, error) {
	if err := ensurePool(s.Pool); err != nil {
		return false, err
	}
	cmd, err := s.Pool.Exec(ctx, `DELETE FROM storage_objects WHERE tenant=$1 AND id=$2`, tenant, id)
	return cmd.RowsAffected() > 0, pgBackend(err)
}

func (s *PostgresObjectStore) list(ctx context.Context, where string, args []any, page storageabstraction.Page) (storageabstraction.PagedResult[storageabstraction.Object], error) {
	if err := ensurePool(s.Pool); err != nil {
		return storageabstraction.PagedResult[storageabstraction.Object]{}, err
	}
	limit, offset, err := pageLimitOffset(page)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.Object]{}, err
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `SELECT tenant, id, type_id, version, payload, organization_id, created_at_ms, updated_at_ms, owner, markings FROM storage_objects WHERE `+where+` ORDER BY updated_at_ms DESC, id ASC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.Object]{}, pgBackend(err)
	}
	defer rows.Close()
	items := []storageabstraction.Object{}
	for rows.Next() {
		obj, err := scanObject(rows)
		if err != nil {
			return storageabstraction.PagedResult[storageabstraction.Object]{}, pgBackend(err)
		}
		items = append(items, *obj)
	}
	if err := rows.Err(); err != nil {
		return storageabstraction.PagedResult[storageabstraction.Object]{}, pgBackend(err)
	}
	return storageabstraction.PagedResult[storageabstraction.Object]{Items: items, NextToken: nextOffsetToken(offset, len(items), limit)}, nil
}

func (s *PostgresObjectStore) ListByType(ctx context.Context, tenant storageabstraction.TenantId, typeID storageabstraction.TypeId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.Object], error) {
	return s.list(ctx, "tenant=$1 AND type_id=$2", []any{tenant, typeID}, page)
}
func (s *PostgresObjectStore) ListByOwner(ctx context.Context, tenant storageabstraction.TenantId, owner storageabstraction.OwnerId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.Object], error) {
	return s.list(ctx, "tenant=$1 AND owner=$2", []any{tenant, owner}, page)
}
func (s *PostgresObjectStore) ListByMarking(ctx context.Context, tenant storageabstraction.TenantId, marking storageabstraction.MarkingId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.Object], error) {
	return s.list(ctx, "tenant=$1 AND $2 = ANY(markings)", []any{tenant, marking}, page)
}

type objectScanner interface{ Scan(dest ...any) error }

func scanObject(row objectScanner) (*storageabstraction.Object, error) {
	var obj storageabstraction.Object
	err := row.Scan(&obj.Tenant, &obj.ID, &obj.TypeID, &obj.Version, &obj.Payload, &obj.OrganizationID, &obj.CreatedAtMs, &obj.UpdatedAtMs, &obj.Owner, &obj.Markings)
	return &obj, err
}

// PostgresLinkStore wraps a pgx pool and exposes [LinkStore].
type PostgresLinkStore struct{ Pool *pgxpool.Pool }

func NewPostgresLinkStore(pool *pgxpool.Pool) *PostgresLinkStore {
	return &PostgresLinkStore{Pool: pool}
}

var _ storageabstraction.LinkStore = (*PostgresLinkStore)(nil)

func (s *PostgresLinkStore) Put(ctx context.Context, link storageabstraction.Link) error {
	if err := ensurePool(s.Pool); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO storage_links (tenant,link_type,from_id,to_id,payload,created_at_ms) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant,link_type,from_id,to_id) DO NOTHING`, link.Tenant, link.LinkType, link.From, link.To, link.Payload, link.CreatedAtMs)
	return pgBackend(err)
}
func (s *PostgresLinkStore) Delete(ctx context.Context, tenant storageabstraction.TenantId, linkType storageabstraction.LinkTypeId, from, to storageabstraction.ObjectId) (bool, error) {
	if err := ensurePool(s.Pool); err != nil {
		return false, err
	}
	cmd, err := s.Pool.Exec(ctx, `DELETE FROM storage_links WHERE tenant=$1 AND link_type=$2 AND from_id=$3 AND to_id=$4`, tenant, linkType, from, to)
	return cmd.RowsAffected() > 0, pgBackend(err)
}
func (s *PostgresLinkStore) list(ctx context.Context, where string, args []any, page storageabstraction.Page) (storageabstraction.PagedResult[storageabstraction.Link], error) {
	if err := ensurePool(s.Pool); err != nil {
		return storageabstraction.PagedResult[storageabstraction.Link]{}, err
	}
	limit, offset, err := pageLimitOffset(page)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.Link]{}, err
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `SELECT tenant, link_type, from_id, to_id, payload, created_at_ms FROM storage_links WHERE `+where+` ORDER BY created_at_ms DESC, from_id ASC, to_id ASC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.Link]{}, pgBackend(err)
	}
	defer rows.Close()
	items := []storageabstraction.Link{}
	for rows.Next() {
		var l storageabstraction.Link
		if err := rows.Scan(&l.Tenant, &l.LinkType, &l.From, &l.To, &l.Payload, &l.CreatedAtMs); err != nil {
			return storageabstraction.PagedResult[storageabstraction.Link]{}, pgBackend(err)
		}
		items = append(items, l)
	}
	return storageabstraction.PagedResult[storageabstraction.Link]{Items: items, NextToken: nextOffsetToken(offset, len(items), limit)}, pgBackend(rows.Err())
}
func (s *PostgresLinkStore) ListOutgoing(ctx context.Context, tenant storageabstraction.TenantId, linkType storageabstraction.LinkTypeId, from storageabstraction.ObjectId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.Link], error) {
	return s.list(ctx, "tenant=$1 AND link_type=$2 AND from_id=$3", []any{tenant, linkType, from}, page)
}
func (s *PostgresLinkStore) ListIncoming(ctx context.Context, tenant storageabstraction.TenantId, linkType storageabstraction.LinkTypeId, to storageabstraction.ObjectId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.Link], error) {
	return s.list(ctx, "tenant=$1 AND link_type=$2 AND to_id=$3", []any{tenant, linkType, to}, page)
}

// PostgresActionLogStore wraps a pgx pool and exposes [ActionLogStore].
type PostgresActionLogStore struct{ Pool *pgxpool.Pool }

func NewPostgresActionLogStore(pool *pgxpool.Pool) *PostgresActionLogStore {
	return &PostgresActionLogStore{Pool: pool}
}

var _ storageabstraction.ActionLogStore = (*PostgresActionLogStore)(nil)

func (s *PostgresActionLogStore) Append(ctx context.Context, e storageabstraction.ActionLogEntry) error {
	if err := ensurePool(s.Pool); err != nil {
		return err
	}
	eventID := e.EventID
	if eventID == nil {
		v := fmt.Sprintf("%s:%s:%d", e.Tenant, e.ActionID, e.RecordedAtMs)
		eventID = &v
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO storage_action_log (tenant,event_id,action_id,kind,subject,object_id,payload,recorded_at_ms) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (tenant,event_id) DO NOTHING`, e.Tenant, eventID, e.ActionID, e.Kind, e.Subject, e.Object, e.Payload, e.RecordedAtMs)
	return pgBackend(err)
}
func (s *PostgresActionLogStore) list(ctx context.Context, where string, args []any, page storageabstraction.Page) (storageabstraction.PagedResult[storageabstraction.ActionLogEntry], error) {
	if err := ensurePool(s.Pool); err != nil {
		return storageabstraction.PagedResult[storageabstraction.ActionLogEntry]{}, err
	}
	limit, offset, err := pageLimitOffset(page)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.ActionLogEntry]{}, err
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `SELECT tenant,event_id,action_id,kind,subject,object_id,payload,recorded_at_ms FROM storage_action_log WHERE `+where+` ORDER BY recorded_at_ms DESC, event_id ASC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return storageabstraction.PagedResult[storageabstraction.ActionLogEntry]{}, pgBackend(err)
	}
	defer rows.Close()
	items := []storageabstraction.ActionLogEntry{}
	for rows.Next() {
		var e storageabstraction.ActionLogEntry
		if err := rows.Scan(&e.Tenant, &e.EventID, &e.ActionID, &e.Kind, &e.Subject, &e.Object, &e.Payload, &e.RecordedAtMs); err != nil {
			return storageabstraction.PagedResult[storageabstraction.ActionLogEntry]{}, pgBackend(err)
		}
		items = append(items, e)
	}
	return storageabstraction.PagedResult[storageabstraction.ActionLogEntry]{Items: items, NextToken: nextOffsetToken(offset, len(items), limit)}, pgBackend(rows.Err())
}
func (s *PostgresActionLogStore) ListRecent(ctx context.Context, tenant storageabstraction.TenantId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.ActionLogEntry], error) {
	return s.list(ctx, "tenant=$1", []any{tenant}, page)
}
func (s *PostgresActionLogStore) ListForObject(ctx context.Context, tenant storageabstraction.TenantId, object storageabstraction.ObjectId, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.ActionLogEntry], error) {
	return s.list(ctx, "tenant=$1 AND object_id=$2", []any{tenant, object}, page)
}
func (s *PostgresActionLogStore) ListForAction(ctx context.Context, tenant storageabstraction.TenantId, actionID string, page storageabstraction.Page, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.ActionLogEntry], error) {
	return s.list(ctx, "tenant=$1 AND action_id=$2", []any{tenant, actionID}, page)
}

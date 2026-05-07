package cassandrakernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ReadModelStore is the Cassandra/Scylla-backed implementation of
// repos.ReadModelStore. It stores generic warm read models such as
// action what-if branches in the same configurable ontology keyspace
// used by ontology-actions-service runtime stores.
type ReadModelStore struct {
	session  *gocql.Session
	keyspace string
}

// NewReadModelStore builds a store bound to the standard ontology keyspace.
func NewReadModelStore(session *gocql.Session) *ReadModelStore {
	return &ReadModelStore{session: session, keyspace: "ontology_objects"}
}

// NewReadModelStoreWithKeyspace allows service-level keyspace overrides.
func NewReadModelStoreWithKeyspace(session *gocql.Session, keyspace string) *ReadModelStore {
	return &ReadModelStore{session: session, keyspace: keyspace}
}

var _ repos.ReadModelStore = (*ReadModelStore)(nil)

func (s *ReadModelStore) cqlSelectByID() string {
	return fmt.Sprintf(`SELECT parent_id, payload, version, updated_at
          FROM %s.read_models
         WHERE kind = ? AND tenant = ? AND id = ?`, s.keyspace)
}

func (s *ReadModelStore) cqlSelectByKind() string {
	return fmt.Sprintf(`SELECT id, parent_id, payload, version, updated_at
          FROM %s.read_models
         WHERE kind = ? AND tenant = ?`, s.keyspace)
}

func (s *ReadModelStore) cqlSelectByParent() string {
	return fmt.Sprintf(`SELECT id, payload, version, updated_at
          FROM %s.read_models_by_parent
         WHERE kind = ? AND tenant = ? AND parent_id = ?`, s.keyspace)
}

func (s *ReadModelStore) cqlInsertMain() string {
	return fmt.Sprintf(`INSERT INTO %s.read_models
            (kind, tenant, id, parent_id, payload, version, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`, s.keyspace)
}

func (s *ReadModelStore) cqlInsertParent() string {
	return fmt.Sprintf(`INSERT INTO %s.read_models_by_parent
            (kind, tenant, parent_id, id, payload, version, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`, s.keyspace)
}

func (s *ReadModelStore) cqlDeleteMain() string {
	return fmt.Sprintf(`DELETE FROM %s.read_models
         WHERE kind = ? AND tenant = ? AND id = ?`, s.keyspace)
}

func (s *ReadModelStore) cqlDeleteParent() string {
	return fmt.Sprintf(`DELETE FROM %s.read_models_by_parent
         WHERE kind = ? AND tenant = ? AND parent_id = ? AND id = ?`, s.keyspace)
}

func (s *ReadModelStore) Get(ctx context.Context, kind repos.ReadModelKind, tenant repos.TenantId, id repos.ReadModelId, consistency repos.ReadConsistency) (*repos.ReadModelRecord, error) {
	var parent *string
	var payload string
	var version int64
	var updated time.Time
	err := s.session.Query(s.cqlSelectByID(), string(kind), tenantStr(tenant), string(id)).
		WithContext(ctx).
		Consistency(cqlConsistency(consistency)).
		Scan(&parent, &payload, &version, &updated)
	if err == gocql.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, driverErr(err)
	}
	return readModelRecord(kind, tenant, id, parent, payload, version, updated), nil
}

func (s *ReadModelStore) List(ctx context.Context, query repos.ReadModelQuery, consistency repos.ReadConsistency) (repos.PagedResult[repos.ReadModelRecord], error) {
	if query.ParentID != nil {
		return s.listByParent(ctx, query, consistency)
	}
	q := s.session.Query(s.cqlSelectByKind(), string(query.Kind), tenantStr(query.Tenant)).
		WithContext(ctx).
		Consistency(cqlConsistency(consistency))
	return s.scanList(q, query, false)
}

func (s *ReadModelStore) listByParent(ctx context.Context, query repos.ReadModelQuery, consistency repos.ReadConsistency) (repos.PagedResult[repos.ReadModelRecord], error) {
	q := s.session.Query(s.cqlSelectByParent(), string(query.Kind), tenantStr(query.Tenant), string(*query.ParentID)).
		WithContext(ctx).
		Consistency(cqlConsistency(consistency))
	return s.scanList(q, query, true)
}

func (s *ReadModelStore) scanList(q *gocql.Query, query repos.ReadModelQuery, parentQuery bool) (repos.PagedResult[repos.ReadModelRecord], error) {
	size := int(query.Page.Size)
	if size <= 0 {
		size = 50
	}
	q.PageSize(size)
	if query.Page.Token != nil && *query.Page.Token != "" {
		paging, err := base64.RawURLEncoding.DecodeString(*query.Page.Token)
		if err != nil {
			return repos.PagedResult[repos.ReadModelRecord]{}, repos.Invalid("read-model page token is invalid")
		}
		q.PageState(paging)
	}
	iter := q.Iter()
	items := make([]repos.ReadModelRecord, 0, size)
	if parentQuery {
		var id string
		var payload string
		var version int64
		var updated time.Time
		for iter.Scan(&id, &payload, &version, &updated) {
			parent := string(*query.ParentID)
			items = append(items, *readModelRecord(query.Kind, query.Tenant, repos.ReadModelId(id), &parent, payload, version, updated))
		}
	} else {
		var id string
		var parent *string
		var payload string
		var version int64
		var updated time.Time
		for iter.Scan(&id, &parent, &payload, &version, &updated) {
			items = append(items, *readModelRecord(query.Kind, query.Tenant, repos.ReadModelId(id), parent, payload, version, updated))
		}
	}
	if err := iter.Close(); err != nil {
		return repos.PagedResult[repos.ReadModelRecord]{}, driverErr(err)
	}
	var next *string
	if state := iter.PageState(); len(state) > 0 {
		encoded := base64.RawURLEncoding.EncodeToString(state)
		next = &encoded
	}
	return repos.PagedResult[repos.ReadModelRecord]{Items: items, NextToken: next}, nil
}

func (s *ReadModelStore) Put(ctx context.Context, record repos.ReadModelRecord) (repos.PutOutcome, error) {
	version := record.Version
	if version == 0 {
		version = uint64(time.Now().UTC().UnixMilli())
	}
	existing, err := s.Get(ctx, record.Kind, record.Tenant, record.ID, repos.Strong())
	if err != nil {
		return repos.PutOutcome{}, err
	}
	if existing != nil && existing.Version >= version {
		return repos.PutOutcome{Kind: repos.PutVersionConflict, ExpectedVersion: version, ActualVersion: existing.Version}, nil
	}
	updated := time.UnixMilli(int64(version)).UTC()
	payload := canonicalReadModelPayload(record.Payload)
	var parent *string
	if record.ParentID != nil {
		v := string(*record.ParentID)
		parent = &v
	}
	if err := s.session.Query(s.cqlInsertMain(), string(record.Kind), tenantStr(record.Tenant), string(record.ID), parent, payload, int64(version), updated).
		WithContext(ctx).
		Consistency(gocql.LocalQuorum).
		Exec(); err != nil {
		return repos.PutOutcome{}, driverErr(err)
	}
	if existing != nil && existing.ParentID != nil && (parent == nil || *parent != string(*existing.ParentID)) {
		if err := s.session.Query(s.cqlDeleteParent(), string(record.Kind), tenantStr(record.Tenant), string(*existing.ParentID), string(record.ID)).
			WithContext(ctx).
			Consistency(gocql.LocalQuorum).
			Exec(); err != nil {
			return repos.PutOutcome{}, driverErr(err)
		}
	}
	if parent != nil {
		if err := s.session.Query(s.cqlInsertParent(), string(record.Kind), tenantStr(record.Tenant), *parent, string(record.ID), payload, int64(version), updated).
			WithContext(ctx).
			Consistency(gocql.LocalQuorum).
			Exec(); err != nil {
			return repos.PutOutcome{}, driverErr(err)
		}
	}
	if existing == nil {
		return repos.Inserted(), nil
	}
	return repos.Updated(existing.Version, version), nil
}

func (s *ReadModelStore) Delete(ctx context.Context, kind repos.ReadModelKind, tenant repos.TenantId, id repos.ReadModelId) (bool, error) {
	existing, err := s.Get(ctx, kind, tenant, id, repos.Strong())
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, nil
	}
	if err := s.session.Query(s.cqlDeleteMain(), string(kind), tenantStr(tenant), string(id)).
		WithContext(ctx).
		Consistency(gocql.LocalQuorum).
		Exec(); err != nil {
		return false, driverErr(err)
	}
	if existing.ParentID != nil {
		if err := s.session.Query(s.cqlDeleteParent(), string(kind), tenantStr(tenant), string(*existing.ParentID), string(id)).
			WithContext(ctx).
			Consistency(gocql.LocalQuorum).
			Exec(); err != nil {
			return false, driverErr(err)
		}
	}
	return true, nil
}

func readModelRecord(kind repos.ReadModelKind, tenant repos.TenantId, id repos.ReadModelId, parent *string, payload string, version int64, updated time.Time) *repos.ReadModelRecord {
	rec := repos.ReadModelRecord{Kind: kind, Tenant: tenant, ID: id, Payload: json.RawMessage(payload)}
	if parent != nil && *parent != "" {
		p := repos.ReadModelId(*parent)
		rec.ParentID = &p
	}
	if version >= 0 {
		rec.Version = uint64(version)
	}
	rec.UpdatedAtMs = updated.UnixMilli()
	return &rec
}

func canonicalReadModelPayload(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}

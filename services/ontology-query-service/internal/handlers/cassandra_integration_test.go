//go:build integration

package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	cassandrakernel "github.com/openfoundry/openfoundry-go/libs/cassandra-kernel"
	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-query-service/internal/handlers"
)

func cassandraAddrsOrSkip(t *testing.T) []string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("CASSANDRA_ADDR"))
	if raw == "" {
		t.Skip("CASSANDRA_ADDR not set; skipping real Cassandra/Scylla integration test")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		t.Skip("CASSANDRA_ADDR resolved to no contact points")
	}
	return out
}

func newCassandraObjectStore(t *testing.T, addrs []string, keyspace string) *cassandrakernel.ObjectStore {
	t.Helper()
	cluster := gocql.NewCluster(addrs...)
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Consistency = gocql.One
	cluster.DisableInitialHostLookup = true
	if username := strings.TrimSpace(os.Getenv("CASSANDRA_USERNAME")); username != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: username,
			Password: os.Getenv("CASSANDRA_PASSWORD"),
		}
	}
	bootstrap, err := cluster.CreateSession()
	if err != nil {
		t.Fatalf("connect Cassandra for keyspace bootstrap: %v", err)
	}
	if err := bootstrap.Query(fmt.Sprintf(`CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class':'SimpleStrategy','replication_factor':1} AND durable_writes = true`, keyspace)).Exec(); err != nil {
		bootstrap.Close()
		t.Fatalf("create keyspace %s: %v", keyspace, err)
	}
	bootstrap.Close()

	cluster.Keyspace = keyspace
	session, err := cluster.CreateSession()
	if err != nil {
		t.Fatalf("connect keyspace %s: %v", keyspace, err)
	}
	applyObjectSchema(t, session, keyspace)
	t.Cleanup(func() { session.Close() })
	return cassandrakernel.NewObjectStoreWithKeyspace(session, keyspace)
}

func applyObjectSchema(t *testing.T, session *gocql.Session, keyspace string) {
	t.Helper()
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.objects_by_id (
			tenant text,
			object_id timeuuid,
			type_id text,
			owner_id uuid,
			properties text,
			marking frozen<set<text>>,
			organization_id uuid,
			revision_number bigint STATIC,
			created_at timestamp,
			updated_at timestamp,
			deleted boolean,
			PRIMARY KEY ((tenant, object_id))
		)`, keyspace),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.objects_by_type (
			tenant text,
			type_id text,
			updated_at timestamp,
			object_id timeuuid,
			owner_id uuid,
			marking frozen<set<text>>,
			properties_summary text,
			deleted boolean,
			PRIMARY KEY ((tenant, type_id), updated_at, object_id)
		) WITH CLUSTERING ORDER BY (updated_at DESC, object_id ASC)`, keyspace),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.objects_by_owner (
			tenant text,
			owner_id uuid,
			type_id text,
			object_id timeuuid,
			updated_at timestamp,
			deleted boolean,
			PRIMARY KEY ((tenant, owner_id), type_id, object_id)
		)`, keyspace),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.objects_by_marking (
			tenant text,
			marking_id text,
			object_id timeuuid,
			type_id text,
			owner_id uuid,
			updated_at timestamp,
			deleted boolean,
			PRIMARY KEY ((tenant, marking_id), object_id)
		)`, keyspace),
	}
	for _, stmt := range stmts {
		if err := session.Query(stmt).Exec(); err != nil {
			t.Fatalf("apply schema: %v\n%s", err, stmt)
		}
	}
}

func TestHandlersReadObjectsFromRealCassandra(t *testing.T) {
	addrs := cassandraAddrsOrSkip(t)
	keyspace := "of_it_query_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	store := newCassandraObjectStore(t, addrs, keyspace)

	tenantUUID := uuid.New()
	tenant := repos.TenantId(tenantUUID.String())
	objectID := repos.ObjectId(gocql.TimeUUID().String())
	owner := repos.OwnerId(uuid.NewString())
	org := tenantUUID.String()
	created := time.Now().Add(-time.Minute).UnixMilli()
	obj := repos.Object{
		Tenant:         tenant,
		ID:             objectID,
		TypeID:         repos.TypeId("aircraft"),
		Payload:        json.RawMessage(`{"callsign":"OF-QUERY-1"}`),
		OrganizationID: &org,
		CreatedAtMs:    &created,
		UpdatedAtMs:    time.Now().UnixMilli(),
		Owner:          &owner,
		Markings:       []repos.MarkingId{"PUBLIC", "CONTROLLED"},
	}
	outcome, err := store.Put(context.Background(), obj, nil)
	require.NoError(t, err)
	require.Equal(t, repos.PutInserted, outcome.Kind)

	h := handlers.New(handlers.AppState{Objects: store})
	allowedClaims := &authmw.Claims{
		Sub:          uuid.New(),
		OrgID:        &tenantUUID,
		Roles:        []string{"user"},
		SessionScope: &authmw.SessionScope{AllowedMarkings: []string{"PUBLIC", "CONTROLLED"}},
	}

	req := authedReq(http.MethodGet, "/objects/"+string(tenant)+"/"+string(objectID), map[string]string{
		"tenant": string(tenant), "object_id": string(objectID),
	}, allowedClaims)
	rec := httptest.NewRecorder()
	h.GetObject(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got repos.Object
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, objectID, got.ID)
	require.Equal(t, tenant, got.Tenant)
	require.JSONEq(t, string(obj.Payload), string(got.Payload))
	require.ElementsMatch(t, []repos.MarkingId{"PUBLIC", "CONTROLLED"}, got.Markings)

	req = authedReq(http.MethodGet, "/objects/"+string(tenant)+"/by-type/aircraft?size=10", map[string]string{
		"tenant": string(tenant), "type_id": "aircraft",
	}, allowedClaims)
	rec = httptest.NewRecorder()
	h.ListObjectsByType(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list handlers.ListResponse[repos.Object]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	require.Equal(t, objectID, list.Items[0].ID)

	otherTenant := uuid.New()
	forbiddenTenantClaims := &authmw.Claims{Sub: uuid.New(), OrgID: &otherTenant, Roles: []string{"user"}}
	req = authedReq(http.MethodGet, "/objects/"+string(tenant)+"/"+string(objectID), map[string]string{
		"tenant": string(tenant), "object_id": string(objectID),
	}, forbiddenTenantClaims)
	rec = httptest.NewRecorder()
	h.GetObject(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "tenant access denied")

	markingDeniedClaims := &authmw.Claims{
		Sub:          uuid.New(),
		OrgID:        &tenantUUID,
		Roles:        []string{"user"},
		SessionScope: &authmw.SessionScope{AllowedMarkings: []string{"PUBLIC"}},
	}
	req = authedReq(http.MethodGet, "/objects/"+string(tenant)+"/"+string(objectID), map[string]string{
		"tenant": string(tenant), "object_id": string(objectID),
	}, markingDeniedClaims)
	rec = httptest.NewRecorder()
	h.GetObject(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "marking access denied")
}

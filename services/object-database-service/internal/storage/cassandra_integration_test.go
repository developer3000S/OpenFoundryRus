//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cassandrakernel "github.com/openfoundry/openfoundry-go/libs/cassandra-kernel"
)

func TestCassandraAdaptersIntegration(t *testing.T) {
	addrs := cassandraAddrsOrSkip(t)
	keyspace := "ods_adapter_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	session := newAdapterIntegrationSession(t, addrs, keyspace)
	objects, links := NewCassandraStores(session, keyspace, keyspace)
	ctx := context.Background()

	tenant := TenantId("tenant-a")
	objectID := ObjectId(uuid.NewString())
	owner := OwnerId(uuid.NewString())
	createdAt := time.Now().Add(-time.Second).UnixMilli()
	updatedAt := time.Now().UnixMilli()

	out, err := objects.Put(ctx, Object{
		Tenant:      tenant,
		ID:          objectID,
		TypeID:      "aircraft",
		Payload:     []byte(`{"tail":"N123OF"}`),
		CreatedAtMs: &createdAt,
		UpdatedAtMs: updatedAt,
		Owner:       &owner,
		Markings:    []MarkingId{"public"},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, PutInserted, out.Kind)

	got, err := objects.Get(ctx, tenant, objectID, ReadStrong)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(1), got.Version)
	assert.JSONEq(t, `{"tail":"N123OF"}`, string(got.Payload))

	linkPayload := json.RawMessage(`{"kind":"primary"}`)
	link := Link{Tenant: tenant, LinkType: "related_to", From: objectID, To: ObjectId(uuid.NewString()), Payload: &linkPayload, CreatedAtMs: updatedAt}
	require.NoError(t, links.Put(ctx, link))
	outgoing, err := links.ListOutgoing(ctx, tenant, link.LinkType, link.From, Page{Size: 10}, ReadStrong)
	require.NoError(t, err)
	require.Len(t, outgoing.Items, 1)
	require.NotNil(t, outgoing.Items[0].Payload)
	assert.JSONEq(t, `{"kind":"primary"}`, string(*outgoing.Items[0].Payload))
}

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

func newAdapterIntegrationSession(t *testing.T, addrs []string, keyspace string) *gocql.Session {
	t.Helper()
	cluster := gocql.NewCluster(addrs...)
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Consistency = gocql.One
	cluster.DisableInitialHostLookup = true
	if username := strings.TrimSpace(os.Getenv("CASSANDRA_USERNAME")); username != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{Username: username, Password: os.Getenv("CASSANDRA_PASSWORD")}
	}
	bootstrap, err := cluster.CreateSession()
	require.NoError(t, err, "connect Cassandra for keyspace bootstrap")
	defer bootstrap.Close()
	require.NoError(t, bootstrap.Query(fmt.Sprintf(`CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class':'SimpleStrategy','replication_factor':1} AND durable_writes = true`, keyspace)).Exec())

	cluster.Keyspace = keyspace
	session, err := cluster.CreateSession()
	require.NoError(t, err, "connect keyspace %s", keyspace)
	require.NoError(t, cassandrakernel.Apply(session, keyspace, cassandrakernel.OntologyObjectStoreMigrations(keyspace)))
	require.NoError(t, cassandrakernel.Apply(session, keyspace, cassandrakernel.OntologyLinkStoreMigrations(keyspace)))
	t.Cleanup(func() { session.Close() })
	return session
}

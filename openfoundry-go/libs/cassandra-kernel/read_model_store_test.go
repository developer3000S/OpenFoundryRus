package cassandrakernel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestReadModelStoreSatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ repos.ReadModelStore = (*ReadModelStore)(nil)
}

func TestReadModelStoreCQLReferencesKeyspace(t *testing.T) {
	t.Parallel()
	s := &ReadModelStore{keyspace: "ontology_runtime"}
	assert.Contains(t, s.cqlSelectByID(), "FROM ontology_runtime.read_models")
	assert.Contains(t, s.cqlSelectByParent(), "FROM ontology_runtime.read_models_by_parent")
	assert.Contains(t, s.cqlInsertMain(), "INSERT INTO ontology_runtime.read_models")
	assert.Contains(t, s.cqlInsertParent(), "INSERT INTO ontology_runtime.read_models_by_parent")
	assert.Contains(t, s.cqlDeleteMain(), "DELETE FROM ontology_runtime.read_models")
}

func TestOntologyRuntimeMigrationsIncludeRuntimeTables(t *testing.T) {
	t.Parallel()
	migrations := OntologyRuntimeMigrations("ontology_runtime")
	names := make([]string, 0, len(migrations))
	for _, m := range migrations {
		names = append(names, m.Name)
		assert.Contains(t, m.DDL, "ontology_runtime.")
	}
	assert.Contains(t, names, "ontology_objects.objects_by_id")
	assert.Contains(t, names, "ontology_indexes.links_outgoing")
	assert.Contains(t, names, "actions_log.actions_log")
	assert.Contains(t, names, "read_models")
	assert.Contains(t, names, "read_models_by_parent")
}

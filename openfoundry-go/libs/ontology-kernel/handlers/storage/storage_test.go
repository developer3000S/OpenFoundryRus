package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestGetStorageInsightsUsesRealInMemoryStores(t *testing.T) {
	ctx := context.Background()
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}
	objectTypeID := uuid.New()
	seedStorageObjectTypeDefinition(t, state, objectTypeID)
	seedStoragePropertyDefinition(t, state, objectTypeID, "external_id")
	seedStorageFunnelSourceDefinition(t, state, objectTypeID)

	now := time.Now().UTC().UnixMilli()
	_, err := state.Stores.Objects.Put(ctx, storageabstraction.Object{Tenant: "default", ID: storageabstraction.ObjectId(uuid.New().String()), TypeID: storageabstraction.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{"external_id":"ticket-1"}`), UpdatedAtMs: now}, nil)
	if err != nil {
		t.Fatalf("seed object: %v", err)
	}
	claims := &authmw.Claims{Sub: uuid.New(), Email: "storage@example.com", Roles: []string{"admin"}}
	req := httptest.NewRequest(http.MethodGet, "/storage/insights", nil).WithContext(authmw.ContextWithClaims(ctx, claims))
	rec := httptest.NewRecorder()

	GetStorageInsights(state).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"object_instances"`, `"record_count":1`, `"funnel_sources"`, `"index_definitions"`, `"search_documents_total"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("storage insights missing %s: %s", want, body)
		}
	}
}

func seedStorageObjectTypeDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	pk := "external_id"
	payload, _ := json.Marshal(models.ObjectType{ID: objectTypeID, Name: "ticket", DisplayName: "Ticket", PrimaryKeyProperty: &pk, OwnerID: uuid.New(), CreatedAt: now, UpdatedAt: now})
	_, err := state.Stores.Definitions.Put(context.Background(), storageabstraction.DefinitionRecord{Kind: storageabstraction.DefinitionKind(domain.ActionRepoObjectKind), ID: storageabstraction.DefinitionId(objectTypeID.String()), Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed object type: %v", err)
	}
}

func seedStoragePropertyDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, name string) {
	t.Helper()
	now := time.Now().UTC()
	propertyID := uuid.New()
	payload, _ := json.Marshal(models.Property{ID: propertyID, ObjectTypeID: objectTypeID, Name: name, DisplayName: name, PropertyType: "string", CreatedAt: now, UpdatedAt: now})
	parent := storageabstraction.DefinitionId(objectTypeID.String())
	_, err := state.Stores.Definitions.Put(context.Background(), storageabstraction.DefinitionRecord{Kind: storageabstraction.DefinitionKind(domain.ActionRepoPropertyKind), ID: storageabstraction.DefinitionId(propertyID.String()), ParentID: &parent, Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed property: %v", err)
	}
}

func seedStorageFunnelSourceDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	sourceID := uuid.New()
	owner := uuid.New()
	payload, _ := json.Marshal(models.OntologyFunnelSource{ID: sourceID, Name: "tickets", ObjectTypeID: objectTypeID, DatasetID: uuid.New(), PreviewLimit: 25, DefaultMarking: "public", Status: "active", OwnerID: owner, CreatedAt: now, UpdatedAt: now})
	ownerStr := owner.String()
	parent := storageabstraction.DefinitionId(objectTypeID.String())
	_, err := state.Stores.Definitions.Put(context.Background(), storageabstraction.DefinitionRecord{Kind: storageabstraction.DefinitionKind("funnel_source"), ID: storageabstraction.DefinitionId(sourceID.String()), OwnerID: &ownerStr, ParentID: &parent, Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed funnel source: %v", err)
	}
}

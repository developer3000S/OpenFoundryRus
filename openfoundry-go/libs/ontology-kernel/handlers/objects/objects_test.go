package objects

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func TestValueAsStoreText_Strings(t *testing.T) {
	t.Parallel()
	got, err := ValueAsStoreText(json.RawMessage(`"foo"`))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "foo" {
		t.Fatalf("expected foo (no quotes), got %q", got)
	}
}

func TestValueAsStoreText_NumberAndObject(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`42`:        "42",
		`true`:      "true",
		`{"a":1}`:   `{"a":1}`,
		`[1,2,3]`:   "[1,2,3]",
	}
	for in, want := range cases {
		got, err := ValueAsStoreText(json.RawMessage(in))
		if err != nil {
			t.Errorf("ValueAsStoreText(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ValueAsStoreText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValueAsStoreText_NullRejected(t *testing.T) {
	t.Parallel()
	if _, err := ValueAsStoreText(json.RawMessage(`null`)); err == nil ||
		!strings.Contains(err.Error(), "primary key value cannot be null") {
		t.Fatalf("expected null rejection, got %v", err)
	}
}

func TestInstanceToRepoObject_RoundTrips(t *testing.T) {
	t.Parallel()
	obj := &domain.ObjectInstance{
		ID:           uuid.New(),
		ObjectTypeID: uuid.New(),
		Properties:   json.RawMessage(`{"a":1}`),
		CreatedBy:    uuid.New(),
		Marking:      "public",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	tenant := storage.TenantId("t")
	repo := InstanceToRepoObject(tenant, obj, 7, obj.Properties, obj.Marking)
	if repo.Tenant != tenant {
		t.Fatalf("tenant: %s", repo.Tenant)
	}
	if string(repo.ID) != obj.ID.String() {
		t.Fatalf("id: %s", repo.ID)
	}
	if repo.Version != 7 {
		t.Fatalf("version: %d", repo.Version)
	}
	if string(repo.Markings[0]) != "public" {
		t.Fatalf("marking: %s", repo.Markings[0])
	}
}

// FindObjectIDByProperty walks the in-memory ObjectStore and finds
// an existing row by primary-key property — used by the funnel
// upsert path before deciding insert vs update.
func TestFindObjectIDByProperty_FindsExistingRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	objStore := stores.NewInMemoryObjectStore()
	state := &ontologykernel.AppState{
		Stores: stores.Stores{
			Objects: objStore,
			Links:   stores.NewInMemoryLinkStore(),
			Actions: stores.NewInMemoryActionLogStore(),
		},
	}

	typeID := uuid.New()
	target := uuid.New()
	tenant := storage.TenantId("default")

	for i, externalID := range []string{"acct-001", "acct-002", "acct-003"} {
		props, _ := json.Marshal(map[string]any{"external_id": externalID})
		id := uuid.New()
		if i == 1 {
			id = target
		}
		_, _ = objStore.Put(ctx, storage.Object{
			Tenant:      tenant,
			ID:          storage.ObjectId(id.String()),
			TypeID:      storage.TypeId(typeID.String()),
			Version:     0,
			Payload:     props,
			UpdatedAtMs: time.Now().UnixMilli(),
		}, nil)
	}

	got, err := FindObjectIDByProperty(ctx, state, &claimsWithDefaultTenant{}, typeID, "external_id", "acct-002", storage.Strong())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil || *got != target {
		t.Fatalf("expected %s, got %v", target, got)
	}
}

func TestFindObjectIDByProperty_NotFound(t *testing.T) {
	t.Parallel()
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}
	got, err := FindObjectIDByProperty(context.Background(), state, &claimsWithDefaultTenant{},
		uuid.New(), "external_id", "absent", storage.Strong())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

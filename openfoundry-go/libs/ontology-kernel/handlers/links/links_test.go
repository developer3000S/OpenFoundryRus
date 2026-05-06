package links

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// CollectLinkInstancesForType walks every source object's outgoing
// links of the given type. With InMemory stores we can drive a
// 2-objects × 2-links fixture and assert the resulting list shape +
// stable ordering (created_at DESC, then id ASC). The Rust source
// reverses the timestamp comparison via
// `right.created_at.cmp(&left.created_at)`.
func TestCollectLinkInstancesForTypeWalksFullGraph(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	objectStore := stores.NewInMemoryObjectStore()
	linkStore := stores.NewInMemoryLinkStore()
	state := &ontologykernel.AppState{
		Stores: stores.Stores{
			Objects: objectStore,
			Links:   linkStore,
			Actions: stores.NewInMemoryActionLogStore(),
		},
	}

	tenant := storage.TenantId("tenant-1")
	sourceTypeID := uuid.New()
	linkTypeID := uuid.New()
	targetTypeID := uuid.New()

	// Two source objects.
	src1 := storage.ObjectId(uuid.New().String())
	src2 := storage.ObjectId(uuid.New().String())
	insertObject(t, ctx, objectStore, tenant, src1, sourceTypeID)
	insertObject(t, ctx, objectStore, tenant, src2, sourceTypeID)

	// Two outgoing links per source.
	tgt1 := storage.ObjectId(uuid.New().String())
	tgt2 := storage.ObjectId(uuid.New().String())
	now := time.Now().UTC().UnixMilli()
	for i, pair := range [][2]storage.ObjectId{
		{src1, tgt1}, {src1, tgt2}, {src2, tgt1}, {src2, tgt2},
	} {
		_ = linkStore.Put(ctx, storage.Link{
			Tenant:      tenant,
			LinkType:    storage.LinkTypeId(linkTypeID.String()),
			From:        pair[0],
			To:          pair[1],
			Payload:     json.RawMessage(`{}`),
			CreatedAtMs: now + int64(i*1000),
		})
	}

	got, err := CollectLinkInstancesForType(ctx, state, tenant, &models.LinkType{
		ID:           linkTypeID,
		SourceTypeID: sourceTypeID,
		TargetTypeID: targetTypeID,
	})
	if err != nil {
		t.Fatalf("CollectLinkInstancesForType: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 link instances, got %d", len(got))
	}
	// Rust DESC ordering: each successor must NOT be after its
	// predecessor (predecessor.created_at >= successor.created_at).
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.After(got[i-1].CreatedAt) {
			t.Errorf("DESC ordering violated at index %d: %v after %v", i, got[i].CreatedAt, got[i-1].CreatedAt)
		}
	}
	if got[0].LinkTypeID != linkTypeID {
		t.Errorf("link_type_id drift: got %s, want %s", got[0].LinkTypeID, linkTypeID)
	}
}

func TestCollectLinkInstancesForTypeReturnsEmptyOnNoSources(t *testing.T) {
	t.Parallel()
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}
	got, err := CollectLinkInstancesForType(
		context.Background(),
		state,
		storage.TenantId("empty"),
		&models.LinkType{ID: uuid.New(), SourceTypeID: uuid.New()},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}

func insertObject(
	t *testing.T,
	ctx context.Context,
	store storage.ObjectStore,
	tenant storage.TenantId,
	id storage.ObjectId,
	typeID uuid.UUID,
) {
	t.Helper()
	if _, err := store.Put(ctx, storage.Object{
		Tenant:      tenant,
		ID:          id,
		TypeID:      storage.TypeId(typeID.String()),
		Version:     0,
		Payload:     json.RawMessage(`{}`),
		UpdatedAtMs: time.Now().UnixMilli(),
	}, nil); err != nil {
		t.Fatalf("insert object: %v", err)
	}
}

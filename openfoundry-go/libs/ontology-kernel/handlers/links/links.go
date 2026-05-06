// Package links ports the slice of `libs/ontology-kernel/src/handlers/links.rs`
// the storage handler reaches into: `collect_link_instances_for_type`
// plus the `link_instance_from_store_link` decoder.
//
// Full link CRUD (create_link / delete_link / list_outgoing /
// list_incoming) lands together with the `composition.rs` port — the
// storage handler only consumes the read-side enumerator below.
package links

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

const linkPageSize = uint32(256)

// CollectLinkInstancesForType mirrors
// `pub(crate) async fn collect_link_instances_for_type`. Walks every
// source object of the link type's `source_type_id`, then every
// outgoing link of the type from each, sorts the result the same
// way the Rust impl does (created_at ASC, then ID ASC for stability).
func CollectLinkInstancesForType(
	ctx context.Context,
	state *ontologykernel.AppState,
	tenant storage.TenantId,
	linkType *models.LinkType,
) ([]domain.LinkInstance, error) {
	if linkType == nil {
		return nil, fmt.Errorf("link type is required")
	}
	sourceType := storage.TypeId(linkType.SourceTypeID.String())
	linkTypeID := storage.LinkTypeId(linkType.ID.String())

	var instances []domain.LinkInstance
	var objectToken *string
	for {
		objects, err := state.Stores.Objects.ListByType(
			ctx, tenant, sourceType,
			storage.Page{Size: linkPageSize, Token: objectToken},
			storage.Eventual(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to enumerate source objects for links: %w", err)
		}

		for _, object := range objects.Items {
			var linkToken *string
			for {
				links, err := state.Stores.Links.ListOutgoing(
					ctx, tenant, linkTypeID, object.ID,
					storage.Page{Size: linkPageSize, Token: linkToken},
					storage.Eventual(),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to enumerate link instances: %w", err)
				}
				for _, link := range links.Items {
					instance, err := linkInstanceFromStoreLink(link)
					if err != nil {
						return nil, err
					}
					instances = append(instances, instance)
				}
				if links.NextToken == nil {
					break
				}
				linkToken = links.NextToken
			}
		}

		if objects.NextToken == nil {
			break
		}
		objectToken = objects.NextToken
	}

	// Stable order — created_at DESC, then ID ASC. Mirrors the Rust
	// `instances.sort_by` tail of `collect_link_instances_for_type`
	// where `right.created_at.cmp(&left.created_at)` reverses the
	// timestamp ordering. The previous Go port (and its stale
	// doc-comment) used ASC, which made the listing endpoint surface
	// oldest-first instead of newest-first.
	sortLinkInstances(instances)
	return instances, nil
}

// linkInstanceFromStoreLink mirrors `fn link_instance_from_store_link`.
// Surfaces parse errors as Go errors with the same message shapes.
func linkInstanceFromStoreLink(link storage.Link) (domain.LinkInstance, error) {
	linkTypeID, err := uuid.Parse(string(link.LinkType))
	if err != nil {
		return domain.LinkInstance{}, fmt.Errorf("invalid link type id '%s' in LinkStore: %w", link.LinkType, err)
	}
	sourceObjectID, err := uuid.Parse(string(link.From))
	if err != nil {
		return domain.LinkInstance{}, fmt.Errorf("invalid source object id '%s' in LinkStore: %w", link.From, err)
	}
	targetObjectID, err := uuid.Parse(string(link.To))
	if err != nil {
		return domain.LinkInstance{}, fmt.Errorf("invalid target object id '%s' in LinkStore: %w", link.To, err)
	}
	createdAt := time.UnixMilli(link.CreatedAtMs).UTC()
	return domain.LinkInstance{
		ID:             domain.StableLinkID(link.LinkType, link.From, link.To),
		LinkTypeID:     linkTypeID,
		SourceObjectID: sourceObjectID,
		TargetObjectID: targetObjectID,
		Properties:     link.Payload,
		CreatedBy:      uuid.Nil,
		CreatedAt:      createdAt,
	}, nil
}

func sortLinkInstances(out []domain.LinkInstance) {
	// Insertion sort — the input is typically small (one tenant, one
	// link type) and the Rust sort_by is also a stable sort. Avoiding
	// the standard library's slices.SortStableFunc keeps the dep
	// surface minimal.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if linkInstanceLess(out[j], out[j-1]) {
				out[j], out[j-1] = out[j-1], out[j]
			} else {
				break
			}
		}
	}
}

// linkInstanceLess: Rust sorts by created_at DESC then id ASC, so
// `a` precedes `b` when its timestamp is LATER. The id tie-breaker
// stays ASC.
func linkInstanceLess(a, b domain.LinkInstance) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID.String() < b.ID.String()
}

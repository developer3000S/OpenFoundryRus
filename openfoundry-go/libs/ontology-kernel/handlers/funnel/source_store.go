package funnel

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

const funnelSourceDefinitionKind = storage.DefinitionKind("funnel_source")

func loadFunnelSource(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (*models.OntologyFunnelSource, error) {
	if state.DB != nil {
		return domain.LoadSource(ctx, state.DB, id)
	}
	rec, err := state.Stores.Definitions.Get(ctx, funnelSourceDefinitionKind, storage.DefinitionId(id.String()), storage.Strong())
	if err != nil || rec == nil {
		return nil, err
	}
	return funnelSourceFromRecord(*rec)
}

func listFunnelSources(ctx context.Context, state *ontologykernel.AppState, params domain.ListSourcesParams) ([]models.OntologyFunnelSource, error) {
	if state.DB != nil {
		return domain.ListSources(ctx, state.DB, params)
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: funnelSourceDefinitionKind, Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, err
	}
	out := make([]models.OntologyFunnelSource, 0, len(page.Items))
	for _, rec := range page.Items {
		src, err := funnelSourceFromRecord(rec)
		if err != nil {
			return nil, err
		}
		if params.ObjectTypeID != nil && src.ObjectTypeID != *params.ObjectTypeID {
			continue
		}
		if params.StatusFilter != "" && src.Status != params.StatusFilter {
			continue
		}
		if !params.IsAdmin && src.OwnerID != params.ActorID {
			continue
		}
		out = append(out, *src)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return paginateSources(out, params.Offset, params.Limit), nil
}

func countFunnelSources(ctx context.Context, state *ontologykernel.AppState, objectTypeID *uuid.UUID, statusFilter string, isAdmin bool, actorID uuid.UUID) (int64, error) {
	if state.DB != nil {
		return domain.CountSources(ctx, state.DB, objectTypeID, statusFilter, isAdmin, actorID)
	}
	items, err := listFunnelSources(ctx, state, domain.ListSourcesParams{ObjectTypeID: objectTypeID, StatusFilter: statusFilter, IsAdmin: isAdmin, ActorID: actorID, Limit: 10_000})
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func listFunnelSourcesForHealth(ctx context.Context, state *ontologykernel.AppState, params domain.HealthSourcesParams) ([]models.OntologyFunnelSource, error) {
	return listFunnelSources(ctx, state, domain.ListSourcesParams{ObjectTypeID: params.ObjectTypeID, IsAdmin: params.IsAdmin, ActorID: params.ActorID, Limit: 10_000})
}

func createFunnelSource(ctx context.Context, state *ontologykernel.AppState, input domain.CreateSourceInput) (*models.OntologyFunnelSource, error) {
	if state.DB != nil {
		return domain.CreateSource(ctx, state.DB, input)
	}
	now := time.Now().UTC()
	src := models.OntologyFunnelSource{ID: input.ID, Name: input.Name, Description: input.Description, ObjectTypeID: input.ObjectTypeID, DatasetID: input.DatasetID, PipelineID: input.PipelineID, DatasetBranch: input.DatasetBranch, DatasetVersion: input.DatasetVersion, PreviewLimit: input.PreviewLimit, DefaultMarking: input.DefaultMarking, Status: input.Status, TriggerContext: input.TriggerContext, OwnerID: input.OwnerID, CreatedAt: now, UpdatedAt: now}
	_ = json.Unmarshal(input.PropertyMappings, &src.PropertyMappings)
	if len(src.TriggerContext) == 0 {
		src.TriggerContext = json.RawMessage(`{}`)
	}
	if err := putFunnelSource(ctx, state, src); err != nil {
		return nil, err
	}
	return &src, nil
}

func updateFunnelSource(ctx context.Context, state *ontologykernel.AppState, input domain.UpdateSourceInput) (*models.OntologyFunnelSource, error) {
	if state.DB != nil {
		return domain.UpdateSource(ctx, state.DB, input)
	}
	src, err := loadFunnelSource(ctx, state, input.ID)
	if err != nil || src == nil {
		return src, err
	}
	if input.Name != nil {
		src.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		src.Description = *input.Description
	}
	src.PipelineID = input.PipelineID
	src.DatasetBranch = input.DatasetBranch
	src.DatasetVersion = input.DatasetVersion
	src.PreviewLimit = input.PreviewLimit
	src.DefaultMarking = input.DefaultMarking
	src.Status = input.Status
	src.TriggerContext = input.TriggerContext
	src.UpdatedAt = time.Now().UTC()
	src.PropertyMappings = []models.OntologyFunnelPropertyMapping{}
	_ = json.Unmarshal(input.PropertyMappings, &src.PropertyMappings)
	if err := putFunnelSource(ctx, state, *src); err != nil {
		return nil, err
	}
	return src, nil
}

func deleteFunnelSource(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (bool, error) {
	if state.DB != nil {
		return domain.DeleteSource(ctx, state.DB, id)
	}
	return state.Stores.Definitions.Delete(ctx, funnelSourceDefinitionKind, storage.DefinitionId(id.String()))
}

func markFunnelSourceRan(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID, ranAt time.Time) error {
	if state.DB != nil {
		return domain.MarkSourceRan(ctx, state.DB, id, ranAt)
	}
	src, err := loadFunnelSource(ctx, state, id)
	if err != nil || src == nil {
		return err
	}
	src.LastRunAt = &ranAt
	src.UpdatedAt = time.Now().UTC()
	return putFunnelSource(ctx, state, *src)
}

func putFunnelSource(ctx context.Context, state *ontologykernel.AppState, src models.OntologyFunnelSource) error {
	payload, err := json.Marshal(src)
	if err != nil {
		return err
	}
	owner := src.OwnerID.String()
	parent := storage.DefinitionId(src.ObjectTypeID.String())
	created := src.CreatedAt.UnixMilli()
	updated := src.UpdatedAt.UnixMilli()
	_, err = state.Stores.Definitions.Put(ctx, storage.DefinitionRecord{Kind: funnelSourceDefinitionKind, ID: storage.DefinitionId(src.ID.String()), OwnerID: &owner, ParentID: &parent, Payload: payload, CreatedAtMs: &created, UpdatedAtMs: &updated}, nil)
	return err
}

func funnelSourceFromRecord(rec storage.DefinitionRecord) (*models.OntologyFunnelSource, error) {
	var src models.OntologyFunnelSource
	if err := json.Unmarshal(rec.Payload, &src); err != nil {
		return nil, err
	}
	return &src, nil
}

func paginateSources(items []models.OntologyFunnelSource, offset, limit int64) []models.OntologyFunnelSource {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = int64(len(items))
	}
	if int(offset) > len(items) {
		return []models.OntologyFunnelSource{}
	}
	end := int(offset + limit)
	if end > len(items) {
		end = len(items)
	}
	return items[int(offset):end]
}

func funnelObjectTypeExists(ctx context.Context, state *ontologykernel.AppState, objectTypeID uuid.UUID) (bool, error) {
	if state.DB != nil {
		return domain.ObjectTypeExists(ctx, state.DB, objectTypeID)
	}
	return domain.ObjectTypeExistsInDefinitionStore(ctx, state.Stores.Definitions, objectTypeID)
}

func funnelDatasetExists(ctx context.Context, state *ontologykernel.AppState, datasetID uuid.UUID) (bool, error) {
	if state.DB != nil {
		return domain.DatasetExists(ctx, state.DB, datasetID)
	}
	return true, nil
}

func funnelPipelineExists(ctx context.Context, state *ontologykernel.AppState, pipelineID uuid.UUID) (bool, error) {
	if state.DB != nil {
		return domain.PipelineExists(ctx, state.DB, pipelineID)
	}
	return true, nil
}

func loadFunnelObjectType(ctx context.Context, state *ontologykernel.AppState, objectTypeID uuid.UUID) (*models.ObjectType, error) {
	if state.DB != nil {
		return domain.LoadObjectType(ctx, state.DB, objectTypeID)
	}
	rec, err := state.Stores.Definitions.Get(ctx, storage.DefinitionKind(domain.ActionRepoObjectKind), storage.DefinitionId(objectTypeID.String()), storage.Strong())
	if err != nil || rec == nil {
		return nil, err
	}
	var objectType models.ObjectType
	if err := json.Unmarshal(rec.Payload, &objectType); err != nil {
		return nil, err
	}
	return &objectType, nil
}

func loadFunnelEffectiveProperties(ctx context.Context, state *ontologykernel.AppState, objectTypeID uuid.UUID) ([]domain.EffectivePropertyDefinition, error) {
	if state.DB != nil {
		return domain.LoadEffectiveProperties(ctx, state.DB, objectTypeID)
	}
	return domain.LoadEffectivePropertiesViaStore(ctx, state.Stores.Definitions, objectTypeID)
}

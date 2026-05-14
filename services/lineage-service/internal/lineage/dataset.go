package lineage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/lineage-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/lineage-service/internal/queryrouter"
)

// EnsureDatasetSnapshot ports `ensure_dataset_snapshot`. Fetches the
// canonical metadata from dataset-service, computes the effective
// marking (preferring the new value when stricter than the existing
// overlay), and upserts the row.
func EnsureDatasetSnapshot(ctx context.Context, state *AppState, datasetID uuid.UUID) (*models.LineageNode, error) {
	url := fmt.Sprintf("%s/internal/datasets/%s/metadata",
		strings.TrimRight(state.DatasetServiceURL, "/"), datasetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := state.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dataset metadata fetch failed: %s", resp.Status)
	}
	var dataset models.DatasetMetadata
	if err := json.NewDecoder(resp.Body).Decode(&dataset); err != nil {
		return nil, fmt.Errorf("decode dataset metadata: %w", err)
	}

	existing, err := GetNodeRecord(ctx, state.DB, dataset.ID, models.KindDataset)
	if err != nil {
		return nil, err
	}
	baseMarking := MarkingFromDatasetTags(dataset.Tags)
	if normalized := NormalizeMarking(&dataset.Marking); normalized != nil {
		baseMarking = *normalized
	}

	values := []*string{
		strPtr(baseMarking),
	}
	if existing != nil {
		values = append(values, strPtr(existing.Marking))
	}
	effective := MaxMarkings(values)

	overlay, err := json.Marshal(map[string]any{
		"rid":                   dataset.RID,
		"display_name":          firstString(dataset.DisplayName, dataset.Name),
		"format":                dataset.Format,
		"tags":                  dataset.Tags,
		"current_version":       dataset.CurrentVersion,
		"active_branch":         dataset.ActiveBranch,
		"owner_id":              dataset.OwnerID,
		"parent_folder_rid":     dataset.ParentFolderRID,
		"folder_path":           dataset.FolderPath,
		"project_id":            dataset.ProjectID,
		"project_rid":           dataset.ProjectRID,
		"path":                  dataset.Path,
		"resource_visibility":   dataset.ResourceVisibility,
		"links":                 dataset.Links,
		"dataset_marking":       dataset.Marking,
		"base_marking":          baseMarking,
		"metadata_refreshed_at": dataset.UpdatedAt,
	})
	if err != nil {
		return nil, err
	}

	var existingMeta json.RawMessage
	if existing != nil {
		existingMeta = existing.Metadata
	}
	merged := MergeMetadata(existingMeta, overlay, nil)
	record, err := UpsertNode(ctx, state.DB, dataset.ID, models.KindDataset, dataset.Name, effective, merged)
	if err != nil {
		return nil, err
	}
	view := NodeFromRecord(record)
	return &view, nil
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// EnsurePipelineSnapshot ports `ensure_pipeline_snapshot`. Reads the
// pipeline row and overlays the canonical metadata (status,
// description, owner, schedule, retry_policy, next_run_at).
func EnsurePipelineSnapshot(ctx context.Context, state *AppState, pipelineID uuid.UUID) (*models.LineageNode, error) {
	existing, err := GetNodeRecord(ctx, state.DB, pipelineID, models.KindPipeline)
	if err != nil {
		return nil, err
	}
	pipeline, err := LoadPipelineByID(ctx, state.DB, pipelineID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		if existing != nil {
			view := NodeFromRecord(*existing)
			return &view, nil
		}
		return nil, nil
	}

	marking := "public"
	if existing != nil {
		marking = existing.Marking
	}
	overlay, err := json.Marshal(map[string]any{
		"status":          pipeline.Status,
		"description":     pipeline.Description,
		"owner_id":        pipeline.OwnerID,
		"next_run_at":     pipeline.NextRunAt,
		"schedule_config": jsonOrNull(pipeline.ScheduleConfig),
		"retry_policy":    jsonOrNull(pipeline.RetryPolicy),
	})
	if err != nil {
		return nil, err
	}
	var baseMeta json.RawMessage
	if existing != nil {
		baseMeta = existing.Metadata
	}
	merged := MergeMetadata(baseMeta, overlay, nil)
	record, err := UpsertNode(ctx, state.DB, pipeline.ID, models.KindPipeline, pipeline.Name, marking, merged)
	if err != nil {
		return nil, err
	}
	view := NodeFromRecord(record)
	return &view, nil
}

// PropagatePipelineRuntimeLineage ports `propagate_pipeline_runtime_lineage`.
//
// Records (source dataset → pipeline) consume edges, refreshes the
// pipeline row's marking + metadata, and writes the (pipeline →
// output dataset) produces edge.
func PropagatePipelineRuntimeLineage(ctx context.Context, state *AppState, pipeline *models.Pipeline, nodeID, nodeLabel, transformType string, inputDatasetIDs []uuid.UUID, outputDatasetID uuid.UUID, explicitMarking *string) error {
	sourceNodes := make([]models.LineageNode, 0, len(inputDatasetIDs))
	for _, id := range inputDatasetIDs {
		node, err := EnsureDatasetSnapshot(ctx, state, id)
		if err != nil {
			return err
		}
		if node != nil {
			sourceNodes = append(sourceNodes, *node)
		}
	}

	target, err := EnsureDatasetSnapshot(ctx, state, outputDatasetID)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}

	values := []*string{}
	for _, n := range sourceNodes {
		m := n.Marking
		values = append(values, &m)
	}
	tm := target.Marking
	values = append(values, &tm)
	if explicitMarking != nil {
		values = append(values, explicitMarking)
	}
	pipelineMarking := MaxMarkings(values)

	pipelineMeta, err := json.Marshal(map[string]any{
		"status":                      pipeline.Status,
		"description":                 pipeline.Description,
		"owner_id":                    pipeline.OwnerID,
		"next_run_at":                 pipeline.NextRunAt,
		"last_lineage_node_id":        nodeID,
		"last_lineage_transform_type": transformType,
	})
	if err != nil {
		return err
	}
	if _, err := UpsertNode(ctx, state.DB, pipeline.ID, models.KindPipeline, pipeline.Name, pipelineMarking, pipelineMeta); err != nil {
		return err
	}

	overlay, err := json.Marshal(map[string]any{
		"propagated_from_pipeline_id": pipeline.ID,
		"propagated_at":               nowUTC(),
	})
	if err != nil {
		return err
	}
	mergedTargetMeta := MergeMetadata(target.Metadata, overlay, nil)
	targetMarking := MaxMarkingStrings(target.Marking, pipelineMarking)
	if _, err := UpsertNode(ctx, state.DB, outputDatasetID, models.KindDataset, target.Label, targetMarking, mergedTargetMeta); err != nil {
		return err
	}

	for _, source := range sourceNodes {
		consumeMeta, err := json.Marshal(map[string]any{
			"node_label":     nodeLabel,
			"transform_type": transformType,
		})
		if err != nil {
			return err
		}
		producerKey := fmt.Sprintf("pipeline:%s:node:%s:input:%s", pipeline.ID, nodeID, source.ID)
		nID := nodeID
		if err := PersistRelation(ctx, state, RelationWriteInput{
			SourceID:        source.ID,
			SourceKind:      models.KindDataset,
			TargetID:        pipeline.ID,
			TargetKind:      models.KindPipeline,
			RelationKind:    models.RelationKindConsumes,
			ProducerKey:     producerKey,
			PipelineID:      &pipeline.ID,
			NodeID:          &nID,
			ExplicitMarking: explicitMarking,
			Metadata:        consumeMeta,
		}); err != nil {
			return err
		}
	}

	produceMeta, err := json.Marshal(map[string]any{
		"node_label":     nodeLabel,
		"transform_type": transformType,
	})
	if err != nil {
		return err
	}
	producerKey := fmt.Sprintf("pipeline:%s:node:%s:output:%s", pipeline.ID, nodeID, outputDatasetID)
	nID := nodeID
	return PersistRelation(ctx, state, RelationWriteInput{
		SourceID:        pipeline.ID,
		SourceKind:      models.KindPipeline,
		TargetID:        outputDatasetID,
		TargetKind:      models.KindDataset,
		RelationKind:    models.RelationKindProduces,
		ProducerKey:     producerKey,
		PipelineID:      &pipeline.ID,
		NodeID:          &nID,
		ExplicitMarking: explicitMarking,
		Metadata:        produceMeta,
	})
}

// RecordLineage ports `record_lineage` — direct dataset → dataset edge.
func RecordLineage(ctx context.Context, state *AppState, sourceDatasetID, targetDatasetID uuid.UUID, pipelineID *uuid.UUID, nodeID *string) error {
	pipelineLabel := "direct"
	if pipelineID != nil {
		pipelineLabel = pipelineID.String()
	}
	nodeLabel := "root"
	if nodeID != nil {
		nodeLabel = *nodeID
	}
	meta, err := json.Marshal(map[string]any{"source": "runtime_lineage"})
	if err != nil {
		return err
	}
	return PersistRelation(ctx, state, RelationWriteInput{
		SourceID:     sourceDatasetID,
		SourceKind:   models.KindDataset,
		TargetID:     targetDatasetID,
		TargetKind:   models.KindDataset,
		RelationKind: models.RelationKindDerives,
		ProducerKey:  fmt.Sprintf("dataset:%s:%s:%s:%s", sourceDatasetID, targetDatasetID, pipelineLabel, nodeLabel),
		PipelineID:   pipelineID,
		NodeID:       nodeID,
		Metadata:     meta,
	})
}

// RecordColumnLineage ports `record_column_lineage`.
func RecordColumnLineage(ctx context.Context, state *AppState, sourceDatasetID uuid.UUID, sourceColumn string, targetDatasetID uuid.UUID, targetColumn string, pipelineID *uuid.UUID, nodeID *string) error {
	pipelineLabel := "direct"
	if pipelineID != nil {
		pipelineLabel = pipelineID.String()
	}
	nodeLabel := "root"
	if nodeID != nil {
		nodeLabel = *nodeID
	}
	meta, err := json.Marshal(map[string]any{
		models.MetadataSourceColumnKey: sourceColumn,
		models.MetadataTargetColumnKey: targetColumn,
		"source":                       "runtime_lineage",
	})
	if err != nil {
		return err
	}
	return PersistRelation(ctx, state, RelationWriteInput{
		SourceID:     sourceDatasetID,
		SourceKind:   models.KindDataset,
		TargetID:     targetDatasetID,
		TargetKind:   models.KindDataset,
		RelationKind: models.RelationKindColumnDerives,
		ProducerKey:  fmt.Sprintf("column:%s:%s:%s:%s:%s:%s", sourceDatasetID, sourceColumn, targetDatasetID, targetColumn, pipelineLabel, nodeLabel),
		PipelineID:   pipelineID,
		NodeID:       nodeID,
		Metadata:     meta,
	})
}

// GetDatasetColumnLineage ports `get_dataset_column_lineage`.
func GetDatasetColumnLineage(ctx context.Context, state *AppState, datasetID uuid.UUID, plan queryrouter.QueryPlan) ([]models.ColumnLineageEdge, error) {
	if plan.SelectedSource != queryrouter.SourceCassandra {
		return nil, ErrUnsupportedSource
	}
	return state.Store.DatasetColumnLineage(ctx, datasetID)
}

// GetLineageGraph ports `get_lineage_graph`.
func GetLineageGraph(ctx context.Context, state *AppState, datasetID uuid.UUID, plan queryrouter.QueryPlan) (models.LineageGraph, error) {
	root := models.NodeKey{ID: datasetID, Kind: models.KindDataset}
	snap, err := LoadConnectedSnapshot(ctx, state, root, plan)
	if err != nil {
		return models.LineageGraph{}, err
	}
	return BuildGraph(snap.NodeOverlays, snap.Relations, nil), nil
}

// GetFullLineageGraph ports `get_full_lineage_graph`.
func GetFullLineageGraph(ctx context.Context, state *AppState, plan queryrouter.QueryPlan) (models.LineageGraph, error) {
	snap, err := LoadCompleteSnapshot(ctx, state, plan)
	if err != nil {
		return models.LineageGraph{}, err
	}
	return BuildGraph(snap.NodeOverlays, snap.Relations, nil), nil
}

// GetLineageImpactAnalysis ports `get_lineage_impact_analysis`. Returns
// (nil, nil) when the root has no overlay (the Rust impl returns
// `Ok(None)` so callers can map to 404).
func GetLineageImpactAnalysis(ctx context.Context, state *AppState, datasetID uuid.UUID, plan queryrouter.QueryPlan) (*models.LineageImpactAnalysis, error) {
	root := models.NodeKey{ID: datasetID, Kind: models.KindDataset}
	snap, err := LoadConnectedSnapshot(ctx, state, root, plan)
	if err != nil {
		return nil, err
	}
	rootView, ok := BuildNodeView(root, snap.NodeOverlays)
	if !ok {
		return nil, nil
	}
	upstreamPaths := BFSPaths(root, snap.Relations, Incoming)
	downstreamPaths := BFSPaths(root, snap.Relations, Outgoing)

	upstream := BuildImpactItems(root, upstreamPaths, snap.NodeOverlays, snap.Relations)
	downstream := BuildImpactItems(root, downstreamPaths, snap.NodeOverlays, snap.Relations)
	candidates := []models.LineageBuildCandidate{}
	for _, item := range downstream {
		kind, ok := models.ParseNodeKind(item.Kind)
		if !ok {
			continue
		}
		if kind == models.KindPipeline || kind == models.KindWorkflow {
			candidates = append(candidates, BuildCandidate(item, snap.NodeOverlays))
		}
	}

	return &models.LineageImpactAnalysis{
		Root:              rootView,
		PropagatedMarking: rootView.Marking,
		Upstream:          upstream,
		Downstream:        downstream,
		BuildCandidates:   candidates,
	}, nil
}

// executeSourceRun — full 1:1 port of `execute_source_run` from
// `libs/ontology-kernel/src/handlers/funnel.rs`, plus the upsert /
// pipeline-trigger / dataset-preview helpers it composes.
//
// Pipeline:
//
//  1. Load the source's ObjectType (must declare a primary_key_property).
//  2. Load the effective property schema.
//  3. Trigger the upstream pipeline run via HTTP into
//     pipeline-build-service (skipped when `skip_pipeline = true`).
//  4. Fetch the dataset preview rows via HTTP into
//     dataset-versioning-service.
//  5. For each row: transform via property_mappings, validate against
//     the object-type schema, look up the existing object id by
//     primary key, then upsert via `objects.ApplyObjectWrite` +
//     `objects.AppendObjectRevision`.
//  6. Compose the FunnelExecutionOutcome carrying counters + the
//     status string the public API surfaces.
package funnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/objects"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// FunnelExecutionOutcome mirrors the private Rust struct.
type funnelExecutionOutcome struct {
	rowsRead      int32
	insertedCount int32
	updatedCount  int32
	skippedCount  int32
	errorCount    int32
	details       json.RawMessage
	errorMessage  *string
	pipelineRunID *uuid.UUID
	status        string
}

type pipelineRunSummary struct {
	ID           uuid.UUID `json:"id"`
	Status       string    `json:"status"`
	ErrorMessage *string   `json:"error_message,omitempty"`
}

type datasetPreviewPayload struct {
	TotalRows int64             `json:"total_rows"`
	Rows      []json.RawMessage `json:"rows"`
	Warnings  []string          `json:"warnings"`
	Errors    []string          `json:"errors"`
}

// executeSourceRun mirrors `async fn execute_source_run`.
func executeSourceRun(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	source *models.OntologyFunnelSource,
	body *models.TriggerOntologyFunnelRunRequest,
) (funnelExecutionOutcome, error) {
	objectType, err := loadFunnelObjectType(ctx, state, source.ObjectTypeID)
	if err != nil {
		return funnelExecutionOutcome{}, fmt.Errorf("failed to load object type: %w", err)
	}
	if objectType == nil {
		return funnelExecutionOutcome{}, errors.New("object type for funnel source was not found")
	}
	if objectType.PrimaryKeyProperty == nil {
		return funnelExecutionOutcome{}, errors.New("object type must define primary_key_property for ontology funnel sync")
	}
	primaryKeyProperty := *objectType.PrimaryKeyProperty

	defs, err := loadFunnelEffectiveProperties(ctx, state, source.ObjectTypeID)
	if err != nil {
		return funnelExecutionOutcome{}, fmt.Errorf("failed to load object type properties: %w", err)
	}

	// Pipeline trigger (optional).
	var pipelineRun *pipelineRunSummary
	if !body.SkipPipeline {
		pipelineRun, err = triggerPipelineRun(ctx, state, claims, source, body.TriggerContext)
		if err != nil {
			return funnelExecutionOutcome{}, err
		}
	}

	// Preview limit.
	previewLimit := models.NormalizePreviewLimit(body.Limit)
	if body.Limit == nil {
		previewLimit = clampPreviewLimit(source.PreviewLimit)
	}

	// Dataset branch / version overrides.
	branch := source.DatasetBranch
	if body.DatasetBranch != nil {
		branch = body.DatasetBranch
	}
	version := source.DatasetVersion
	if body.DatasetVersion != nil {
		version = body.DatasetVersion
	}

	preview, err := fetchDatasetPreview(ctx, state, claims, source, previewLimit, branch, version)
	if err != nil {
		return funnelExecutionOutcome{}, err
	}

	var (
		insertedCount int32
		updatedCount  int32
		skippedCount  int32
		errorCount    int32
	)
	rowErrors := []map[string]any{}

	for index, row := range preview.Rows {
		transformed, terr := transformRow(row, source.PropertyMappings)
		if terr != nil {
			errorCount++
			rowErrors = append(rowErrors, map[string]any{"row_index": index, "error": terr.Error()})
			continue
		}
		normalized, verr := domain.ValidateObjectProperties(defs, transformed)
		if verr != nil {
			errorCount++
			rowErrors = append(rowErrors, map[string]any{"row_index": index, "error": verr.Error()})
			continue
		}
		pkValue, perr := primaryKeyValue(normalized, primaryKeyProperty)
		if perr != nil {
			skippedCount++
			rowErrors = append(rowErrors, map[string]any{"row_index": index, "error": perr.Error()})
			continue
		}
		existingID, lerr := objects.FindObjectIDByProperty(ctx, state, claims,
			source.ObjectTypeID, primaryKeyProperty, pkValue, storage.Strong())
		if lerr != nil {
			return funnelExecutionOutcome{}, fmt.Errorf("failed to look up existing object: %w", lerr)
		}

		if body.DryRun {
			if existingID != nil {
				updatedCount++
			} else {
				insertedCount++
			}
			continue
		}

		op, uerr := upsertObjectInstance(ctx, state, claims, existingID, source.ObjectTypeID, normalized, source.DefaultMarking)
		if uerr != nil {
			return funnelExecutionOutcome{}, uerr
		}
		switch op {
		case "update":
			updatedCount++
		case "insert":
			insertedCount++
		default:
			skippedCount++
		}
	}

	status := "completed"
	switch {
	case errorCount > 0 && body.DryRun:
		status = "dry_run_with_errors"
	case errorCount > 0:
		status = "completed_with_errors"
	case body.DryRun:
		status = "dry_run"
	}

	pipelineRunMeta := any(nil)
	var pipelineRunID *uuid.UUID
	if pipelineRun != nil {
		pipelineRunID = &pipelineRun.ID
		pipelineRunMeta = map[string]any{
			"id":     pipelineRun.ID,
			"status": pipelineRun.Status,
		}
	}
	details, _ := json.Marshal(map[string]any{
		"preview_total_rows":   preview.TotalRows,
		"warnings":             preview.Warnings,
		"preview_errors":       preview.Errors,
		"row_errors":           rowErrors,
		"primary_key_property": primaryKeyProperty,
		"dry_run":              body.DryRun,
		"pipeline_run":         pipelineRunMeta,
	})

	return funnelExecutionOutcome{
		rowsRead:      int32(len(preview.Rows)),
		insertedCount: insertedCount,
		updatedCount:  updatedCount,
		skippedCount:  skippedCount,
		errorCount:    errorCount,
		details:       details,
		pipelineRunID: pipelineRunID,
		status:        status,
	}, nil
}

// transformRow mirrors `fn transform_row`.
func transformRow(
	row json.RawMessage,
	mappings []models.OntologyFunnelPropertyMapping,
) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil {
		return nil, errors.New("dataset preview row is not a JSON object")
	}
	if len(mappings) == 0 {
		out, _ := json.Marshal(obj)
		return out, nil
	}
	mapped := map[string]json.RawMessage{}
	for _, m := range mappings {
		if v, ok := obj[m.SourceField]; ok {
			mapped[m.TargetProperty] = v
		}
	}
	out, _ := json.Marshal(mapped)
	return out, nil
}

// primaryKeyValue mirrors `fn primary_key_value`.
func primaryKeyValue(properties json.RawMessage, primaryKeyProperty string) (string, error) {
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(properties, &asMap); err != nil {
		return "", fmt.Errorf("missing primary key property '%s'", primaryKeyProperty)
	}
	raw, ok := asMap[primaryKeyProperty]
	if !ok {
		return "", fmt.Errorf("missing primary key property '%s'", primaryKeyProperty)
	}
	text, err := objects.ValueAsStoreText(raw)
	if err != nil {
		return "", fmt.Errorf("failed to serialize primary key property '%s': %w", primaryKeyProperty, err)
	}
	return text, nil
}

// upsertObjectInstance mirrors `async fn upsert_object_instance`.
// Returns "insert" / "update" / "skip" so the caller can update its
// counters.
func upsertObjectInstance(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	existingID *uuid.UUID,
	objectTypeID uuid.UUID,
	properties json.RawMessage,
	marking string,
) (string, error) {
	now := time.Now().UTC()
	var (
		instance        *domain.ObjectInstance
		expectedVersion *uint64
		operation       string
	)
	if existingID != nil {
		stored, err := objects.LoadRepoObjectFromStore(ctx, state, claims, *existingID, storage.Strong())
		if err != nil {
			return "", fmt.Errorf("failed to load existing funnel object: %w", err)
		}
		if stored == nil {
			return "", errors.New("existing funnel object was not found in object store")
		}
		createdBy := claims.Sub
		if stored.Owner != nil {
			if parsed, err := uuid.Parse(string(*stored.Owner)); err == nil {
				createdBy = parsed
			}
		}
		var orgID *uuid.UUID
		if stored.OrganizationID != nil {
			if parsed, err := uuid.Parse(*stored.OrganizationID); err == nil {
				orgID = &parsed
			}
		}
		if orgID == nil {
			orgID = claims.OrgID
		}
		createdMs := stored.UpdatedAtMs
		if stored.CreatedAtMs != nil {
			createdMs = *stored.CreatedAtMs
		}
		instance = &domain.ObjectInstance{
			ID:             *existingID,
			ObjectTypeID:   objectTypeID,
			Properties:     properties,
			CreatedBy:      createdBy,
			OrganizationID: orgID,
			Marking:        marking,
			CreatedAt:      time.UnixMilli(createdMs).UTC(),
			UpdatedAt:      now,
		}
		ev := stored.Version
		expectedVersion = &ev
		operation = "update"
	} else {
		newID, err := uuid.NewV7()
		if err != nil {
			return "", fmt.Errorf("uuid v7 for new funnel object: %w", err)
		}
		instance = &domain.ObjectInstance{
			ID:             newID,
			ObjectTypeID:   objectTypeID,
			Properties:     properties,
			CreatedBy:      claims.Sub,
			OrganizationID: claims.OrgID,
			Marking:        marking,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		operation = "insert"
	}

	extra, _ := json.Marshal(map[string]any{"source": "ontology_funnel"})
	outcome, err := objects.ApplyObjectWrite(ctx, state, claims, instance, expectedVersion, operation, extra)
	if err != nil {
		return "", err
	}
	if err := objects.AppendObjectRevision(ctx, state, claims, instance, operation,
		int64(outcome.CommittedVersion), nil); err != nil {
		return "", err
	}
	return operation, nil
}

// triggerPipelineRun mirrors `async fn trigger_pipeline_run`. POSTs
// to `${pipeline_service_url}/api/v1/pipelines/{pipeline_id}/run`
// with the same body shape Rust constructs.
func triggerPipelineRun(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	source *models.OntologyFunnelSource,
	overrideContext json.RawMessage,
) (*pipelineRunSummary, error) {
	if source.PipelineID == nil {
		return nil, nil
	}
	authHeader, err := issueServiceToken(state, claims)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(state.PipelineServiceURL, "/") +
		"/api/v1/pipelines/" + source.PipelineID.String() + "/run"

	context := mergeContexts(source.TriggerContext, overrideContext)
	payload := map[string]any{
		"skip_unchanged": true,
		"context": map[string]any{
			"trigger": map[string]any{
				"type":           "ontology-funnel",
				"source_id":      source.ID,
				"object_type_id": source.ObjectTypeID,
				"dataset_id":     source.DatasetID,
			},
			"funnel": json.RawMessage(context),
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build pipeline trigger request: %w", err)
	}
	req.Header.Set("authorization", authHeader)
	req.Header.Set("content-type", "application/json")

	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger funnel pipeline: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := readAllLimited(resp.Body, 1<<20)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("pipeline trigger failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var run pipelineRunSummary
	if err := json.Unmarshal(respBody, &run); err != nil {
		return nil, fmt.Errorf("failed to decode pipeline run response: %w", err)
	}
	if run.Status != "completed" {
		if run.ErrorMessage != nil && *run.ErrorMessage != "" {
			return nil, errors.New(*run.ErrorMessage)
		}
		return nil, fmt.Errorf("pipeline run finished with status '%s'", run.Status)
	}
	return &run, nil
}

// fetchDatasetPreview mirrors `async fn fetch_dataset_preview`. GETs
// `${dataset_service_url}/api/v1/datasets/{dataset_id}/preview` with
// the configured limit / branch / version query string.
func fetchDatasetPreview(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	source *models.OntologyFunnelSource,
	limit int32,
	datasetBranch *string,
	datasetVersion *int32,
) (*datasetPreviewPayload, error) {
	authHeader, err := issueServiceToken(state, claims)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(state.DatasetServiceURL, "/") +
		"/api/v1/datasets/" + source.DatasetID.String() + "/preview"
	q := "limit=" + strconv.Itoa(int(limit))
	if datasetBranch != nil && *datasetBranch != "" {
		q += "&branch=" + *datasetBranch
	}
	if datasetVersion != nil {
		q += "&version=" + strconv.Itoa(int(*datasetVersion))
	}
	url += "?" + q

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build dataset preview request: %w", err)
	}
	req.Header.Set("authorization", authHeader)

	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dataset preview: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := readAllLimited(resp.Body, 16<<20)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("dataset preview failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var preview datasetPreviewPayload
	if err := json.Unmarshal(respBody, &preview); err != nil {
		return nil, fmt.Errorf("failed to decode dataset preview payload: %w", err)
	}
	return &preview, nil
}

// issueServiceToken mirrors `fn issue_service_token`. Re-mints a
// service-grade token (admin role + *:* permissions, classification
// pii) so the funnel ingestion can call back into ontology /
// pipeline / dataset services without leaking the user's clearance.
func issueServiceToken(state *ontologykernel.AppState, claims *authmw.Claims) (string, error) {
	if state.JWTConfig == nil {
		return "", errors.New("service token requires a JWT config on AppState")
	}
	now := time.Now().UTC()
	jti, _ := uuid.NewV7()
	attrs, _ := json.Marshal(map[string]any{
		"service":                  "ontology-service",
		"classification_clearance": "pii",
		"impersonated_actor_id":    claims.Sub,
	})
	authMethods := []string{"service"}
	tokenUse := "access"
	c := &authmw.Claims{
		Sub:         jti,
		IAT:         now.Unix(),
		EXP:         now.Add(state.JWTConfig.AccessTTL).Unix(),
		JTI:         jti,
		Email:       "ontology-service@internal.openfoundry",
		Name:        "ontology-service",
		Roles:       []string{"admin"},
		Permissions: []string{"*:*"},
		OrgID:       claims.OrgID,
		Attributes:  attrs,
		AuthMethods: authMethods,
		TokenUse:    &tokenUse,
	}
	if c.EXP <= c.IAT {
		c.EXP = now.Add(time.Hour).Unix()
	}
	token, err := authmw.EncodeToken(state.JWTConfig, c)
	if err != nil {
		return "", fmt.Errorf("failed to issue service token: %w", err)
	}
	return "Bearer " + token, nil
}

// mergeContexts mirrors `fn merge_contexts`. Object-on-object merge
// where `override` keys win; non-object inputs return the override
// (or the base when no override). Pure JSON.
func mergeContexts(base, override json.RawMessage) json.RawMessage {
	var asBase, asOverride map[string]json.RawMessage
	baseOK := json.Unmarshal(base, &asBase) == nil
	overrideOK := len(override) > 0 && json.Unmarshal(override, &asOverride) == nil

	switch {
	case baseOK && overrideOK:
		merged := make(map[string]json.RawMessage, len(asBase)+len(asOverride))
		for k, v := range asBase {
			merged[k] = v
		}
		for k, v := range asOverride {
			merged[k] = v
		}
		out, _ := json.Marshal(merged)
		return out
	case baseOK:
		out, _ := json.Marshal(asBase)
		return out
	case len(override) > 0:
		return override
	default:
		return base
	}
}

// readAllLimited reads up to `limit` bytes from r. Avoids importing
// io.ReadAll's unbounded variant — these endpoints can return large
// payloads (16 MB cap matches the Rust default).
func readAllLimited(r interface{ Read([]byte) (int, error) }, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	for buf.Len() < int(limit) {
		n, err := r.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.Bytes(), nil
}

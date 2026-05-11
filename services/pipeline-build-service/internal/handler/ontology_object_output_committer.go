package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

// OntologyObjectOutputCommitter deploys Pipeline Builder object-type outputs
// after their backing dataset commit succeeds.
type OntologyObjectOutputCommitter struct {
	Dataset                  executor.OutputCommitter
	Metadata                 executor.OutputCommitter
	OntologyDefinitionURL    string
	OntologyDefinitionBearer string
	ObjectDatabaseURL        string
	ObjectDatabaseBearer     string
	Client                   *http.Client
}

func (c OntologyObjectOutputCommitter) Commit(ctx context.Context, tx executor.OutputTransaction, result executor.NodeResult) error {
	if c.Dataset != nil {
		if err := c.Dataset.Commit(ctx, tx, result); err != nil {
			return err
		}
	}
	if !isObjectTypeOutput(tx) {
		if isLinkTypeOutput(tx) {
			linkType, err := c.deployLinkType(ctx, tx)
			if err != nil {
				return err
			}
			if err := c.materializeLinks(ctx, linkType.ID, tx, result); err != nil {
				return err
			}
		}
		return commitOutputMetadata(ctx, c.Metadata, tx, result)
	}
	objectType, mappings, err := c.deployObjectType(ctx, tx, result)
	if err != nil {
		return err
	}
	if err := c.materializeObjects(ctx, objectType.ID, tx, result, mappings); err != nil {
		return err
	}
	return commitOutputMetadata(ctx, c.Metadata, tx, result)
}

func isObjectTypeOutput(tx executor.OutputTransaction) bool {
	if isLinkTypeOutput(tx) {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(tx.OutputKind))
	return strings.Contains(kind, "object") || strings.Contains(kind, "ontology")
}

func isLinkTypeOutput(tx executor.OutputTransaction) bool {
	kind := strings.ToLower(strings.TrimSpace(tx.OutputKind))
	return strings.Contains(kind, "link")
}

type ontologyObjectTypeWire struct {
	ID                 string  `json:"id,omitempty"`
	Name               string  `json:"name,omitempty"`
	DisplayName        string  `json:"display_name,omitempty"`
	PluralDisplayName  *string `json:"plural_display_name,omitempty"`
	Description        string  `json:"description,omitempty"`
	PrimaryKeyProperty *string `json:"primary_key_property,omitempty"`
	Icon               *string `json:"icon,omitempty"`
	Color              *string `json:"color,omitempty"`
	Editable           *bool   `json:"editable,omitempty"`
	BackingDatasetID   *string `json:"backing_dataset_id,omitempty"`
	BackingDatasetRID  *string `json:"backing_dataset_rid,omitempty"`
	PipelineRID        *string `json:"pipeline_rid,omitempty"`
	ManagedBy          *string `json:"managed_by,omitempty"`
}

type ontologyPropertyWire struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name"`
	DisplayName      string `json:"display_name,omitempty"`
	Description      string `json:"description,omitempty"`
	PropertyType     string `json:"property_type"`
	Required         bool   `json:"required,omitempty"`
	UniqueConstraint bool   `json:"unique_constraint,omitempty"`
}

type ontologyLinkTypeWire struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Description  string `json:"description,omitempty"`
	SourceTypeID string `json:"source_type_id,omitempty"`
	TargetTypeID string `json:"target_type_id,omitempty"`
	Cardinality  string `json:"cardinality,omitempty"`
}

func (c OntologyObjectOutputCommitter) deployObjectType(ctx context.Context, tx executor.OutputTransaction, result executor.NodeResult) (ontologyObjectTypeWire, []executor.OutputPropertyMapping, error) {
	if strings.TrimSpace(c.OntologyDefinitionURL) == "" {
		return ontologyObjectTypeWire{}, nil, fmt.Errorf("ontology_object_output_not_configured: set ONTOLOGY_DEFINITION_SERVICE_URL to deploy object outputs")
	}
	rows := resultRows(result.Metadata)
	columns := resultColumns(result.Metadata)
	if len(columns) == 0 {
		columns = inferResultColumns(rows)
	}
	mappings := objectPropertyMappings(tx, columns, rows)
	if len(mappings) == 0 {
		return ontologyObjectTypeWire{}, nil, fmt.Errorf("ontology_object_output_invalid: object output %s has no properties to map", tx.OutputNodeID)
	}
	primaryKey := strings.TrimSpace(tx.ObjectTypePrimaryKey)
	if primaryKey == "" {
		primaryKey = mappings[0].TargetProperty
	}
	if !mappingHasTarget(mappings, primaryKey) {
		return ontologyObjectTypeWire{}, nil, fmt.Errorf("ontology_object_output_invalid: primary key property %q is not mapped", primaryKey)
	}
	objectType, found, err := c.findObjectType(ctx, tx)
	if err != nil {
		return ontologyObjectTypeWire{}, nil, err
	}
	body := objectTypeBody(tx, primaryKey)
	if found {
		updated, err := c.updateObjectType(ctx, objectType.ID, body)
		if err != nil {
			return ontologyObjectTypeWire{}, nil, err
		}
		objectType = updated
	} else {
		created, err := c.createObjectType(ctx, body)
		if err != nil {
			return ontologyObjectTypeWire{}, nil, err
		}
		objectType = created
	}
	if objectType.ID == "" {
		return ontologyObjectTypeWire{}, nil, fmt.Errorf("ontology_object_output_failed: ontology-definition returned an object type without id")
	}
	if err := c.ensureProperties(ctx, objectType.ID, mappings); err != nil {
		return ontologyObjectTypeWire{}, nil, err
	}
	return objectType, mappings, nil
}

func (c OntologyObjectOutputCommitter) findObjectType(ctx context.Context, tx executor.OutputTransaction) (ontologyObjectTypeWire, bool, error) {
	if id := strings.TrimSpace(tx.ObjectTypeID); id != "" {
		got, err := c.getObjectType(ctx, id)
		if err == nil {
			return got, true, nil
		}
		if !isHTTPNotFound(err) {
			return ontologyObjectTypeWire{}, false, err
		}
	}
	name := normaliseObjectTypeName(tx.ObjectTypeName)
	if name == "" {
		return ontologyObjectTypeWire{}, false, nil
	}
	items, err := c.listObjectTypes(ctx)
	if err != nil {
		return ontologyObjectTypeWire{}, false, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, name) {
			return item, true, nil
		}
	}
	return ontologyObjectTypeWire{}, false, nil
}

func (c OntologyObjectOutputCommitter) getObjectType(ctx context.Context, id string) (ontologyObjectTypeWire, error) {
	var out ontologyObjectTypeWire
	err := c.doJSON(ctx, http.MethodGet, c.ontologyURL("/api/v1/ontology/types/"+url.PathEscape(id)), nil, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) listObjectTypes(ctx context.Context) ([]ontologyObjectTypeWire, error) {
	var envelope struct {
		Items []ontologyObjectTypeWire `json:"items"`
		Data  []ontologyObjectTypeWire `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.ontologyURL("/api/v1/ontology/types"), nil, &envelope, c.OntologyDefinitionBearer); err != nil {
		return nil, err
	}
	if len(envelope.Items) > 0 {
		return envelope.Items, nil
	}
	return envelope.Data, nil
}

func (c OntologyObjectOutputCommitter) createObjectType(ctx context.Context, body ontologyObjectTypeWire) (ontologyObjectTypeWire, error) {
	var out ontologyObjectTypeWire
	err := c.doJSON(ctx, http.MethodPost, c.ontologyURL("/api/v1/ontology/types"), body, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) updateObjectType(ctx context.Context, id string, body ontologyObjectTypeWire) (ontologyObjectTypeWire, error) {
	var out ontologyObjectTypeWire
	err := c.doJSON(ctx, http.MethodPatch, c.ontologyURL("/api/v1/ontology/types/"+url.PathEscape(id)), body, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) ensureProperties(ctx context.Context, objectTypeID string, mappings []executor.OutputPropertyMapping) error {
	existing, err := c.listProperties(ctx, objectTypeID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, prop := range existing {
		seen[strings.ToLower(strings.TrimSpace(prop.Name))] = struct{}{}
	}
	for _, mapping := range mappings {
		name := normalisePropertyName(mapping.TargetProperty)
		if name == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(name)]; ok {
			continue
		}
		body := ontologyPropertyWire{
			Name:             name,
			DisplayName:      firstNonEmpty(mapping.DisplayName, displayNameFromIdentifier(name)),
			PropertyType:     normaliseOntologyPropertyType(mapping.PropertyType),
			Required:         mapping.Required,
			UniqueConstraint: mapping.UniqueConstraint,
		}
		var out ontologyPropertyWire
		if err := c.doJSON(ctx, http.MethodPost, c.ontologyURL("/api/v1/ontology/types/"+url.PathEscape(objectTypeID)+"/properties"), body, &out, c.OntologyDefinitionBearer); err != nil {
			return err
		}
		seen[strings.ToLower(name)] = struct{}{}
	}
	return nil
}

func (c OntologyObjectOutputCommitter) listProperties(ctx context.Context, objectTypeID string) ([]ontologyPropertyWire, error) {
	var envelope struct {
		Items []ontologyPropertyWire `json:"items"`
		Data  []ontologyPropertyWire `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.ontologyURL("/api/v1/ontology/types/"+url.PathEscape(objectTypeID)+"/properties"), nil, &envelope, c.OntologyDefinitionBearer); err != nil {
		return nil, err
	}
	if len(envelope.Data) > 0 {
		return envelope.Data, nil
	}
	return envelope.Items, nil
}

func (c OntologyObjectOutputCommitter) materializeObjects(ctx context.Context, objectTypeID string, tx executor.OutputTransaction, result executor.NodeResult, mappings []executor.OutputPropertyMapping) error {
	if strings.TrimSpace(c.ObjectDatabaseURL) == "" {
		return fmt.Errorf("ontology_object_output_not_configured: set OBJECT_DATABASE_SERVICE_URL to materialize object outputs")
	}
	rows := resultRows(result.Metadata)
	if len(rows) == 0 {
		return nil
	}
	primaryKey := strings.TrimSpace(tx.ObjectTypePrimaryKey)
	if primaryKey == "" && len(mappings) > 0 {
		primaryKey = mappings[0].TargetProperty
	}
	for _, row := range rows {
		props := projectObjectProperties(row, mappings)
		if len(props) == 0 {
			continue
		}
		pkValue := strings.TrimSpace(fmt.Sprint(props[primaryKey]))
		if pkValue == "" || pkValue == "<nil>" {
			return fmt.Errorf("ontology_object_output_invalid: row is missing primary key property %q", primaryKey)
		}
		existingID, err := c.findObjectByPrimaryKey(ctx, objectTypeID, primaryKey, props[primaryKey])
		if err != nil {
			return err
		}
		if existingID != "" {
			if err := c.patchObject(ctx, objectTypeID, existingID, props); err != nil {
				return err
			}
			continue
		}
		if err := c.createObject(ctx, objectTypeID, props); err != nil {
			return err
		}
	}
	return nil
}

func (c OntologyObjectOutputCommitter) findObjectByPrimaryKey(ctx context.Context, objectTypeID string, primaryKey string, value any) (string, error) {
	body := map[string]any{"equals": map[string]any{primaryKey: value}, "limit": 1}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.objectDBURL("/api/v1/ontology/types/"+url.PathEscape(objectTypeID)+"/objects/query"), body, &out, c.ObjectDatabaseBearer); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", nil
	}
	return out.Data[0].ID, nil
}

func (c OntologyObjectOutputCommitter) createObject(ctx context.Context, objectTypeID string, properties map[string]any) error {
	body := map[string]any{"properties": properties}
	var out map[string]any
	return c.doJSON(ctx, http.MethodPost, c.objectDBURL("/api/v1/ontology/types/"+url.PathEscape(objectTypeID)+"/objects/"), body, &out, c.ObjectDatabaseBearer)
}

func (c OntologyObjectOutputCommitter) patchObject(ctx context.Context, objectTypeID string, objectID string, properties map[string]any) error {
	body := map[string]any{"properties": properties, "replace": true}
	var out map[string]any
	return c.doJSON(ctx, http.MethodPatch, c.objectDBURL("/api/v1/ontology/types/"+url.PathEscape(objectTypeID)+"/objects/"+url.PathEscape(objectID)), body, &out, c.ObjectDatabaseBearer)
}

func (c OntologyObjectOutputCommitter) deployLinkType(ctx context.Context, tx executor.OutputTransaction) (ontologyLinkTypeWire, error) {
	if strings.TrimSpace(c.OntologyDefinitionURL) == "" {
		return ontologyLinkTypeWire{}, fmt.Errorf("ontology_link_output_not_configured: set ONTOLOGY_DEFINITION_SERVICE_URL to deploy link outputs")
	}
	if strings.TrimSpace(tx.LinkSourceObjectTypeID) == "" || strings.TrimSpace(tx.LinkTargetObjectTypeID) == "" {
		return ontologyLinkTypeWire{}, fmt.Errorf("ontology_link_output_invalid: link output %s must reference source and target object outputs", tx.OutputNodeID)
	}
	cardinality := normaliseLinkCardinality(tx.LinkTypeCardinality)
	switch cardinality {
	case "one_to_many", "many_to_many", "many_to_one":
	case "one_to_one":
		return ontologyLinkTypeWire{}, fmt.Errorf("ontology_link_output_invalid: one-to-one cardinality is not supported for Pipeline Builder link outputs")
	default:
		return ontologyLinkTypeWire{}, fmt.Errorf("ontology_link_output_invalid: unsupported link cardinality %q", tx.LinkTypeCardinality)
	}
	linkType, found, err := c.findLinkType(ctx, tx)
	if err != nil {
		return ontologyLinkTypeWire{}, err
	}
	body := linkTypeBody(tx, cardinality)
	if found {
		updated, err := c.updateLinkType(ctx, linkType.ID, body)
		if err != nil {
			return ontologyLinkTypeWire{}, err
		}
		linkType = updated
	} else {
		created, err := c.createLinkType(ctx, body)
		if err != nil {
			return ontologyLinkTypeWire{}, err
		}
		linkType = created
	}
	if linkType.ID == "" {
		return ontologyLinkTypeWire{}, fmt.Errorf("ontology_link_output_failed: ontology-definition returned a link type without id")
	}
	return linkType, nil
}

func (c OntologyObjectOutputCommitter) findLinkType(ctx context.Context, tx executor.OutputTransaction) (ontologyLinkTypeWire, bool, error) {
	if id := strings.TrimSpace(tx.LinkTypeID); id != "" {
		got, err := c.getLinkType(ctx, id)
		if err == nil {
			return got, true, nil
		}
		if !isHTTPNotFound(err) {
			return ontologyLinkTypeWire{}, false, err
		}
	}
	items, err := c.listLinkTypes(ctx, tx.LinkSourceObjectTypeID)
	if err != nil {
		return ontologyLinkTypeWire{}, false, err
	}
	name := normaliseObjectTypeName(tx.LinkTypeName)
	for _, item := range items {
		if strings.TrimSpace(tx.LinkTypeID) != "" && item.ID == strings.TrimSpace(tx.LinkTypeID) {
			return item, true, nil
		}
		if name != "" &&
			strings.EqualFold(item.Name, name) &&
			strings.EqualFold(item.SourceTypeID, tx.LinkSourceObjectTypeID) &&
			strings.EqualFold(item.TargetTypeID, tx.LinkTargetObjectTypeID) {
			return item, true, nil
		}
	}
	return ontologyLinkTypeWire{}, false, nil
}

func (c OntologyObjectOutputCommitter) getLinkType(ctx context.Context, id string) (ontologyLinkTypeWire, error) {
	var out ontologyLinkTypeWire
	err := c.doJSON(ctx, http.MethodGet, c.ontologyURL("/api/v1/ontology/links/"+url.PathEscape(id)), nil, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) listLinkTypes(ctx context.Context, objectTypeID string) ([]ontologyLinkTypeWire, error) {
	var envelope struct {
		Items []ontologyLinkTypeWire `json:"items"`
		Data  []ontologyLinkTypeWire `json:"data"`
	}
	path := "/api/v1/ontology/links"
	if strings.TrimSpace(objectTypeID) != "" {
		path += "?object_type_id=" + url.QueryEscape(strings.TrimSpace(objectTypeID))
	}
	if err := c.doJSON(ctx, http.MethodGet, c.ontologyURL(path), nil, &envelope, c.OntologyDefinitionBearer); err != nil {
		return nil, err
	}
	if len(envelope.Data) > 0 {
		return envelope.Data, nil
	}
	return envelope.Items, nil
}

func (c OntologyObjectOutputCommitter) createLinkType(ctx context.Context, body ontologyLinkTypeWire) (ontologyLinkTypeWire, error) {
	var out ontologyLinkTypeWire
	err := c.doJSON(ctx, http.MethodPost, c.ontologyURL("/api/v1/ontology/links"), body, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) updateLinkType(ctx context.Context, id string, body ontologyLinkTypeWire) (ontologyLinkTypeWire, error) {
	var out ontologyLinkTypeWire
	err := c.doJSON(ctx, http.MethodPatch, c.ontologyURL("/api/v1/ontology/links/"+url.PathEscape(id)), body, &out, c.OntologyDefinitionBearer)
	return out, err
}

func (c OntologyObjectOutputCommitter) materializeLinks(ctx context.Context, linkTypeID string, tx executor.OutputTransaction, result executor.NodeResult) error {
	if strings.TrimSpace(c.ObjectDatabaseURL) == "" {
		return fmt.Errorf("ontology_link_output_not_configured: set OBJECT_DATABASE_SERVICE_URL to materialize link outputs")
	}
	rows := resultRows(result.Metadata)
	if len(rows) == 0 {
		return nil
	}
	sourcePK := firstNonEmpty(tx.LinkSourcePrimaryKey, "id")
	targetPK := firstNonEmpty(tx.LinkTargetPrimaryKey, "id")
	sourceColumn := strings.TrimSpace(tx.LinkSourceKeyColumn)
	targetColumn := strings.TrimSpace(tx.LinkTargetKeyColumn)
	if sourceColumn == "" || targetColumn == "" {
		return fmt.Errorf("ontology_link_output_invalid: link output %s requires source and target key columns", tx.OutputNodeID)
	}
	tenant := firstNonEmpty(tx.LinkTenant, "default")
	cardinality := normaliseLinkCardinality(tx.LinkTypeCardinality)
	targetToSource := map[string]string{}
	sourceToTarget := map[string]string{}
	for _, row := range rows {
		sourceValue, ok := rowValue(row, sourceColumn)
		if !ok || strings.TrimSpace(fmt.Sprint(sourceValue)) == "" || fmt.Sprint(sourceValue) == "<nil>" {
			return fmt.Errorf("ontology_link_output_invalid: row is missing source key column %q", sourceColumn)
		}
		targetValue, ok := rowValue(row, targetColumn)
		if !ok || strings.TrimSpace(fmt.Sprint(targetValue)) == "" || fmt.Sprint(targetValue) == "<nil>" {
			return fmt.Errorf("ontology_link_output_invalid: row is missing target key column %q", targetColumn)
		}
		sourceKey := fmt.Sprint(sourceValue)
		targetKey := fmt.Sprint(targetValue)
		if err := validateLinkCardinality(cardinality, sourceKey, targetKey, sourceToTarget, targetToSource); err != nil {
			return err
		}
		sourceObjectID, err := c.findObjectByPrimaryKey(ctx, tx.LinkSourceObjectTypeID, sourcePK, sourceValue)
		if err != nil {
			return err
		}
		if sourceObjectID == "" {
			return fmt.Errorf("ontology_link_output_invalid: source object %s.%s=%q not found", tx.LinkSourceObjectTypeID, sourcePK, sourceKey)
		}
		targetObjectID, err := c.findObjectByPrimaryKey(ctx, tx.LinkTargetObjectTypeID, targetPK, targetValue)
		if err != nil {
			return err
		}
		if targetObjectID == "" {
			return fmt.Errorf("ontology_link_output_invalid: target object %s.%s=%q not found", tx.LinkTargetObjectTypeID, targetPK, targetKey)
		}
		payload := map[string]any{
			"source_key_property": sourcePK,
			"source_key_value":    sourceValue,
			"target_key_property": targetPK,
			"target_key_value":    targetValue,
			"pipeline_run":        tx.TransactionRID,
		}
		if err := c.createLink(ctx, tenant, linkTypeID, sourceObjectID, targetObjectID, payload); err != nil {
			return err
		}
	}
	return nil
}

func (c OntologyObjectOutputCommitter) createLink(ctx context.Context, tenant string, linkTypeID string, from string, to string, payload map[string]any) error {
	body := map[string]any{"from": from, "to": to, "payload": payload}
	var out map[string]any
	return c.doJSON(ctx, http.MethodPost, c.objectDBURL("/api/v1/object-database/links/"+url.PathEscape(tenant)+"/"+url.PathEscape(linkTypeID)), body, &out, c.ObjectDatabaseBearer)
}

func (c OntologyObjectOutputCommitter) ontologyURL(path string) string {
	return strings.TrimRight(strings.TrimSpace(c.OntologyDefinitionURL), "/") + path
}

func (c OntologyObjectOutputCommitter) objectDBURL(path string) string {
	return strings.TrimRight(strings.TrimSpace(c.ObjectDatabaseURL), "/") + path
}

func (c OntologyObjectOutputCommitter) doJSON(ctx context.Context, method string, endpoint string, in any, out any, bearer string) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := strings.TrimSpace(bearer); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ontology_object_output_http_failed: %w", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpStatusError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(raw))}
	}
	if readErr != nil {
		return readErr
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("ontology_object_output_decode_failed: %w", err)
		}
	}
	return nil
}

type httpStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("ontology_object_output_failed: upstream returned %s: %s", e.Status, e.Body)
}

func isHTTPNotFound(err error) bool {
	if status, ok := err.(httpStatusError); ok {
		return status.StatusCode == http.StatusNotFound
	}
	return false
}

func objectTypeBody(tx executor.OutputTransaction, primaryKey string) ontologyObjectTypeWire {
	name := normaliseObjectTypeName(tx.ObjectTypeName)
	if name == "" {
		name = normaliseObjectTypeName(tx.OutputNodeID)
	}
	display := firstNonEmpty(tx.ObjectTypeDisplayName, tx.ObjectTypeName, displayNameFromIdentifier(name))
	description := "Managed by OpenFoundry Pipeline Builder"
	editable := tx.ObjectTypeEditable
	managedBy := "pipeline-builder"
	body := ontologyObjectTypeWire{
		ID:                 strings.TrimSpace(tx.ObjectTypeID),
		Name:               name,
		DisplayName:        display,
		Description:        description,
		PrimaryKeyProperty: &primaryKey,
		Editable:           &editable,
		ManagedBy:          &managedBy,
	}
	if strings.TrimSpace(tx.ObjectTypePluralName) != "" {
		value := strings.TrimSpace(tx.ObjectTypePluralName)
		body.PluralDisplayName = &value
	}
	if strings.TrimSpace(tx.ObjectTypeIcon) != "" {
		value := strings.TrimSpace(tx.ObjectTypeIcon)
		body.Icon = &value
	}
	if strings.TrimSpace(tx.ObjectTypeColor) != "" {
		value := strings.TrimSpace(tx.ObjectTypeColor)
		body.Color = &value
	}
	if id, ok := datasetUUID(tx.DatasetRID); ok {
		body.BackingDatasetID = &id
	}
	if strings.TrimSpace(tx.DatasetRID) != "" {
		value := routeDatasetRID(tx.DatasetRID)
		body.BackingDatasetRID = &value
	}
	if strings.TrimSpace(tx.PipelineRID) != "" {
		value := strings.TrimSpace(tx.PipelineRID)
		body.PipelineRID = &value
	}
	return body
}

func objectPropertyMappings(tx executor.OutputTransaction, columns []string, rows []map[string]json.RawMessage) []executor.OutputPropertyMapping {
	if len(tx.ObjectPropertyMappings) > 0 {
		out := make([]executor.OutputPropertyMapping, 0, len(tx.ObjectPropertyMappings))
		for _, mapping := range tx.ObjectPropertyMappings {
			target := normalisePropertyName(mapping.TargetProperty)
			source := strings.TrimSpace(mapping.SourceField)
			if source == "" {
				source = target
			}
			if target == "" {
				continue
			}
			if mapping.PropertyType == "" {
				mapping.PropertyType = inferOntologyPropertyType(source, rows)
			}
			mapping.SourceField = source
			mapping.TargetProperty = target
			if mapping.DisplayName == "" {
				mapping.DisplayName = displayNameFromIdentifier(target)
			}
			out = append(out, mapping)
		}
		return out
	}
	out := make([]executor.OutputPropertyMapping, 0, len(columns))
	for _, column := range columns {
		name := normalisePropertyName(column)
		if name == "" {
			continue
		}
		out = append(out, executor.OutputPropertyMapping{
			SourceField:      column,
			TargetProperty:   name,
			DisplayName:      displayNameFromIdentifier(name),
			PropertyType:     inferOntologyPropertyType(column, rows),
			Required:         !inferNullable(column, rows),
			UniqueConstraint: false,
		})
	}
	return out
}

func mappingHasTarget(mappings []executor.OutputPropertyMapping, target string) bool {
	target = normalisePropertyName(target)
	for _, mapping := range mappings {
		if normalisePropertyName(mapping.TargetProperty) == target {
			return true
		}
	}
	return false
}

func projectObjectProperties(row map[string]json.RawMessage, mappings []executor.OutputPropertyMapping) map[string]any {
	out := make(map[string]any, len(mappings))
	for _, mapping := range mappings {
		source := strings.TrimSpace(mapping.SourceField)
		if source == "" {
			source = mapping.TargetProperty
		}
		raw, ok := row[source]
		if !ok {
			continue
		}
		value, ok := decodeJSONValue(raw)
		if !ok {
			continue
		}
		out[normalisePropertyName(mapping.TargetProperty)] = value
	}
	return out
}

func rowValue(row map[string]json.RawMessage, column string) (any, bool) {
	raw, ok := row[strings.TrimSpace(column)]
	if !ok {
		return nil, false
	}
	return decodeJSONValue(raw)
}

func decodeJSONValue(raw json.RawMessage) (any, bool) {
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	if number, ok := value.(json.Number); ok {
		if strings.ContainsAny(number.String(), ".eE") {
			if f, err := number.Float64(); err == nil {
				value = f
			}
		} else if i, err := number.Int64(); err == nil {
			value = i
		}
	}
	return value, true
}

func inferOntologyPropertyType(column string, rows []map[string]json.RawMessage) string {
	switch inferSchemaType(column, rows) {
	case "BOOLEAN":
		return "boolean"
	case "BYTE", "SHORT", "INTEGER", "LONG":
		return "integer"
	case "FLOAT", "DOUBLE", "DECIMAL":
		return "float"
	case "DATE":
		return "date"
	case "TIMESTAMP":
		return "timestamp"
	default:
		return "string"
	}
}

func normaliseOntologyPropertyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "boolean", "bool":
		return "boolean"
	case "integer", "int", "long", "short", "byte":
		return "integer"
	case "float", "double", "decimal":
		return "float"
	case "date":
		return "date"
	case "timestamp", "datetime":
		return "timestamp"
	case "json", "array", "vector", "reference", "geo_point", "media_reference", "struct", "attachment":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "string"
	}
}

var identifierCleaner = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func normaliseObjectTypeName(value string) string {
	cleaned := identifierCleaner.ReplaceAllString(strings.TrimSpace(value), "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return ""
	}
	if !strings.Contains(cleaned, "_") {
		return strings.ToUpper(cleaned[:1]) + cleaned[1:]
	}
	parts := strings.Split(cleaned, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, "")
}

func normalisePropertyName(value string) string {
	cleaned := identifierCleaner.ReplaceAllString(strings.TrimSpace(value), "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return ""
	}
	return strings.ToLower(cleaned[:1]) + cleaned[1:]
}

func displayNameFromIdentifier(value string) string {
	value = identifierCleaner.ReplaceAllString(strings.TrimSpace(value), " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return "Property"
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func datasetUUID(datasetRID string) (string, bool) {
	trimmed := strings.TrimSpace(datasetRID)
	trimmed = strings.TrimPrefix(trimmed, datasetRIDPrefix)
	if _, err := uuid.Parse(trimmed); err != nil {
		return "", false
	}
	return trimmed, true
}

func linkTypeBody(tx executor.OutputTransaction, cardinality string) ontologyLinkTypeWire {
	name := normaliseObjectTypeName(tx.LinkTypeName)
	if name == "" {
		name = normaliseObjectTypeName(tx.OutputNodeID)
	}
	return ontologyLinkTypeWire{
		ID:           strings.TrimSpace(tx.LinkTypeID),
		Name:         name,
		DisplayName:  firstNonEmpty(tx.LinkTypeDisplayName, tx.LinkTypeName, displayNameFromIdentifier(name)),
		Description:  firstNonEmpty(tx.LinkTypeDescription, "Managed by OpenFoundry Pipeline Builder"),
		SourceTypeID: strings.TrimSpace(tx.LinkSourceObjectTypeID),
		TargetTypeID: strings.TrimSpace(tx.LinkTargetObjectTypeID),
		Cardinality:  cardinality,
	}
}

func validateLinkCardinality(cardinality string, sourceKey string, targetKey string, sourceToTarget map[string]string, targetToSource map[string]string) error {
	switch normaliseLinkCardinality(cardinality) {
	case "one_to_many":
		if previous, ok := targetToSource[targetKey]; ok && previous != sourceKey {
			return fmt.Errorf("ontology_link_output_invalid: one-to-many link maps target key %q to multiple source keys", targetKey)
		}
		targetToSource[targetKey] = sourceKey
	case "many_to_one":
		if previous, ok := sourceToTarget[sourceKey]; ok && previous != targetKey {
			return fmt.Errorf("ontology_link_output_invalid: many-to-one link maps source key %q to multiple target keys", sourceKey)
		}
		sourceToTarget[sourceKey] = targetKey
	}
	return nil
}

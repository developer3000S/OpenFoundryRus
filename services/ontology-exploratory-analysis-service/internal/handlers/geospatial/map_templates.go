package geospatial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

const mapTemplateSelectColumns = `id, name, description, parameters, layers, overlay_layers, viewport, interface_options, tags, created_at, updated_at`

func LoadAllMapTemplates(rctx context.Context, db DB) ([]models.MapTemplateDefinition, error) {
	rows, err := db.Query(rctx, `SELECT `+mapTemplateSelectColumns+` FROM geospatial_map_templates ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.MapTemplateDefinition, 0)
	for rows.Next() {
		row, err := scanMapTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		def, err := row.ToDefinition()
		if err != nil {
			return nil, fmt.Errorf("decode map template row: %w", err)
		}
		out = append(out, def)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func LoadMapTemplateRow(rctx context.Context, db DB, id uuid.UUID) (models.MapTemplateDefinition, bool, error) {
	row := db.QueryRow(rctx, `SELECT `+mapTemplateSelectColumns+` FROM geospatial_map_templates WHERE id = $1`, id)
	r, err := scanMapTemplateRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.MapTemplateDefinition{}, false, nil
	}
	if err != nil {
		return models.MapTemplateDefinition{}, false, err
	}
	def, err := r.ToDefinition()
	if err != nil {
		return models.MapTemplateDefinition{}, false, fmt.Errorf("decode map template row: %w", err)
	}
	return def, true, nil
}

func scanMapTemplateRow(rs rowScanner) (models.MapTemplateRow, error) {
	var r models.MapTemplateRow
	err := rs.Scan(
		&r.ID,
		&r.Name,
		&r.Description,
		&r.Parameters,
		&r.Layers,
		&r.OverlayLayers,
		&r.Viewport,
		&r.InterfaceOptions,
		&r.Tags,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	return r, err
}

func (s *AppState) ListMapTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := LoadAllMapTemplates(r.Context(), s.DB)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.MapTemplateDefinition]{Items: templates})
}

func (s *AppState) GetMapTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mapTemplateIDFromRequest(w, r)
	if !ok {
		return
	}
	template, found, err := LoadMapTemplateRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "map template not found")
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *AppState) CreateMapTemplate(w http.ResponseWriter, r *http.Request) {
	var req models.CreateMapTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCreateMapTemplateRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	parametersBytes, err := json.Marshal(req.Parameters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	layersBytes, err := json.Marshal(req.Layers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overlaysBytes, err := json.Marshal(req.OverlayLayers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	viewportBytes, err := json.Marshal(req.Viewport)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	interfaceBytes, err := json.Marshal(req.InterfaceOptions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsBytes, err := json.Marshal(req.Tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = s.DB.Exec(r.Context(),
		`INSERT INTO geospatial_map_templates (id, name, description, parameters, layers, overlay_layers, viewport, interface_options, tags, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9::jsonb, $10, $11)`,
		id,
		req.Name,
		req.Description,
		parametersBytes,
		layersBytes,
		overlaysBytes,
		viewportBytes,
		interfaceBytes,
		tagsBytes,
		now,
		now,
	)
	if err != nil {
		s.dbError(w, err)
		return
	}
	template, found, err := LoadMapTemplateRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusInternalServerError, "created map template could not be reloaded")
		return
	}
	writeJSON(w, http.StatusOK, template)
}

func (s *AppState) RenderMapTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mapTemplateIDFromRequest(w, r)
	if !ok {
		return
	}
	var req models.RenderMapTemplateRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	template, found, err := LoadMapTemplateRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "map template not found")
		return
	}
	writeJSON(w, http.StatusOK, RenderMapTemplate(template, req))
}

func mapTemplateIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid map template id")
		return uuid.UUID{}, false
	}
	return id, true
}

func validateCreateMapTemplateRequest(req models.CreateMapTemplateRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("map template name is required")
	}
	if len(req.Layers) == 0 && len(req.OverlayLayers) == 0 {
		return errors.New("map template requires at least one layer or overlay")
	}
	for _, layer := range req.Layers {
		if !layer.Mode.Valid() {
			return fmt.Errorf("unsupported map template layer mode %q", layer.Mode)
		}
	}
	return nil
}

func RenderMapTemplate(def models.MapTemplateDefinition, req models.RenderMapTemplateRequest) models.RenderMapTemplateResponse {
	substitutions := mapTemplateParameterSubstitutions(def.Parameters, req.ParameterValues)
	parametersByID := mapTemplateParametersByID(def.Parameters)
	props := map[string]any{
		"map_template_id":   def.ID.String(),
		"map_template_name": def.Name,
		"base_layer_kind":   firstNonEmpty(def.Viewport.BaseLayer, "blank"),
		"show_legend":       true,
	}
	if def.Viewport.CenterLat != nil {
		props["center_lat"] = *def.Viewport.CenterLat
	}
	if def.Viewport.CenterLon != nil {
		props["center_lon"] = *def.Viewport.CenterLon
	}
	if def.Viewport.Zoom != nil {
		props["zoom"] = *def.Viewport.Zoom
	}
	if def.InterfaceOptions.ShowLegend != nil {
		props["show_legend"] = *def.InterfaceOptions.ShowLegend
	}
	if def.InterfaceOptions.FlyToObjects {
		props["fly_to_objects"] = true
	}
	if def.InterfaceOptions.SeriesPanelOpen {
		props["series_panel_open"] = true
	}
	if def.InterfaceOptions.WorkshopModuleLink {
		props["workshop_module_link"] = true
	}

	layers := make([]map[string]any, 0, len(def.Layers))
	for _, layer := range def.Layers {
		if normalizedTemplateLayerMode(layer.Mode) == models.MapTemplateLayerModeRemove {
			continue
		}
		layers = append(layers, renderTemplateLayer(layer, parametersByID, req.VariableMappings, substitutions))
	}
	overlays := make([]map[string]any, 0, len(def.OverlayLayers))
	for _, overlay := range def.OverlayLayers {
		if strings.EqualFold(strings.TrimSpace(overlay.Mode), "remove") {
			continue
		}
		overlays = append(overlays, renderTemplateOverlay(overlay, substitutions))
	}
	props["layers"] = layers
	props["overlay_layers"] = overlays
	props["template_parameter_values"] = decodeTemplateParameterValues(req.ParameterValues)
	props["template_variable_mappings"] = req.VariableMappings

	return models.RenderMapTemplateResponse{
		TemplateID:   def.ID,
		TemplateName: def.Name,
		Parameters:   def.Parameters,
		WidgetProps:  props,
	}
}

func renderTemplateLayer(
	layer models.MapTemplateLayer,
	parameters map[string]models.MapTemplateParameter,
	variableMappings map[string]string,
	substitutions map[string]string,
) map[string]any {
	out := renderRawConfig(layer.Config, substitutions)
	renderRawConfigInto(out, layer.Style, substitutions)
	setDefault(out, "id", firstNonEmpty(layer.ID, "template-layer"))
	setDefault(out, "title", firstNonEmpty(layer.Title, layer.ID, "Template layer"))
	setDefault(out, "visible", true)
	setDefault(out, "template_layer_mode", string(normalizedTemplateLayerMode(layer.Mode)))
	setDefault(out, "geometry_type", firstNonEmpty(layer.GeometryType, "auto"))
	if layer.ObjectTypeID != "" {
		setDefault(out, "object_type_id", layer.ObjectTypeID)
	}
	if layer.Source != "" {
		setDefault(out, "source", layer.Source)
	}
	if layer.SourceVariableID != "" {
		setDefault(out, "source_variable_id", layer.SourceVariableID)
	}
	if layer.TileLayerID != "" {
		setDefault(out, "tile_layer_id", layer.TileLayerID)
		setDefault(out, "source", "geospatial_tile")
	}
	if len(layer.Features) > 0 {
		out["features"] = layer.Features
		setDefault(out, "source", "binding")
	}
	if normalizedTemplateLayerMode(layer.Mode) == models.MapTemplateLayerModeStyling {
		parameter := parameters[layer.ObjectParameterID]
		mappedVariable := templateVariableMapping(layer.ObjectParameterID, parameter, variableMappings)
		if mappedVariable != "" {
			out["source"] = "object_set"
			out["source_variable_id"] = mappedVariable
		} else if firstNonEmpty(layer.ObjectTypeID, parameter.ObjectTypeID) != "" {
			out["source"] = "object_type"
			out["object_type_id"] = firstNonEmpty(layer.ObjectTypeID, parameter.ObjectTypeID)
		}
	}
	setDefault(out, "source", "binding")
	return out
}

func renderTemplateOverlay(overlay models.MapTemplateOverlayLayer, substitutions map[string]string) map[string]any {
	out := renderRawConfig(overlay.Config, substitutions)
	setDefault(out, "id", firstNonEmpty(overlay.ID, "template-overlay"))
	setDefault(out, "title", firstNonEmpty(overlay.Title, overlay.ID, "Template overlay"))
	setDefault(out, "visible", true)
	if overlay.Source != "" {
		setDefault(out, "source", overlay.Source)
	}
	if overlay.URL != "" {
		setDefault(out, "url", substituteTemplateString(overlay.URL, substitutions))
	}
	if overlay.ResourceID != "" {
		setDefault(out, "resource_id", substituteTemplateString(overlay.ResourceID, substitutions))
	}
	setDefault(out, "source", "geojson_url")
	return out
}

func normalizedTemplateLayerMode(mode models.MapTemplateLayerMode) models.MapTemplateLayerMode {
	if mode == "" {
		return models.MapTemplateLayerModeConstant
	}
	return mode
}

func mapTemplateParametersByID(parameters []models.MapTemplateParameter) map[string]models.MapTemplateParameter {
	out := map[string]models.MapTemplateParameter{}
	for _, parameter := range parameters {
		if parameter.ID != "" {
			out[parameter.ID] = parameter
		}
		if parameter.Name != "" {
			out[parameter.Name] = parameter
		}
	}
	return out
}

func templateVariableMapping(parameterID string, parameter models.MapTemplateParameter, mappings map[string]string) string {
	for _, key := range []string{parameterID, parameter.ID, parameter.Name} {
		if value := strings.TrimSpace(mappings[key]); value != "" {
			return value
		}
	}
	return ""
}

func mapTemplateParameterSubstitutions(parameters []models.MapTemplateParameter, values map[string]json.RawMessage) map[string]string {
	out := map[string]string{}
	for _, parameter := range parameters {
		raw := values[parameter.ID]
		if len(raw) == 0 && parameter.Name != "" {
			raw = values[parameter.Name]
		}
		if len(raw) == 0 {
			raw = parameter.DefaultValue
		}
		value := rawTemplateValueToString(raw)
		if parameter.ID != "" {
			out[parameter.ID] = value
		}
		if parameter.Name != "" {
			out[parameter.Name] = value
		}
	}
	return out
}

func decodeTemplateParameterValues(values map[string]json.RawMessage) map[string]any {
	out := map[string]any{}
	for key, raw := range values {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err == nil {
			out[key] = decoded
			continue
		}
		out[key] = string(raw)
	}
	return out
}

func renderRawConfig(raw map[string]json.RawMessage, substitutions map[string]string) map[string]any {
	out := map[string]any{}
	renderRawConfigInto(out, raw, substitutions)
	return out
}

func renderRawConfigInto(out map[string]any, raw map[string]json.RawMessage, substitutions map[string]string) {
	for key, value := range raw {
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			continue
		}
		out[key] = substituteTemplateValue(decoded, substitutions)
	}
}

func substituteTemplateValue(value any, substitutions map[string]string) any {
	switch typed := value.(type) {
	case string:
		return substituteTemplateString(typed, substitutions)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = substituteTemplateValue(item, substitutions)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, item := range typed {
			out[key] = substituteTemplateValue(item, substitutions)
		}
		return out
	default:
		return value
	}
}

func substituteTemplateString(value string, substitutions map[string]string) string {
	out := value
	for key, replacement := range substitutions {
		out = strings.ReplaceAll(out, "{{"+key+"}}", replacement)
		out = strings.ReplaceAll(out, "{{ "+key+" }}", replacement)
	}
	return out
}

func rawTemplateValueToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	switch value := decoded.(type) {
	case nil:
		return ""
	case float64, bool:
		return fmt.Sprint(value)
	default:
		return strings.Trim(string(raw), `"`)
	}
}

func setDefault(out map[string]any, key string, value any) {
	if _, ok := out[key]; ok {
		return
	}
	if str, ok := value.(string); ok && strings.TrimSpace(str) == "" {
		return
	}
	out[key] = value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

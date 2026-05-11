package geospatial

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

func TestRenderMapTemplateBuildsWorkshopWidgetProps(t *testing.T) {
	t.Parallel()
	zoom := 11.5
	centerLat := 40.016
	centerLon := -105.29
	showLegend := true
	templateID := uuid.New()

	response := RenderMapTemplate(models.MapTemplateDefinition{
		ID:          templateID,
		Name:        "Trail run template",
		Description: "Trailheads, routes, and coffee overlays.",
		Parameters: []models.MapTemplateParameter{
			{ID: "trail_set", Name: "trailSet", Kind: "object_set", ObjectTypeID: "Trail"},
			{ID: "radius", Name: "radius", Kind: "double", DefaultValue: json.RawMessage(`"2.5"`)},
		},
		Viewport: models.MapTemplateViewport{CenterLat: &centerLat, CenterLon: &centerLon, Zoom: &zoom, BaseLayer: "blank"},
		InterfaceOptions: models.MapTemplateInterfaceOptions{
			FlyToObjects: true,
			ShowLegend:   &showLegend,
		},
		Layers: []models.MapTemplateLayer{
			{
				ID:                "trail-style",
				Title:             "Trail starts",
				Mode:              models.MapTemplateLayerModeStyling,
				ObjectParameterID: "trail_set",
				GeometryType:      "point",
				Config: map[string]json.RawMessage{
					"color":        json.RawMessage(`"#16a34a"`),
					"filter_field": json.RawMessage(`"distance_miles"`),
					"filter_value": json.RawMessage(`"{{radius}}"`),
				},
			},
			{
				ID:           "coffee-constant",
				Title:        "Coffee",
				Mode:         models.MapTemplateLayerModeConstant,
				GeometryType: "point",
				Config: map[string]json.RawMessage{
					"color": json.RawMessage(`"#92400e"`),
				},
				Features: []models.MapFeature{{
					ID:       "coffee-1",
					Label:    "Trailhead Coffee",
					Geometry: models.Geometry{Type: models.GeometryTypePoint, Point: &models.Coordinate{Lat: 40.0161, Lon: -105.3441}},
				}},
			},
		},
		OverlayLayers: []models.MapTemplateOverlayLayer{{
			ID:     "open-space",
			Title:  "Open space",
			Source: "geojson_url",
			URL:    "/tiles/{{radius}}/open-space.geojson",
			Config: map[string]json.RawMessage{
				"geometry_type": json.RawMessage(`"polygon"`),
			},
		}},
	}, models.RenderMapTemplateRequest{
		ParameterValues: map[string]json.RawMessage{"radius": json.RawMessage(`"4.0"`)},
		VariableMappings: map[string]string{
			"trail_set": "var-trails",
		},
	})

	require.Equal(t, templateID, response.TemplateID)
	require.Equal(t, "Trail run template", response.TemplateName)
	require.Equal(t, 40.016, response.WidgetProps["center_lat"])
	require.Equal(t, -105.29, response.WidgetProps["center_lon"])
	require.Equal(t, true, response.WidgetProps["fly_to_objects"])

	layers, ok := response.WidgetProps["layers"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, layers, 2)
	require.Equal(t, "object_set", layers[0]["source"])
	require.Equal(t, "var-trails", layers[0]["source_variable_id"])
	require.Equal(t, "4.0", layers[0]["filter_value"])
	require.Equal(t, "binding", layers[1]["source"])
	require.Len(t, layers[1]["features"], 1)

	overlays, ok := response.WidgetProps["overlay_layers"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, overlays, 1)
	require.Equal(t, "/tiles/4.0/open-space.geojson", overlays[0]["url"])
}

func TestRenderMapTemplateAPI(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	templateID := uuid.New()
	addMapTemplateExpectation(t, mock, templateID)

	state := &AppState{DB: mock}
	req := httptest.NewRequest(http.MethodPost, "/templates/"+templateID.String()+"/render", bytes.NewReader([]byte(`{
		"parameter_values":{"radius":"1.5"},
		"variable_mappings":{"trail_set":"var-trails"}
	}`)))
	rr := httptest.NewRecorder()
	state.Routes().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var response models.RenderMapTemplateResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.Equal(t, templateID, response.TemplateID)
	layers, ok := response.WidgetProps["layers"].([]any)
	require.True(t, ok)
	require.Len(t, layers, 1)
	layer, ok := layers[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object_set", layer["source"])
	require.Equal(t, "var-trails", layer["source_variable_id"])
	require.Equal(t, "1.5", layer["filter_value"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func addMapTemplateExpectation(t *testing.T, mock pgxmock.PgxPoolIface, templateID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	parameters, err := json.Marshal([]models.MapTemplateParameter{
		{ID: "trail_set", Name: "trailSet", Kind: "object_set", ObjectTypeID: "Trail"},
		{ID: "radius", Name: "radius", Kind: "double", DefaultValue: json.RawMessage(`"2.5"`)},
	})
	require.NoError(t, err)
	layers, err := json.Marshal([]models.MapTemplateLayer{{
		ID:                "trail-style",
		Title:             "Trail starts",
		Mode:              models.MapTemplateLayerModeStyling,
		ObjectParameterID: "trail_set",
		GeometryType:      "point",
		Config: map[string]json.RawMessage{
			"filter_value": json.RawMessage(`"{{radius}}"`),
		},
	}})
	require.NoError(t, err)
	overlays, err := json.Marshal([]models.MapTemplateOverlayLayer{})
	require.NoError(t, err)
	viewport, err := json.Marshal(models.MapTemplateViewport{})
	require.NoError(t, err)
	interfaceOptions, err := json.Marshal(models.MapTemplateInterfaceOptions{})
	require.NoError(t, err)
	tags, err := json.Marshal([]string{"trail"})
	require.NoError(t, err)

	rows := pgxmock.NewRows([]string{
		"id", "name", "description", "parameters", "layers", "overlay_layers",
		"viewport", "interface_options", "tags", "created_at", "updated_at",
	}).AddRow(templateID, "Trail template", "Trail map", parameters, layers, overlays, viewport, interfaceOptions, tags, now, now)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description, parameters, layers, overlay_layers, viewport, interface_options, tags, created_at, updated_at FROM geospatial_map_templates WHERE id = $1")).
		WithArgs(templateID).
		WillReturnRows(rows)
}

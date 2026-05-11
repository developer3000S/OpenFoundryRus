package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAppContractCarriesWorkshopSchema(t *testing.T) {
	t.Parallel()

	pages := json.RawMessage(`[
		{
			"id": "main",
			"name": "Main",
			"path": "",
			"layout": {"kind": "columns", "columns": 16},
			"widgets": [
				{
					"id": "trail-map",
					"widget_type": "map",
					"title": "Trail map",
					"position": {"x": 0, "y": 0, "width": 0, "height": 0},
					"props": {"map_template_id": "template-trails"},
					"binding": {"source_type": "object_set", "source_id": "trail-set"},
					"events": [
						{"id": "select-trail", "trigger": "select", "action": "set_variable"}
					],
					"actions": [
						{"id": "open-detail", "kind": "navigate", "config": {"page_id": "detail"}}
					],
					"children": []
				}
			],
			"sections": [
				{
					"id": "summary-section",
					"layout": {"kind": "toolbar"},
					"widgets": [],
					"sections": []
				}
			],
			"visible": true
		}
	]`)
	settings := json.RawMessage(`{
		"workshop_variables": [
			{"id": "trail-set", "kind": "object_set", "name": "Trails", "object_type_id": "Trail"}
		]
	}`)

	contract, err := NormalizeAppContract("Trail Demo", "trail-demo", "draft", pages, json.RawMessage(`{"name":"Trail"}`), settings)
	require.NoError(t, err)

	var gotPages []AppPage
	require.NoError(t, json.Unmarshal(contract.Pages, &gotPages))
	require.Equal(t, "/", gotPages[0].Path)
	require.Equal(t, "columns", gotPages[0].Layout.Kind)
	require.Equal(t, 16, gotPages[0].Layout.Columns)
	require.Equal(t, 12, gotPages[0].Widgets[0].Position.Width)
	require.Equal(t, 2, gotPages[0].Widgets[0].Position.Height)
	require.Equal(t, []string{}, gotPages[0].Widgets[0].Binding.Fields)
	require.Equal(t, map[string]any{}, gotPages[0].Widgets[0].Events[0].Config)
	require.Equal(t, "toolbar", gotPages[0].Sections[0].Layout.Kind)

	var gotSettings map[string]any
	require.NoError(t, json.Unmarshal(contract.Settings, &gotSettings))
	require.Equal(t, WorkshopAppSchemaVersion, gotSettings["schema_version"])
	metadata, ok := gotSettings["runtime_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "trail-demo", metadata["public_slug"])
	require.Equal(t, "workshop", metadata["runtime_mode"])
	require.Equal(t, "main", metadata["home_page_id"])
	require.Len(t, gotSettings["workshop_variables"], 1)
}

func TestNormalizeAppContractDefaultsRenderableHomePage(t *testing.T) {
	t.Parallel()

	contract, err := NormalizeAppContract("Empty Demo", "empty-demo", "", nil, nil, nil)
	require.NoError(t, err)

	var pages []AppPage
	require.NoError(t, json.Unmarshal(contract.Pages, &pages))
	require.Len(t, pages, 1)
	require.Equal(t, "main", pages[0].ID)
	require.Equal(t, "/", pages[0].Path)
	require.True(t, pages[0].Visible)
	require.Empty(t, pages[0].Widgets)
}

func TestValidateAppContractRejectsInvalidReferences(t *testing.T) {
	t.Parallel()

	err := ValidateAppContract("Bad", "Bad Slug", "draft", nil, nil, nil)
	require.ErrorContains(t, err, "must use lowercase")

	err = ValidateAppContract("Bad", "bad", "draft", json.RawMessage(`[
		{"id":"main","name":"Main","path":"/","layout":{"kind":"grid"},"visible":true,"widgets":[{"id":"w","widget_type":"text","position":{"width":1,"height":1},"events":[],"children":[]}]},
		{"id":"other","name":"Other","path":"/other","layout":{"kind":"grid"},"visible":true,"widgets":[{"id":"w","widget_type":"text","position":{"width":1,"height":1},"events":[],"children":[]}]}
	]`), nil, nil)
	require.ErrorContains(t, err, "duplicate widget id")

	err = ValidateAppContract("Bad", "bad", "draft", json.RawMessage(`[
		{"id":"main","name":"Main","path":"/","layout":{"kind":"grid"},"visible":true,"widgets":[{"id":"w","widget_type":"","position":{"width":1,"height":1},"events":[],"children":[]}]}
	]`), nil, nil)
	require.ErrorContains(t, err, "widget_type is required")
}

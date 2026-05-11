package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type widgetCatalogSnapshot struct {
	CatalogVersion string                      `json:"catalog_version"`
	SchemaVersion  string                      `json:"schema_version"`
	Items          []widgetCatalogSnapshotItem `json:"items"`
}

type widgetCatalogSnapshotItem struct {
	WidgetType        string   `json:"widget_type"`
	WidgetKind        string   `json:"widget_kind"`
	Category          string   `json:"category"`
	DisplayIcon       string   `json:"display_icon"`
	InputCount        int      `json:"input_count"`
	OutputCount       int      `json:"output_count"`
	EventNames        []string `json:"event_names"`
	Permissions       []string `json:"permissions"`
	SupportedBindings []string `json:"supported_bindings"`
	SupportsChildren  bool     `json:"supports_children"`
}

func TestLoadWidgetCatalogValidatesDataDrivenContract(t *testing.T) {
	doc, err := LoadWidgetCatalog()
	require.NoError(t, err)
	require.Equal(t, "2026-05-11.ws.3", doc.CatalogVersion)
	require.Equal(t, WidgetCatalogSchemaVersion, doc.SchemaVersion)
	require.Len(t, doc.Items, 16)

	seen := map[string]bool{}
	for _, item := range doc.Items {
		require.NotEmpty(t, item.CatalogVersion, item.WidgetType)
		require.NotEmpty(t, item.SchemaVersion, item.WidgetType)
		require.NotEmpty(t, item.WidgetType)
		require.False(t, seen[item.WidgetType], "duplicate widget type %s", item.WidgetType)
		seen[item.WidgetType] = true
		require.NotEmpty(t, item.WidgetKind, item.WidgetType)
		require.NotEmpty(t, item.Category, item.WidgetType)
		require.NotEmpty(t, item.ConfigSchema, item.WidgetType)
		require.True(t, json.Valid(item.ConfigSchema), item.WidgetType)
		require.True(t, json.Valid(item.DefaultProps), item.WidgetType)
		require.NotNil(t, item.InputVariables, item.WidgetType)
		require.NotNil(t, item.OutputVariables, item.WidgetType)
		require.NotNil(t, item.Events, item.WidgetType)
		require.NotNil(t, item.Permissions, item.WidgetType)
		require.NotEmpty(t, item.Display.Icon, item.WidgetType)
		require.NotEmpty(t, item.DefaultProps, item.WidgetType)
		require.Positive(t, item.DefaultSize.Width, item.WidgetType)
		require.Positive(t, item.DefaultSize.Height, item.WidgetType)
	}
	require.True(t, seen["map"])
	require.True(t, seen["object_table"])
	require.True(t, seen["button_group"])
}

func TestWidgetCatalogSnapshot(t *testing.T) {
	doc, err := LoadWidgetCatalog()
	require.NoError(t, err)

	actual, err := json.MarshalIndent(snapshotWidgetCatalog(doc), "", "  ")
	require.NoError(t, err)
	actual = append(actual, '\n')

	expected, err := os.ReadFile(filepath.Join("testdata", "widget_catalog_snapshot.json"))
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
}

func snapshotWidgetCatalog(doc WidgetCatalogDocument) widgetCatalogSnapshot {
	out := widgetCatalogSnapshot{
		CatalogVersion: doc.CatalogVersion,
		SchemaVersion:  doc.SchemaVersion,
		Items:          make([]widgetCatalogSnapshotItem, 0, len(doc.Items)),
	}
	for _, item := range doc.Items {
		eventNames := make([]string, 0, len(item.Events))
		for _, event := range item.Events {
			eventNames = append(eventNames, event.Name)
		}
		out.Items = append(out.Items, widgetCatalogSnapshotItem{
			WidgetType:        item.WidgetType,
			WidgetKind:        item.WidgetKind,
			Category:          item.Category,
			DisplayIcon:       item.Display.Icon,
			InputCount:        len(item.InputVariables),
			OutputCount:       len(item.OutputVariables),
			EventNames:        eventNames,
			Permissions:       item.Permissions,
			SupportedBindings: item.SupportedBindings,
			SupportsChildren:  item.SupportsChildren,
		})
	}
	return out
}

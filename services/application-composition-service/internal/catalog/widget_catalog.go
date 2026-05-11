package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	_ "embed"

	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/models"
)

const (
	WidgetCatalogSchemaVersion = "openfoundry.workshop.widget_catalog.v1"
)

//go:embed widget_catalog.v1.json
var widgetCatalogBytes []byte

type WidgetCatalogDocument struct {
	CatalogVersion string                     `json:"catalog_version"`
	SchemaVersion  string                     `json:"schema_version"`
	Items          []models.WidgetCatalogItem `json:"items"`
}

func LoadWidgetCatalog() (WidgetCatalogDocument, error) {
	return ParseWidgetCatalog(widgetCatalogBytes)
}

func ParseWidgetCatalog(data []byte) (WidgetCatalogDocument, error) {
	var doc WidgetCatalogDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return WidgetCatalogDocument{}, fmt.Errorf("decode widget catalog: %w", err)
	}
	if err := validateWidgetCatalog(&doc); err != nil {
		return WidgetCatalogDocument{}, err
	}
	return doc, nil
}

func validateWidgetCatalog(doc *WidgetCatalogDocument) error {
	if doc.CatalogVersion == "" {
		return errors.New("widget catalog version is required")
	}
	if doc.SchemaVersion != WidgetCatalogSchemaVersion {
		return fmt.Errorf("unsupported widget catalog schema version %q", doc.SchemaVersion)
	}
	if len(doc.Items) == 0 {
		return errors.New("widget catalog must include at least one item")
	}
	seen := make(map[string]struct{}, len(doc.Items))
	for idx := range doc.Items {
		item := &doc.Items[idx]
		item.CatalogVersion = doc.CatalogVersion
		item.SchemaVersion = doc.SchemaVersion
		if item.WidgetType == "" {
			return fmt.Errorf("widget catalog item %d missing widget_type", idx)
		}
		if _, ok := seen[item.WidgetType]; ok {
			return fmt.Errorf("duplicate widget catalog item %q", item.WidgetType)
		}
		seen[item.WidgetType] = struct{}{}
		if item.WidgetKind == "" {
			return fmt.Errorf("widget catalog item %q missing widget_kind", item.WidgetType)
		}
		if item.Label == "" || item.Description == "" || item.Category == "" {
			return fmt.Errorf("widget catalog item %q missing label, description, or category", item.WidgetType)
		}
		if item.DefaultSize.Width <= 0 || item.DefaultSize.Height <= 0 {
			return fmt.Errorf("widget catalog item %q must declare positive default_size", item.WidgetType)
		}
		if len(item.DefaultProps) == 0 || !json.Valid(item.DefaultProps) {
			return fmt.Errorf("widget catalog item %q has invalid default_props", item.WidgetType)
		}
		if len(item.ConfigSchema) == 0 || !json.Valid(item.ConfigSchema) {
			return fmt.Errorf("widget catalog item %q has invalid config_schema", item.WidgetType)
		}
		if item.InputVariables == nil {
			item.InputVariables = []models.WidgetCatalogVariable{}
		}
		if item.OutputVariables == nil {
			item.OutputVariables = []models.WidgetCatalogVariable{}
		}
		if item.Events == nil {
			item.Events = []models.WidgetCatalogEvent{}
		}
		if item.Permissions == nil {
			item.Permissions = []string{}
		}
		if item.SupportedBindings == nil {
			item.SupportedBindings = []string{}
		}
		if item.Display.Icon == "" {
			return fmt.Errorf("widget catalog item %q missing display.icon", item.WidgetType)
		}
		if item.Display.Tags == nil {
			item.Display.Tags = []string{}
		}
	}
	return nil
}

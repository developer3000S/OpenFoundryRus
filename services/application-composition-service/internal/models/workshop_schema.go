package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const WorkshopAppSchemaVersion = "2026-05-11.ws.1"

var appSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type AppContract struct {
	Pages    json.RawMessage
	Theme    json.RawMessage
	Settings json.RawMessage
}

type AppRuntimeMetadata struct {
	SchemaVersion string `json:"schema_version"`
	PublicSlug    string `json:"public_slug"`
	RuntimeMode   string `json:"runtime_mode"`
	Status        string `json:"status"`
	HomePageID    string `json:"home_page_id,omitempty"`
}

type WorkshopVariable struct {
	ID               string           `json:"id"`
	Kind             string           `json:"kind"`
	Name             string           `json:"name"`
	ObjectTypeID     string           `json:"object_type_id,omitempty"`
	SourceWidgetID   string           `json:"source_widget_id,omitempty"`
	SourceVariableID string           `json:"source_variable_id,omitempty"`
	FilterVariableID string           `json:"filter_variable_id,omitempty"`
	StaticFilter     map[string]any   `json:"static_filter,omitempty"`
	StaticFilters    []map[string]any `json:"static_filters,omitempty"`
	DefaultValue     json.RawMessage  `json:"default_value,omitempty"`
	Metadata         map[string]any   `json:"metadata,omitempty"`
}

type PageLayout struct {
	Kind       string `json:"kind"`
	Columns    int    `json:"columns,omitempty"`
	Gap        string `json:"gap,omitempty"`
	MaxWidth   string `json:"max_width,omitempty"`
	Direction  string `json:"direction,omitempty"`
	Scrollable bool   `json:"scrollable,omitempty"`
}

type AppPage struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Path            string         `json:"path"`
	Description     string         `json:"description"`
	Layout          PageLayout     `json:"layout"`
	Widgets         []AppWidget    `json:"widgets"`
	Sections        []AppSection   `json:"sections,omitempty"`
	Overlays        []AppOverlay   `json:"overlays,omitempty"`
	Visible         bool           `json:"visible"`
	RuntimeMetadata map[string]any `json:"runtime_metadata,omitempty"`
}

type AppSection struct {
	ID          string           `json:"id"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Layout      PageLayout       `json:"layout"`
	Widgets     []AppWidget      `json:"widgets,omitempty"`
	Sections    []AppSection     `json:"sections,omitempty"`
	Visible     *bool            `json:"visible,omitempty"`
	Props       map[string]any   `json:"props,omitempty"`
	Events      []WidgetEvent    `json:"events,omitempty"`
	Actions     []WorkshopAction `json:"actions,omitempty"`
}

type AppOverlay struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	OverlayType     string           `json:"overlay_type"`
	VisibleVariable string           `json:"visible_variable_id,omitempty"`
	Layout          PageLayout       `json:"layout"`
	Sections        []AppSection     `json:"sections,omitempty"`
	Widgets         []AppWidget      `json:"widgets,omitempty"`
	Props           map[string]any   `json:"props,omitempty"`
	Events          []WidgetEvent    `json:"events,omitempty"`
	Actions         []WorkshopAction `json:"actions,omitempty"`
}

type AppWidgetPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type WidgetBinding struct {
	SourceType string         `json:"source_type"`
	SourceID   string         `json:"source_id,omitempty"`
	QueryText  string         `json:"query_text,omitempty"`
	Path       string         `json:"path,omitempty"`
	Fields     []string       `json:"fields"`
	Parameters map[string]any `json:"parameters"`
	Limit      *int           `json:"limit,omitempty"`
}

type WidgetEvent struct {
	ID      string         `json:"id"`
	Trigger string         `json:"trigger"`
	Action  string         `json:"action"`
	Label   string         `json:"label,omitempty"`
	Config  map[string]any `json:"config"`
}

type WorkshopAction struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	ActionTypeID string         `json:"action_type_id,omitempty"`
	Label        string         `json:"label,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
}

type AppWidget struct {
	ID              string            `json:"id"`
	WidgetType      string            `json:"widget_type"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Position        AppWidgetPosition `json:"position"`
	Props           map[string]any    `json:"props"`
	Config          map[string]any    `json:"config,omitempty"`
	Binding         *WidgetBinding    `json:"binding,omitempty"`
	Bindings        []WidgetBinding   `json:"bindings,omitempty"`
	Events          []WidgetEvent     `json:"events"`
	Actions         []WorkshopAction  `json:"actions,omitempty"`
	Children        []AppWidget       `json:"children"`
	RuntimeMetadata map[string]any    `json:"runtime_metadata,omitempty"`
}

func NormalizeAppContract(name, slug, status string, pagesRaw, themeRaw, settingsRaw json.RawMessage) (AppContract, error) {
	slug = strings.TrimSpace(slug)
	if !appSlugPattern.MatchString(slug) {
		return AppContract{}, fmt.Errorf("slug %q must use lowercase letters, numbers, and hyphens", slug)
	}
	status = defaultString(status, "draft")

	pages, err := normalizePages(pagesRaw)
	if err != nil {
		return AppContract{}, err
	}
	theme, err := normalizeJSONObject(themeRaw, map[string]any{})
	if err != nil {
		return AppContract{}, fmt.Errorf("theme must be a JSON object: %w", err)
	}
	settings, err := normalizeSettings(settingsRaw, pages, slug, status)
	if err != nil {
		return AppContract{}, err
	}

	return AppContract{
		Pages:    mustMarshalRaw(pages),
		Theme:    mustMarshalRaw(theme),
		Settings: mustMarshalRaw(settings),
	}, nil
}

func ValidateAppContract(name, slug, status string, pagesRaw, themeRaw, settingsRaw json.RawMessage) error {
	_, err := NormalizeAppContract(name, slug, status, pagesRaw, themeRaw, settingsRaw)
	return err
}

func normalizePages(raw json.RawMessage) ([]AppPage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []AppPage{defaultHomePage()}, nil
	}
	var rawPages []json.RawMessage
	if err := json.Unmarshal(raw, &rawPages); err != nil {
		return nil, fmt.Errorf("pages must be a JSON array: %w", err)
	}
	pages := make([]AppPage, 0, len(rawPages))
	for i, rawPage := range rawPages {
		var page AppPage
		if err := json.Unmarshal(rawPage, &page); err != nil {
			return nil, fmt.Errorf("pages[%d] must be an object: %w", i, err)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(rawPage, &obj); err == nil {
			if _, ok := obj["visible"]; !ok {
				page.Visible = true
			}
		}
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		pages = []AppPage{defaultHomePage()}
	}

	seenPages := map[string]bool{}
	seenWidgets := map[string]bool{}
	for i := range pages {
		if err := normalizePage(&pages[i], i, seenPages, seenWidgets); err != nil {
			return nil, err
		}
	}
	return pages, nil
}

func normalizePage(page *AppPage, index int, seenPages, seenWidgets map[string]bool) error {
	page.ID = strings.TrimSpace(page.ID)
	if page.ID == "" {
		return fmt.Errorf("pages[%d].id is required", index)
	}
	if seenPages[page.ID] {
		return fmt.Errorf("duplicate page id %q", page.ID)
	}
	seenPages[page.ID] = true
	page.Name = defaultString(page.Name, page.ID)
	page.Path = normalizePagePath(page.Path, index)
	page.Layout = normalizeLayout(page.Layout)
	for i := range page.Widgets {
		if err := normalizeWidget(&page.Widgets[i], fmt.Sprintf("pages[%d].widgets[%d]", index, i), seenWidgets); err != nil {
			return err
		}
	}
	for i := range page.Sections {
		if err := normalizeSection(&page.Sections[i], fmt.Sprintf("pages[%d].sections[%d]", index, i), seenWidgets); err != nil {
			return err
		}
	}
	for i := range page.Overlays {
		if err := normalizeOverlay(&page.Overlays[i], fmt.Sprintf("pages[%d].overlays[%d]", index, i), seenWidgets); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSection(section *AppSection, path string, seenWidgets map[string]bool) error {
	section.ID = strings.TrimSpace(section.ID)
	if section.ID == "" {
		return fmt.Errorf("%s.id is required", path)
	}
	section.Layout = normalizeLayout(section.Layout)
	if section.Props == nil {
		section.Props = map[string]any{}
	}
	if err := normalizeEvents(section.Events, path+".events"); err != nil {
		return err
	}
	if err := normalizeActions(section.Actions, path+".actions"); err != nil {
		return err
	}
	for i := range section.Widgets {
		if err := normalizeWidget(&section.Widgets[i], fmt.Sprintf("%s.widgets[%d]", path, i), seenWidgets); err != nil {
			return err
		}
	}
	for i := range section.Sections {
		if err := normalizeSection(&section.Sections[i], fmt.Sprintf("%s.sections[%d]", path, i), seenWidgets); err != nil {
			return err
		}
	}
	return nil
}

func normalizeOverlay(overlay *AppOverlay, path string, seenWidgets map[string]bool) error {
	overlay.ID = strings.TrimSpace(overlay.ID)
	if overlay.ID == "" {
		return fmt.Errorf("%s.id is required", path)
	}
	overlay.Name = defaultString(overlay.Name, overlay.ID)
	overlay.OverlayType = defaultString(overlay.OverlayType, "drawer")
	if overlay.OverlayType != "drawer" && overlay.OverlayType != "modal" {
		return fmt.Errorf("%s.overlay_type must be drawer or modal", path)
	}
	overlay.Layout = normalizeLayout(overlay.Layout)
	if overlay.Props == nil {
		overlay.Props = map[string]any{}
	}
	if err := normalizeEvents(overlay.Events, path+".events"); err != nil {
		return err
	}
	if err := normalizeActions(overlay.Actions, path+".actions"); err != nil {
		return err
	}
	for i := range overlay.Widgets {
		if err := normalizeWidget(&overlay.Widgets[i], fmt.Sprintf("%s.widgets[%d]", path, i), seenWidgets); err != nil {
			return err
		}
	}
	for i := range overlay.Sections {
		if err := normalizeSection(&overlay.Sections[i], fmt.Sprintf("%s.sections[%d]", path, i), seenWidgets); err != nil {
			return err
		}
	}
	return nil
}

func normalizeWidget(widget *AppWidget, path string, seenWidgets map[string]bool) error {
	widget.ID = strings.TrimSpace(widget.ID)
	if widget.ID == "" {
		return fmt.Errorf("%s.id is required", path)
	}
	if seenWidgets[widget.ID] {
		return fmt.Errorf("duplicate widget id %q", widget.ID)
	}
	seenWidgets[widget.ID] = true
	widget.WidgetType = strings.TrimSpace(widget.WidgetType)
	if widget.WidgetType == "" {
		return fmt.Errorf("%s.widget_type is required", path)
	}
	widget.Title = defaultString(widget.Title, widget.WidgetType)
	if widget.Position.Width <= 0 {
		widget.Position.Width = 12
	}
	if widget.Position.Height <= 0 {
		widget.Position.Height = 2
	}
	if widget.Position.X < 0 || widget.Position.Y < 0 {
		return fmt.Errorf("%s.position cannot use negative x/y", path)
	}
	if widget.Props == nil {
		widget.Props = map[string]any{}
	}
	if widget.Config == nil {
		widget.Config = map[string]any{}
	}
	if widget.Binding != nil {
		if err := normalizeBinding(widget.Binding, path+".binding"); err != nil {
			return err
		}
	}
	for i := range widget.Bindings {
		if err := normalizeBinding(&widget.Bindings[i], fmt.Sprintf("%s.bindings[%d]", path, i)); err != nil {
			return err
		}
	}
	if err := normalizeEvents(widget.Events, path+".events"); err != nil {
		return err
	}
	if err := normalizeActions(widget.Actions, path+".actions"); err != nil {
		return err
	}
	for i := range widget.Children {
		if err := normalizeWidget(&widget.Children[i], fmt.Sprintf("%s.children[%d]", path, i), seenWidgets); err != nil {
			return err
		}
	}
	return nil
}

func normalizeBinding(binding *WidgetBinding, path string) error {
	binding.SourceType = strings.TrimSpace(binding.SourceType)
	if binding.SourceType == "" {
		return fmt.Errorf("%s.source_type is required", path)
	}
	switch binding.SourceType {
	case "query", "ontology", "object_set", "dataset", "variable", "function":
	default:
		return fmt.Errorf("%s.source_type %q is unsupported", path, binding.SourceType)
	}
	if binding.Fields == nil {
		binding.Fields = []string{}
	}
	if binding.Parameters == nil {
		binding.Parameters = map[string]any{}
	}
	return nil
}

func normalizeEvents(events []WidgetEvent, path string) error {
	seen := map[string]bool{}
	for i := range events {
		event := &events[i]
		event.ID = strings.TrimSpace(event.ID)
		if event.ID == "" {
			return fmt.Errorf("%s[%d].id is required", path, i)
		}
		if seen[event.ID] {
			return fmt.Errorf("duplicate event id %q", event.ID)
		}
		seen[event.ID] = true
		if strings.TrimSpace(event.Trigger) == "" {
			return fmt.Errorf("%s[%d].trigger is required", path, i)
		}
		if strings.TrimSpace(event.Action) == "" {
			return fmt.Errorf("%s[%d].action is required", path, i)
		}
		if event.Config == nil {
			event.Config = map[string]any{}
		}
	}
	return nil
}

func normalizeActions(actions []WorkshopAction, path string) error {
	seen := map[string]bool{}
	for i := range actions {
		action := &actions[i]
		action.ID = strings.TrimSpace(action.ID)
		if action.ID == "" {
			return fmt.Errorf("%s[%d].id is required", path, i)
		}
		if seen[action.ID] {
			return fmt.Errorf("duplicate action id %q", action.ID)
		}
		seen[action.ID] = true
		if strings.TrimSpace(action.Kind) == "" {
			return fmt.Errorf("%s[%d].kind is required", path, i)
		}
		if action.Config == nil {
			action.Config = map[string]any{}
		}
	}
	return nil
}

func normalizeLayout(layout PageLayout) PageLayout {
	layout.Kind = defaultString(layout.Kind, "grid")
	switch layout.Kind {
	case "grid", "columns", "rows", "tabs", "flow", "toolbar", "loop":
	default:
		layout.Kind = "grid"
	}
	if layout.Columns <= 0 {
		layout.Columns = 12
	}
	if layout.Columns > 24 {
		layout.Columns = 24
	}
	layout.Gap = defaultString(layout.Gap, "1rem")
	layout.MaxWidth = defaultString(layout.MaxWidth, "1280px")
	return layout
}

func normalizeSettings(raw json.RawMessage, pages []AppPage, slug, status string) (map[string]any, error) {
	settings, err := normalizeJSONObject(raw, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("settings must be a JSON object: %w", err)
	}
	homePageID := ""
	if len(pages) > 0 {
		homePageID = pages[0].ID
	}
	if current, ok := settings["home_page_id"].(string); ok && strings.TrimSpace(current) != "" {
		homePageID = current
	}
	settings["schema_version"] = WorkshopAppSchemaVersion
	settings["runtime_metadata"] = AppRuntimeMetadata{
		SchemaVersion: WorkshopAppSchemaVersion,
		PublicSlug:    slug,
		RuntimeMode:   "workshop",
		Status:        status,
		HomePageID:    homePageID,
	}
	if _, ok := settings["home_page_id"]; !ok && homePageID != "" {
		settings["home_page_id"] = homePageID
	}
	if _, ok := settings["navigation_style"]; !ok {
		settings["navigation_style"] = "tabs"
	}
	if _, ok := settings["max_width"]; !ok {
		settings["max_width"] = "1280px"
	}
	if _, ok := settings["show_branding"]; !ok {
		settings["show_branding"] = true
	}
	if _, ok := settings["workshop_variables"]; !ok {
		settings["workshop_variables"] = []WorkshopVariable{}
	}
	if err := validateWorkshopVariables(settings["workshop_variables"]); err != nil {
		return nil, err
	}
	return settings, nil
}

func validateWorkshopVariables(raw any) error {
	bytes, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var variables []WorkshopVariable
	if err := json.Unmarshal(bytes, &variables); err != nil {
		return fmt.Errorf("settings.workshop_variables must be an array: %w", err)
	}
	seen := map[string]bool{}
	for i, variable := range variables {
		if strings.TrimSpace(variable.ID) == "" {
			return fmt.Errorf("settings.workshop_variables[%d].id is required", i)
		}
		if seen[variable.ID] {
			return fmt.Errorf("duplicate workshop variable id %q", variable.ID)
		}
		seen[variable.ID] = true
		if strings.TrimSpace(variable.Kind) == "" {
			return fmt.Errorf("settings.workshop_variables[%d].kind is required", i)
		}
		if strings.TrimSpace(variable.Name) == "" {
			return fmt.Errorf("settings.workshop_variables[%d].name is required", i)
		}
	}
	return nil
}

func normalizeJSONObject(raw json.RawMessage, fallback map[string]any) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return fallback, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return fallback, nil
	}
	return obj, nil
}

func defaultHomePage() AppPage {
	return AppPage{
		ID:          "main",
		Name:        "Main",
		Path:        "/",
		Description: "",
		Layout:      normalizeLayout(PageLayout{}),
		Widgets:     []AppWidget{},
		Visible:     true,
	}
}

func normalizePagePath(path string, index int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		if index == 0 {
			return "/"
		}
		return fmt.Sprintf("/page-%d", index+1)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mustMarshalRaw(value any) json.RawMessage {
	bytes, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(bytes)
}

func BuildAppRuntimeMetadata(slug, status, homePageID string) AppRuntimeMetadata {
	return AppRuntimeMetadata{
		SchemaVersion: WorkshopAppSchemaVersion,
		PublicSlug:    slug,
		RuntimeMode:   "workshop",
		Status:        defaultString(status, "draft"),
		HomePageID:    homePageID,
	}
}

func HomePageIDFromPages(raw json.RawMessage) (string, error) {
	var pages []AppPage
	if err := json.Unmarshal(raw, &pages); err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "", errors.New("no pages")
	}
	return pages[0].ID, nil
}

package trailrunningdemo

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	geospatialcore "github.com/openfoundry/openfoundry-go/libs/geospatial-core"
)

type fixtureManifest struct {
	ID             string `json:"id"`
	ExpectedCounts struct {
		StravaActivities int `json:"strava_activities"`
		GPXTrails        int `json:"gpx_trails"`
		CoffeeShops      int `json:"coffee_shops"`
		WeatherSnapshots int `json:"weather_snapshots"`
	} `json:"expected_counts"`
	Files struct {
		StravaActivities string   `json:"strava_activities"`
		CoffeeShopsCSV   string   `json:"coffee_shops_csv"`
		CoffeeShopsJSON  string   `json:"coffee_shops_json"`
		WeatherResponse  string   `json:"weather_response"`
		GPXTrails        []string `json:"gpx_trails"`
	} `json:"files"`
}

type demoPipelineIR struct {
	Version string             `json:"ir_version"`
	Nodes   []demoPipelineNode `json:"nodes"`
	Outputs []struct {
		ID         string `json:"id"`
		OutputType string `json:"output_type"`
		ProducedBy string `json:"produced_by"`
	} `json:"outputs"`
}

type demoPipelineNode struct {
	ID            string              `json:"id"`
	TransformType string              `json:"transform_type"`
	DependsOn     []string            `json:"depends_on"`
	Config        json.RawMessage     `json:"config"`
	OutputSchema  *demoPipelineSchema `json:"output_schema"`
}

type demoPipelineSchema struct {
	Fields []struct {
		Name      string `json:"name"`
		FieldType string `json:"field_type"`
		Nullable  bool   `json:"nullable"`
	} `json:"fields"`
}

type workshopDemoApp struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Slug     string             `json:"slug"`
	Status   string             `json:"status"`
	Pages    []workshopDemoPage `json:"pages"`
	Settings struct {
		HomePageID        string                 `json:"home_page_id"`
		NavigationStyle   string                 `json:"navigation_style"`
		WorkshopVariables []workshopDemoVariable `json:"workshop_variables"`
	} `json:"settings"`
}

type workshopDemoPage struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Path    string               `json:"path"`
	Visible bool                 `json:"visible"`
	Widgets []workshopDemoWidget `json:"widgets"`
}

type workshopDemoWidget struct {
	ID         string         `json:"id"`
	WidgetType string         `json:"widget_type"`
	Title      string         `json:"title"`
	Props      map[string]any `json:"props"`
}

type workshopDemoVariable struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ObjectTypeID string `json:"object_type_id"`
	SourceWidget string `json:"source_widget_id"`
}

func TestTrailRunningFixturePack(t *testing.T) {
	root := "fixtures"
	manifest := readFixtureJSON[fixtureManifest](t, filepath.Join(root, "manifest.json"))
	if manifest.ID == "" {
		t.Fatal("manifest id is required")
	}

	var activityDoc struct {
		Activities []map[string]any `json:"activities"`
	}
	readFixtureJSONInto(t, filepath.Join(root, manifest.Files.StravaActivities), &activityDoc)
	if len(activityDoc.Activities) != manifest.ExpectedCounts.StravaActivities {
		t.Fatalf("activity count mismatch: got %d want %d", len(activityDoc.Activities), manifest.ExpectedCounts.StravaActivities)
	}
	hasRun, hasTrailRun, hasFilterCase := false, false, false
	for _, activity := range activityDoc.Activities {
		rejectPrivateFixtureKeys(t, activity)
		switch activity["type"] {
		case "Run":
			hasRun = true
		case "Trail Run":
			hasTrailRun = true
		default:
			hasFilterCase = true
		}
		if _, ok := activity["athlete"]; ok {
			t.Fatal("fixture must not contain athlete payloads")
		}
	}
	if !hasRun || !hasTrailRun || !hasFilterCase {
		t.Fatalf("activity fixture should contain Run, Trail Run, and non-run filter cases")
	}

	for _, rel := range manifest.Files.GPXTrails {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		trail, err := geospatialcore.ParseGPXTrail(body, geospatialcore.GPXParseOptions{SourceName: rel})
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		if trail.PointCount < 2 || trail.DistanceMiles <= 0 {
			t.Fatalf("invalid parsed trail metrics for %s: %+v", rel, trail)
		}
		if trail.RouteGeoJSON.Type != "LineString" {
			t.Fatalf("trail %s should parse to LineString, got %s", rel, trail.RouteGeoJSON.Type)
		}
		ontologyPoint, err := trail.TrailheadOntologyString()
		if err != nil {
			t.Fatalf("ontology point for %s: %v", rel, err)
		}
		if !strings.Contains(ontologyPoint, ",") {
			t.Fatalf("ontology point should use lat,lon format, got %q", ontologyPoint)
		}
	}
	if len(manifest.Files.GPXTrails) != manifest.ExpectedCounts.GPXTrails {
		t.Fatalf("gpx count mismatch: got %d want %d", len(manifest.Files.GPXTrails), manifest.ExpectedCounts.GPXTrails)
	}

	coffeeCSV := readCoffeeCSV(t, filepath.Join(root, manifest.Files.CoffeeShopsCSV))
	var coffeeJSON []map[string]any
	readFixtureJSONInto(t, filepath.Join(root, manifest.Files.CoffeeShopsJSON), &coffeeJSON)
	if len(coffeeCSV) != manifest.ExpectedCounts.CoffeeShops || len(coffeeJSON) != manifest.ExpectedCounts.CoffeeShops {
		t.Fatalf("coffee count mismatch: csv=%d json=%d want=%d", len(coffeeCSV), len(coffeeJSON), manifest.ExpectedCounts.CoffeeShops)
	}
	for _, row := range coffeeJSON {
		if row["source"] != "synthetic" {
			t.Fatalf("coffee fixture source must be synthetic: %+v", row)
		}
	}

	var weather map[string]any
	readFixtureJSONInto(t, filepath.Join(root, manifest.Files.WeatherResponse), &weather)
	current, ok := weather["current"].(map[string]any)
	if !ok {
		t.Fatal("weather fixture must contain current object")
	}
	for _, key := range []string{"temperature_2m", "relative_humidity_2m", "wind_speed_10m", "wind_direction_10m"} {
		if _, ok := current[key]; !ok {
			t.Fatalf("weather fixture missing %s", key)
		}
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the trail-running seed command")
	}
	outDir := t.TempDir()
	cmd := exec.Command(python, "seed.py", "--output", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, output)
	}
	seedManifest := readFixtureJSON[struct {
		Counts map[string]int `json:"counts"`
	}](t, filepath.Join(outDir, "manifest.json"))
	if seedManifest.Counts["trails"] != manifest.ExpectedCounts.GPXTrails {
		t.Fatalf("seeded trail count mismatch: %+v", seedManifest.Counts)
	}
	if seedManifest.Counts["trail_effort_estimates"] != manifest.ExpectedCounts.GPXTrails {
		t.Fatalf("seeded effort estimate count mismatch: %+v", seedManifest.Counts)
	}
	if seedManifest.Counts["trail_coffee_recommendations"] != manifest.ExpectedCounts.GPXTrails*3 {
		t.Fatalf("seeded coffee recommendation count mismatch: %+v", seedManifest.Counts)
	}
	if seedManifest.Counts["trail_coffee_links"] != manifest.ExpectedCounts.GPXTrails*3 {
		t.Fatalf("seeded coffee link count mismatch: %+v", seedManifest.Counts)
	}
	for _, name := range []string{"strava_activities.json", "run_activities.json", "trails.json", "trail_effort_estimates.json", "coffee_shops.csv", "coffee_shops.json", "trail_coffee_recommendations.json", "trail_coffee_links.json", "weather_snapshot.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("seed output missing %s: %v", name, err)
		}
	}
}

func TestStravaActivityIngestionPipelineFixture(t *testing.T) {
	pipeline := readFixtureJSON[demoPipelineIR](t, filepath.Join("pipelines", "strava_activity_ingestion.pipeline.json"))
	if pipeline.Version != "pipeline_ir.v1" {
		t.Fatalf("unexpected pipeline IR version: %s", pipeline.Version)
	}
	parseNode := pipelineNodeByID(t, pipeline, "parse_run_activities")
	if parseNode.TransformType != "python" {
		t.Fatalf("parse node should be python, got %s", parseNode.TransformType)
	}
	if !reflect.DeepEqual(parseNode.DependsOn, []string{"strava_export_input"}) {
		t.Fatalf("parse node dependencies mismatch: %+v", parseNode.DependsOn)
	}
	requiredFields := map[string]string{
		"activity_id":          "string",
		"activity_type":        "string",
		"distance_miles":       "double",
		"pace_min_per_mile":    "double",
		"elevation_gain_ft":    "double",
		"average_heartrate":    "double",
		"max_heartrate":        "double",
		"average_elevation_ft": "double",
		"temperature_f":        "double",
		"humidity_percent":     "double",
		"wind_speed_mph":       "double",
		"start_lat":            "double",
		"start_lon":            "double",
	}
	for name, typ := range requiredFields {
		if got := pipelineFieldType(parseNode.OutputSchema, name); got != typ {
			t.Fatalf("parse node field %s type mismatch: got %q want %q", name, got, typ)
		}
	}
	datasetNode := pipelineNodeByID(t, pipeline, "run_activity_dataset_output")
	assertPipelineOutputKind(t, datasetNode, "dataset")
	objectNode := pipelineNodeByID(t, pipeline, "run_activity_object_output")
	assertPipelineOutputKind(t, objectNode, "object_type")
	assertObjectOutputPrimaryKey(t, objectNode, "activity_id")

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the Strava ingestion transform")
	}
	actualPath := filepath.Join(t.TempDir(), "run_activities.json")
	cmd := exec.Command(python, "pipelines/strava_activity_ingestion.py", "fixtures/strava_activities.json", "--output", actualPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("strava ingestion transform failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("expected", "run_activities.golden.json"), actualPath)

	var rows []map[string]any
	readFixtureJSONInto(t, actualPath, &rows)
	if len(rows) != 9 {
		t.Fatalf("normalized run count mismatch: got %d want 9", len(rows))
	}
	for _, row := range rows {
		switch row["activity_type"] {
		case "Run", "Trail Run":
		default:
			t.Fatalf("non-run activity was not filtered out: %+v", row)
		}
		if row["source"] != "synthetic-strava-export" {
			t.Fatalf("unexpected source on RunActivity row: %+v", row)
		}
		for _, key := range []string{"distance_miles", "pace_min_per_mile", "elevation_gain_ft"} {
			value, ok := row[key].(float64)
			if !ok || value <= 0 {
				t.Fatalf("row %s must have positive numeric %s: %+v", row["activity_id"], key, row[key])
			}
		}
	}

	seedOut := t.TempDir()
	seedCmd := exec.Command(python, "seed.py", "--output", seedOut)
	seedOutput, err := seedCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, seedOutput)
	}
	assertSameJSONFile(t, filepath.Join("expected", "run_activities.golden.json"), filepath.Join(seedOut, "run_activities.json"))
}

func TestGPXTrailIngestionPipelineFixture(t *testing.T) {
	manifest := readFixtureJSON[fixtureManifest](t, filepath.Join("fixtures", "manifest.json"))
	pipeline := readFixtureJSON[demoPipelineIR](t, filepath.Join("pipelines", "gpx_trail_ingestion.pipeline.json"))
	if pipeline.Version != "pipeline_ir.v1" {
		t.Fatalf("unexpected pipeline IR version: %s", pipeline.Version)
	}
	parseNode := pipelineNodeByID(t, pipeline, "parse_gpx_trails")
	if parseNode.TransformType != "gpx_parse" {
		t.Fatalf("parse node should be gpx_parse, got %s", parseNode.TransformType)
	}
	if !reflect.DeepEqual(parseNode.DependsOn, []string{"gpx_fixture_input"}) {
		t.Fatalf("parse node dependencies mismatch: %+v", parseNode.DependsOn)
	}
	requiredFields := map[string]string{
		"trail_id":           "string",
		"trail_name":         "string",
		"distance_miles":     "double",
		"elevation_gain_ft":  "double",
		"start_lat":          "double",
		"start_lon":          "double",
		"trailhead_geopoint": "geopoint",
		"route_bbox":         "bbox",
		"route_geojson":      "geojson",
	}
	for name, typ := range requiredFields {
		if got := pipelineFieldType(parseNode.OutputSchema, name); got != typ {
			t.Fatalf("parse node field %s type mismatch: got %q want %q", name, got, typ)
		}
	}
	datasetNode := pipelineNodeByID(t, pipeline, "trail_dataset_output")
	assertPipelineOutputKind(t, datasetNode, "dataset")
	objectNode := pipelineNodeByID(t, pipeline, "trail_object_output")
	assertPipelineOutputKind(t, objectNode, "object_type")
	assertObjectOutputPrimaryKey(t, objectNode, "trail_id")

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the GPX ingestion transform")
	}
	args := []string{"pipelines/gpx_trail_ingestion.py"}
	for _, rel := range manifest.Files.GPXTrails {
		args = append(args, filepath.Join("fixtures", rel))
	}
	actualPath := filepath.Join(t.TempDir(), "trails.json")
	args = append(args, "--output", actualPath)
	cmd := exec.Command(python, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GPX ingestion transform failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trails.golden.json"), actualPath)

	var rows []map[string]any
	readFixtureJSONInto(t, actualPath, &rows)
	if len(rows) != manifest.ExpectedCounts.GPXTrails {
		t.Fatalf("trail count mismatch: got %d want %d", len(rows), manifest.ExpectedCounts.GPXTrails)
	}
	for _, row := range rows {
		if row["trail_id"] == "" || row["trail_name"] == "" {
			t.Fatalf("trail row must have id and name: %+v", row)
		}
		for _, key := range []string{"distance_miles", "start_lat", "start_lon", "end_lat", "end_lon"} {
			if _, ok := row[key].(float64); !ok {
				t.Fatalf("trail %s must have numeric %s: %+v", row["trail_id"], key, row[key])
			}
		}
		if _, ok := row["trailhead_geopoint"].(string); !ok {
			t.Fatalf("trail %s must have ontology GeoPoint string", row["trail_id"])
		}
		bbox, ok := row["route_bbox"].([]any)
		if !ok || len(bbox) != 4 {
			t.Fatalf("trail %s must have [minLon,minLat,maxLon,maxLat] bbox: %+v", row["trail_id"], row["route_bbox"])
		}
		route, ok := row["route_geojson"].(map[string]any)
		if !ok || route["type"] != "LineString" {
			t.Fatalf("trail %s must have GeoJSON LineString: %+v", row["trail_id"], row["route_geojson"])
		}
		coords, ok := route["coordinates"].([]any)
		if !ok || len(coords) < 2 {
			t.Fatalf("trail %s must have at least two route coordinates", row["trail_id"])
		}
		firstCoord, ok := coords[0].([]any)
		if !ok || len(firstCoord) != 2 {
			t.Fatalf("trail %s first coordinate must use [lon,lat] order", row["trail_id"])
		}
	}

	seedOut := t.TempDir()
	seedCmd := exec.Command(python, "seed.py", "--output", seedOut)
	seedOutput, err := seedCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, seedOutput)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trails.golden.json"), filepath.Join(seedOut, "trails.json"))
}

func TestTrailEffortEstimatorFixture(t *testing.T) {
	var contract struct {
		APIName    string `json:"api_name"`
		Runtime    string `json:"runtime"`
		Entrypoint string `json:"entrypoint"`
		Model      struct {
			Version string `json:"version"`
		} `json:"model"`
	}
	readFixtureJSONInto(t, filepath.Join("functions", "effort_estimator.function.json"), &contract)
	if contract.APIName != "estimateTrailEffort" || contract.Runtime != "python" || contract.Entrypoint != "estimate_effort" {
		t.Fatalf("unexpected effort estimator function contract: %+v", contract)
	}
	if contract.Model.Version != "weighted_knn_trail_profile_v2" {
		t.Fatalf("unexpected effort model version: %s", contract.Model.Version)
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the effort estimator function")
	}
	actualPath := filepath.Join(t.TempDir(), "trail_effort_estimates.json")
	cmd := exec.Command(
		python,
		"functions/effort_estimator.py",
		"--trails", filepath.Join("expected", "trails.golden.json"),
		"--runs", filepath.Join("expected", "run_activities.golden.json"),
		"--top-n", "5",
		"--output", actualPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("effort estimator failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trail_effort_estimates.golden.json"), actualPath)

	var estimates []map[string]any
	readFixtureJSONInto(t, actualPath, &estimates)
	if len(estimates) != 3 {
		t.Fatalf("effort estimate count mismatch: got %d want 3", len(estimates))
	}
	byTrail := map[string]map[string]any{}
	for _, estimate := range estimates {
		trailID, _ := estimate["trail_id"].(string)
		byTrail[trailID] = estimate
		if estimate["similarity_model"] != "weighted_knn_trail_profile_v2" {
			t.Fatalf("unexpected similarity model: %+v", estimate)
		}
		if _, ok := estimate["feature_weights"].(map[string]any); !ok {
			t.Fatalf("estimate %s must expose feature_weights: %+v", trailID, estimate["feature_weights"])
		}
		if _, ok := estimate["target_profile"].(map[string]any); !ok {
			t.Fatalf("estimate %s must expose target_profile: %+v", trailID, estimate["target_profile"])
		}
		if _, ok := estimate["similarity_explanation"].(map[string]any); !ok {
			t.Fatalf("estimate %s must expose similarity_explanation: %+v", trailID, estimate["similarity_explanation"])
		}
		if count, ok := estimate["similar_run_count"].(float64); !ok || count != 5 {
			t.Fatalf("estimate %s must have five similar runs: %+v", trailID, estimate["similar_run_count"])
		}
		for _, key := range []string{"estimated_pace_min_per_mile", "estimated_perceived_effort", "estimated_average_heartrate", "estimated_max_heartrate"} {
			value, ok := estimate[key].(float64)
			if !ok || value <= 0 {
				t.Fatalf("estimate %s must have positive %s: %+v", trailID, key, estimate[key])
			}
		}
		confidence, ok := estimate["confidence"].(float64)
		if !ok || confidence <= 0 || confidence > 1 {
			t.Fatalf("estimate %s confidence out of range: %+v", trailID, estimate["confidence"])
		}
		similarRuns, ok := estimate["similar_runs"].([]any)
		if !ok || len(similarRuns) != 5 {
			t.Fatalf("estimate %s must include top five similar run details", trailID)
		}
		firstRun, ok := similarRuns[0].(map[string]any)
		if !ok {
			t.Fatalf("estimate %s first similar run should be an object", trailID)
		}
		for _, key := range []string{"feature_scores", "weighted_feature_scores", "similarity_vector_delta", "match_explanation"} {
			if _, ok := firstRun[key].(map[string]any); !ok {
				t.Fatalf("estimate %s similar run should expose %s: %+v", trailID, key, firstRun[key])
			}
		}
	}
	assertSimilarRunOrder(t, byTrail["boulder-creek-path"], []string{"run-2026-0006", "run-2026-0003", "run-2026-0008"})
	assertSimilarRunOrder(t, byTrail["green-mountain-ascent"], []string{"run-2026-0007", "run-2026-0005", "run-2026-0002"})
	assertSimilarRunOrder(t, byTrail["mesa-overlook-loop"], []string{"run-2026-0001", "run-2026-0003", "run-2026-0008"})

	seedOut := t.TempDir()
	seedCmd := exec.Command(python, "seed.py", "--output", seedOut)
	seedOutput, err := seedCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, seedOutput)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trail_effort_estimates.golden.json"), filepath.Join(seedOut, "trail_effort_estimates.json"))
}

func TestCoffeeRecommendationPipelineFixture(t *testing.T) {
	pipeline := readFixtureJSON[demoPipelineIR](t, filepath.Join("pipelines", "coffee_recommendations.pipeline.json"))
	if pipeline.Version != "pipeline_ir.v1" {
		t.Fatalf("unexpected pipeline IR version: %s", pipeline.Version)
	}
	nearestNode := pipelineNodeByID(t, pipeline, "nearest_coffee_by_trail")
	if nearestNode.TransformType != "geo_nearest_neighbor_join" {
		t.Fatalf("nearest node should be geo_nearest_neighbor_join, got %s", nearestNode.TransformType)
	}
	if !reflect.DeepEqual(nearestNode.DependsOn, []string{"trail_rows_input", "coffee_shop_rows_input"}) {
		t.Fatalf("nearest node dependencies mismatch: %+v", nearestNode.DependsOn)
	}
	var nearestConfig struct {
		GeoJoin struct {
			Mode           string `json:"mode"`
			LeftLatColumn  string `json:"left_lat_column"`
			LeftLonColumn  string `json:"left_lon_column"`
			RightLatColumn string `json:"right_lat_column"`
			RightLonColumn string `json:"right_lon_column"`
			Unit           string `json:"unit"`
			K              int    `json:"k"`
			DistanceColumn string `json:"distance_column"`
			RankColumn     string `json:"rank_column"`
		} `json:"_geo_join"`
	}
	if err := json.Unmarshal(nearestNode.Config, &nearestConfig); err != nil {
		t.Fatalf("unmarshal nearest coffee config: %v", err)
	}
	if nearestConfig.GeoJoin.Mode != "nearest" || nearestConfig.GeoJoin.K != 3 || nearestConfig.GeoJoin.Unit != "miles" {
		t.Fatalf("unexpected nearest coffee geo join config: %+v", nearestConfig.GeoJoin)
	}
	if nearestConfig.GeoJoin.LeftLatColumn != "start_lat" || nearestConfig.GeoJoin.LeftLonColumn != "start_lon" {
		t.Fatalf("unexpected trail coordinate columns: %+v", nearestConfig.GeoJoin)
	}
	if nearestConfig.GeoJoin.RightLatColumn != "latitude" || nearestConfig.GeoJoin.RightLonColumn != "longitude" {
		t.Fatalf("unexpected coffee coordinate columns: %+v", nearestConfig.GeoJoin)
	}
	for name, typ := range map[string]string{
		"recommendation_id": "string",
		"trail_id":          "string",
		"coffee_shop_id":    "string",
		"rank":              "integer",
		"distance_miles":    "double",
		"line_geojson":      "geojson",
	} {
		if got := pipelineFieldType(nearestNode.OutputSchema, name); got != typ {
			t.Fatalf("nearest node field %s type mismatch: got %q want %q", name, got, typ)
		}
	}
	linkNode := pipelineNodeByID(t, pipeline, "trail_coffee_link_table")
	if linkNode.TransformType != "python" {
		t.Fatalf("link table node should be python, got %s", linkNode.TransformType)
	}
	if got := pipelineFieldType(linkNode.OutputSchema, "source_object_id"); got != "string" {
		t.Fatalf("link table source_object_id type mismatch: %q", got)
	}
	if got := pipelineFieldType(linkNode.OutputSchema, "target_object_id"); got != "string" {
		t.Fatalf("link table target_object_id type mismatch: %q", got)
	}
	assertPipelineOutputKind(t, pipelineNodeByID(t, pipeline, "coffee_shop_dataset_output"), "dataset")
	assertPipelineOutputKind(t, pipelineNodeByID(t, pipeline, "trail_coffee_recommendation_dataset_output"), "dataset")
	assertPipelineOutputKind(t, pipelineNodeByID(t, pipeline, "trail_coffee_link_dataset_output"), "dataset")
	assertObjectOutputPrimaryKey(t, pipelineNodeByID(t, pipeline, "coffee_shop_object_output"), "coffee_shop_id")
	assertObjectOutputPrimaryKey(t, pipelineNodeByID(t, pipeline, "trail_coffee_recommendation_object_output"), "recommendation_id")

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the coffee recommendation transform")
	}
	outDir := t.TempDir()
	recommendationsPath := filepath.Join(outDir, "trail_coffee_recommendations.json")
	linksPath := filepath.Join(outDir, "trail_coffee_links.json")
	cmd := exec.Command(
		python,
		"pipelines/coffee_recommendations.py",
		"--trails", filepath.Join("expected", "trails.golden.json"),
		"--coffee-shops", filepath.Join("fixtures", "coffee_shops.json"),
		"--nearest-n", "3",
		"--recommendations-output", recommendationsPath,
		"--links-output", linksPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("coffee recommendation transform failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_recommendations.golden.json"), recommendationsPath)
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_links.golden.json"), linksPath)

	var recommendations []map[string]any
	readFixtureJSONInto(t, recommendationsPath, &recommendations)
	if len(recommendations) != 9 {
		t.Fatalf("recommendation count mismatch: got %d want 9", len(recommendations))
	}
	byTrail := map[string][]map[string]any{}
	for _, recommendation := range recommendations {
		trailID, _ := recommendation["trail_id"].(string)
		byTrail[trailID] = append(byTrail[trailID], recommendation)
		if recommendation["recommendation_model"] != "haversine_trailhead_to_cafe_v1" {
			t.Fatalf("unexpected recommendation model: %+v", recommendation)
		}
		distance, ok := recommendation["distance_miles"].(float64)
		if !ok || distance <= 0 {
			t.Fatalf("recommendation must have positive distance_miles: %+v", recommendation)
		}
		line, ok := recommendation["line_geojson"].(map[string]any)
		if !ok || line["type"] != "LineString" {
			t.Fatalf("recommendation must include LineString geometry: %+v", recommendation["line_geojson"])
		}
		coords, ok := line["coordinates"].([]any)
		if !ok || len(coords) != 2 {
			t.Fatalf("recommendation line must contain two coordinates: %+v", line)
		}
	}
	assertCoffeeRecommendationOrder(t, byTrail["mesa-overlook-loop"], []string{"cafe-002", "cafe-006", "cafe-004"}, []float64{0.2314, 0.9385, 0.9721})
	assertCoffeeRecommendationOrder(t, byTrail["green-mountain-ascent"], []string{"cafe-004", "cafe-006", "cafe-002"}, []float64{0.288, 1.1263, 1.2872})
	assertCoffeeRecommendationOrder(t, byTrail["boulder-creek-path"], []string{"cafe-001", "cafe-005", "cafe-006"}, []float64{0.3429, 0.7366, 1.2267})

	var links []map[string]any
	readFixtureJSONInto(t, linksPath, &links)
	if len(links) != len(recommendations) {
		t.Fatalf("link count mismatch: got %d want %d", len(links), len(recommendations))
	}
	for _, link := range links {
		if link["link_type_api_name"] != "TrailNearbyCoffeeShop" {
			t.Fatalf("unexpected link type: %+v", link)
		}
		if link["source_object_type"] != "Trail" || link["target_object_type"] != "CoffeeShop" {
			t.Fatalf("unexpected link object types: %+v", link)
		}
	}

	seedOut := t.TempDir()
	seedCmd := exec.Command(python, "seed.py", "--output", seedOut)
	seedOutput, err := seedCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, seedOutput)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_recommendations.golden.json"), filepath.Join(seedOut, "trail_coffee_recommendations.json"))
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_links.golden.json"), filepath.Join(seedOut, "trail_coffee_links.json"))
}

func TestWeatherWebhookActionFixture(t *testing.T) {
	var source struct {
		SourceID      string `json:"source_id"`
		ConnectorType string `json:"connector_type"`
		Config        struct {
			Domain  string `json:"domain"`
			BaseURL string `json:"base_url"`
			Webhook struct {
				ID      string `json:"id"`
				APIName string `json:"api_name"`
				Method  string `json:"method"`
				Path    string `json:"path"`
				Inputs  []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Required bool   `json:"required"`
				} `json:"inputs"`
				Calls []struct {
					ID          string            `json:"id"`
					Method      string            `json:"method"`
					Path        string            `json:"path"`
					QueryParams map[string]string `json:"query_params"`
				} `json:"calls"`
				Outputs []struct {
					ID        string `json:"id"`
					Type      string `json:"type"`
					Extractor struct {
						FromCall string `json:"from_call"`
						Path     string `json:"path"`
					} `json:"extractor"`
				} `json:"outputs"`
				History struct {
					Enabled      bool `json:"enabled"`
					StoreInputs  bool `json:"store_inputs"`
					StoreOutputs bool `json:"store_outputs"`
				} `json:"history"`
			} `json:"webhook"`
		} `json:"config"`
	}
	readFixtureJSONInto(t, filepath.Join("data_connections", "open_meteo_weather.source.json"), &source)
	if source.ConnectorType != "rest_api" || source.Config.Domain != "api.open-meteo.com" {
		t.Fatalf("unexpected Open-Meteo REST source contract: %+v", source)
	}
	if source.Config.Webhook.ID != "open_meteo_current_weather" || source.Config.Webhook.Method != "GET" || source.Config.Webhook.Path != "/v1/forecast" {
		t.Fatalf("unexpected weather webhook contract: %+v", source.Config.Webhook)
	}
	if len(source.Config.Webhook.Inputs) != 2 || len(source.Config.Webhook.Calls) != 1 {
		t.Fatalf("weather webhook should define lat/lon inputs and one call: %+v", source.Config.Webhook)
	}
	outputs := map[string]string{}
	for _, output := range source.Config.Webhook.Outputs {
		outputs[output.ID] = output.Extractor.Path
	}
	for id, path := range map[string]string{
		"weather_time":           "/current/time",
		"temperature_f":          "/current/temperature_2m",
		"humidity_percent":       "/current/relative_humidity_2m",
		"wind_speed_mph":         "/current/wind_speed_10m",
		"wind_direction_degrees": "/current/wind_direction_10m",
	} {
		if outputs[id] != path {
			t.Fatalf("weather output extractor mismatch for %s: got %q want %q", id, outputs[id], path)
		}
	}
	if !source.Config.Webhook.History.Enabled || source.Config.Webhook.History.StoreInputs || !source.Config.Webhook.History.StoreOutputs {
		t.Fatalf("weather webhook should retain sanitized outputs but not inputs: %+v", source.Config.Webhook.History)
	}

	var objectType struct {
		APIName    string `json:"api_name"`
		PrimaryKey string `json:"primary_key"`
		Properties []struct {
			ID       string `json:"id"`
			BaseType string `json:"base_type"`
			Required bool   `json:"required"`
		} `json:"properties"`
	}
	readFixtureJSONInto(t, filepath.Join("ontology", "weather_snapshot.object_type.json"), &objectType)
	if objectType.APIName != "WeatherSnapshot" || objectType.PrimaryKey != "weather_snapshot_id" {
		t.Fatalf("unexpected WeatherSnapshot object type: %+v", objectType)
	}
	propertyTypes := map[string]string{}
	for _, property := range objectType.Properties {
		propertyTypes[property.ID] = property.BaseType
	}
	for id, typ := range map[string]string{
		"weather_snapshot_id":    "string",
		"trail_id":               "string",
		"trailhead_geopoint":     "geopoint",
		"weather_time":           "timestamp",
		"temperature_f":          "double",
		"humidity_percent":       "double",
		"wind_speed_mph":         "double",
		"wind_direction_degrees": "double",
	} {
		if propertyTypes[id] != typ {
			t.Fatalf("WeatherSnapshot property %s type mismatch: got %q want %q", id, propertyTypes[id], typ)
		}
	}

	var action struct {
		APIName           string `json:"api_name"`
		OperationKind     string `json:"operation_kind"`
		ObjectTypeAPIName string `json:"object_type_api_name"`
		InputSchema       []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"input_schema"`
		Config struct {
			Operation struct {
				ObjectIDInputName string `json:"object_id_input_name"`
				PropertyMappings  []struct {
					PropertyName string `json:"property_name"`
					InputName    string `json:"input_name"`
				} `json:"property_mappings"`
			} `json:"operation"`
			WebhookWriteback struct {
				WebhookID            string `json:"webhook_id"`
				OutputParameterAlias string `json:"output_parameter_alias"`
				InputMappings        []struct {
					WebhookInputName string `json:"webhook_input_name"`
					ActionInputName  string `json:"action_input_name"`
				} `json:"input_mappings"`
			} `json:"webhook_writeback"`
		} `json:"config"`
	}
	readFixtureJSONInto(t, filepath.Join("actions", "fetch_trail_weather.action.json"), &action)
	if action.APIName != "FetchTrailWeather" || action.OperationKind != "create_or_modify_object" || action.ObjectTypeAPIName != "WeatherSnapshot" {
		t.Fatalf("unexpected weather action contract: %+v", action)
	}
	if action.Config.WebhookWriteback.WebhookID != source.Config.Webhook.ID || action.Config.WebhookWriteback.OutputParameterAlias != "webhook_output" {
		t.Fatalf("weather action webhook writeback drift: %+v", action.Config.WebhookWriteback)
	}
	requiredInputs := map[string]bool{}
	for _, input := range action.InputSchema {
		if input.Required {
			requiredInputs[input.Name] = true
		}
	}
	for _, name := range []string{"trail_id", "trail_name", "latitude", "longitude", "trailhead_geopoint"} {
		if !requiredInputs[name] {
			t.Fatalf("weather action missing required input %s", name)
		}
	}

	weatherResponse, err := os.ReadFile(filepath.Join("fixtures", "weather_open_meteo_boulder.json"))
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/v1/forecast" {
			t.Fatalf("unexpected weather webhook path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("latitude") != "40.0153" || query.Get("longitude") != "-105.289" {
			t.Fatalf("weather webhook coordinate query drift: %s", r.URL.RawQuery)
		}
		if query.Get("temperature_unit") != "fahrenheit" || query.Get("wind_speed_unit") != "mph" {
			t.Fatalf("weather webhook unit query drift: %s", r.URL.RawQuery)
		}
		if !strings.Contains(query.Get("current"), "temperature_2m") || !strings.Contains(query.Get("current"), "wind_direction_10m") {
			t.Fatalf("weather webhook current fields drift: %s", r.URL.RawQuery)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(weatherResponse)
	}))
	defer server.Close()

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the weather action fixture")
	}
	actualPath := filepath.Join(t.TempDir(), "trail_weather_snapshot.json")
	cmd := exec.Command(
		python,
		"actions/fetch_trail_weather.py",
		"--source", filepath.Join("data_connections", "open_meteo_weather.source.json"),
		"--action", filepath.Join("actions", "fetch_trail_weather.action.json"),
		"--trails", filepath.Join("expected", "trails.golden.json"),
		"--trail-id", "boulder-creek-path",
		"--base-url", server.URL,
		"--output", actualPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("weather action fixture failed: %v\n%s", err, output)
	}
	if hits != 1 {
		t.Fatalf("expected one weather webhook hit, got %d", hits)
	}
	assertSameJSONFile(t, filepath.Join("expected", "trail_weather_snapshot.golden.json"), actualPath)

	var snapshot map[string]any
	readFixtureJSONInto(t, actualPath, &snapshot)
	if snapshot["weather_snapshot_id"] != "weather-boulder-creek-path-2026-05-11T14-00" {
		t.Fatalf("weather snapshot id drift: %+v", snapshot)
	}
	if snapshot["temperature_f"] != 68.4 || snapshot["wind_speed_mph"] != 6.2 || snapshot["humidity_percent"] != float64(34) {
		t.Fatalf("weather snapshot metrics drift: %+v", snapshot)
	}
}

func TestWorkshopAppFixture(t *testing.T) {
	app := readFixtureJSON[workshopDemoApp](t, filepath.Join("workshop", "run_fast.workshop_app.json"))
	if app.ID != "trail-running-run-fast" || app.Name != "Run Fast" || app.Slug != "run-fast" || app.Status != "published" {
		t.Fatalf("unexpected Workshop app identity: %+v", app)
	}
	if app.Settings.HomePageID != "trail-overview" || app.Settings.NavigationStyle != "tabs" {
		t.Fatalf("Workshop app should publish a navigable tabbed runtime: %+v", app.Settings)
	}
	if len(app.Pages) != 4 {
		t.Fatalf("Workshop app page count mismatch: got %d want 4", len(app.Pages))
	}
	for _, expected := range []struct {
		id          string
		path        string
		widgetTypes []string
	}{
		{"trail-overview", "/", []string{"filter_list", "object_table", "chart_xy", "metric", "button_group"}},
		{"trail-map-page", "/map", []string{"map", "object_table"}},
		{"trail-detail-page", "/trail", []string{"object_set_title", "property_list", "metric", "object_table"}},
		{"custom-gpx-upload-page", "/upload", []string{"media_uploader", "object_table"}},
	} {
		page := workshopPageByID(t, app, expected.id)
		if page.Path != expected.path || !page.Visible {
			t.Fatalf("page %s path/visibility drift: %+v", expected.id, page)
		}
		for _, widgetType := range expected.widgetTypes {
			if !workshopPageHasWidgetType(page, widgetType) {
				t.Fatalf("page %s missing widget type %s", expected.id, widgetType)
			}
		}
	}

	mapWidget := workshopWidgetByID(t, workshopPageByID(t, app, "trail-map-page"), "trail-coffee-map")
	layers, ok := mapWidget.Props["layers"].([]any)
	if !ok || len(layers) < 4 {
		t.Fatalf("map widget should define trail, route, coffee, and distance layers: %+v", mapWidget.Props["layers"])
	}
	for _, layerID := range []string{"trail-starts", "trail-routes", "coffee-shops", "trail-coffee-lines"} {
		if !workshopMapHasLayer(layers, layerID) {
			t.Fatalf("map widget missing layer %s", layerID)
		}
	}

	weatherWidget := workshopWidgetByID(t, workshopPageByID(t, app, "trail-overview"), "weather-action")
	buttons, ok := weatherWidget.Props["buttons"].([]any)
	if !ok || len(buttons) != 1 {
		t.Fatalf("weather action should define one Button Group action: %+v", weatherWidget.Props["buttons"])
	}
	button, _ := buttons[0].(map[string]any)
	if button["action_type_id"] != "FetchTrailWeather" {
		t.Fatalf("weather button should call FetchTrailWeather: %+v", button)
	}

	uploadWidget := workshopWidgetByID(t, workshopPageByID(t, app, "custom-gpx-upload-page"), "custom-gpx-upload")
	if uploadWidget.Props["upload_mode"] != "gpx_trail" {
		t.Fatalf("custom GPX upload widget should use gpx_trail mode: %+v", uploadWidget.Props)
	}
	if uploadWidget.Props["trail_object_type_id"] != "Trail" || uploadWidget.Props["estimate_object_type_id"] != "TrailEffortEstimate" {
		t.Fatalf("custom GPX upload widget object type wiring drift: %+v", uploadWidget.Props)
	}
	if uploadWidget.Props["estimate_function_package_id"] != "estimateTrailEffort" {
		t.Fatalf("custom GPX upload widget should call estimateTrailEffort: %+v", uploadWidget.Props)
	}

	variables := map[string]workshopDemoVariable{}
	for _, variable := range app.Settings.WorkshopVariables {
		variables[variable.ID] = variable
	}
	for id, want := range map[string]string{
		"trail-set":                    "Trail",
		"filtered-trails":              "Trail",
		"trail-estimates":              "TrailEffortEstimate",
		"coffee-set":                   "CoffeeShop",
		"trail-coffee-recommendations": "TrailCoffeeRecommendation",
		"selected-trail-detail":        "Trail",
		"selected-trail-estimate":      "TrailEffortEstimate",
		"selected-trail-coffee":        "TrailCoffeeRecommendation",
	} {
		if variables[id].ObjectTypeID != want {
			t.Fatalf("Workshop variable %s object type mismatch: got %+v want %s", id, variables[id], want)
		}
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the trail-running seed command")
	}
	outDir := t.TempDir()
	cmd := exec.Command(python, "seed.py", "--output", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed.py failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("workshop", "run_fast.workshop_app.json"), filepath.Join(outDir, "workshop", "run_fast.workshop_app.json"))
}

func TestCustomGPXWorkshopUploadFixture(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("fixtures", "gpx", "custom_dawn_ridge.gpx"))
	if err != nil {
		t.Fatal(err)
	}
	trail, err := geospatialcore.ParseGPXTrail(body, geospatialcore.GPXParseOptions{SourceName: "gpx/custom_dawn_ridge.gpx"})
	if err != nil {
		t.Fatalf("parse custom upload GPX: %v", err)
	}
	if trail.TrailName != "Custom Dawn Ridge" || trail.PointCount != 6 {
		t.Fatalf("custom GPX fixture drift: %+v", trail)
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the Workshop GPX upload flow")
	}
	outDir := t.TempDir()
	trailPath := filepath.Join(outDir, "custom_gpx_upload_trail.json")
	estimatePath := filepath.Join(outDir, "custom_gpx_upload_estimate.json")
	cmd := exec.Command(
		python,
		filepath.Join("workshop", "gpx_upload_flow.py"),
		"--gpx", filepath.Join("fixtures", "gpx", "custom_dawn_ridge.gpx"),
		"--runs", filepath.Join("expected", "run_activities.golden.json"),
		"--trail-output", trailPath,
		"--estimate-output", estimatePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Workshop GPX upload flow failed: %v\n%s", err, output)
	}
	assertSameJSONFile(t, filepath.Join("expected", "custom_gpx_upload_trail.golden.json"), trailPath)
	assertSameJSONFile(t, filepath.Join("expected", "custom_gpx_upload_estimate.golden.json"), estimatePath)

	var uploadedTrails []map[string]any
	readFixtureJSONInto(t, trailPath, &uploadedTrails)
	if len(uploadedTrails) != 1 || uploadedTrails[0]["trail_id"] != "custom-dawn-ridge" {
		t.Fatalf("custom upload Trail output drift: %+v", uploadedTrails)
	}
	var estimates []map[string]any
	readFixtureJSONInto(t, estimatePath, &estimates)
	if len(estimates) != 1 || estimates[0]["estimate_id"] != "effort-custom-dawn-ridge" {
		t.Fatalf("custom upload estimate output drift: %+v", estimates)
	}
}

func readFixtureJSON[T any](t *testing.T, path string) T {
	t.Helper()
	var out T
	readFixtureJSONInto(t, path, &out)
	return out
}

func readFixtureJSONInto(t *testing.T, path string, dst any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}

func readCoffeeCSV(t *testing.T, path string) []map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("%s should contain a header and at least one row", path)
	}
	out := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		item := map[string]string{}
		for i, key := range rows[0] {
			item[key] = row[i]
		}
		out = append(out, item)
	}
	return out
}

func pipelineNodeByID(t *testing.T, pipeline demoPipelineIR, id string) demoPipelineNode {
	t.Helper()
	for _, node := range pipeline.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("pipeline node %s not found", id)
	return demoPipelineNode{}
}

func pipelineFieldType(schema *demoPipelineSchema, name string) string {
	if schema == nil {
		return ""
	}
	for _, field := range schema.Fields {
		if field.Name == name {
			return field.FieldType
		}
	}
	return ""
}

func assertPipelineOutputKind(t *testing.T, node demoPipelineNode, want string) {
	t.Helper()
	var cfg struct {
		Output struct {
			Kind string `json:"kind"`
		} `json:"_output"`
	}
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		t.Fatalf("unmarshal output config for %s: %v", node.ID, err)
	}
	if cfg.Output.Kind != want {
		t.Fatalf("output kind for %s mismatch: got %q want %q", node.ID, cfg.Output.Kind, want)
	}
}

func assertObjectOutputPrimaryKey(t *testing.T, node demoPipelineNode, want string) {
	t.Helper()
	var cfg struct {
		Output struct {
			PrimaryKey      string `json:"primary_key"`
			PropertyMapping []struct {
				SourceField      string `json:"source_field"`
				UniqueConstraint bool   `json:"unique_constraint"`
			} `json:"property_mapping"`
		} `json:"_output"`
	}
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		t.Fatalf("unmarshal object output config for %s: %v", node.ID, err)
	}
	if cfg.Output.PrimaryKey != want {
		t.Fatalf("object primary key mismatch: got %q want %q", cfg.Output.PrimaryKey, want)
	}
	for _, mapping := range cfg.Output.PropertyMapping {
		if mapping.SourceField == want && mapping.UniqueConstraint {
			return
		}
	}
	t.Fatalf("object output does not mark %s as unique in property_mapping", want)
}

func assertSameJSONFile(t *testing.T, expectedPath, actualPath string) {
	t.Helper()
	var expected any
	var actual any
	readFixtureJSONInto(t, expectedPath, &expected)
	readFixtureJSONInto(t, actualPath, &actual)
	if !reflect.DeepEqual(expected, actual) {
		expectedBody, _ := os.ReadFile(expectedPath)
		actualBody, _ := os.ReadFile(actualPath)
		t.Fatalf("JSON mismatch\nexpected %s:\n%s\nactual %s:\n%s", expectedPath, expectedBody, actualPath, actualBody)
	}
}

func assertSimilarRunOrder(t *testing.T, estimate map[string]any, wantPrefix []string) {
	t.Helper()
	if estimate == nil {
		t.Fatalf("missing estimate for expected trail")
	}
	rawIDs, ok := estimate["similar_run_ids"].([]any)
	if !ok || len(rawIDs) < len(wantPrefix) {
		t.Fatalf("estimate has invalid similar_run_ids: %+v", estimate["similar_run_ids"])
	}
	for i, want := range wantPrefix {
		if rawIDs[i] != want {
			t.Fatalf("similar run order mismatch at %d: got %v want %s", i, rawIDs[i], want)
		}
	}
}

func assertCoffeeRecommendationOrder(t *testing.T, rows []map[string]any, wantCafeIDs []string, wantMiles []float64) {
	t.Helper()
	if len(rows) != len(wantCafeIDs) {
		t.Fatalf("recommendation count mismatch: got %d want %d", len(rows), len(wantCafeIDs))
	}
	for i, wantCafeID := range wantCafeIDs {
		if rows[i]["coffee_shop_id"] != wantCafeID {
			t.Fatalf("coffee recommendation order mismatch at %d: got %v want %s", i, rows[i]["coffee_shop_id"], wantCafeID)
		}
		rank, ok := rows[i]["rank"].(float64)
		if !ok || int(rank) != i+1 {
			t.Fatalf("coffee recommendation rank mismatch at %d: got %+v", i, rows[i]["rank"])
		}
		gotMiles, ok := rows[i]["distance_miles"].(float64)
		if !ok || math.Abs(gotMiles-wantMiles[i]) > 0.0001 {
			t.Fatalf("coffee distance mismatch at %d: got %.4f want %.4f", i, gotMiles, wantMiles[i])
		}
	}
}

func workshopPageByID(t *testing.T, app workshopDemoApp, id string) workshopDemoPage {
	t.Helper()
	for _, page := range app.Pages {
		if page.ID == id {
			return page
		}
	}
	t.Fatalf("Workshop page %s not found", id)
	return workshopDemoPage{}
}

func workshopWidgetByID(t *testing.T, page workshopDemoPage, id string) workshopDemoWidget {
	t.Helper()
	for _, widget := range page.Widgets {
		if widget.ID == id {
			return widget
		}
	}
	t.Fatalf("Workshop widget %s not found on page %s", id, page.ID)
	return workshopDemoWidget{}
}

func workshopPageHasWidgetType(page workshopDemoPage, widgetType string) bool {
	for _, widget := range page.Widgets {
		if widget.WidgetType == widgetType {
			return true
		}
	}
	return false
}

func workshopMapHasLayer(layers []any, id string) bool {
	for _, layer := range layers {
		entry, ok := layer.(map[string]any)
		if ok && entry["id"] == id {
			return true
		}
	}
	return false
}

func rejectPrivateFixtureKeys(t *testing.T, value any) {
	t.Helper()
	private := map[string]bool{
		"athlete":        true,
		"athlete_id":     true,
		"device_id":      true,
		"email":          true,
		"firstname":      true,
		"lastname":       true,
		"profile":        true,
		"profile_medium": true,
		"resource_state": true,
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if private[strings.ToLower(key)] {
				t.Fatalf("private fixture key %q is not allowed", key)
			}
			rejectPrivateFixtureKeys(t, nested)
		}
	case []any:
		for _, nested := range typed {
			rejectPrivateFixtureKeys(t, nested)
		}
	}
}

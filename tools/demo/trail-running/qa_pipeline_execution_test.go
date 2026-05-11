package trailrunningdemo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQAFixtureDrivenPipelineExecutionGoldenOutputs(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the Trail Running pipeline fixtures")
	}

	manifest := readFixtureJSON[fixtureManifest](t, filepath.Join("fixtures", "manifest.json"))
	outDir := t.TempDir()

	runActivitiesPath := filepath.Join(outDir, "run_activities.json")
	runPythonFixture(t, python,
		"pipelines/strava_activity_ingestion.py",
		filepath.Join("fixtures", manifest.Files.StravaActivities),
		"--output", runActivitiesPath,
	)
	assertSameJSONFile(t, filepath.Join("expected", "run_activities.golden.json"), runActivitiesPath)

	gpxArgs := []string{"pipelines/gpx_trail_ingestion.py"}
	for _, rel := range manifest.Files.GPXTrails {
		gpxArgs = append(gpxArgs, filepath.Join("fixtures", rel))
	}
	trailsPath := filepath.Join(outDir, "trails.json")
	gpxArgs = append(gpxArgs, "--output", trailsPath)
	runPythonFixture(t, python, gpxArgs...)
	assertSameJSONFile(t, filepath.Join("expected", "trails.golden.json"), trailsPath)

	effortPath := filepath.Join(outDir, "trail_effort_estimates.json")
	runPythonFixture(t, python,
		"functions/effort_estimator.py",
		"--trails", trailsPath,
		"--runs", runActivitiesPath,
		"--top-n", "5",
		"--output", effortPath,
	)
	assertSameJSONFile(t, filepath.Join("expected", "trail_effort_estimates.golden.json"), effortPath)

	jsonCoffeeRecommendationsPath := filepath.Join(outDir, "trail_coffee_recommendations.json")
	jsonCoffeeLinksPath := filepath.Join(outDir, "trail_coffee_links.json")
	runPythonFixture(t, python,
		"pipelines/coffee_recommendations.py",
		"--trails", trailsPath,
		"--coffee-shops", filepath.Join("fixtures", manifest.Files.CoffeeShopsJSON),
		"--nearest-n", "3",
		"--recommendations-output", jsonCoffeeRecommendationsPath,
		"--links-output", jsonCoffeeLinksPath,
	)
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_recommendations.golden.json"), jsonCoffeeRecommendationsPath)
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_links.golden.json"), jsonCoffeeLinksPath)

	csvCoffeeRecommendationsPath := filepath.Join(outDir, "trail_coffee_recommendations_from_csv.json")
	csvCoffeeLinksPath := filepath.Join(outDir, "trail_coffee_links_from_csv.json")
	runPythonFixture(t, python,
		"pipelines/coffee_recommendations.py",
		"--trails", trailsPath,
		"--coffee-shops", filepath.Join("fixtures", manifest.Files.CoffeeShopsCSV),
		"--nearest-n", "3",
		"--recommendations-output", csvCoffeeRecommendationsPath,
		"--links-output", csvCoffeeLinksPath,
	)
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_recommendations.golden.json"), csvCoffeeRecommendationsPath)
	assertSameJSONFile(t, filepath.Join("expected", "trail_coffee_links.golden.json"), csvCoffeeLinksPath)

	weatherFixture, err := os.ReadFile(filepath.Join("fixtures", manifest.Files.WeatherResponse))
	if err != nil {
		t.Fatal(err)
	}
	weatherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			t.Fatalf("unexpected weather path: %s", r.URL.Path)
		}
		if query := r.URL.Query(); query.Get("latitude") == "" || query.Get("longitude") == "" {
			t.Fatalf("weather request must include selected trail coordinates: %s", r.URL.RawQuery)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(weatherFixture)
	}))
	defer weatherServer.Close()

	weatherSnapshotPath := filepath.Join(outDir, "trail_weather_snapshot.json")
	runPythonFixture(t, python,
		"actions/fetch_trail_weather.py",
		"--source", filepath.Join("data_connections", "open_meteo_weather.source.json"),
		"--action", filepath.Join("actions", "fetch_trail_weather.action.json"),
		"--trails", trailsPath,
		"--trail-id", "boulder-creek-path",
		"--base-url", weatherServer.URL,
		"--output", weatherSnapshotPath,
	)
	assertSameJSONFile(t, filepath.Join("expected", "trail_weather_snapshot.golden.json"), weatherSnapshotPath)

	seedOut := filepath.Join(outDir, "seed")
	runPythonFixture(t, python, "seed.py", "--output", seedOut)
	for _, pair := range []struct {
		expected string
		actual   string
	}{
		{"run_activities.golden.json", "run_activities.json"},
		{"trails.golden.json", "trails.json"},
		{"trail_effort_estimates.golden.json", "trail_effort_estimates.json"},
		{"trail_coffee_recommendations.golden.json", "trail_coffee_recommendations.json"},
		{"trail_coffee_links.golden.json", "trail_coffee_links.json"},
	} {
		assertSameJSONFile(t, filepath.Join("expected", pair.expected), filepath.Join(seedOut, pair.actual))
	}
}

func runPythonFixture(t *testing.T, python string, args ...string) {
	t.Helper()
	cmd := exec.Command(python, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", python, strings.Join(args, " "), err, output)
	}
}

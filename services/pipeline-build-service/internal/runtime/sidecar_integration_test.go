//go:build integration

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
)

func startPipelineSidecar(t *testing.T) *pythonsidecar.Manager {
	t.Helper()
	bin := os.Getenv("PYTHON_SIDECAR_BINARY")
	if bin == "" {
		t.Skip("PYTHON_SIDECAR_BINARY not set — install openfoundry-pyruntime in a venv and re-run")
	}
	mgr, err := pythonsidecar.New(pythonsidecar.Config{
		BinaryPath:      bin,
		StartupTimeout:  5 * time.Second,
		HardCallTimeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("New sidecar manager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start sidecar: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })
	return mgr
}

func TestExecutePythonTransformWithRealSidecar(t *testing.T) {
	mgr := startPipelineSidecar(t)
	exec := NewSidecarTransformExecutor(SidecarTransform{Mgr: mgr})

	got, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{
		Source:             `print("pipeline stdout")` + "\n" + `rows_affected = 2` + "\n" + `result_rows = [{"id": "a", "score": 1}, {"id": "b", "score": 2}]` + "\n" + `result = {"status": "ok"}`,
		ConfigJSON:         []byte(`{"kind":"integration"}`),
		PreparedInputsJSON: []byte(`[{"dataset_id":"input-a","rows":[{"score":1}]}]`),
		InputDatasetIDs:    []string{"input-a"},
		OutputDatasetID:    "output-a",
		TimeoutSeconds:     10,
	})
	if err != nil {
		t.Fatalf("ExecutePythonTransform: %v", err)
	}
	if got.Stdout != "pipeline stdout\n" || got.Stderr != "" {
		t.Fatalf("stdout/stderr drift: stdout=%q stderr=%q", got.Stdout, got.Stderr)
	}
	if got.RowsAffected == nil || *got.RowsAffected != 2 {
		t.Fatalf("rows_affected = %v, want 2", got.RowsAffected)
	}
	if len(got.ResultRows) != 2 || !json.Valid(got.ResultRowsJSON) || !strings.Contains(string(got.ResultRowsJSON), `"score":2`) {
		t.Fatalf("result_rows drift: rows=%s json=%s", got.ResultRows, got.ResultRowsJSON)
	}
	var output map[string]any
	if err := json.Unmarshal(got.Output, &output); err != nil {
		t.Fatalf("output JSON: %v body=%s", err, got.Output)
	}
	if !strings.Contains(string(got.Output), "ok") {
		t.Fatalf("output result drift: %s", got.Output)
	}

	_, err = exec.ExecutePythonTransform(context.Background(), TransformRequest{Source: `raise RuntimeError("pipeline integration boom")`, TimeoutSeconds: 10})
	if err == nil || !strings.Contains(err.Error(), "pipeline integration boom") {
		t.Fatalf("expected pipeline integration error, got %v", err)
	}
}

func TestExecutePythonTransformParsesStravaAndGPXWithRealSidecar(t *testing.T) {
	mgr := startPipelineSidecar(t)
	exec := NewSidecarTransformExecutor(SidecarTransform{Mgr: mgr})

	source := `
import json
import sys
import xml.etree.ElementTree as ET

activity = config["strava_activity"]
root = ET.fromstring(config["gpx"])
points = root.findall(".//{*}trkpt")
elevations = [float(pt.find("{*}ele").text) for pt in points if pt.find("{*}ele") is not None]
gain_m = sum(max(0, elevations[i] - elevations[i - 1]) for i in range(1, len(elevations)))
print("parsed trail demo")
print("gpx points=%d" % len(points), file=sys.stderr)
result_rows = [{
    "activity_id": str(activity["id"]),
    "distance_miles": round(float(activity["distance"]) / 1609.344, 3),
    "elevation_gain_ft": round(gain_m * 3.28084, 1),
    "gpx_points": len(points),
}]
rows_affected = len(result_rows)
result = {"status": "ok", "kind": "trail_parse"}
`
	got, err := exec.ExecutePythonTransform(context.Background(), TransformRequest{
		Source: source,
		ConfigJSON: []byte(`{
			"strava_activity":{"id":987,"distance":8046.72},
			"gpx":"<gpx><trk><trkseg><trkpt lat=\"40.0\" lon=\"-105.0\"><ele>1600</ele></trkpt><trkpt lat=\"40.1\" lon=\"-105.1\"><ele>1610</ele></trkpt><trkpt lat=\"40.2\" lon=\"-105.2\"><ele>1605</ele></trkpt></trkseg></trk></gpx>"
		}`),
		PreparedInputsJSON: []byte(`[]`),
		TimeoutSeconds:     10,
	})
	if err != nil {
		t.Fatalf("ExecutePythonTransform trail parse: %v", err)
	}
	if got.Stdout != "parsed trail demo\n" || !strings.Contains(got.Stderr, "gpx points=3") {
		t.Fatalf("stdout/stderr drift: stdout=%q stderr=%q", got.Stdout, got.Stderr)
	}
	if got.RowsAffected == nil || *got.RowsAffected != 1 {
		t.Fatalf("rows_affected = %v, want 1", got.RowsAffected)
	}
	if len(got.ResultRows) != 1 || !strings.Contains(string(got.ResultRows[0]), `"distance_miles":5`) || !strings.Contains(string(got.ResultRows[0]), `"gpx_points":3`) {
		t.Fatalf("trail result rows drift: %s", got.ResultRows)
	}
}

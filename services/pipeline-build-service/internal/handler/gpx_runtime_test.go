package handler

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestLightweightRuntimeParsesGPXTrailRows(t *testing.T) {
	raw, err := os.ReadFile("testdata/mesa_loop.gpx")
	require.NoError(t, err)

	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()
	inputPayload, err := json.Marshal(map[string]any{"rows": []map[string]any{{
		"raw_gpx":     string(raw),
		"upload_name": "mesa_loop.gpx",
	}}})
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, inputPayload, "dataset_input")
	require.NoError(t, err)

	result, err := rt.Run(ctx, executor.NodeContext{
		BuildID: buildID,
		Node:    executor.Node{ID: "gpx", DependsOn: []string{"input"}},
	}, json.RawMessage(`{"gpx_column":"raw_gpx","file_name_column":"upload_name"}`), "gpx_parse")
	require.NoError(t, err)
	require.Equal(t, "lightweight_table", result.Metadata["runtime"])
	require.Equal(t, 1, result.Metadata["rows_affected"])

	rows := rt.snapshotRows("gpx")
	require.Len(t, rows, 1)
	require.JSONEq(t, `"mesa-loop"`, string(rows[0]["trail_id"]))
	require.JSONEq(t, `"Mesa Loop"`, string(rows[0]["trail_name"]))
	require.JSONEq(t, `4`, string(rows[0]["point_count"]))
	require.JSONEq(t, `35`, string(rows[0]["elevation_gain_meters"]))
	require.JSONEq(t, `"40,-105.3"`, string(rows[0]["trailhead_geo_point"]))

	var route string
	require.NoError(t, json.Unmarshal(rows[0]["route_geojson"], &route))
	require.Contains(t, route, `"type":"LineString"`)
	require.Contains(t, route, `[-105.3,40]`)

	var bbox string
	require.NoError(t, json.Unmarshal(rows[0]["route_bbox"], &bbox))
	require.JSONEq(t, `[-105.3,40,-105.295,40.003]`, bbox)
}

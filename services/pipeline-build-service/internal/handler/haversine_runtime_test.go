package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestLightweightRuntimeHaversineDistanceBlock(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()

	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, json.RawMessage(`{
		"rows":[
			{"trail_id":"mesa","trail_lat":0,"trail_lon":0,"coffee_lat":0,"coffee_lon":1},
			{"trail_id":"null-coffee","trail_lat":0,"trail_lon":0,"coffee_lat":null,"coffee_lon":1}
		]
	}`), "dataset_input")
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "distance", DependsOn: []string{"input"}}}, json.RawMessage(`{
		"_stack":{"blocks":[{
			"kind":"haversine_distance",
			"applied":true,
			"start_lat_column":"trail_lat",
			"start_lon_column":"trail_lon",
			"end_lat_column":"coffee_lat",
			"end_lon_column":"coffee_lon",
			"unit":"miles",
			"target_column":"distance_to_coffee_miles"
		}]}
	}`), "sql")
	require.NoError(t, err)

	rows := rt.snapshotRows("distance")
	require.Len(t, rows, 2)
	var miles float64
	require.NoError(t, json.Unmarshal(rows[0]["distance_to_coffee_miles"], &miles))
	require.InDelta(t, 69.09341957563636, miles, 1e-9)
	require.JSONEq(t, `null`, string(rows[1]["distance_to_coffee_miles"]))
}

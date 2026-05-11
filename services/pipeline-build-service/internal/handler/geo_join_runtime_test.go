package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestLightweightRuntimeGeoIntersectionJoin(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()

	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "trails"}}, json.RawMessage(`{
		"rows":[
			{"trail_id":"betasso","route_geojson":{"type":"LineString","coordinates":[[-105.36,40.01],[-105.32,40.02]]}},
			{"trail_id":"mesa","route_geojson":{"type":"LineString","coordinates":[[-105.20,39.95],[-105.18,39.96]]}}
		]
	}`), "dataset_input")
	require.NoError(t, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "areas"}}, json.RawMessage(`{
		"rows":[
			{"area_id":"boulder-open-space","boundary_geojson":{"type":"Polygon","coordinates":[[[-105.37,40.00],[-105.30,40.00],[-105.30,40.05],[-105.37,40.05],[-105.37,40.00]]]}}
		]
	}`), "dataset_input")
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "geo_join", DependsOn: []string{"trails", "areas"}}}, json.RawMessage(`{
		"_geo_join":{
			"mode":"intersection",
			"left_geometry_column":"route_geojson",
			"right_geometry_column":"boundary_geojson",
			"right_prefix":"area_",
			"auto_select_left":true,
			"auto_select_right":true
		}
	}`), "geo_join")
	require.NoError(t, err)

	rows := rt.snapshotRows("geo_join")
	require.Len(t, rows, 1)
	require.JSONEq(t, `"betasso"`, string(rows[0]["trail_id"]))
	require.JSONEq(t, `"boulder-open-space"`, string(rows[0]["area_area_id"]))
}

func TestLightweightRuntimeGeoNearestNeighborCoffeeRecommendation(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()

	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "trails"}}, json.RawMessage(`{
		"rows":[
			{"trail_id":"betasso","trail_name":"Betasso Preserve","start_lat":40.016353,"start_lon":-105.34458},
			{"trail_id":"mesa","trail_name":"Mesa Trail","start_lat":39.9995,"start_lon":-105.2888}
		]
	}`), "dataset_input")
	require.NoError(t, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "coffee"}}, json.RawMessage(`{
		"rows":[
			{"coffee_id":"alpine","coffee_name":"Alpine Modern","lat":40.01851,"lon":-105.28291},
			{"coffee_id":"trailhead","coffee_name":"Trailhead Coffee","lat":40.0161,"lon":-105.3441},
			{"coffee_id":"south","coffee_name":"South Side Coffee","lat":39.999,"lon":-105.289}
		]
	}`), "dataset_input")
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "recommend", DependsOn: []string{"trails", "coffee"}}}, json.RawMessage(`{
		"_geo_join":{
			"mode":"nearest",
			"join_type":"left",
			"left_lat_column":"start_lat",
			"left_lon_column":"start_lon",
			"right_lat_column":"lat",
			"right_lon_column":"lon",
			"k":1,
			"unit":"miles",
			"distance_column":"distance_to_coffee_miles",
			"rank_column":"coffee_rank",
			"right_prefix":"coffee_",
			"auto_select_left":true,
			"right_columns":["coffee_id","coffee_name"]
		}
	}`), "geo_nearest_neighbor_join")
	require.NoError(t, err)

	rows := rt.snapshotRows("recommend")
	require.Len(t, rows, 2)
	require.JSONEq(t, `"trailhead"`, string(rows[0]["coffee_coffee_id"]))
	require.JSONEq(t, `1`, string(rows[0]["coffee_rank"]))
	require.JSONEq(t, `"south"`, string(rows[1]["coffee_coffee_id"]))

	var distance float64
	require.NoError(t, json.Unmarshal(rows[0]["distance_to_coffee_miles"], &distance))
	require.Less(t, distance, 0.05)
}

func TestLightweightRuntimeGeoNearestNeighborMediumFixtureAndScaleLimit(t *testing.T) {
	rt := newLightweightTableRuntime()
	buildID := uuid.New()
	ctx := context.Background()

	trails := make([]map[string]json.RawMessage, 120)
	for i := range trails {
		trails[i] = map[string]json.RawMessage{
			"trail_id":  mustRuntimeJSON(i),
			"start_lat": mustRuntimeJSON(40.0 + float64(i)*0.0001),
			"start_lon": mustRuntimeJSON(-105.0 - float64(i)*0.0001),
		}
	}
	coffee := make([]map[string]json.RawMessage, 40)
	for i := range coffee {
		coffee[i] = map[string]json.RawMessage{
			"coffee_id": mustRuntimeJSON(i),
			"lat":       mustRuntimeJSON(40.0 + float64(i)*0.0003),
			"lon":       mustRuntimeJSON(-105.0 - float64(i)*0.0003),
		}
	}
	trailPayload, err := json.Marshal(map[string]any{"rows": trails})
	require.NoError(t, err)
	coffeePayload, err := json.Marshal(map[string]any{"rows": coffee})
	require.NoError(t, err)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "medium_trails"}}, trailPayload, "dataset_input")
	require.NoError(t, err)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "medium_coffee"}}, coffeePayload, "dataset_input")
	require.NoError(t, err)

	geoJoinConfig := json.RawMessage(`{
		"_geo_join":{
			"mode":"nearest",
			"left_lat_column":"start_lat",
			"left_lon_column":"start_lon",
			"right_lat_column":"lat",
			"right_lon_column":"lon",
			"k":1,
			"max_candidate_pairs":5000,
			"auto_select_left":true,
			"right_columns":["coffee_id"]
		}
	}`)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "medium_knn", DependsOn: []string{"medium_trails", "medium_coffee"}}}, geoJoinConfig, "geo_join")
	require.NoError(t, err)
	require.Len(t, rt.snapshotRows("medium_knn"), 120)

	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "too_large", DependsOn: []string{"medium_trails", "medium_coffee"}}}, json.RawMessage(`{
		"_geo_join":{
			"mode":"nearest",
			"left_lat_column":"start_lat",
			"left_lon_column":"start_lon",
			"right_lat_column":"lat",
			"right_lon_column":"lon",
			"max_candidate_pairs":100
		}
	}`), "geo_join")
	require.Error(t, err)
	require.Contains(t, err.Error(), "candidate_scale_exceeded")
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestPreviewPipelineNodeRunsUpstreamChain(t *testing.T) {
	repo := newFakePipelineAuthoringRepo()
	restore := SetPipelineAuthoringRepository(repo)
	defer restore()

	createRR := httptest.NewRecorder()
	CreatePipeline(createRR, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines", bytes.NewReader([]byte(`{
		"name": "trail effort preview",
		"nodes": [
			{
				"id": "source_trails",
				"label": "Trail source",
				"transform_type": "external",
				"config": {
					"rows": [
						{"trail": "Anemone", "distance": 2.9, "gain": 700},
						{"trail": "Mesa", "distance": 6.0, "gain": 900},
						{"trail": "Betasso", "distance": 6.4, "gain": 775}
					]
				}
			},
			{
				"id": "filter_trails",
				"label": "Filter runnable trails",
				"transform_type": "filter",
				"depends_on": ["source_trails"],
				"config": {"predicate": "distance > 3"}
			},
			{
				"id": "select_trails",
				"label": "Select preview columns",
				"transform_type": "select",
				"depends_on": ["filter_trails"],
				"config": {"columns": ["trail", "distance"]}
			},
			{
				"id": "output_trails",
				"label": "Trail output",
				"transform_type": "output_dataset",
				"depends_on": ["select_trails"]
			}
		]
	}`))))
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createRR.Body).Decode(&created))

	req := requestWithPreviewURLParams(http.MethodGet, "/api/v1/pipelines/"+created.ID+"/nodes/output_trails/preview?sample_size=10", nil, created.ID, "output_trails")
	rr := httptest.NewRecorder()
	PreviewPipelineNode(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var payload pipelineNodePreviewResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, created.ID, payload.PipelineID)
	require.Equal(t, "output_trails", payload.NodeID)
	require.True(t, payload.Fresh)
	require.Nil(t, payload.Error)
	require.Equal(t, []string{"distance", "trail"}, payload.Columns)
	require.Equal(t, []string{"source_trails", "filter_trails", "select_trails", "output_trails"}, payload.SourceChain)
	require.Len(t, payload.Rows, 2)
	require.JSONEq(t, `"Mesa"`, string(payload.Rows[0]["trail"]))
	require.JSONEq(t, `6`, string(payload.Rows[0]["distance"]))
	require.JSONEq(t, `"Betasso"`, string(payload.Rows[1]["trail"]))
	require.NotContains(t, payload.Rows[0], "gain")
}

func TestPreviewPipelineNodeReturnsTypedTransformErrors(t *testing.T) {
	restore := SetPipelineAuthoringRepository(nil)
	defer restore()

	body := []byte(`{
		"sample_size": 25,
		"nodes": [
			{
				"id": "source_trails",
				"label": "Trail source",
				"transform_type": "external",
				"config": {"rows": [{"trail": "Mesa", "distance": 6}]}
			},
			{
				"id": "filter_trails",
				"label": "Bad filter",
				"transform_type": "filter",
				"depends_on": ["source_trails"],
				"config": {"predicate": "missing_column > 3"}
			}
		]
	}`)
	pipelineID := "11111111-1111-1111-1111-111111111111"
	req := requestWithPreviewURLParams(http.MethodPost, "/api/v1/pipelines/"+pipelineID+"/nodes/filter_trails/preview", bytes.NewReader(body), pipelineID, "filter_trails")
	rr := httptest.NewRecorder()
	PreviewPipelineNode(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	var payload pipelineNodePreviewResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.False(t, payload.Fresh)
	require.NotNil(t, payload.Error)
	require.Equal(t, "transform_failed", payload.Error.Kind)
	require.Equal(t, "filter_trails", payload.Error.NodeID)
	require.Equal(t, "filter", payload.Error.Transform)
	require.Contains(t, payload.Error.Message, "unknown column")
	require.Equal(t, []string{"source_trails", "filter_trails"}, payload.SourceChain)
	require.Equal(t, 25, payload.SampleSize)
}

func TestPreviewPipelineNodeRunsJoinAndOutputFromDraftGraph(t *testing.T) {
	restore := SetPipelineAuthoringRepository(nil)
	defer restore()

	pipelineID := "22222222-2222-2222-2222-222222222222"
	body := []byte(`{
		"sample_size": 10,
		"nodes": [
			{
				"id": "trail_source",
				"label": "Trails",
				"transform_type": "external",
				"config": {
					"rows": [
						{"trail_id": "mesa", "trail": "Mesa Trail"},
						{"trail_id": "betasso", "trail": "Betasso Preserve"}
					]
				}
			},
			{
				"id": "coffee_source",
				"label": "Coffee",
				"transform_type": "external",
				"config": {
					"rows": [
						{"trail_id": "mesa", "name": "Post-run Coffee"},
						{"trail_id": "betasso", "name": "Canyon Espresso"}
					]
				}
			},
			{
				"id": "join_coffee",
				"label": "Join coffee",
				"transform_type": "sql",
				"depends_on": ["trail_source", "coffee_source"],
				"config": {
					"_join": {
						"join_type": "left",
						"matches": [{"left_column": "trail_id", "right_column": "trail_id"}],
						"auto_select_left": true,
						"auto_select_right": true,
						"right_prefix": "coffee_"
					}
				}
			},
			{
				"id": "output_joined",
				"label": "Joined output",
				"transform_type": "output_dataset",
				"depends_on": ["join_coffee"]
			}
		]
	}`)
	req := requestWithPreviewURLParams(http.MethodPost, "/api/v1/pipelines/"+pipelineID+"/nodes/output_joined/preview", bytes.NewReader(body), pipelineID, "output_joined")
	rr := httptest.NewRecorder()
	PreviewPipelineNode(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var payload pipelineNodePreviewResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
	require.Equal(t, []string{"coffee_name", "coffee_trail_id", "trail", "trail_id"}, payload.Columns)
	require.Equal(t, []string{"trail_source", "coffee_source", "join_coffee", "output_joined"}, payload.SourceChain)
	require.Len(t, payload.Rows, 2)
	require.JSONEq(t, `"Post-run Coffee"`, string(payload.Rows[0]["coffee_name"]))
	require.JSONEq(t, `"Mesa Trail"`, string(payload.Rows[0]["trail"]))
}

func requestWithPreviewURLParams(method string, target string, body io.Reader, pipelineID string, nodeID string) *http.Request {
	req := httptest.NewRequest(method, target, body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", pipelineID)
	rctx.URLParams.Add("node_id", nodeID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

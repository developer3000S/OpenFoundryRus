package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const expectedTransformCatalogSnapshotSHA256 = "fa05b607279998ceae6dde0fe1cc37aa45ead80486ecfec108728fbfeae1feac"

func TestListPipelineTransformCatalogSnapshot(t *testing.T) {
	rr := httptest.NewRecorder()
	ListPipelineTransformCatalog(rr, httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/transforms/catalog", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

	body := strings.TrimSpace(rr.Body.String())
	sum := sha256.Sum256([]byte(body))
	got := hex.EncodeToString(sum[:])
	if expectedTransformCatalogSnapshotSHA256 == "TODO" {
		t.Fatalf("update expectedTransformCatalogSnapshotSHA256 to %s", got)
	}
	require.Equal(t, expectedTransformCatalogSnapshotSHA256, got)
}

func TestTransformCatalogContainsRequiredPB5Entries(t *testing.T) {
	response := transformCatalogV1()
	require.Equal(t, pipelineTransformCatalogVersion, response.SchemaVersion)
	require.NotEmpty(t, response.Categories)

	seen := map[string]pipelineTransformCatalogEntry{}
	for _, entry := range response.Transforms {
		require.NotEmpty(t, entry.ID)
		require.NotEmpty(t, entry.Label)
		require.NotEmpty(t, entry.Category)
		require.NotEmpty(t, entry.TransformType)
		require.NotEmpty(t, entry.ConfigKind)
		require.NotEmpty(t, entry.BuilderSurface)
		require.NotEmpty(t, entry.ExecutionStatus)
		require.NotEmpty(t, entry.Runtime)
		require.NotEmpty(t, entry.Form.Kind)
		require.NotEmpty(t, entry.Form.Fields, entry.ID)
		require.NotEmpty(t, entry.OutputContract.Mode)
		seen[entry.ID] = entry
	}

	for _, id := range []string{
		"select",
		"drop",
		"rename",
		"cast",
		"filter",
		"formula",
		"normalize_units",
		"derive_column",
		"aggregate",
		"sort",
		"join",
		"union",
		"haversine_distance",
		"geo_intersection_join",
		"geo_distance_join",
		"geo_nearest_neighbor_join",
		"explode",
		"json_extract",
		"csv_parse",
		"gpx_parse",
		"python_transform",
		"llm_node",
		"output_mapping",
	} {
		_, ok := seen[id]
		require.True(t, ok, "missing transform %s", id)
	}

	require.Equal(t, "available", seen["join"].ExecutionStatus)
	require.Equal(t, "join_editor", seen["join"].BuilderSurface)
	require.Equal(t, "available", seen["haversine_distance"].ExecutionStatus)
	require.Equal(t, "transform_stack", seen["haversine_distance"].BuilderSurface)
	require.Equal(t, "available", seen["geo_nearest_neighbor_join"].ExecutionStatus)
	require.Equal(t, "geo_join_editor", seen["geo_nearest_neighbor_join"].BuilderSurface)
	require.Equal(t, "available", seen["gpx_parse"].ExecutionStatus)
	require.Equal(t, "gpx_parser", seen["gpx_parse"].BuilderSurface)
	require.Equal(t, "available", seen["output_mapping"].ExecutionStatus)
	require.Equal(t, "output_drawer", seen["output_mapping"].BuilderSurface)
}

func TestTransformCatalogJSONRoundTrips(t *testing.T) {
	raw, err := json.Marshal(transformCatalogV1())
	require.NoError(t, err)
	var decoded pipelineTransformCatalogResponse
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, transformCatalogV1().SchemaVersion, decoded.SchemaVersion)
	require.Len(t, decoded.Transforms, len(transformCatalogV1().Transforms))
	require.Len(t, decoded.Categories, len(transformCatalogV1().Categories))
}

package handler

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestSchemaGuidanceJoinGolden(t *testing.T) {
	trails := mustReadJSONRows(t, "trails.json")
	coffee := mustReadCSVRows(t, "coffee.csv")
	ir := models.NewPipelineIRFromNodes([]models.PipelineNode{
		{ID: "trails", TransformType: "external", Config: mustRowsConfig(t, trails)},
		{ID: "coffee", TransformType: "external", Config: mustRowsConfig(t, coffee)},
	})
	response, err := schemaGuidanceFromIR("draft", ir, pipelineSchemaGuidanceRequest{
		Kind:        "join",
		LeftNodeID:  "trails",
		RightNodeID: "coffee",
		Join:        &runtimeJoinDraft{JoinType: "left", Matches: []runtimeJoinMatch{{LeftColumn: "trail_id", RightColumn: "trail_id"}}},
	})
	require.NoError(t, err)
	require.NotNil(t, response.Join)

	actual := map[string]any{
		"candidate_keys":    response.Join.CandidateKeys[:2],
		"match_diagnostics": response.Join.MatchDiagnostics,
	}
	assertGoldenJSON(t, "join_guidance.golden.json", actual)
}

func TestSchemaGuidanceUnionGolden(t *testing.T) {
	trails := mustReadJSONRows(t, "trails.json")
	efforts := mustReadJSONRows(t, "trail_efforts.json")
	ir := models.NewPipelineIRFromNodes([]models.PipelineNode{
		{ID: "trails", TransformType: "external", Config: mustRowsConfig(t, trails)},
		{ID: "efforts", TransformType: "external", Config: mustRowsConfig(t, efforts)},
	})
	response, err := schemaGuidanceFromIR("draft", ir, pipelineSchemaGuidanceRequest{
		Kind:         "union",
		InputNodeIDs: []string{"trails", "efforts"},
		Union:        &runtimeUnionDraft{UnionType: "by_name"},
	})
	require.NoError(t, err)
	require.NotNil(t, response.Union)

	actual := map[string]any{
		"input_node_ids": response.Union.InputNodeIDs,
		"union_type":     response.Union.UnionType,
		"diagnostics":    response.Union.Diagnostics,
		"output_schema":  response.Union.OutputSchema,
	}
	assertGoldenJSON(t, "union_guidance.golden.json", actual)
}

func TestPipelineSchemaGuidanceEndpoint(t *testing.T) {
	body := []byte(`{
		"kind": "join",
		"left_node_id": "trails",
		"right_node_id": "coffee",
		"nodes": [
			{"id":"trails","transform_type":"external","config":{"rows":[{"trail_code":"mesa","trail_id":1}]}},
			{"id":"coffee","transform_type":"external","config":{"rows":[{"trail_code":"mesa","trail_id":"1"}]}}
		],
		"join": {"join_type": "left", "matches": [{"left_column":"trail_code","right_column":"trail_code"}]}
	}`)
	rr := httptest.NewRecorder()
	PipelineSchemaGuidance(rr, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/_schema-guidance", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, rr.Code)
	var response pipelineSchemaGuidanceResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.True(t, response.Valid)
	require.NotNil(t, response.Join)
	require.NotEmpty(t, response.Join.CandidateKeys)
	require.Equal(t, "trail_code", response.Join.CandidateKeys[0].LeftColumn)
}

func mustReadJSONRows(t *testing.T, name string) []map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "schema_guidance", name))
	require.NoError(t, err)
	var rows []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &rows))
	return rows
}

func mustReadCSVRows(t *testing.T, name string) []map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "schema_guidance", name))
	require.NoError(t, err)
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)
	header := records[0]
	rows := make([]map[string]json.RawMessage, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]json.RawMessage{}
		for i, name := range header {
			value := ""
			if i < len(record) {
				value = record[i]
			}
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			row[name] = encoded
		}
		rows = append(rows, row)
	}
	return rows
}

func mustRowsConfig(t *testing.T, rows []map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"rows": rows})
	require.NoError(t, err)
	return raw
}

func assertGoldenJSON(t *testing.T, name string, actual any) {
	t.Helper()
	raw, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err)
	raw = append(raw, '\n')
	path := filepath.Join("testdata", "schema_guidance", name)
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s; actual:\n%s", path, raw)
	}
	require.JSONEq(t, string(expected), string(raw))
}

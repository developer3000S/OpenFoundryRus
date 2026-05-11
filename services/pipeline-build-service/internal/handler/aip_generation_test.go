package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestPipelineAIPGenerateUsesInjectedGeneratorAndReturnsPreview(t *testing.T) {
	generator := &recordingPipelineAIPGenerator{
		result: PipelineAIPGenerateResult{
			Description: "Keep runnable trails.",
			Nodes: []models.PipelineNode{
				{ID: "aip_filter", Label: "Long trails", TransformType: "filter", Config: json.RawMessage(`{"predicate":"distance > 3"}`)},
			},
		},
	}
	restore := SetExecutionPorts(ExecutionPorts{AIP: generator})
	defer restore()

	body := []byte(`{
		"prompt": "Keep trails longer than three miles",
		"sample_size": 10,
		"selected_node_ids": ["source"],
		"nodes": [
			{
				"id": "source",
				"label": "Trail source",
				"transform_type": "external",
				"config": {
					"rows": [
						{"trail":"Anemone","distance":2.9},
						{"trail":"Mesa","distance":5.2}
					]
				}
			}
		]
	}`)
	rr := httptest.NewRecorder()
	PipelineAIPGenerate(rr, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/aip/generate", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, rr.Code)

	var response PipelineAIPGenerateResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.Equal(t, "Keep runnable trails.", response.Description)
	require.Equal(t, []string{"source"}, response.SelectedNodeIDs)
	require.Len(t, response.Nodes, 1)
	require.Equal(t, []string{"source"}, response.Nodes[0].DependsOn)
	require.NotNil(t, response.Preview)
	require.Nil(t, response.PreviewError)
	require.Equal(t, "aip_filter", response.Preview.NodeID)
	require.Len(t, response.Preview.Rows, 1)
	require.JSONEq(t, `"Mesa"`, string(response.Preview.Rows[0]["trail"]))
	require.Equal(t, "Keep trails longer than three miles", generator.seen.Prompt)
	require.Len(t, generator.seen.Nodes, 1)
}

func TestAIServicePipelineAIPGeneratorParsesJSONNodeResponse(t *testing.T) {
	var sawPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/ai/chat/completions", r.URL.Path)
		var req aiChatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		sawPrompt = req.UserMessage
		writeJSON(w, http.StatusOK, map[string]any{
			"reply": `{
				"description": "Generated an LLM summary node.",
				"nodes": [
					{
						"id": "aip_llm",
						"label": "Summarize trails",
						"transform_type": "llm",
						"depends_on": ["source"],
						"config": {"prompt":"Summarize {{trail}}","output_column":"summary"}
					}
				]
			}`,
			"provider_name": "mock-ai",
		})
	}))
	defer server.Close()

	generator := NewAIServicePipelineAIPGenerator(PipelineAIPGeneratorConfig{BaseURL: server.URL, Client: server.Client()})
	result, err := generator.GeneratePipelineTransform(context.Background(), PipelineAIPGenerateRequest{
		Prompt:          "Add an LLM summary",
		SelectedNodeIDs: []string{"source"},
		Nodes: []models.PipelineNode{
			{ID: "source", Label: "Trail source", TransformType: "external", Config: json.RawMessage(`{"rows":[{"trail":"Mesa"}]}`)},
		},
	})
	require.NoError(t, err)
	require.Contains(t, sawPrompt, "Add an LLM summary")
	require.Equal(t, "Generated an LLM summary node.", result.Description)
	require.Equal(t, "mock-ai", result.ProviderName)
	require.Len(t, result.Nodes, 1)
	require.Equal(t, "llm", result.Nodes[0].TransformType)
	require.JSONEq(t, `{"prompt":"Summarize {{trail}}","output_column":"summary"}`, string(result.Nodes[0].Config))
}

func TestPipelineAIPProviderSmokeGated(t *testing.T) {
	if os.Getenv("OPENFOUNDRY_PIPELINE_AIP_PROVIDER_SMOKE") != "1" {
		t.Skip("set OPENFOUNDRY_PIPELINE_AIP_PROVIDER_SMOKE=1 with AI_SERVICE_URL to run provider smoke")
	}
	baseURL := strings.TrimSpace(os.Getenv("AI_SERVICE_URL"))
	require.NotEmpty(t, baseURL, "AI_SERVICE_URL is required for provider smoke")
	generator := NewAIServicePipelineAIPGenerator(PipelineAIPGeneratorConfig{BaseURL: baseURL, BearerToken: os.Getenv("AI_SERVICE_BEARER")})
	result, err := generator.GeneratePipelineTransform(context.Background(), PipelineAIPGenerateRequest{
		Prompt:          "Create a filter node that keeps rows where distance is greater than 3.",
		SelectedNodeIDs: []string{"source"},
		Nodes: []models.PipelineNode{
			{ID: "source", Label: "Trail source", TransformType: "external", Config: json.RawMessage(`{"rows":[{"trail":"Mesa","distance":5}]}`)},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Nodes)
}

type recordingPipelineAIPGenerator struct {
	seen   PipelineAIPGenerateRequest
	result PipelineAIPGenerateResult
	err    error
}

func (g *recordingPipelineAIPGenerator) GeneratePipelineTransform(_ context.Context, req PipelineAIPGenerateRequest) (PipelineAIPGenerateResult, error) {
	g.seen = req
	return g.result, g.err
}

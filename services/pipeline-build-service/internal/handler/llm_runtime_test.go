package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestRuntimeNodeRunnerLLMUsesUpstreamRowsAndStoresResultRows(t *testing.T) {
	ctx := context.Background()
	buildID := uuid.New()
	rt := newLightweightTableRuntime()
	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, json.RawMessage(`{
		"rows":[{"trail":"Mesa","distance":5}]
	}`), "dataset_input")
	require.NoError(t, err)

	llm := &recordingLLMTransformRunner{
		result: executor.NodeResult{
			OutputContentHash: "sha256:llm",
			Metadata: map[string]any{
				"runtime":       "llm",
				"engine":        "fake",
				"rows_affected": 1,
				"columns":       []string{"distance", "llm_output", "trail"},
				"data_rows": []map[string]json.RawMessage{
					{"trail": mustRuntimeJSON("Mesa"), "distance": mustRuntimeJSON(5), "llm_output": mustRuntimeJSON("steady effort")},
				},
			},
		},
	}
	runner := runtimeNodeRunner{Table: rt, LLM: llm}
	result, err := runner.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{
		ID:        "llm_summary",
		DependsOn: []string{"input"},
		Metadata: map[string]any{
			"logic_kind":     "transform",
			"transform_type": "llm",
			"logic_payload":  json.RawMessage(`{"prompt":"Summarize {{trail}}","output_column":"llm_output"}`),
			"sample_size":    10,
		},
	}})
	require.NoError(t, err)
	require.Equal(t, "sha256:llm", result.OutputContentHash)
	require.Equal(t, "llm", llm.seen.TransformType)
	require.Equal(t, 10, llm.seen.SampleSize)
	require.Len(t, llm.seen.InputRows, 1)
	require.JSONEq(t, `"Mesa"`, string(llm.seen.InputRows[0]["trail"]))

	rows := rt.snapshotRows("llm_summary")
	require.Len(t, rows, 1)
	require.JSONEq(t, `"steady effort"`, string(rows[0]["llm_output"]))
	require.JSONEq(t, `5`, string(rows[0]["distance"]))
}

func TestAIServiceLLMRunnerCallsAIServiceAndAddsOutputColumn(t *testing.T) {
	var sawAuth bool
	var sawPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/ai/chat/completions", r.URL.Path)
		sawAuth = r.Header.Get("authorization") == "Bearer test-token"
		var req aiChatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		sawPrompt = req.UserMessage
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":         "flowy singletrack",
			"provider_name": "mock-ai",
			"usage": map[string]int{
				"prompt_tokens":     4,
				"completion_tokens": 2,
				"total_tokens":      6,
			},
		})
	}))
	defer server.Close()

	runner := NewAIServiceLLMRunner(AIServiceLLMConfig{BaseURL: server.URL, BearerToken: "test-token", Client: server.Client()})
	result, err := runner.RunLLMTransform(context.Background(), LLMTransformRequest{
		Node:          executor.NodeContext{Node: executor.Node{ID: "llm_node"}},
		Payload:       json.RawMessage(`{"prompt":"Describe {{trail}}","output_column":"summary","max_rows":1,"max_tokens":32}`),
		TransformType: "llm",
		InputRows: []map[string]json.RawMessage{
			{"trail": mustRuntimeJSON("Mesa Trail"), "distance": mustRuntimeJSON(5.2)},
		},
	})
	require.NoError(t, err)
	require.True(t, sawAuth)
	require.Contains(t, sawPrompt, "Mesa Trail")
	require.Equal(t, "mock-ai", result.Metadata["provider_name"])
	require.Equal(t, 1, result.Metadata["rows_affected"])

	rows := rowsFromLLMResult(result)
	require.Len(t, rows, 1)
	require.JSONEq(t, `"flowy singletrack"`, string(rows[0]["summary"]))
	require.JSONEq(t, `"Mesa Trail"`, string(rows[0]["trail"]))
}

func TestAIServiceLLMProviderSmokeGated(t *testing.T) {
	if os.Getenv("OPENFOUNDRY_PIPELINE_LLM_PROVIDER_SMOKE") != "1" {
		t.Skip("set OPENFOUNDRY_PIPELINE_LLM_PROVIDER_SMOKE=1 with AI_SERVICE_URL to run provider smoke")
	}
	baseURL := strings.TrimSpace(os.Getenv("AI_SERVICE_URL"))
	require.NotEmpty(t, baseURL, "AI_SERVICE_URL is required for provider smoke")
	runner := NewAIServiceLLMRunner(AIServiceLLMConfig{BaseURL: baseURL, BearerToken: os.Getenv("AI_SERVICE_BEARER")})
	result, err := runner.RunLLMTransform(context.Background(), LLMTransformRequest{
		Node:          executor.NodeContext{Node: executor.Node{ID: "llm_provider_smoke"}},
		Payload:       json.RawMessage(`{"prompt":"Return exactly: ok","output_column":"result","max_rows":1,"max_tokens":8}`),
		TransformType: "llm",
		InputRows:     []map[string]json.RawMessage{{"trail": mustRuntimeJSON("Mesa")}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Metadata["rows_affected"])
	require.NotEmpty(t, rowsFromLLMResult(result))
}

type recordingLLMTransformRunner struct {
	seen   LLMTransformRequest
	result executor.NodeResult
	err    error
}

func (r *recordingLLMTransformRunner) RunLLMTransform(_ context.Context, req LLMTransformRequest) (executor.NodeResult, error) {
	r.seen = req
	return r.result, r.err
}

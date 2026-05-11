package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestPipelineTransformCatalogIncludesVersionedFunctions(t *testing.T) {
	registry := &fakePipelineFunctionRegistry{}
	registry.set([]PipelineFunctionDefinition{
		{
			ID:          "fn.trail_effort",
			Name:        "trail_effort",
			DisplayName: "Trail effort",
			Version:     "1.0.0",
			Runtime:     PipelineFunctionRuntimeExpression,
			Parameters: []PipelineFunctionParameter{
				{Name: "distance_miles", Type: "Double", Required: true},
				{Name: "elevation_gain_ft", Type: "Double", Required: true},
			},
			ResultType: "Double",
			Expression: "distance_miles + (elevation_gain_ft / 1000)",
		},
		{
			ID:         "fn.parse_gpx",
			Name:       "parse_gpx",
			Version:    "0.2.0",
			Runtime:    PipelineFunctionRuntimePython,
			ResultType: "String",
			Source:     "def compute(gpx): return gpx",
		},
	})
	restore := SetExecutionPorts(ExecutionPorts{Functions: registry})
	defer restore()

	rr := httptest.NewRecorder()
	ListPipelineTransformCatalog(rr, httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/transforms/catalog", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	var response pipelineTransformCatalogResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))

	seen := map[string]pipelineTransformCatalogEntry{}
	for _, entry := range response.Transforms {
		seen[entry.ID] = entry
	}
	require.Contains(t, seen, "function.trail_effort@1.0.0")
	require.Contains(t, seen, "function.parse_gpx@0.2.0")
	require.Equal(t, "function", seen["function.trail_effort@1.0.0"].TransformType)
	require.Equal(t, "available", seen["function.trail_effort@1.0.0"].ExecutionStatus)
	require.Equal(t, "requires_python_sidecar", seen["function.parse_gpx@0.2.0"].ExecutionStatus)
	require.Equal(t, "trail_effort", seen["function.trail_effort@1.0.0"].Function.Name)
	require.Equal(t, "1.0.0", seen["function.trail_effort@1.0.0"].Function.Version)
}

func TestReusableExpressionFunctionRunsAndAutoUpgradesVersion(t *testing.T) {
	registry := &fakePipelineFunctionRegistry{}
	registry.set([]PipelineFunctionDefinition{trailEffortFunction("1.0.0", "distance_miles + (elevation_gain_ft / 1000)")})
	rt := newLightweightTableRuntime(registry)
	buildID := uuid.New()
	ctx := context.Background()

	_, err := rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "input"}}, json.RawMessage(`{
		"rows":[{"trail_id":"mesa","distance_miles":5,"elevation_gain_ft":1000}]
	}`), "dataset_input")
	require.NoError(t, err)

	cfg := json.RawMessage(`{
		"function_name":"trail_effort",
		"function_auto_upgrade":true,
		"target_column":"effort_score"
	}`)
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "effort_v1", DependsOn: []string{"input"}}}, cfg, "function")
	require.NoError(t, err)
	rows := rt.snapshotRows("effort_v1")
	require.Len(t, rows, 1)
	require.JSONEq(t, `6`, string(rows[0]["effort_score"]))

	registry.set([]PipelineFunctionDefinition{
		trailEffortFunction("1.0.0", "distance_miles + (elevation_gain_ft / 1000)"),
		trailEffortFunction("2.0.0", "distance_miles * 2"),
	})
	_, err = rt.Run(ctx, executor.NodeContext{BuildID: buildID, Node: executor.Node{ID: "effort_v2", DependsOn: []string{"input"}}}, cfg, "function")
	require.NoError(t, err)
	rows = rt.snapshotRows("effort_v2")
	require.Len(t, rows, 1)
	require.JSONEq(t, `10`, string(rows[0]["effort_score"]))
}

func trailEffortFunction(version, expression string) PipelineFunctionDefinition {
	return PipelineFunctionDefinition{
		ID:      "fn.trail_effort",
		Name:    "trail_effort",
		Version: version,
		Runtime: PipelineFunctionRuntimeExpression,
		Parameters: []PipelineFunctionParameter{
			{Name: "distance_miles", Type: "Integer", Required: true},
			{Name: "elevation_gain_ft", Type: "Integer", Required: true},
		},
		ResultType:  "Integer",
		Expression:  expression,
		Description: "Estimated trail effort from distance and elevation gain.",
	}
}

type fakePipelineFunctionRegistry struct {
	mu   sync.Mutex
	defs []PipelineFunctionDefinition
	err  error
}

func (r *fakePipelineFunctionRegistry) set(defs []PipelineFunctionDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs = append([]PipelineFunctionDefinition(nil), defs...)
}

func (r *fakePipelineFunctionRegistry) ListPipelineFunctions(context.Context) ([]PipelineFunctionDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return append([]PipelineFunctionDefinition(nil), r.defs...), nil
}

func (r *fakePipelineFunctionRegistry) ResolvePipelineFunction(_ context.Context, ref PipelineFunctionRef) (PipelineFunctionDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return PipelineFunctionDefinition{}, r.err
	}
	var latest *PipelineFunctionDefinition
	for i := range r.defs {
		def := normalisePipelineFunction(r.defs[i])
		if ref.ID != "" && def.ID != ref.ID {
			continue
		}
		if ref.Name != "" && def.Name != ref.Name {
			continue
		}
		if ref.Version != "" && def.Version != ref.Version {
			continue
		}
		if latest == nil || def.Version > latest.Version {
			copy := def
			latest = &copy
		}
	}
	if latest == nil {
		return PipelineFunctionDefinition{}, errors.New("pipeline_function_not_found")
	}
	return *latest, nil
}

package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPipelineIRRoundTripPreservesAuthoringAndRuntimeSemantics(t *testing.T) {
	inputDatasetID := uuid.New()
	outputDatasetID := uuid.New()
	raw := json.RawMessage(`{
		"ir_version":"pipeline_ir.v1",
		"nodes":[{
			"id":"filter_recent",
			"label":"Filter recent runs",
			"transform_type":"filter",
			"config":{"expression":"distance_miles > 3"},
			"depends_on":["source_runs"],
			"input_dataset_ids":["` + inputDatasetID.String() + `"],
			"output_dataset_id":"` + outputDatasetID.String() + `",
			"input_ports":[{"id":"in","direction":"input","port_type":"dataset"}],
			"output_ports":[{"id":"out","direction":"output","port_type":"dataset"}],
			"output_schema":{"fields":[{"name":"distance_miles","field_type":"double","nullable":false}]},
			"preview_schema":{"fields":[{"name":"distance_miles","field_type":"double","nullable":false}]},
			"position":{"x":160,"y":80},
			"metadata":{"ui_state":{"collapsed":false}}
		},{
			"id":"source_runs",
			"label":"Runs",
			"transform_type":"dataset_input",
			"config":{"dataset_rid":"ri.dataset.main.runs"},
			"output_ports":[{"id":"out","direction":"output","port_type":"dataset"}]
		}],
		"edges":[{"id":"source_runs->filter_recent","source_node_id":"source_runs","source_port_id":"out","target_node_id":"filter_recent","target_port_id":"in","edge_type":"data_dependency"}],
		"resources":[{"id":"runs_dataset","rid":"ri.dataset.main.runs","resource_type":"dataset","branch":"master"}],
		"inputs":[{"id":"runs","resource_id":"runs_dataset"}],
		"outputs":[{"id":"filtered_runs","output_type":"dataset_output","produced_by":"filter_recent"}],
		"version_metadata":{"authoring_version":7,"graph_hash":"sha256:test","created_from":"unit-test"}
	}`)

	ir, err := ParsePipelineIR(raw)
	require.NoError(t, err)
	require.Equal(t, PipelineIRVersion, ir.Version)
	require.Len(t, ir.Nodes, 2)
	require.Equal(t, "filter_recent", ir.Nodes[0].ID)
	require.Equal(t, "filter", ir.Nodes[0].TransformType)
	require.JSONEq(t, `{"expression":"distance_miles > 3"}`, string(ir.Nodes[0].Config))
	require.Equal(t, outputDatasetID, *ir.Nodes[0].OutputDatasetID)
	require.Equal(t, "valid", ir.Validation.Status)
	require.Equal(t, 7, ir.VersionMeta.AuthoringVersion)

	encoded, err := json.Marshal(ir)
	require.NoError(t, err)
	var decoded PipelineIR
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, ir.Nodes[0].Position, decoded.Nodes[0].Position)
	require.JSONEq(t, string(ir.Nodes[0].Metadata["ui_state"]), string(decoded.Nodes[0].Metadata["ui_state"]))

	legacyNodes := decoded.LegacyNodes()
	require.Len(t, legacyNodes, 2)
	require.Equal(t, []string{"source_runs"}, legacyNodes[0].DependsOn)
	require.Equal(t, []uuid.UUID{inputDatasetID}, legacyNodes[0].InputDatasetIDs)
}

func TestPipelineIRParsesLegacyNodeArrayAndDerivesEdges(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"source","label":"Source","transform_type":"dataset_input","config":{"dataset_rid":"ri.dataset.main.source"}},
		{"id":"select","label":"Select","transform_type":"select","depends_on":["source"],"config":{"columns":["id"]}}
	]`)

	ir, err := ParsePipelineIR(raw)
	require.NoError(t, err)
	require.Equal(t, PipelineIRVersion, ir.Version)
	require.Equal(t, "legacy_nodes", ir.VersionMeta.CreatedFrom)
	require.Len(t, ir.Edges, 1)
	require.Equal(t, "source->select", ir.Edges[0].ID)
	require.Equal(t, "valid", ir.Validation.Status)
}

func TestPipelineIRValidationRejectsDuplicateMissingDependencyAndCycle(t *testing.T) {
	duplicate := NewPipelineIRFromNodes([]PipelineNode{
		{ID: "a", TransformType: "dataset_input"},
		{ID: "a", TransformType: "filter"},
	})
	report := duplicate.Validate()
	require.False(t, report.Valid())
	require.Contains(t, report.Error(), "duplicate pipeline node id")

	missing := NewPipelineIRFromNodes([]PipelineNode{
		{ID: "b", TransformType: "filter", DependsOn: []string{"missing"}},
	})
	report = missing.Validate()
	require.False(t, report.Valid())
	require.Contains(t, report.Error(), "depends on missing node")
	require.Equal(t, "invalid", report.NodeState("b").Status)

	cyclic := NewPipelineIRFromNodes([]PipelineNode{
		{ID: "a", TransformType: "filter", DependsOn: []string{"b"}},
		{ID: "b", TransformType: "filter", DependsOn: []string{"a"}},
	})
	report = cyclic.Validate()
	require.False(t, report.Valid())
	require.Contains(t, report.Error(), "cycle detected")
}

func TestPipelineIRValidationRejectsEdgesWithMissingPorts(t *testing.T) {
	ir := PipelineIR{
		Version: PipelineIRVersion,
		Nodes: []PipelineIRNode{
			{ID: "source", TransformType: "dataset_input", Config: json.RawMessage(`{}`), OutputPorts: []PipelineIRPort{{ID: "out", Direction: "output"}}},
			{ID: "filter", TransformType: "filter", Config: json.RawMessage(`{}`), DependsOn: []string{"source"}, InputPorts: []PipelineIRPort{{ID: "in", Direction: "input"}}},
		},
		Edges: []PipelineIREdge{{
			ID:           "source->filter",
			SourceNodeID: "source",
			SourcePortID: "missing_out",
			TargetNodeID: "filter",
			TargetPortID: "in",
		}},
	}

	report := ir.Normalize().Validate()
	require.False(t, report.Valid())
	require.Contains(t, report.Error(), "missing source port")
}

func TestCreatePipelineRequestCanonicalDAGAcceptsIRAndNodes(t *testing.T) {
	raw, err := (CreatePipelineRequest{
		Name: "trail",
		Nodes: []PipelineNode{
			{ID: "source", TransformType: "dataset_input"},
			{ID: "output", TransformType: "dataset_output", DependsOn: []string{"source"}},
		},
	}).CanonicalDAG()
	require.NoError(t, err)
	var fromNodes PipelineIR
	require.NoError(t, json.Unmarshal(raw, &fromNodes))
	require.Len(t, fromNodes.Edges, 1)

	raw, err = (CreatePipelineRequest{
		Name: "trail",
		IR: &PipelineIR{
			Version: PipelineIRVersion,
			Nodes:   []PipelineIRNode{{ID: "source", TransformType: "dataset_input", Config: json.RawMessage(`{}`)}},
			VersionMeta: PipelineIRVersionMetadata{
				AuthoringVersion: 3,
				CreatedFrom:      "workshop-canvas",
			},
		},
	}).CanonicalDAG()
	require.NoError(t, err)
	var fromIR PipelineIR
	require.NoError(t, json.Unmarshal(raw, &fromIR))
	require.Equal(t, 3, fromIR.VersionMeta.AuthoringVersion)
	require.Equal(t, "workshop-canvas", fromIR.VersionMeta.CreatedFrom)
}

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const PipelineIRVersion = "pipeline_ir.v1"

type PipelineIR struct {
	Version     string                     `json:"ir_version"`
	Nodes       []PipelineIRNode           `json:"nodes"`
	Edges       []PipelineIREdge           `json:"edges,omitempty"`
	Resources   []PipelineIRResource       `json:"resources,omitempty"`
	Inputs      []PipelineIRInput          `json:"inputs,omitempty"`
	Outputs     []PipelineIROutput         `json:"outputs,omitempty"`
	Validation  PipelineIRValidationState  `json:"validation"`
	VersionMeta PipelineIRVersionMetadata  `json:"version_metadata"`
	Metadata    map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRNode struct {
	ID              string                     `json:"id"`
	Label           string                     `json:"label"`
	TransformType   string                     `json:"transform_type"`
	Config          json.RawMessage            `json:"config"`
	DependsOn       []string                   `json:"depends_on,omitempty"`
	InputDatasetIDs []uuid.UUID                `json:"input_dataset_ids,omitempty"`
	OutputDatasetID *uuid.UUID                 `json:"output_dataset_id,omitempty"`
	InputPorts      []PipelineIRPort           `json:"input_ports,omitempty"`
	OutputPorts     []PipelineIRPort           `json:"output_ports,omitempty"`
	OutputSchema    *PipelineIRSchema          `json:"output_schema,omitempty"`
	PreviewSchema   *PipelineIRSchema          `json:"preview_schema,omitempty"`
	Validation      PipelineIRValidationState  `json:"validation"`
	Position        *PipelineIRNodePosition    `json:"position,omitempty"`
	Metadata        map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRPort struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name,omitempty"`
	Direction    string                     `json:"direction"`
	PortType     string                     `json:"port_type,omitempty"`
	Schema       *PipelineIRSchema          `json:"schema,omitempty"`
	ResourceRefs []string                   `json:"resource_refs,omitempty"`
	Metadata     map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIREdge struct {
	ID           string                     `json:"id"`
	SourceNodeID string                     `json:"source_node_id"`
	SourcePortID string                     `json:"source_port_id,omitempty"`
	TargetNodeID string                     `json:"target_node_id"`
	TargetPortID string                     `json:"target_port_id,omitempty"`
	EdgeType     string                     `json:"edge_type,omitempty"`
	Metadata     map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRResource struct {
	ID           string                     `json:"id"`
	RID          string                     `json:"rid,omitempty"`
	ResourceType string                     `json:"resource_type"`
	Name         string                     `json:"name,omitempty"`
	Branch       string                     `json:"branch,omitempty"`
	Schema       *PipelineIRSchema          `json:"schema,omitempty"`
	Metadata     map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRInput struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name,omitempty"`
	ResourceID string                     `json:"resource_id,omitempty"`
	Schema     *PipelineIRSchema          `json:"schema,omitempty"`
	Metadata   map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIROutput struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name,omitempty"`
	OutputType string                     `json:"output_type"`
	ResourceID string                     `json:"resource_id,omitempty"`
	Schema     *PipelineIRSchema          `json:"schema,omitempty"`
	ProducedBy string                     `json:"produced_by,omitempty"`
	Metadata   map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRSchema struct {
	Fields   []PipelineIRField          `json:"fields"`
	Metadata map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRField struct {
	Name        string                     `json:"name"`
	FieldType   string                     `json:"field_type"`
	Nullable    bool                       `json:"nullable"`
	Description string                     `json:"description,omitempty"`
	Metadata    map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRValidationState struct {
	Status    string                      `json:"status"`
	Errors    []PipelineIRValidationError `json:"errors"`
	UpdatedAt string                      `json:"updated_at,omitempty"`
}

type PipelineIRValidationError struct {
	NodeID  string `json:"node_id,omitempty"`
	EdgeID  string `json:"edge_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PipelineIRVersionMetadata struct {
	AuthoringVersion int                        `json:"authoring_version"`
	GraphHash        string                     `json:"graph_hash,omitempty"`
	CreatedFrom      string                     `json:"created_from,omitempty"`
	UpdatedBy        string                     `json:"updated_by,omitempty"`
	Metadata         map[string]json.RawMessage `json:"metadata,omitempty"`
}

type PipelineIRNodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func NewPipelineIRFromNodes(nodes []PipelineNode) PipelineIR {
	irNodes := make([]PipelineIRNode, 0, len(nodes))
	for _, node := range nodes {
		irNodes = append(irNodes, pipelineIRNodeFromLegacy(node))
	}
	ir := PipelineIR{
		Version: PipelineIRVersion,
		Nodes:   irNodes,
		VersionMeta: PipelineIRVersionMetadata{
			AuthoringVersion: 1,
			CreatedFrom:      "legacy_nodes",
		},
	}
	return ir.Normalize()
}

func ParsePipelineIR(raw json.RawMessage) (PipelineIR, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return NewPipelineIRFromNodes(nil), nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var nodes []PipelineNode
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return PipelineIR{}, fmt.Errorf("decode legacy pipeline nodes: %w", err)
		}
		return NewPipelineIRFromNodes(nodes), nil
	}
	var ir PipelineIR
	if err := json.Unmarshal(raw, &ir); err != nil {
		return PipelineIR{}, fmt.Errorf("decode pipeline IR: %w", err)
	}
	return ir.Normalize(), nil
}

func CanonicalPipelineDAG(raw json.RawMessage) (json.RawMessage, error) {
	ir, err := ParsePipelineIR(raw)
	if err != nil {
		return nil, err
	}
	if report := ir.Validate(); !report.Valid() {
		return nil, report
	}
	return json.Marshal(ir)
}

func CanonicalPipelineDAGFromNodes(nodes []PipelineNode) (json.RawMessage, error) {
	ir := NewPipelineIRFromNodes(nodes)
	if report := ir.Validate(); !report.Valid() {
		return nil, report
	}
	return json.Marshal(ir)
}

func (ir PipelineIR) Normalize() PipelineIR {
	if strings.TrimSpace(ir.Version) == "" {
		ir.Version = PipelineIRVersion
	}
	if ir.VersionMeta.AuthoringVersion == 0 {
		ir.VersionMeta.AuthoringVersion = 1
	}
	for i := range ir.Nodes {
		ir.Nodes[i] = ir.Nodes[i].Normalize()
	}
	if len(ir.Edges) == 0 {
		ir.Edges = derivePipelineIREdges(ir.Nodes)
	}
	report := ir.Validate()
	ir.Validation = report.State()
	for i := range ir.Nodes {
		ir.Nodes[i].Validation = report.NodeState(ir.Nodes[i].ID)
	}
	return ir
}

func (ir PipelineIR) LegacyNodes() []PipelineNode {
	nodes := make([]PipelineNode, 0, len(ir.Nodes))
	for _, node := range ir.Nodes {
		nodes = append(nodes, node.LegacyNode())
	}
	return nodes
}

func (ir PipelineIR) Validate() PipelineIRValidationReport {
	report := PipelineIRValidationReport{ErrorsByNode: map[string][]PipelineIRValidationError{}}
	if ir.Version != "" && ir.Version != PipelineIRVersion {
		report.Add(PipelineIRValidationError{Code: "unsupported_ir_version", Message: fmt.Sprintf("unsupported pipeline IR version %q", ir.Version)})
	}
	ids := map[string]struct{}{}
	inputPortsByNode := map[string]map[string]struct{}{}
	outputPortsByNode := map[string]map[string]struct{}{}
	for _, node := range ir.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			report.Add(PipelineIRValidationError{Code: "node_id_required", Message: "pipeline node id is required"})
			continue
		}
		if _, exists := ids[id]; exists {
			report.Add(PipelineIRValidationError{NodeID: id, Code: "duplicate_node_id", Message: fmt.Sprintf("duplicate pipeline node id %q", id)})
		}
		ids[id] = struct{}{}
		if strings.TrimSpace(node.TransformType) == "" {
			report.Add(PipelineIRValidationError{NodeID: id, Code: "transform_type_required", Message: fmt.Sprintf("pipeline node %q requires transform_type", id)})
		}
		validatePorts(id, node.InputPorts, "input", &report)
		validatePorts(id, node.OutputPorts, "output", &report)
		inputPortsByNode[id] = portIDSet(node.InputPorts)
		outputPortsByNode[id] = portIDSet(node.OutputPorts)
	}
	for _, node := range ir.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := ids[dep]; !ok {
				report.Add(PipelineIRValidationError{NodeID: node.ID, Code: "missing_dependency", Message: fmt.Sprintf("pipeline node %q depends on missing node %q", node.ID, dep)})
			}
		}
	}
	for _, edge := range ir.Edges {
		if strings.TrimSpace(edge.ID) == "" {
			report.Add(PipelineIRValidationError{Code: "edge_id_required", Message: "pipeline edge id is required"})
		}
		if _, ok := ids[edge.SourceNodeID]; !ok {
			report.Add(PipelineIRValidationError{EdgeID: edge.ID, Code: "edge_source_missing", Message: fmt.Sprintf("pipeline edge %q references missing source node %q", edge.ID, edge.SourceNodeID)})
		}
		if _, ok := ids[edge.TargetNodeID]; !ok {
			report.Add(PipelineIRValidationError{EdgeID: edge.ID, Code: "edge_target_missing", Message: fmt.Sprintf("pipeline edge %q references missing target node %q", edge.ID, edge.TargetNodeID)})
		}
		if edge.SourcePortID != "" {
			if _, ok := outputPortsByNode[edge.SourceNodeID][edge.SourcePortID]; !ok {
				report.Add(PipelineIRValidationError{EdgeID: edge.ID, Code: "edge_source_port_missing", Message: fmt.Sprintf("pipeline edge %q references missing source port %q on node %q", edge.ID, edge.SourcePortID, edge.SourceNodeID)})
			}
		}
		if edge.TargetPortID != "" {
			if _, ok := inputPortsByNode[edge.TargetNodeID][edge.TargetPortID]; !ok {
				report.Add(PipelineIRValidationError{EdgeID: edge.ID, Code: "edge_target_port_missing", Message: fmt.Sprintf("pipeline edge %q references missing target port %q on node %q", edge.ID, edge.TargetPortID, edge.TargetNodeID)})
			}
		}
	}
	if len(report.Errors) == 0 {
		if cycle := findPipelineIRCycle(ir.Nodes); len(cycle) > 0 {
			report.Add(PipelineIRValidationError{Code: "cycle_detected", Message: "cycle detected in pipeline DAG: " + strings.Join(cycle, " -> ")})
		}
	}
	return report
}

func (node PipelineIRNode) Normalize() PipelineIRNode {
	node.ID = strings.TrimSpace(node.ID)
	node.TransformType = strings.TrimSpace(node.TransformType)
	if len(node.Config) == 0 {
		node.Config = json.RawMessage(`{}`)
	}
	if node.InputPorts == nil {
		node.InputPorts = []PipelineIRPort{}
	}
	if node.OutputPorts == nil {
		node.OutputPorts = []PipelineIRPort{}
	}
	if node.Validation.Status == "" {
		node.Validation = PipelineIRValidationState{Status: "unknown", Errors: []PipelineIRValidationError{}}
	}
	return node
}

func (node PipelineIRNode) LegacyNode() PipelineNode {
	return PipelineNode{
		ID:              node.ID,
		Label:           node.Label,
		TransformType:   node.TransformType,
		Config:          node.Config,
		DependsOn:       append([]string(nil), node.DependsOn...),
		InputDatasetIDs: append([]uuid.UUID(nil), node.InputDatasetIDs...),
		OutputDatasetID: node.OutputDatasetID,
	}
}

type PipelineIRValidationReport struct {
	Errors       []PipelineIRValidationError
	ErrorsByNode map[string][]PipelineIRValidationError
}

func (r *PipelineIRValidationReport) Add(err PipelineIRValidationError) {
	r.Errors = append(r.Errors, err)
	if err.NodeID != "" {
		if r.ErrorsByNode == nil {
			r.ErrorsByNode = map[string][]PipelineIRValidationError{}
		}
		r.ErrorsByNode[err.NodeID] = append(r.ErrorsByNode[err.NodeID], err)
	}
}

func (r PipelineIRValidationReport) Valid() bool { return len(r.Errors) == 0 }

func (r PipelineIRValidationReport) Error() string {
	if len(r.Errors) == 0 {
		return "pipeline IR validation failed"
	}
	messages := make([]string, 0, len(r.Errors))
	for _, err := range r.Errors {
		messages = append(messages, err.Message)
	}
	return strings.Join(messages, "; ")
}

func (r PipelineIRValidationReport) State() PipelineIRValidationState {
	if r.Valid() {
		return PipelineIRValidationState{Status: "valid", Errors: []PipelineIRValidationError{}}
	}
	return PipelineIRValidationState{Status: "invalid", Errors: append([]PipelineIRValidationError(nil), r.Errors...)}
}

func (r PipelineIRValidationReport) NodeState(nodeID string) PipelineIRValidationState {
	errors := append([]PipelineIRValidationError(nil), r.ErrorsByNode[nodeID]...)
	if len(errors) == 0 {
		return PipelineIRValidationState{Status: "valid", Errors: []PipelineIRValidationError{}}
	}
	return PipelineIRValidationState{Status: "invalid", Errors: errors}
}

func pipelineIRNodeFromLegacy(node PipelineNode) PipelineIRNode {
	cfg := node.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	return PipelineIRNode{
		ID:              node.ID,
		Label:           node.Label,
		TransformType:   node.TransformType,
		Config:          cfg,
		DependsOn:       append([]string(nil), node.DependsOn...),
		InputDatasetIDs: append([]uuid.UUID(nil), node.InputDatasetIDs...),
		OutputDatasetID: node.OutputDatasetID,
		InputPorts:      []PipelineIRPort{},
		OutputPorts:     []PipelineIRPort{},
		Validation:      PipelineIRValidationState{Status: "unknown", Errors: []PipelineIRValidationError{}},
	}
}

func derivePipelineIREdges(nodes []PipelineIRNode) []PipelineIREdge {
	edges := []PipelineIREdge{}
	seen := map[string]struct{}{}
	for _, node := range nodes {
		deps := append([]string(nil), node.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			id := dep + "->" + node.ID
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			edges = append(edges, PipelineIREdge{
				ID:           id,
				SourceNodeID: dep,
				TargetNodeID: node.ID,
				EdgeType:     "data_dependency",
			})
		}
	}
	return edges
}

func validatePorts(nodeID string, ports []PipelineIRPort, direction string, report *PipelineIRValidationReport) {
	ids := map[string]struct{}{}
	for _, port := range ports {
		id := strings.TrimSpace(port.ID)
		if id == "" {
			report.Add(PipelineIRValidationError{NodeID: nodeID, Code: "port_id_required", Message: fmt.Sprintf("%s port id is required on node %q", direction, nodeID)})
			continue
		}
		if _, ok := ids[id]; ok {
			report.Add(PipelineIRValidationError{NodeID: nodeID, Code: "duplicate_port_id", Message: fmt.Sprintf("duplicate %s port %q on node %q", direction, id, nodeID)})
		}
		ids[id] = struct{}{}
		if port.Direction != "" && port.Direction != direction {
			report.Add(PipelineIRValidationError{NodeID: nodeID, Code: "port_direction_mismatch", Message: fmt.Sprintf("port %q on node %q has direction %q, expected %q", id, nodeID, port.Direction, direction)})
		}
	}
}

func portIDSet(ports []PipelineIRPort) map[string]struct{} {
	out := map[string]struct{}{}
	for _, port := range ports {
		if id := strings.TrimSpace(port.ID); id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func findPipelineIRCycle(nodes []PipelineIRNode) []string {
	byID := map[string]PipelineIRNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	stack := []string{}

	var walk func(string) []string
	walk = func(id string) []string {
		if visiting[id] {
			for i, item := range stack {
				if item == id {
					return append(append([]string(nil), stack[i:]...), id)
				}
			}
			return []string{id, id}
		}
		if visited[id] {
			return nil
		}
		node, ok := byID[id]
		if !ok {
			return nil
		}
		visiting[id] = true
		stack = append(stack, id)
		for _, dep := range node.DependsOn {
			if cycle := walk(dep); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		visiting[id] = false
		visited[id] = true
		return nil
	}

	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if cycle := walk(id); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

var ErrNoPipelineGraph = errors.New("pipeline graph is required")

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	pipelineexpression "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

const (
	defaultPipelinePreviewSampleSize = 50
	maxPipelinePreviewSampleSize     = 500
)

type previewPipelineNodeRequest struct {
	DAG        json.RawMessage       `json:"dag,omitempty"`
	IR         *models.PipelineIR    `json:"ir,omitempty"`
	Nodes      []models.PipelineNode `json:"nodes,omitempty"`
	SampleSize int                   `json:"sample_size,omitempty"`
}

type pipelineNodePreviewResponse struct {
	PipelineID  string                       `json:"pipeline_id"`
	NodeID      string                       `json:"node_id"`
	Columns     []string                     `json:"columns"`
	Rows        []map[string]json.RawMessage `json:"rows"`
	SampleSize  int                          `json:"sample_size"`
	GeneratedAt time.Time                    `json:"generated_at"`
	Seed        uint64                       `json:"seed"`
	SourceChain []string                     `json:"source_chain"`
	Fresh       bool                         `json:"fresh"`
	Error       *pipelineNodePreviewError    `json:"error,omitempty"`
}

type pipelineNodePreviewError struct {
	Kind      string `json:"kind"`
	NodeID    string `json:"node_id,omitempty"`
	Transform string `json:"transform,omitempty"`
	Message   string `json:"message"`
}

type previewExecutionError struct {
	status int
	err    pipelineNodePreviewError
}

func (e previewExecutionError) Error() string { return e.err.Message }

// PreviewPipelineNode executes the selected node's upstream dependency chain
// in the local lightweight table runtime and returns a deterministic row
// sample. POST may carry an unsaved draft DAG from the editor; GET previews the
// persisted pipeline row.
func PreviewPipelineNode(w http.ResponseWriter, r *http.Request) {
	pipelineID, err := pipelineIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pipeline_id", "detail": err.Error()})
		return
	}
	nodeID, err := pipelineNodeIDFromRequest(r)
	if err != nil {
		writePipelinePreviewError(w, pipelineID, "", defaultPipelinePreviewSampleSize, nil, http.StatusBadRequest, pipelineNodePreviewError{Kind: "invalid_node_id", Message: err.Error()})
		return
	}
	body, err := readPreviewPipelineNodeRequest(r)
	if err != nil {
		writePipelinePreviewError(w, pipelineID, nodeID, defaultPipelinePreviewSampleSize, nil, http.StatusBadRequest, pipelineNodePreviewError{Kind: "invalid_json", NodeID: nodeID, Message: err.Error()})
		return
	}
	sampleSize := clampPipelinePreviewSampleSize(firstPositiveInt(body.SampleSize, queryInt(r, "sample_size")))
	nodes, err := previewNodesForRequest(r.Context(), pipelineID, body)
	if err != nil {
		var execErr previewExecutionError
		if errors.As(err, &execErr) {
			writePipelinePreviewError(w, pipelineID, nodeID, sampleSize, nil, execErr.status, execErr.err)
			return
		}
		writePipelinePreviewError(w, pipelineID, nodeID, sampleSize, nil, http.StatusBadRequest, pipelineNodePreviewError{Kind: "invalid_pipeline_graph", NodeID: nodeID, Message: err.Error()})
		return
	}
	response, err := executeLocalPipelinePreview(r.Context(), pipelineID, nodeID, nodes, sampleSize)
	if err != nil {
		var execErr previewExecutionError
		if errors.As(err, &execErr) {
			writePipelinePreviewError(w, pipelineID, nodeID, sampleSize, execErr.errChain(nodes, nodeID), execErr.status, execErr.err)
			return
		}
		writePipelinePreviewError(w, pipelineID, nodeID, sampleSize, nil, http.StatusBadRequest, pipelineNodePreviewError{Kind: "preview_failed", NodeID: nodeID, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func previewNodesForRequest(ctx context.Context, pipelineID uuid.UUID, req previewPipelineNodeRequest) ([]models.PipelineNode, error) {
	if req.IR != nil {
		return req.IR.Normalize().LegacyNodes(), nil
	}
	if len(req.Nodes) > 0 {
		return append([]models.PipelineNode(nil), req.Nodes...), nil
	}
	if len(bytes.TrimSpace(req.DAG)) > 0 {
		ir, err := models.ParsePipelineIR(req.DAG)
		if err != nil {
			return nil, err
		}
		return ir.LegacyNodes(), nil
	}
	repo, ok := currentPipelineAuthoringRepository()
	if !ok {
		return nil, previewExecutionError{status: http.StatusServiceUnavailable, err: pipelineNodePreviewError{Kind: "pipeline_authoring_repository_not_configured", Message: "PreviewPipelineNode requires a draft graph body or a configured pipeline authoring repository"}}
	}
	pipeline, err := repo.GetPipeline(ctx, pipelineID)
	if err != nil {
		return nil, err
	}
	if pipeline == nil {
		return nil, previewExecutionError{status: http.StatusNotFound, err: pipelineNodePreviewError{Kind: "pipeline_not_found", Message: "pipeline not found"}}
	}
	return pipeline.ParsedNodes()
}

func executeLocalPipelinePreview(ctx context.Context, pipelineID uuid.UUID, nodeID string, nodes []models.PipelineNode, sampleSize int) (pipelineNodePreviewResponse, error) {
	ports, _ := currentExecutionPorts()
	return executeLocalPipelinePreviewWithPorts(ctx, pipelineID, nodeID, nodes, sampleSize, ports)
}

func executeLocalPipelinePreviewWithPorts(ctx context.Context, pipelineID uuid.UUID, nodeID string, nodes []models.PipelineNode, sampleSize int, ports ExecutionPorts) (pipelineNodePreviewResponse, error) {
	chain, err := upstreamExecutionChain(nodes, nodeID)
	if err != nil {
		return pipelineNodePreviewResponse{}, err
	}
	runtime := newLightweightTableRuntime()
	runner := runtimeNodeRunner{Table: runtime, LLM: ports.LLM}
	for _, node := range chain {
		transform := strings.TrimSpace(node.TransformType)
		if !runtime.Supports(transform) && !isLLMTransform(transform) {
			return pipelineNodePreviewResponse{}, previewExecutionError{status: http.StatusBadRequest, err: pipelineNodePreviewError{Kind: "unsupported_transform", NodeID: node.ID, Transform: transform, Message: fmt.Sprintf("local preview does not support transform %q", transform)}}
		}
		_, err := runner.Run(ctx, executor.NodeContext{
			BuildID:     pipelineID,
			BuildBranch: "preview",
			Attempt:     1,
			Node: executor.Node{
				ID:        node.ID,
				JobID:     uuid.NewSHA1(uuid.NameSpaceOID, []byte(pipelineID.String()+":preview:"+node.ID)),
				DependsOn: node.DependsOn,
				Metadata: map[string]any{
					"logic_kind":     "transform",
					"transform_type": transform,
					"logic_payload":  previewConfigWithSampleSize(node.Config, sampleSize),
					"label":          node.Label,
					"sample_size":    sampleSize,
				},
			},
		})
		if err != nil {
			return pipelineNodePreviewResponse{}, previewExecutionError{status: http.StatusBadRequest, err: pipelineNodePreviewError{Kind: "transform_failed", NodeID: node.ID, Transform: transform, Message: err.Error()}}
		}
		runtime.trimRows(node.ID, sampleSize)
	}
	rows := limitRuntimeRows(runtime.snapshotRows(nodeID), sampleSize)
	return pipelineNodePreviewResponse{
		PipelineID:  pipelineID.String(),
		NodeID:      nodeID,
		Columns:     deriveRuntimeColumns(rows),
		Rows:        rowsToMaps(rows),
		SampleSize:  sampleSize,
		GeneratedAt: time.Now().UTC(),
		Seed:        pipelinePreviewSeed(pipelineID, nodeID),
		SourceChain: chainIDs(chain),
		Fresh:       true,
	}, nil
}

func upstreamExecutionChain(nodes []models.PipelineNode, target string) ([]models.PipelineNode, error) {
	byID := make(map[string]models.PipelineNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	if _, ok := byID[target]; !ok {
		return nil, previewExecutionError{status: http.StatusNotFound, err: pipelineNodePreviewError{Kind: "node_not_found", NodeID: target, Message: "node not found in pipeline graph"}}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	out := []models.PipelineNode{}
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return previewExecutionError{status: http.StatusBadRequest, err: pipelineNodePreviewError{Kind: "cycle_detected", NodeID: id, Message: "cycle detected in upstream preview chain"}}
		}
		node, ok := byID[id]
		if !ok {
			return previewExecutionError{status: http.StatusBadRequest, err: pipelineNodePreviewError{Kind: "missing_dependency", NodeID: id, Message: "node dependency is missing from pipeline graph"}}
		}
		visiting[id] = true
		for _, dep := range node.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		out = append(out, node)
		return nil
	}
	if err := visit(target); err != nil {
		return nil, err
	}
	return out, nil
}

func readPreviewPipelineNodeRequest(r *http.Request) (previewPipelineNodeRequest, error) {
	var req previewPipelineNodeRequest
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		return req, nil
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, err
	}
	return req, nil
}

func previewConfigWithSampleSize(raw json.RawMessage, sampleSize int) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return mustRuntimeJSON(map[string]any{"sample_size": sampleSize})
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return raw
	}
	if _, ok := cfg["sample_size"]; !ok {
		cfg["sample_size"] = mustRuntimeJSON(sampleSize)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return out
}

func writePipelinePreviewError(w http.ResponseWriter, pipelineID uuid.UUID, nodeID string, sampleSize int, chain []string, status int, err pipelineNodePreviewError) {
	if chain == nil {
		chain = []string{}
	}
	writeJSON(w, status, pipelineNodePreviewResponse{
		PipelineID:  pipelineID.String(),
		NodeID:      nodeID,
		Columns:     []string{},
		Rows:        []map[string]json.RawMessage{},
		SampleSize:  sampleSize,
		GeneratedAt: time.Now().UTC(),
		Seed:        pipelinePreviewSeed(pipelineID, nodeID),
		SourceChain: chain,
		Fresh:       false,
		Error:       &err,
	})
}

func (e previewExecutionError) errChain(nodes []models.PipelineNode, target string) []string {
	chain, err := upstreamExecutionChain(nodes, target)
	if err != nil {
		return nil
	}
	return chainIDs(chain)
}

func pipelineNodeIDFromRequest(r *http.Request) (string, error) {
	for _, key := range []string{"node_id", "nodeId"} {
		if raw := chi.URLParam(r, key); raw != "" {
			decoded, err := url.PathUnescape(raw)
			if err != nil {
				return "", err
			}
			return decoded, nil
		}
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "nodes" && i+1 < len(parts) {
			decoded, err := url.PathUnescape(parts[i+1])
			if err != nil {
				return "", err
			}
			return decoded, nil
		}
	}
	return "", errors.New("node id is required")
}

func limitRuntimeRows(rows []pipelineexpression.Row, limit int) []pipelineexpression.Row {
	if limit <= 0 || len(rows) <= limit {
		return cloneRows(rows)
	}
	return cloneRows(rows[:limit])
}

func chainIDs(nodes []models.PipelineNode) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = node.ID
	}
	return out
}

func pipelinePreviewSeed(pipelineID uuid.UUID, nodeID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(pipelineID.String()))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(nodeID))
	return h.Sum64()
}

func queryInt(r *http.Request, key string) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return parsed
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return defaultPipelinePreviewSampleSize
}

func clampPipelinePreviewSampleSize(value int) int {
	if value <= 0 {
		return defaultPipelinePreviewSampleSize
	}
	if value > maxPipelinePreviewSampleSize {
		return maxPipelinePreviewSampleSize
	}
	return value
}

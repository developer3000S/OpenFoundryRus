package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestPipelineCRUDReturnsExplicit503WithoutRepository(t *testing.T) {
	restore := SetPipelineAuthoringRepository(nil)
	defer restore()
	id := uuid.New().String()
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		h      http.HandlerFunc
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/pipelines", h: ListPipelines},
		{name: "create", method: http.MethodPost, path: "/api/v1/pipelines", body: `{"name":"p","nodes":[]}`, h: CreatePipeline},
		{name: "get", method: http.MethodGet, path: "/api/v1/pipelines/" + id, h: GetPipeline},
		{name: "patch", method: http.MethodPatch, path: "/api/v1/pipelines/" + id, body: `{"name":"renamed"}`, h: UpdatePipeline},
		{name: "put", method: http.MethodPut, path: "/api/v1/pipelines/" + id, body: `{"name":"renamed"}`, h: UpdatePipeline},
		{name: "versions", method: http.MethodGet, path: "/api/v1/pipelines/" + id + "/versions", h: ListPipelineVersions},
		{name: "publish", method: http.MethodPost, path: "/api/v1/pipelines/" + id + "/publish", body: `{}`, h: PublishPipeline},
		{name: "proposal", method: http.MethodPost, path: "/api/v1/pipelines/" + id + "/proposals", body: `{"title":"proposal"}`, h: CreatePipelineProposal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			if tc.path != "/api/v1/pipelines" {
				r = requestWithURLParam(tc.method, tc.path, bytes.NewReader([]byte(tc.body)), "id", id)
			}
			rr := httptest.NewRecorder()
			tc.h(rr, r)
			require.Equal(t, http.StatusServiceUnavailable, rr.Code)
			var payload map[string]string
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&payload))
			require.Equal(t, "pipeline_authoring_repository_not_configured", payload["error"])
			require.NotEmpty(t, payload["detail"])
		})
	}
}

func TestPipelineCRUDUsesConfiguredRepository(t *testing.T) {
	repo := newFakePipelineAuthoringRepo()
	restore := SetPipelineAuthoringRepository(repo)
	defer restore()

	createRR := httptest.NewRecorder()
	CreatePipeline(createRR, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines", bytes.NewReader([]byte(`{"name":"daily","description":"initial","nodes":[{"id":"n1","transform_type":"noop"}],"schedule_config":{"enabled":true},"retry_policy":{"max_attempts":2,"retry_on_failure":true,"allow_partial_reexecution":false}}`))))
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created models.Pipeline
	require.NoError(t, json.NewDecoder(createRR.Body).Decode(&created))
	require.Equal(t, "daily", created.Name)
	var createdIR models.PipelineIR
	require.NoError(t, json.Unmarshal(created.DAG, &createdIR))
	require.Equal(t, models.PipelineIRVersion, createdIR.Version)
	require.Len(t, createdIR.Nodes, 1)
	require.Equal(t, "n1", createdIR.Nodes[0].ID)
	require.Equal(t, "noop", createdIR.Nodes[0].TransformType)

	listRR := httptest.NewRecorder()
	ListPipelines(listRR, httptest.NewRequest(http.MethodGet, "/api/v1/pipelines?page=1&per_page=10&status=draft", nil))
	require.Equal(t, http.StatusOK, listRR.Code)
	var list models.ListPipelinesResponse
	require.NoError(t, json.NewDecoder(listRR.Body).Decode(&list))
	require.Equal(t, int64(1), list.Total)
	require.Len(t, list.Data, 1)

	getRR := httptest.NewRecorder()
	GetPipeline(getRR, requestWithURLParam(http.MethodGet, "/api/v1/pipelines/"+created.ID.String(), nil, "id", created.ID.String()))
	require.Equal(t, http.StatusOK, getRR.Code)

	patchRR := httptest.NewRecorder()
	UpdatePipeline(patchRR, requestWithURLParam(http.MethodPatch, "/api/v1/pipelines/"+created.ID.String(), bytes.NewReader([]byte(`{"name":"daily-v2"}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusOK, patchRR.Code)
	var patched models.Pipeline
	require.NoError(t, json.NewDecoder(patchRR.Body).Decode(&patched))
	require.Equal(t, "daily-v2", patched.Name)

	putRR := httptest.NewRecorder()
	UpdatePipeline(putRR, requestWithURLParam(http.MethodPut, "/api/v1/pipelines/"+created.ID.String(), bytes.NewReader([]byte(`{"description":"from put"}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusOK, putRR.Code)
	var put models.Pipeline
	require.NoError(t, json.NewDecoder(putRR.Body).Decode(&put))
	require.Equal(t, "from put", put.Description)
}

func TestPipelineCRUDPersistsLightweightPipelineTypeAlias(t *testing.T) {
	repo := newFakePipelineAuthoringRepo()
	restore := SetPipelineAuthoringRepository(repo)
	defer restore()

	createRR := httptest.NewRecorder()
	CreatePipeline(createRR, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines", bytes.NewReader([]byte(`{"name":"fast-trails","pipeline_type":"LIGHTWEIGHT","nodes":[{"id":"source","transform_type":"dataset_input","config":{"rows":[{"id":"mesa","distance":5}]}}]}`))))
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created models.Pipeline
	require.NoError(t, json.NewDecoder(createRR.Body).Decode(&created))
	require.Equal(t, models.PipelineTypeFaster, created.PipelineType)
	require.Equal(t, models.PipelineLifecycleDraft, created.Lifecycle)

	patchRR := httptest.NewRecorder()
	UpdatePipeline(patchRR, requestWithURLParam(http.MethodPatch, "/api/v1/pipelines/"+created.ID.String(), bytes.NewReader([]byte(`{"pipeline_type":"BATCH"}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusOK, patchRR.Code)
	var patched models.Pipeline
	require.NoError(t, json.NewDecoder(patchRR.Body).Decode(&patched))
	require.Equal(t, models.PipelineTypeBatch, patched.PipelineType)
}

func TestPipelineCRUDPersistsDistributedPipelineConfig(t *testing.T) {
	repo := newFakePipelineAuthoringRepo()
	restore := SetPipelineAuthoringRepository(repo)
	defer restore()

	createRR := httptest.NewRecorder()
	CreatePipeline(createRR, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines", bytes.NewReader([]byte(`{"name":"cluster-trails","pipeline_type":"spark","distributed":{"engine":"spark","runner_image":"img"},"nodes":[{"id":"sql","transform_type":"sql","config":{"sql":"SELECT * FROM trails"}}]}`))))
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created models.Pipeline
	require.NoError(t, json.NewDecoder(createRR.Body).Decode(&created))
	require.Equal(t, models.PipelineTypeDistributed, created.PipelineType)
	require.JSONEq(t, `{"engine":"spark","runner_image":"img"}`, string(created.DistributedConfig))
}

func TestPipelineLifecycleVersionsPublishProposalRestore(t *testing.T) {
	repo := newFakePipelineAuthoringRepo()
	restore := SetPipelineAuthoringRepository(repo)
	defer restore()

	createRR := httptest.NewRecorder()
	CreatePipeline(createRR, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines", bytes.NewReader([]byte(`{"name":"trail-demo","nodes":[{"id":"source","transform_type":"external","config":{"rows":[{"id":"mesa"}]}}]}`))))
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created models.Pipeline
	require.NoError(t, json.NewDecoder(createRR.Body).Decode(&created))
	require.Empty(t, created.PublishedAt)

	updateRR := httptest.NewRecorder()
	UpdatePipeline(updateRR, requestWithURLParam(http.MethodPatch, "/api/v1/pipelines/"+created.ID.String(), bytes.NewReader([]byte(`{"nodes":[{"id":"source","transform_type":"external","config":{"rows":[{"id":"green"}]}}]}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusOK, updateRR.Code)

	proposalRR := httptest.NewRecorder()
	CreatePipelineProposal(proposalRR, requestWithURLParam(http.MethodPost, "/api/v1/pipelines/"+created.ID.String()+"/proposals", bytes.NewReader([]byte(`{"title":"Review trail demo","description":"ready for publish","branch_name":"feature/trails"}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusCreated, proposalRR.Code)
	var proposal models.PipelinePublishResponse
	require.NoError(t, json.NewDecoder(proposalRR.Body).Decode(&proposal))
	require.Equal(t, "proposal", proposal.Version.VersionKind)
	require.Equal(t, "open", proposal.Pipeline.ProposalState)

	publishRR := httptest.NewRecorder()
	PublishPipeline(publishRR, requestWithURLParam(http.MethodPost, "/api/v1/pipelines/"+created.ID.String()+"/publish", bytes.NewReader([]byte(`{"message":"ship it"}`)), "id", created.ID.String()))
	require.Equal(t, http.StatusOK, publishRR.Code)
	var published models.PipelinePublishResponse
	require.NoError(t, json.NewDecoder(publishRR.Body).Decode(&published))
	require.Equal(t, "published", published.Version.VersionKind)
	require.Equal(t, published.Version.ID, *published.Pipeline.ActiveVersionID)
	require.True(t, len(published.Pipeline.PublishedDAG) > 0)

	listRR := httptest.NewRecorder()
	ListPipelineVersions(listRR, requestWithURLParam(http.MethodGet, "/api/v1/pipelines/"+created.ID.String()+"/versions", nil, "id", created.ID.String()))
	require.Equal(t, http.StatusOK, listRR.Code)
	var versions models.ListPipelineVersionsResponse
	require.NoError(t, json.NewDecoder(listRR.Body).Decode(&versions))
	require.GreaterOrEqual(t, len(versions.Data), 4)

	restoreRR := httptest.NewRecorder()
	RestorePipelineVersion(restoreRR, requestWithVersionURLParams(http.MethodPost, "/api/v1/pipelines/"+created.ID.String()+"/versions/"+versions.Data[len(versions.Data)-1].ID.String()+"/restore", bytes.NewReader([]byte(`{"as_draft":true,"message":"back to initial"}`)), created.ID.String(), versions.Data[len(versions.Data)-1].ID.String()))
	require.Equal(t, http.StatusOK, restoreRR.Code)
	var restored models.PipelinePublishResponse
	require.NoError(t, json.NewDecoder(restoreRR.Body).Decode(&restored))
	require.Equal(t, "restored", restored.Version.VersionKind)
	require.Equal(t, "none", restored.Pipeline.ProposalState)
}

type fakePipelineAuthoringRepo struct {
	items    map[uuid.UUID]models.Pipeline
	versions map[uuid.UUID][]models.PipelineVersion
}

func newFakePipelineAuthoringRepo() *fakePipelineAuthoringRepo {
	return &fakePipelineAuthoringRepo{items: map[uuid.UUID]models.Pipeline{}, versions: map[uuid.UUID][]models.PipelineVersion{}}
}

func (f *fakePipelineAuthoringRepo) ListPipelines(context.Context, models.ListPipelinesQuery) (models.ListPipelinesResponse, error) {
	items := make([]models.Pipeline, 0, len(f.items))
	for _, p := range f.items {
		items = append(items, p)
	}
	return models.ListPipelinesResponse{Data: items, Total: int64(len(items)), Page: 1, PerPage: 50}, nil
}

func (f *fakePipelineAuthoringRepo) CreatePipeline(_ context.Context, req models.CreatePipelineRequest, ownerID uuid.UUID) (*models.Pipeline, error) {
	dag, err := req.CanonicalDAG()
	if err != nil {
		return nil, err
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	status := "draft"
	if req.Status != nil {
		status = *req.Status
	}
	now := time.Now().UTC()
	pipelineType := models.PipelineTypeBatch
	if req.PipelineType != nil {
		pipelineType = models.NormalizePipelineType(*req.PipelineType)
	}
	lifecycle := models.PipelineLifecycleDraft
	if req.Lifecycle != nil {
		lifecycle = models.NormalizePipelineLifecycle(*req.Lifecycle)
	}
	p := models.Pipeline{ID: uuid.New(), Name: req.Name, Description: description, OwnerID: ownerID, DAG: dag, Status: status, PipelineType: pipelineType, Lifecycle: lifecycle, ScheduleConfig: json.RawMessage(`{}`), RetryPolicy: json.RawMessage(`{"max_attempts":1,"retry_on_failure":false,"allow_partial_reexecution":true}`), CreatedAt: now, UpdatedAt: now}
	p.DistributedConfig = append(json.RawMessage(nil), req.Distributed...)
	p.DraftDAG = dag
	p.BranchName = "main"
	if req.BranchName != nil && *req.BranchName != "" {
		p.BranchName = *req.BranchName
	}
	p.DraftUpdatedAt = &now
	p.ProposalState = "none"
	if req.ScheduleConfig != nil {
		p.ScheduleConfig, err = json.Marshal(req.ScheduleConfig)
		if err != nil {
			return nil, err
		}
	}
	if req.RetryPolicy != nil {
		p.RetryPolicy, err = json.Marshal(req.RetryPolicy)
		if err != nil {
			return nil, err
		}
	}
	f.items[p.ID] = p
	f.appendVersion(p, "draft", "Initial draft", &ownerID, nil)
	return &p, nil
}

func (f *fakePipelineAuthoringRepo) GetPipeline(_ context.Context, id uuid.UUID) (*models.Pipeline, error) {
	p, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (f *fakePipelineAuthoringRepo) UpdatePipeline(_ context.Context, id uuid.UUID, req models.UpdatePipelineRequest) (*models.Pipeline, error) {
	p, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	if req.PipelineType != nil {
		p.PipelineType = models.NormalizePipelineType(*req.PipelineType)
	}
	if req.Lifecycle != nil {
		p.Lifecycle = models.NormalizePipelineLifecycle(*req.Lifecycle)
	}
	if len(req.Distributed) > 0 {
		p.DistributedConfig = append(json.RawMessage(nil), req.Distributed...)
	}
	if req.BranchName != nil {
		p.BranchName = *req.BranchName
	}
	if req.HasGraphUpdate() {
		dag, err := req.CanonicalDAG()
		if err != nil {
			return nil, err
		}
		p.DAG = dag
		p.DraftDAG = dag
	}
	now := time.Now().UTC()
	p.DraftUpdatedAt = &now
	p.UpdatedAt = now
	f.items[id] = p
	if req.HasGraphUpdate() {
		f.appendVersion(p, "draft", "Draft saved", nil, nil)
	}
	return &p, nil
}

func (f *fakePipelineAuthoringRepo) DeletePipeline(_ context.Context, id uuid.UUID) (bool, error) {
	if _, ok := f.items[id]; !ok {
		return false, nil
	}
	delete(f.items, id)
	return true, nil
}

func (f *fakePipelineAuthoringRepo) ListPipelineVersions(_ context.Context, pipelineID uuid.UUID) ([]models.PipelineVersion, error) {
	items := append([]models.PipelineVersion(nil), f.versions[pipelineID]...)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func (f *fakePipelineAuthoringRepo) PublishPipeline(_ context.Context, id uuid.UUID, req models.PublishPipelineRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	p, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	if req.BranchName != nil {
		p.BranchName = *req.BranchName
	}
	version := f.appendVersion(p, "published", req.Message, actorID, nil)
	now := time.Now().UTC()
	p.PublishedDAG = p.DAG
	p.PublishedAt = &now
	p.ActiveVersionID = &version.ID
	p.Status = "active"
	p.ProposalState = "merged"
	f.items[id] = p
	return &models.PipelinePublishResponse{Pipeline: p, Version: version}, nil
}

func (f *fakePipelineAuthoringRepo) CreatePipelineProposal(_ context.Context, id uuid.UUID, req models.CreatePipelineProposalRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	p, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	if req.BranchName != nil {
		p.BranchName = *req.BranchName
	}
	title := req.Title
	p.ProposalState = "open"
	p.ProposalTitle = &title
	p.ProposalDescription = &req.Description
	version := f.appendVersion(p, "proposal", title, actorID, nil)
	f.items[id] = p
	return &models.PipelinePublishResponse{Pipeline: p, Version: version}, nil
}

func (f *fakePipelineAuthoringRepo) RestorePipelineVersion(_ context.Context, pipelineID, versionID uuid.UUID, req models.RestorePipelineVersionRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	p, ok := f.items[pipelineID]
	if !ok {
		return nil, nil
	}
	var selected *models.PipelineVersion
	for i := range f.versions[pipelineID] {
		if f.versions[pipelineID][i].ID == versionID {
			selected = &f.versions[pipelineID][i]
			break
		}
	}
	if selected == nil {
		return nil, nil
	}
	p.Name = selected.Name
	p.Description = selected.Description
	p.DAG = selected.DAG
	p.DraftDAG = selected.DAG
	p.ScheduleConfig = selected.ScheduleConfig
	p.RetryPolicy = selected.RetryPolicy
	p.BranchName = selected.BranchName
	p.ProposalState = "none"
	p.ProposalTitle = nil
	p.ProposalDescription = nil
	restoredFrom := selected.ID
	version := f.appendVersion(p, "restored", req.Message, actorID, &restoredFrom)
	if !req.AsDraft {
		now := time.Now().UTC()
		p.PublishedDAG = selected.DAG
		p.PublishedAt = &now
		p.ActiveVersionID = &version.ID
		p.Status = "active"
	}
	f.items[pipelineID] = p
	return &models.PipelinePublishResponse{Pipeline: p, Version: version}, nil
}

func (f *fakePipelineAuthoringRepo) appendVersion(p models.Pipeline, kind string, message string, actorID *uuid.UUID, restoredFrom *uuid.UUID) models.PipelineVersion {
	version := models.PipelineVersion{
		ID:                    uuid.New(),
		PipelineID:            p.ID,
		VersionNumber:         int64(len(f.versions[p.ID]) + 1),
		BranchName:            p.BranchName,
		VersionKind:           kind,
		DAG:                   append(json.RawMessage(nil), p.DAG...),
		Name:                  p.Name,
		Description:           p.Description,
		ScheduleConfig:        append(json.RawMessage(nil), p.ScheduleConfig...),
		RetryPolicy:           append(json.RawMessage(nil), p.RetryPolicy...),
		CreatedBy:             actorID,
		CreatedAt:             time.Now().UTC(),
		Message:               message,
		RestoredFromVersionID: restoredFrom,
	}
	f.versions[p.ID] = append(f.versions[p.ID], version)
	return version
}

func requestWithVersionURLParams(method, target string, body *bytes.Reader, pipelineID string, versionID string) *http.Request {
	req := requestWithURLParam(method, target, body, "id", pipelineID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", pipelineID)
	rctx.URLParams.Add("version_id", versionID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

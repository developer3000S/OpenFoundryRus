package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

func TestGetBuildV1StableETagAndNotModified(t *testing.T) {
	repo := newFakeV1Repo()
	restore := SetBuildQueryRepository(repo)
	defer restore()
	buildID := uuid.New()
	repo.builds = []models.BuildEnvelope{{Build: models.Build{ID: buildID, RID: "ri.foundry.main.build." + buildID.String(), PipelineRID: "pipe", BuildBranch: "master", State: string(models.BuildResolution), TriggerKind: "MANUAL", AbortPolicy: string(models.AbortDependentOnly), RequestedBy: "u", CreatedAt: time.Unix(100, 0).UTC()}}}

	req := requestWithURLParam(http.MethodGet, "/v1/builds/"+repo.builds[0].RID, nil, "rid", repo.builds[0].RID)
	rr := httptest.NewRecorder()
	GetBuildV1(rr, req)
	res := rr.Result()
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	etag := res.Header.Get("ETag")
	require.NotEmpty(t, etag)

	req = requestWithURLParam(http.MethodGet, "/v1/builds/"+repo.builds[0].RID, nil, "rid", repo.builds[0].RID)
	req.Header.Set("If-None-Match", etag)
	rr = httptest.NewRecorder()
	GetBuildV1(rr, req)
	require.Equal(t, http.StatusNotModified, rr.Result().StatusCode)
	require.Equal(t, etag, rr.Result().Header.Get("ETag"))
}

func TestListBuildsV1PaginationEnvelopeAndLimitClamp(t *testing.T) {
	repo := newFakeV1Repo()
	restore := SetBuildQueryRepository(repo)
	defer restore()
	repo.builds = []models.BuildEnvelope{
		{Build: models.Build{ID: uuid.New(), RID: "b2", CreatedAt: time.Unix(200, 0).UTC()}},
		{Build: models.Build{ID: uuid.New(), RID: "b1", CreatedAt: time.Unix(100, 0).UTC()}},
	}
	rr := httptest.NewRecorder()
	ListBuildsV1(rr, httptest.NewRequest(http.MethodGet, "/v1/builds?limit=99999", nil))
	res := rr.Result()
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Equal(t, float64(200), payload["limit"])
	require.NotNil(t, payload["next_cursor"])
}

func TestCreateJobSpecV1IdempotentOnContentHash(t *testing.T) {
	repo := newFakeV1Repo()
	restore := SetBuildQueryRepository(repo)
	defer restore()
	body := []byte(`{"pipeline_rid":"pipe","branch_name":"master","output_dataset_rids":["out"],"logic_payload":{"sql":"select 1"},"content_hash":"hash-A"}`)
	rr := httptest.NewRecorder()
	CreateJobSpecV1(rr, requestWithURLParam(http.MethodPost, "/v1/job-specs/TRANSFORM", bytes.NewReader(body), "kind", "TRANSFORM"))
	require.Equal(t, http.StatusCreated, rr.Result().StatusCode)
	var first PublishedJobSpec
	require.NoError(t, json.NewDecoder(rr.Result().Body).Decode(&first))

	rr = httptest.NewRecorder()
	CreateJobSpecV1(rr, requestWithURLParam(http.MethodPost, "/v1/job-specs/TRANSFORM", bytes.NewReader(body), "kind", "TRANSFORM"))
	require.Equal(t, http.StatusCreated, rr.Result().StatusCode)
	var second PublishedJobSpec
	require.NoError(t, json.NewDecoder(rr.Result().Body).Decode(&second))
	require.Equal(t, first.RID, second.RID)
	require.Equal(t, "hash-A", second.ContentHash)
}

func TestJobLogsV1ListAndEmit(t *testing.T) {
	mem := livellogs.NewMemoryService()
	restore := SetJobLogService(&livellogs.Service{Store: &fakeAppendLogStore{MemoryService: mem}})
	defer restore()
	rr := httptest.NewRecorder()
	EmitJobLogV1(rr, requestWithURLParam(http.MethodPost, "/v1/jobs/job-1/logs", bytes.NewReader([]byte(`{"level":"INFO","message":"hello","params":{"x":1}}`)), "rid", "job-1"))
	require.Equal(t, http.StatusOK, rr.Result().StatusCode)

	rr = httptest.NewRecorder()
	ListJobLogsV1(rr, requestWithURLParam(http.MethodGet, "/v1/jobs/job-1/logs?follow=false", nil, "rid", "job-1"))
	require.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(rr.Result().Body).Decode(&payload))
	require.Equal(t, float64(1), payload["total"])
}

func requestWithURLParam(method string, target string, body io.Reader, key string, value string) *http.Request {
	req := httptest.NewRequest(method, target, body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

type fakeV1Repo struct {
	builds []models.BuildEnvelope
	specs  map[string]PublishedJobSpec
}

func newFakeV1Repo() *fakeV1Repo { return &fakeV1Repo{specs: map[string]PublishedJobSpec{}} }

func (f *fakeV1Repo) ListBuilds(context.Context, models.ListBuildsQuery) ([]models.BuildEnvelope, error) {
	return append([]models.BuildEnvelope(nil), f.builds...), nil
}
func (f *fakeV1Repo) GetBuild(_ context.Context, idOrRID string) (*models.BuildEnvelope, error) {
	for _, b := range f.builds {
		if b.RID == idOrRID || b.ID.String() == idOrRID {
			bb := b
			return &bb, nil
		}
	}
	return nil, nil
}
func (f *fakeV1Repo) ListJobsForBuildID(context.Context, string) ([]models.Job, error) {
	return nil, nil
}
func (f *fakeV1Repo) GetJob(context.Context, string) (*models.Job, error) {
	return nil, nil
}
func (f *fakeV1Repo) ListDatasetBuilds(context.Context, string, int64) ([]models.Build, error) {
	return nil, nil
}
func (f *fakeV1Repo) GetJobOutputs(context.Context, string) (*JobOutputsResponse, error) {
	return nil, nil
}
func (f *fakeV1Repo) GetJobInputResolutions(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}
func (f *fakeV1Repo) PublishJobSpec(_ context.Context, kind string, req CreateJobSpecRequest, _ string) (PublishedJobSpec, error) {
	key := req.PipelineRID + "\x00" + req.BranchName + "\x00" + kind + "\x00" + *req.ContentHash
	if found, ok := f.specs[key]; ok {
		return found, nil
	}
	out := PublishedJobSpec{RID: "ri.foundry.main.job_spec." + uuid.NewString(), LogicKind: kind, ContentHash: *req.ContentHash}
	f.specs[key] = out
	return out, nil
}

type fakeAppendLogStore struct{ *livellogs.MemoryService }

func (f *fakeAppendLogStore) AppendLogByRID(_ context.Context, jobRID string, level livellogs.LogLevel, message string, params json.RawMessage) (livellogs.LogEntry, error) {
	return f.Emit(jobRID, level, message, params), nil
}

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

func TestConnectionJSONShape(t *testing.T) {
	t.Parallel()
	c := models.Connection{
		ID: uuid.New(), Name: "snowflake-prod",
		ConnectorType: "snowflake",
		Config:        json.RawMessage(`{"account":"x"}`),
		Status:        "disconnected",
		OwnerID:       uuid.New(),
		CreatedAt:     time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(c)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "name", "connector_type", "config", "status",
		"owner_id", "last_sync_at", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
}

func TestCreateConnectionRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("POST", "/connections",
		strings.NewReader(`{"name":"x","connector_type":"y"}`))
	rec := httptest.NewRecorder()
	h.CreateConnection(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestCreateConnectionRejectsEmptyFields(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/connections",
		strings.NewReader(`{"name":"","connector_type":""}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateConnection(rec, req)
	assert.Equal(t, 400, rec.Code)
}

func TestListConnectionsRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("GET", "/connections", nil)
	rec := httptest.NewRecorder()
	h.ListConnections(rec, req)
	assert.Equal(t, 401, rec.Code)
}

type fakeStore struct {
	connections []models.Connection
	syncJobs    map[uuid.UUID][]models.SyncJob
	mediaSyncs  map[uuid.UUID][]models.MediaSetSync
	runs        map[uuid.UUID][]models.SyncRun
	links       map[string]models.VirtualTableSourceLink
	vtables     map[string]models.VirtualTable
}

func newFakeStore(owner uuid.UUID) *fakeStore {
	conn := models.Connection{ID: uuid.New(), Name: "pg", ConnectorType: "postgresql", Config: json.RawMessage(`{}`), Status: "connected", OwnerID: owner, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return &fakeStore{connections: []models.Connection{conn}, syncJobs: map[uuid.UUID][]models.SyncJob{}, mediaSyncs: map[uuid.UUID][]models.MediaSetSync{}, runs: map[uuid.UUID][]models.SyncRun{}, links: map[string]models.VirtualTableSourceLink{}, vtables: map[string]models.VirtualTable{}}
}

func (f *fakeStore) ListConnections(_ context.Context, ownerID *uuid.UUID) ([]models.Connection, error) {
	return f.connections, nil
}
func (f *fakeStore) GetConnection(_ context.Context, id uuid.UUID) (*models.Connection, error) {
	for i := range f.connections {
		if f.connections[i].ID == id {
			return &f.connections[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) GetConnectionForOwner(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Connection, error) {
	for i := range f.connections {
		if f.connections[i].ID == id && f.connections[i].OwnerID == ownerID {
			return &f.connections[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateConnection(_ context.Context, body *models.CreateConnectionRequest, ownerID uuid.UUID) (*models.Connection, error) {
	c := models.Connection{ID: uuid.New(), Name: body.Name, ConnectorType: body.ConnectorType, Config: body.Config, OwnerID: ownerID}
	return &c, nil
}
func (f *fakeStore) UpdateConnection(_ context.Context, id uuid.UUID, _ *models.UpdateConnectionRequest) (*models.Connection, error) {
	return f.GetConnection(context.Background(), id)
}
func (f *fakeStore) DeleteConnection(_ context.Context, id uuid.UUID) (bool, error) {
	c, _ := f.GetConnection(context.Background(), id)
	return c != nil, nil
}
func (f *fakeStore) ListSyncJobs(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SyncJob, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return []models.SyncJob{}, nil
	}
	return f.syncJobs[sourceID], nil
}
func (f *fakeStore) GetSyncJob(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncJob, error) {
	for source, jobs := range f.syncJobs {
		if c, _ := f.GetConnectionForOwner(context.Background(), source, ownerID); c == nil {
			continue
		}
		for i := range jobs {
			if jobs[i].ID == id {
				return &jobs[i], nil
			}
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateSyncJob(_ context.Context, body *models.CreateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), body.SourceID, ownerID); c == nil {
		return nil, nil
	}
	j := models.SyncJob{ID: uuid.New(), SourceID: body.SourceID, OutputDatasetID: body.OutputDatasetID, FileGlob: body.FileGlob, ScheduleCron: body.ScheduleCron, CreatedAt: time.Now().UTC()}
	f.syncJobs[body.SourceID] = append([]models.SyncJob{j}, f.syncJobs[body.SourceID]...)
	return &j, nil
}
func (f *fakeStore) UpdateSyncJob(_ context.Context, id uuid.UUID, body *models.UpdateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error) {
	for source, jobs := range f.syncJobs {
		if c, _ := f.GetConnectionForOwner(context.Background(), source, ownerID); c == nil {
			continue
		}
		for i := range jobs {
			if jobs[i].ID == id {
				if body.OutputDatasetID != nil {
					jobs[i].OutputDatasetID = *body.OutputDatasetID
				}
				if body.FileGlob != nil {
					jobs[i].FileGlob = body.FileGlob
				}
				if body.ScheduleCron != nil {
					jobs[i].ScheduleCron = body.ScheduleCron
				}
				f.syncJobs[source] = jobs
				return &jobs[i], nil
			}
		}
	}
	return nil, nil
}
func (f *fakeStore) RunSyncJob(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncRun, error) {
	if _, err := f.GetSyncJob(context.Background(), id, ownerID); err != nil {
		return nil, err
	} else if _, _ = f.GetSyncJob(context.Background(), id, ownerID); false {
	}
	job, _ := f.GetSyncJob(context.Background(), id, ownerID)
	if job == nil {
		return nil, nil
	}
	run := models.SyncRun{ID: uuid.New(), SyncDefID: id, Status: "running", StartedAt: time.Now().UTC()}
	f.runs[id] = append(f.runs[id], run)
	return &run, nil
}
func (f *fakeStore) EnableVirtualTableSource(_ context.Context, sourceRID string, body *models.EnableVirtualTableSourceRequest) (*models.VirtualTableSourceLink, error) {
	if body.Provider == "" {
		return nil, assert.AnError
	}
	l := models.VirtualTableSourceLink{SourceRID: sourceRID, Provider: body.Provider, VirtualTablesEnabled: true, ExportControls: []byte(`{}`), AutoRegisterTagFilters: []byte(`[]`), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.links[sourceRID] = l
	return &l, nil
}
func (f *fakeStore) CreateVirtualTable(_ context.Context, sourceRID string, actorID string, body *models.CreateVirtualTableRequest) (*models.VirtualTable, error) {
	if _, ok := f.links[sourceRID]; !ok {
		return nil, nil
	}
	loc, err := body.Locator.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	name := body.Locator.DefaultDisplayName()
	if body.Name != nil {
		name = *body.Name
	}
	rid := "ri.foundry.main.virtual-table." + uuid.NewString()
	creator := actorID
	v := models.VirtualTable{ID: uuid.New(), RID: rid, SourceRID: sourceRID, ProjectRID: body.ProjectRID, Name: name, Locator: loc, TableType: body.TableType, SchemaInferred: []byte(`[]`), Capabilities: []byte(`{}`), Markings: body.Markings, Properties: []byte(`{}`), CreatedBy: &creator, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.vtables[rid] = v
	return &v, nil
}
func (f *fakeStore) ListVirtualTables(_ context.Context, ownerID string, project, source string, _ int) ([]models.VirtualTable, error) {
	out := []models.VirtualTable{}
	for _, v := range f.vtables {
		if v.CreatedBy != nil && *v.CreatedBy == ownerID && (project == "" || v.ProjectRID == project) && (source == "" || v.SourceRID == source) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (f *fakeStore) GetVirtualTable(_ context.Context, rid string, ownerID string) (*models.VirtualTable, error) {
	v, ok := f.vtables[rid]
	if !ok || v.CreatedBy == nil || *v.CreatedBy != ownerID {
		return nil, nil
	}
	return &v, nil
}

func (f *fakeStore) ListMediaSetSyncs(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.MediaSetSync, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return []models.MediaSetSync{}, nil
	}
	return f.mediaSyncs[sourceID], nil
}
func (f *fakeStore) GetMediaSetSync(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	for source, syncs := range f.mediaSyncs {
		if c, _ := f.GetConnectionForOwner(context.Background(), source, ownerID); c == nil {
			continue
		}
		for i := range syncs {
			if syncs[i].ID == id {
				return &syncs[i], nil
			}
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateMediaSetSync(_ context.Context, sourceID uuid.UUID, body *models.CreateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return nil, nil
	}
	m := models.MediaSetSync{ID: uuid.New(), SourceID: sourceID, Kind: body.Kind, TargetMediaSetRID: body.TargetMediaSetRID, Subfolder: strings.Trim(body.Subfolder, "/"), Filters: body.Filters, ScheduleCron: body.ScheduleCron, CreatedAt: time.Now().UTC()}
	f.mediaSyncs[sourceID] = append([]models.MediaSetSync{m}, f.mediaSyncs[sourceID]...)
	return &m, nil
}
func (f *fakeStore) UpdateMediaSetSync(_ context.Context, id uuid.UUID, body *models.UpdateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	for source, syncs := range f.mediaSyncs {
		if c, _ := f.GetConnectionForOwner(context.Background(), source, ownerID); c == nil {
			continue
		}
		for i := range syncs {
			if syncs[i].ID == id {
				if body.Kind != nil {
					syncs[i].Kind = *body.Kind
				}
				if body.TargetMediaSetRID != nil {
					syncs[i].TargetMediaSetRID = *body.TargetMediaSetRID
				}
				if body.Subfolder != nil {
					syncs[i].Subfolder = strings.Trim(*body.Subfolder, "/")
				}
				if body.Filters != nil {
					syncs[i].Filters = *body.Filters
				}
				if body.ScheduleCron != nil {
					syncs[i].ScheduleCron = body.ScheduleCron
				}
				f.mediaSyncs[source] = syncs
				return &syncs[i], nil
			}
		}
	}
	return nil, nil
}

type fakeRuntime struct {
	report *models.MediaSetSyncExecutionReport
	err    error
	called bool
}

func (f *fakeRuntime) ExecuteMediaSetSync(_ context.Context, _ *models.MediaSetSync, _ *models.RunMediaSetSyncRequest, _ string) (*models.MediaSetSyncExecutionReport, error) {
	f.called = true
	return f.report, f.err
}

func authedReq(method, target, body string, sub uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(authmw.ContextWithClaims(context.Background(), &authmw.Claims{Sub: sub}))
}

func withRouteParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.RouteContext(req.Context())
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestCreateListGetUpdateSyncJobAndRun(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID
	out := uuid.New()
	req := authedReq("POST", "/syncs", `{"source_id":"`+source.String()+`","output_dataset_id":"`+out.String()+`","file_glob":"*.csv"}`, owner)
	rec := httptest.NewRecorder()
	h.CreateSyncJob(rec, req)
	require.Equal(t, 201, rec.Code)
	var created models.SyncJob
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, source, created.SourceID)

	req = withRouteParam(authedReq("GET", "/sources/"+source.String()+"/syncs", "", owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.ListSyncJobs(rec, req)
	require.Equal(t, 200, rec.Code)
	var list []models.SyncJob
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)

	req = withRouteParam(authedReq("GET", "/syncs/"+created.ID.String(), "", owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.GetSyncJob(rec, req)
	assert.Equal(t, 200, rec.Code)
	cron := "0 * * * *"
	req = withRouteParam(authedReq("PATCH", "/syncs/"+created.ID.String(), `{"schedule_cron":"`+cron+`"}`, owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.UpdateSyncJob(rec, req)
	assert.Equal(t, 200, rec.Code)
	req = withRouteParam(authedReq("POST", "/syncs/"+created.ID.String()+"/run", "", owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.RunSyncJob(rec, req)
	require.Equal(t, 202, rec.Code)
	assert.Contains(t, rec.Body.String(), "running")
}

func TestCreateListGetVirtualTable(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	sourceRID := "ri.foundry.main.source." + uuid.NewString()
	req := withRouteParam(authedReq("POST", "/sources/enable", `{"provider":"BIGQUERY"}`, owner), "source_rid", sourceRID)
	rec := httptest.NewRecorder()
	h.EnableVirtualTableSource(rec, req)
	require.Equal(t, 200, rec.Code)
	body := `{"project_rid":"ri.project.main","locator":{"kind":"tabular","database":"db","schema":"public","table":"orders"},"table_type":"TABLE"}`
	req = withRouteParam(authedReq("POST", "/sources/virtual-tables", body, owner), "source_rid", sourceRID)
	rec = httptest.NewRecorder()
	h.CreateVirtualTable(rec, req)
	require.Equal(t, 201, rec.Code)
	var created models.VirtualTable
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "orders", created.Name)
	req = authedReq("GET", "/virtual-tables", "", owner)
	rec = httptest.NewRecorder()
	h.ListVirtualTables(rec, req)
	require.Equal(t, 200, rec.Code)
	var list models.ListVirtualTablesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	req = withRouteParam(authedReq("GET", "/virtual-tables/"+created.RID, "", owner), "rid", created.RID)
	rec = httptest.NewRecorder()
	h.GetVirtualTable(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func TestSyncAndVirtualValidationErrors(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	req := authedReq("POST", "/syncs", `{}`, owner)
	rec := httptest.NewRecorder()
	h.CreateSyncJob(rec, req)
	assert.Equal(t, 400, rec.Code)
	req = withRouteParam(authedReq("POST", "/sources/virtual-tables", `{"project_rid":"p","locator":{"kind":"bad"},"table_type":"TABLE"}`, owner), "source_rid", "missing")
	rec = httptest.NewRecorder()
	h.CreateVirtualTable(rec, req)
	assert.Equal(t, 404, rec.Code)
}

func TestSyncAndVirtualAuthTenantIsolation(t *testing.T) {
	owner := uuid.New()
	intruder := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID
	out := uuid.New()
	created, err := store.CreateSyncJob(context.Background(), &models.CreateSyncJobRequest{SourceID: source, OutputDatasetID: out}, owner)
	require.NoError(t, err)
	req := withRouteParam(authedReq("GET", "/syncs/"+created.ID.String(), "", intruder), "sync_id", created.ID.String())
	rec := httptest.NewRecorder()
	h.GetSyncJob(rec, req)
	assert.Equal(t, 404, rec.Code)
	req = httptest.NewRequest("GET", "/virtual-tables", nil)
	rec = httptest.NewRecorder()
	h.ListVirtualTables(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestCreateListGetUpdateMediaSetSync(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID
	body := `{"kind":"MEDIA_SET_SYNC","target_media_set_rid":"ri.foundry.main.media_set.` + uuid.NewString() + `","subfolder":"images","filters":{"path_glob":"*.png","file_size_limit":1024},"schedule_cron":"0 * * * *"}`
	req := withRouteParam(authedReq("POST", "/sources/"+source.String()+"/media-set-syncs", body, owner), "id", source.String())
	rec := httptest.NewRecorder()
	h.CreateMediaSetSync(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var created models.MediaSetSync
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, source, created.SourceID)
	assert.Equal(t, models.MediaSetSyncKindCopy, created.Kind)

	req = withRouteParam(authedReq("GET", "/sources/"+source.String()+"/media-set-syncs", "", owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.ListMediaSetSyncs(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []models.MediaSetSync
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)

	req = withRouteParam(authedReq("GET", "/media-set-syncs/"+created.ID.String(), "", owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.GetMediaSetSync(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	patch := `{"kind":"VIRTUAL_MEDIA_SET_SYNC","subfolder":"archive"}`
	req = withRouteParam(authedReq("PATCH", "/media-set-syncs/"+created.ID.String(), patch, owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.UpdateMediaSetSync(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var updated models.MediaSetSync
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, models.MediaSetSyncKindVirtual, updated.Kind)
	assert.Equal(t, "archive", updated.Subfolder)
}

func TestMediaSetSyncValidationErrors(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID
	req := withRouteParam(authedReq("POST", "/sources/"+source.String()+"/media-set-syncs", `{"kind":"MEDIA_SET_SYNC","target_media_set_rid":"bad","filters":{"file_size_limit":0},"schedule_cron":"bad cron"}`, owner), "id", source.String())
	rec := httptest.NewRecorder()
	h.CreateMediaSetSync(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "target_media_set_rid")
	assert.Contains(t, rec.Body.String(), "file_size_limit")

	created, err := store.CreateMediaSetSync(context.Background(), source, &models.CreateMediaSetSyncRequest{Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set." + uuid.NewString()}, owner)
	require.NoError(t, err)
	badKind := `{"kind":"BAD"}`
	req = withRouteParam(authedReq("PATCH", "/media-set-syncs/"+created.ID.String(), badKind, owner), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.UpdateMediaSetSync(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMediaSetSyncAuthTenantIsolation(t *testing.T) {
	owner := uuid.New()
	intruder := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID
	created, err := store.CreateMediaSetSync(context.Background(), source, &models.CreateMediaSetSyncRequest{Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set." + uuid.NewString()}, owner)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/sources/"+source.String()+"/media-set-syncs", nil)
	req = withRouteParam(req, "id", source.String())
	rec := httptest.NewRecorder()
	h.ListMediaSetSyncs(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	req = withRouteParam(authedReq("GET", "/media-set-syncs/"+created.ID.String(), "", intruder), "sync_id", created.ID.String())
	rec = httptest.NewRecorder()
	h.GetMediaSetSync(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	req = withRouteParam(authedReq("POST", "/sources/"+source.String()+"/media-set-syncs", `{"kind":"MEDIA_SET_SYNC","target_media_set_rid":"ri.foundry.main.media_set.x"}`, intruder), "id", source.String())
	rec = httptest.NewRecorder()
	h.CreateMediaSetSync(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRunMediaSetSyncRuntimeErrorMapping(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	source := store.connections[0].ID
	created, err := store.CreateMediaSetSync(context.Background(), source, &models.CreateMediaSetSyncRequest{Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set." + uuid.NewString()}, owner)
	require.NoError(t, err)
	rt := &fakeRuntime{err: &handlers.RuntimeError{Kind: handlers.RuntimeDispatch, Msg: "media-sets-service returned HTTP 500"}}
	h := &handlers.Handlers{Repo: store, MediaSetRuntime: rt}

	req := withRouteParam(authedReq("POST", "/media-set-syncs/"+created.ID.String()+"/run", `{"source_files":[{"path":"a.png","size_bytes":1,"mime_type":"image/png"}]}`, owner), "sync_id", created.ID.String())
	rec := httptest.NewRecorder()
	h.RunMediaSetSync(rec, req)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.True(t, rt.called)
}

func TestRunMediaSetSyncRuntimeSuccess(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	source := store.connections[0].ID
	created, err := store.CreateMediaSetSync(context.Background(), source, &models.CreateMediaSetSyncRequest{Kind: models.MediaSetSyncKindCopy, TargetMediaSetRID: "ri.foundry.main.media_set." + uuid.NewString()}, owner)
	require.NoError(t, err)
	h := &handlers.Handlers{Repo: store, MediaSetRuntime: &fakeRuntime{report: &models.MediaSetSyncExecutionReport{Stats: models.SyncStats{Accepted: 1}, Dispatched: 1}}}

	req := withRouteParam(authedReq("POST", "/media-set-syncs/"+created.ID.String()+"/run", `{"source_files":[{"path":"a.png","size_bytes":1,"mime_type":"image/png"}]}`, owner), "sync_id", created.ID.String())
	rec := httptest.NewRecorder()
	h.RunMediaSetSync(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "dispatched")
}

func TestHTTPMediaSetRuntimeDispatchesAcceptedFiles(t *testing.T) {
	seen := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	limit := uint64(10)
	glob := "*.png"
	sync := &models.MediaSetSync{
		Kind:              models.MediaSetSyncKindCopy,
		TargetMediaSetRID: "ri.foundry.main.media_set.x",
		Filters: models.MediaSetSyncFilters{
			PathGlob:      &glob,
			FileSizeLimit: &limit,
		},
	}
	req := &models.RunMediaSetSyncRequest{SourceFiles: []models.SourceFile{
		{Path: "ok.png", SizeBytes: 1, MimeType: "image/png"},
		{Path: "too-large.png", SizeBytes: 11, MimeType: "image/png"},
		{Path: "notes.txt", SizeBytes: 1, MimeType: "text/plain"},
	}, AllowedMIMETypes: []string{"image/png"}}
	rt := &handlers.HTTPMediaSetRuntime{MediaSetsBaseURL: srv.URL, Client: srv.Client()}
	report, err := rt.ExecuteMediaSetSync(context.Background(), sync, req, "Bearer token")
	require.NoError(t, err)
	require.Equal(t, uint32(1), report.Dispatched)
	require.Equal(t, uint32(1), report.Stats.Accepted)
	require.Equal(t, uint32(2), report.Stats.Skipped)
	require.Equal(t, []string{"/media-sets/ri.foundry.main.media_set.x/items/upload-url"}, seen)
}

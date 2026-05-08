package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
	cmruntime "github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/runtime"
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

type testConnectionAdapter struct {
	result adapters.ConnectionTestResult
	err    error
}

func (a testConnectionAdapter) TestConnection(_ context.Context, _ json.RawMessage) (adapters.ConnectionTestResult, error) {
	return a.result, a.err
}
func (a testConnectionAdapter) DiscoverSources(context.Context, *models.Connection, string) ([]adapters.Source, error) {
	return nil, adapters.ErrNotImplemented
}
func (a testConnectionAdapter) QueryVirtualTable(context.Context, *models.Connection, *adapters.Query, string) (*adapters.Result, error) {
	return nil, adapters.ErrNotImplemented
}
func (a testConnectionAdapter) StreamArrow(context.Context, *models.Connection, *adapters.Query, string) (adapters.ArrowStream, error) {
	return adapters.EmptyArrowStream{}, adapters.ErrNotImplemented
}
func (a testConnectionAdapter) BuildIngestSpec(context.Context, *models.Connection, *adapters.Source) (*adapters.IngestSpec, error) {
	return nil, adapters.ErrNotImplemented
}

func TestTestConnectionUsesAdapterResultAndUpdatesStatus(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore(owner)
	store.connections[0].ConnectorType = "kafka"
	store.connections[0].Status = "disconnected"
	registry := adapters.NewRegistry()
	registry.MustRegister("kafka", adapters.SingletonFactory(testConnectionAdapter{result: adapters.ConnectionTestResult{
		Success:   true,
		Message:   "validated kafka catalog with 1 topic(s)",
		LatencyMS: 7,
		Details:   json.RawMessage(`{"mode":"catalog_backed","topic_count":1}`),
	}}))
	h := &handlers.Handlers{Repo: store, AdapterRegistry: registry}
	req := httptest.NewRequest(http.MethodPost, "/connections/"+store.connections[0].ID.String()+"/test", nil)
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), &authmw.Claims{Sub: owner}))
	rec := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", store.connections[0].ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.TestConnection(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "connected", store.connections[0].Status)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
	assert.Equal(t, "validated kafka catalog with 1 topic(s)", body["message"])
	assert.Equal(t, float64(7), body["latency_ms"])
	assert.Equal(t, map[string]any{"mode": "catalog_backed", "topic_count": float64(1)}, body["details"])
}

func TestTestConnectionAdapterErrorMarksConnectionError(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore(owner)
	store.connections[0].ConnectorType = "kafka"
	registry := adapters.NewRegistry()
	registry.MustRegister("kafka", adapters.SingletonFactory(testConnectionAdapter{err: assert.AnError}))
	h := &handlers.Handlers{Repo: store, AdapterRegistry: registry}
	req := httptest.NewRequest(http.MethodPost, "/connections/"+store.connections[0].ID.String()+"/test", nil)
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), &authmw.Claims{Sub: owner}))
	rec := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", store.connections[0].ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.TestConnection(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "error", store.connections[0].Status)
	assert.Contains(t, rec.Body.String(), assert.AnError.Error())
}

type fakeStore struct {
	connections   []models.Connection
	syncJobs      map[uuid.UUID][]models.SyncJob
	mediaSyncs    map[uuid.UUID][]models.MediaSetSync
	runs          map[uuid.UUID][]models.SyncRun
	links         map[string]models.VirtualTableSourceLink
	vtables       map[string]models.VirtualTable
	registrations map[uuid.UUID][]models.ConnectionRegistration
	policies      map[uuid.UUID][]models.SourcePolicyBindingResponse
}

func newFakeStore(owner uuid.UUID) *fakeStore {
	conn := models.Connection{ID: uuid.New(), Name: "pg", ConnectorType: "postgresql", Config: json.RawMessage(`{}`), Status: "connected", OwnerID: owner, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	return &fakeStore{connections: []models.Connection{conn}, syncJobs: map[uuid.UUID][]models.SyncJob{}, mediaSyncs: map[uuid.UUID][]models.MediaSetSync{}, runs: map[uuid.UUID][]models.SyncRun{}, links: map[string]models.VirtualTableSourceLink{}, vtables: map[string]models.VirtualTable{}, registrations: map[uuid.UUID][]models.ConnectionRegistration{}, policies: map[uuid.UUID][]models.SourcePolicyBindingResponse{}}
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
func (f *fakeStore) UpdateConnection(_ context.Context, id uuid.UUID, body *models.UpdateConnectionRequest) (*models.Connection, error) {
	for i := range f.connections {
		if f.connections[i].ID == id {
			if body.Status != nil {
				f.connections[i].Status = *body.Status
			}
			if body.Name != nil {
				f.connections[i].Name = *body.Name
			}
			if body.Config != nil {
				f.connections[i].Config = body.Config
			}
			return &f.connections[i], nil
		}
	}
	return nil, nil
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
func (f *fakeStore) ListSyncRuns(_ context.Context, syncID uuid.UUID, _ uuid.UUID) ([]models.SyncRun, error) {
	return f.runs[syncID], nil
}
func (f *fakeStore) CompleteSyncRun(_ context.Context, runID uuid.UUID, _ uuid.UUID, status string, bytesWritten int64, filesWritten int64, errMsg *string, ingestJobID *string, datasetVersionID *uuid.UUID, contentHash *string) (*models.SyncRun, error) {
	for syncID, runs := range f.runs {
		for i := range runs {
			if runs[i].ID == runID {
				now := time.Now().UTC()
				runs[i].Status = status
				runs[i].FinishedAt = &now
				runs[i].BytesWritten = bytesWritten
				runs[i].FilesWritten = filesWritten
				runs[i].Error = errMsg
				runs[i].IngestJobID = ingestJobID
				runs[i].DatasetVersionID = datasetVersionID
				runs[i].ContentHash = contentHash
				f.runs[syncID] = runs
				return &runs[i], nil
			}
		}
	}
	return nil, nil
}
func (f *fakeStore) PreviousDatasetVersionForHash(_ context.Context, syncDefID uuid.UUID, contentHash string) (*uuid.UUID, error) {
	for _, run := range f.runs[syncDefID] {
		if run.ContentHash != nil && *run.ContentHash == contentHash && run.DatasetVersionID != nil {
			return run.DatasetVersionID, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) RecordDatasetVersionOnRun(_ context.Context, runID uuid.UUID, datasetVersionID uuid.UUID, contentHash string) error {
	for syncID, runs := range f.runs {
		for i := range runs {
			if runs[i].ID == runID {
				runs[i].DatasetVersionID = &datasetVersionID
				runs[i].ContentHash = &contentHash
				f.runs[syncID] = runs
				return nil
			}
		}
	}
	return nil
}
func (f *fakeStore) ListCredentials(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.CredentialResponse, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return []models.CredentialResponse{}, nil
	}
	return []models.CredentialResponse{}, nil
}
func (f *fakeStore) SetCredential(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID, kind string, _ []byte, fingerprint string) (*models.CredentialResponse, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return nil, nil
	}
	return &models.CredentialResponse{ID: uuid.New(), SourceID: sourceID, Kind: kind, Fingerprint: fingerprint, CreatedAt: time.Now().UTC()}, nil
}
func (f *fakeStore) ListSourcePolicies(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SourcePolicyBindingResponse, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return []models.SourcePolicyBindingResponse{}, nil
	}
	return append([]models.SourcePolicyBindingResponse(nil), f.policies[sourceID]...), nil
}
func (f *fakeStore) AttachPolicy(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID, policyID uuid.UUID, kind string) (*models.SourcePolicyBindingResponse, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return nil, nil
	}
	binding := models.SourcePolicyBindingResponse{SourceID: sourceID, PolicyID: policyID, Kind: kind}
	items := f.policies[sourceID]
	for i := range items {
		if items[i].PolicyID == policyID {
			items[i] = binding
			f.policies[sourceID] = items
			return &items[i], nil
		}
	}
	f.policies[sourceID] = append(items, binding)
	return &binding, nil
}
func (f *fakeStore) DetachPolicy(_ context.Context, sourceID uuid.UUID, ownerID uuid.UUID, policyID uuid.UUID) (bool, error) {
	if c, _ := f.GetConnectionForOwner(context.Background(), sourceID, ownerID); c == nil {
		return false, nil
	}
	items := f.policies[sourceID]
	for i := range items {
		if items[i].PolicyID == policyID {
			f.policies[sourceID] = append(items[:i], items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
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

func (f *fakeStore) ListRegistrations(_ context.Context, sourceID uuid.UUID) ([]models.ConnectionRegistration, error) {
	return f.registrations[sourceID], nil
}
func (f *fakeStore) UpsertRegistration(_ context.Context, sourceID uuid.UUID, source models.DiscoveredSource, mode string, autoSync bool, updateDetection bool, targetDatasetID *uuid.UUID, metadata json.RawMessage) (*models.ConnectionRegistration, error) {
	if len(metadata) == 0 || string(metadata) == "null" {
		metadata = []byte(`{}`)
	}
	for i := range f.registrations[sourceID] {
		if f.registrations[sourceID][i].Selector == source.Selector {
			f.registrations[sourceID][i].DisplayName = source.DisplayName
			f.registrations[sourceID][i].SourceKind = source.SourceKind
			f.registrations[sourceID][i].RegistrationMode = mode
			f.registrations[sourceID][i].AutoSync = autoSync
			f.registrations[sourceID][i].UpdateDetection = updateDetection
			f.registrations[sourceID][i].TargetDatasetID = targetDatasetID
			f.registrations[sourceID][i].Metadata = metadata
			f.registrations[sourceID][i].UpdatedAt = time.Now().UTC()
			return &f.registrations[sourceID][i], nil
		}
	}
	reg := models.ConnectionRegistration{ID: uuid.New(), ConnectionID: sourceID, Selector: source.Selector, DisplayName: source.DisplayName, SourceKind: source.SourceKind, RegistrationMode: mode, AutoSync: autoSync, UpdateDetection: updateDetection, TargetDatasetID: targetDatasetID, LastSourceSignature: source.SourceSignature, Metadata: metadata, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.registrations[sourceID] = append([]models.ConnectionRegistration{reg}, f.registrations[sourceID]...)
	return &reg, nil
}
func (f *fakeStore) GetRegistration(_ context.Context, sourceID uuid.UUID, registrationID uuid.UUID) (*models.ConnectionRegistration, error) {
	for i := range f.registrations[sourceID] {
		if f.registrations[sourceID][i].ID == registrationID {
			return &f.registrations[sourceID][i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) DeleteRegistration(_ context.Context, sourceID uuid.UUID, registrationID uuid.UUID) (bool, error) {
	regs := f.registrations[sourceID]
	for i := range regs {
		if regs[i].ID == registrationID {
			f.registrations[sourceID] = append(regs[:i], regs[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeStore) UpdateConnectionConfig(_ context.Context, id uuid.UUID, config json.RawMessage) (*models.Connection, error) {
	for i := range f.connections {
		if f.connections[i].ID == id {
			f.connections[i].Config = config
			f.connections[i].UpdatedAt = time.Now().UTC()
			return &f.connections[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) ListIcebergNamespaces(_ context.Context) ([]models.Connection, error) {
	out := []models.Connection{}
	for _, c := range f.connections {
		for _, r := range f.registrations[c.ID] {
			var meta map[string]any
			_ = json.Unmarshal(r.Metadata, &meta)
			if meta["supports_zero_copy"] == true {
				out = append(out, c)
				break
			}
		}
	}
	return out, nil
}
func (f *fakeStore) GetIcebergConnection(_ context.Context, namespace string) (*models.Connection, error) {
	for i := range f.connections {
		if f.connections[i].Name == namespace || strings.NewReplacer("-", "-", " ", "_", ".", "_").Replace(f.connections[i].Name) == namespace {
			return &f.connections[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) ListIcebergTables(_ context.Context, connectionID uuid.UUID) ([]models.ConnectionRegistration, error) {
	out := []models.ConnectionRegistration{}
	for _, r := range f.registrations[connectionID] {
		var meta map[string]any
		_ = json.Unmarshal(r.Metadata, &meta)
		if meta["supports_zero_copy"] == true {
			out = append(out, r)
		}
	}
	return out, nil
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

func TestSourcePolicyBindingHandlersMatchRustContract(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	sourceID := store.connections[0].ID
	policyID := uuid.New()
	h := &handlers.Handlers{Repo: store}

	req := withRouteParam(authedReq(http.MethodPost, "/sources/"+sourceID.String()+"/egress-policies", `{"policy_id":"`+policyID.String()+`"}`, owner), "id", sourceID.String())
	rec := httptest.NewRecorder()
	h.AttachPolicy(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var attached models.SourcePolicyBindingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &attached))
	require.Equal(t, sourceID, attached.SourceID)
	require.Equal(t, policyID, attached.PolicyID)
	require.Equal(t, "direct", attached.Kind)

	req = withRouteParam(authedReq(http.MethodPost, "/sources/"+sourceID.String()+"/egress-policies", `{"policy_id":"`+policyID.String()+`","kind":"agent_proxy"}`, owner), "id", sourceID.String())
	rec = httptest.NewRecorder()
	h.AttachPolicy(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, store.policies[sourceID], 1)
	require.Equal(t, "agent_proxy", store.policies[sourceID][0].Kind)

	req = withRouteParam(authedReq(http.MethodGet, "/sources/"+sourceID.String()+"/egress-policies", ``, owner), "id", sourceID.String())
	rec = httptest.NewRecorder()
	h.ListSourcePolicies(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var listed []models.SourcePolicyBindingResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed, 1)
	require.Equal(t, "agent_proxy", listed[0].Kind)

	req = withRouteParam(withRouteParam(authedReq(http.MethodDelete, "/sources/"+sourceID.String()+"/egress-policies/"+policyID.String(), ``, owner), "source_id", sourceID.String()), "policy_id", policyID.String())
	rec = httptest.NewRecorder()
	h.DetachPolicy(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Empty(t, store.policies[sourceID])
}

func TestAttachPolicyRejectsEdgeCases(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	sourceID := store.connections[0].ID
	h := &handlers.Handlers{Repo: store}

	for name, body := range map[string]string{
		"nil policy id":     `{"policy_id":"00000000-0000-0000-0000-000000000000"}`,
		"unsupported kind":  `{"policy_id":"` + uuid.NewString() + `","kind":"bucket_endpoint"}`,
		"malformed payload": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			req := withRouteParam(authedReq(http.MethodPost, "/sources/"+sourceID.String()+"/egress-policies", body, owner), "id", sourceID.String())
			rec := httptest.NewRecorder()
			h.AttachPolicy(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
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

func TestCatalogSurfaceMatchesGoldenFixtures(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	cases := []struct {
		name   string
		handle http.HandlerFunc
		golden string
	}{
		{name: "catalog", handle: h.GetConnectorCatalog, golden: "testdata/catalog.golden.json"},
		{name: "contracts", handle: h.GetConnectorContracts, golden: "testdata/contracts.golden.json"},
		{name: "streaming_sources", handle: h.ListStreamingSources, golden: "testdata/streaming_sources.golden.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handle(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, http.StatusOK, rec.Code)
			assertJSONGolden(t, tc.golden, rec.Body.Bytes())
		})
	}
}

func TestCatalogIncludesAllRustConnectorModules(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	(&handlers.Handlers{}).GetConnectorContracts(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var catalog models.ConnectorContractCatalog
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &catalog))
	byType := map[string]models.ConnectorContractProfile{}
	for _, connector := range catalog.Connectors {
		byType[connector.ConnectorType] = connector
	}
	for _, connectorType := range []string{"azure_blob", "bigquery", "csv", "databricks", "excel", "gcs", "generic", "graphql", "iot", "jdbc", "json", "kafka", "kinesis", "ldap", "mssql", "mysql", "odbc", "onelake", "open_table_catalog", "oracle", "parquet", "postgresql", "power_bi", "rest_api", "s3", "salesforce", "sap", "sftp", "snowflake", "tableau"} {
		require.Contains(t, byType, connectorType)
	}
}

func TestConnectionCapabilitiesCombineContractConfigAndPolicy(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	store := newFakeStore(owner)
	store.connections[0].ConnectorType = "snowflake"
	store.connections[0].Config = json.RawMessage(`{"account":"acct","private_key":"pk","cursor_field":"updated_at","zero_copy":true}`)
	h := &handlers.Handlers{Repo: store, Config: handlers.RuntimeConfig{AllowedEgressHosts: []string{"snowflake.example.com"}}}

	r := chi.NewRouter()
	r.Get("/api/v1/data-connection/sources/{id}/capabilities", h.GetConnectionCapabilities)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data-connection/sources/"+store.connections[0].ID.String()+"/capabilities", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.ConnectionCapabilityResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "snowflake", got.ConnectorType)
	require.Equal(t, "warehouse_zero_copy", got.Contract.TemplateFamily)
	require.True(t, got.Capabilities.SupportsZeroCopy)
	require.True(t, got.Capabilities.SupportsIncremental)
	require.True(t, got.Capabilities.ConfigInferred.HasPrivateKey)
	require.True(t, got.Capabilities.ConfigInferred.HasIncrementalCursor)
	require.True(t, got.Capabilities.PrivateNetworkEgressAllowed)
	require.False(t, got.Capabilities.RequiresPrivateNetworkAgent)
	require.Contains(t, got.Capabilities.Workers, "agent")
	require.Contains(t, got.Capabilities.ConfigKeys, "private_key")
}

func assertJSONGolden(t *testing.T, golden string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	var wantJSON any
	var gotJSON any
	require.NoError(t, json.Unmarshal(want, &wantJSON))
	require.NoError(t, json.Unmarshal(got, &gotJSON))
	assert.Equal(t, wantJSON, gotJSON)
}

func TestRegistrationHandlerFlow(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	source := store.connections[0].ID
	store.connections[0].Config = json.RawMessage(`{"tables":[{"selector":"public.orders","display_name":"Orders","source_kind":"table","supports_zero_copy":true}]}`)
	h := &handlers.Handlers{Repo: store}

	req := withRouteParam(authedReq(http.MethodPost, "/discover", ``, owner), "id", source.String())
	rec := httptest.NewRecorder()
	h.DiscoverRegistrations(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "public.orders")

	body := `{"registrations":[{"selector":"public.orders","registration_mode":"zero_copy","auto_sync":true}]}`
	req = withRouteParam(authedReq(http.MethodPost, "/bulk/preview", body, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.BulkRegisterPreview(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "matched")

	req = withRouteParam(authedReq(http.MethodPost, "/bulk", body, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.BulkRegister(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	reg := store.registrations[source][0]

	req = withRouteParam(authedReq(http.MethodGet, "/registrations", ``, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.ListRegistrations(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(withRouteParam(authedReq(http.MethodPost, "/query", `{"limit":1}`, owner), "source_id", source.String()), "registration_id", reg.ID.String())
	rec = httptest.NewRecorder()
	h.QueryRegistration(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(withRouteParam(authedReq(http.MethodPost, "/query/arrow", `{"limit":1}`, owner), "source_id", source.String()), "registration_id", reg.ID.String())
	rec = httptest.NewRecorder()
	h.QueryRegistrationArrow(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "application/vnd.apache.arrow.stream", rec.Header().Get("Content-Type"))

	req = withRouteParam(withRouteParam(authedReq(http.MethodDelete, "/registrations/"+reg.ID.String(), ``, owner), "source_id", source.String()), "registration_id", reg.ID.String())
	rec = httptest.NewRecorder()
	h.DeleteRegistration(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

func TestAutoRegistrationHandlers(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	source := store.connections[0].ID
	h := &handlers.Handlers{Repo: store}

	req := withRouteParam(authedReq(http.MethodPut, "/auto", `{"enabled":true,"registration_mode":"sync","selectors":["pg"]}`, owner), "id", source.String())
	rec := httptest.NewRecorder()
	h.UpdateAutoRegistration(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(authedReq(http.MethodGet, "/auto/status", ``, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.AutoRegisterStatus(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "settings")

	req = withRouteParam(authedReq(http.MethodPost, "/auto", `{}`, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.AutoRegister(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotEmpty(t, store.registrations[source])
}

func TestConnectionWebhookAndIcebergHandlers(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	source := store.connections[0].ID

	req := withRouteParam(authedReq(http.MethodPost, "/test", ``, owner), "id", source.String())
	rec := httptest.NewRecorder()
	h.TestConnection(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"output_parameters":{"ok":true}}`))
	}))
	defer srv.Close()
	store.connections[0].ConnectorType = "webhook"
	store.connections[0].Config = json.RawMessage(`{"url":"` + srv.URL + `","method":"POST"}`)
	req = withRouteParam(authedReq(http.MethodPost, "/webhooks/"+source.String()+"/invoke", `{"inputs":{"x":1}}`, owner), "id", source.String())
	rec = httptest.NewRecorder()
	h.InvokeWebhook(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "output_parameters")

	store.connections[0].ConnectorType = "postgresql"
	store.registrations[source] = []models.ConnectionRegistration{{ID: uuid.New(), ConnectionID: source, Selector: "public.orders", DisplayName: "Orders", SourceKind: "table", RegistrationMode: "zero_copy", Metadata: json.RawMessage(`{"supports_zero_copy":true}`)}}
	req = authedReq(http.MethodGet, "/iceberg/v1/config", ``, owner)
	rec = httptest.NewRecorder()
	h.IcebergGetConfig(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = authedReq(http.MethodGet, "/iceberg/v1/namespaces", ``, owner)
	rec = httptest.NewRecorder()
	h.IcebergListNamespaces(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(authedReq(http.MethodGet, "/iceberg/v1/namespaces/pg", ``, owner), "namespace", "pg")
	rec = httptest.NewRecorder()
	h.IcebergGetNamespace(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(authedReq(http.MethodGet, "/iceberg/v1/namespaces/pg/tables", ``, owner), "namespace", "pg")
	rec = httptest.NewRecorder()
	h.IcebergListTables(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	req = withRouteParam(withRouteParam(authedReq(http.MethodGet, "/iceberg/v1/namespaces/pg/tables/public.orders", ``, owner), "namespace", "pg"), "table", "public.orders")
	rec = httptest.NewRecorder()
	h.IcebergLoadTable(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

type fakeIngestionPort struct {
	requests []cmruntime.IngestionRequest
	result   cmruntime.IngestionResult
	err      error
}

func (f *fakeIngestionPort) Dispatch(_ context.Context, req cmruntime.IngestionRequest) (*cmruntime.IngestionResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	result := f.result
	if result.IngestJobID == "" {
		result.IngestJobID = "ingest-" + req.RunID.String()
	}
	if result.Payload == nil {
		result.Payload = req.Materialized
	}
	if result.BytesWritten == 0 {
		result.BytesWritten = int64(len(result.Payload))
	}
	if result.FilesWritten == 0 {
		result.FilesWritten = 1
	}
	return &result, nil
}

type fakeDatasetVersioningPort struct {
	requests []cmruntime.DatasetVersionRequest
	id       uuid.UUID
	err      error
}

func (f *fakeDatasetVersioningPort) Register(_ context.Context, req cmruntime.DatasetVersionRequest) (*cmruntime.DatasetVersionResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	id := f.id
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &cmruntime.DatasetVersionResult{DatasetVersionID: id}, nil
}

func TestRunSyncJobDispatchesIngestionAndRegistersDatasetVersion(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	job, err := store.CreateSyncJob(context.Background(), &models.CreateSyncJobRequest{SourceID: store.connections[0].ID, OutputDatasetID: uuid.New()}, owner)
	require.NoError(t, err)
	ingestion := &fakeIngestionPort{result: cmruntime.IngestionResult{RowsWritten: 7, Payload: []byte(`{"rows":7}`)}}
	versionID := uuid.New()
	versions := &fakeDatasetVersioningPort{id: versionID}
	h := &handlers.Handlers{Repo: store, IngestionRuntime: ingestion, DatasetVersioning: versions}

	req := withRouteParam(authedReq(http.MethodPost, "/syncs/"+job.ID.String()+"/run", "", owner), "sync_id", job.ID.String())
	rec := httptest.NewRecorder()
	h.RunSyncJob(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var run models.SyncRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &run))
	assert.Equal(t, "succeeded", run.Status)
	assert.NotNil(t, run.FinishedAt)
	assert.NotNil(t, run.IngestJobID)
	assert.Equal(t, versionID, *run.DatasetVersionID)
	assert.NotEmpty(t, *run.ContentHash)
	require.Len(t, ingestion.requests, 1)
	assert.Equal(t, job.ID, ingestion.requests[0].SyncDefID)
	require.Len(t, versions.requests, 1)
	assert.Equal(t, job.OutputDatasetID, versions.requests[0].OutputDatasetID)
	assert.Equal(t, *run.ContentHash, versions.requests[0].ContentHash)

	req = withRouteParam(authedReq(http.MethodGet, "/syncs/"+job.ID.String()+"/runs", "", owner), "sync_id", job.ID.String())
	rec = httptest.NewRecorder()
	h.ListRuns(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var runs []models.SyncRun
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runs))
	require.Len(t, runs, 1)
	assert.Equal(t, "succeeded", runs[0].Status)
}

func TestRunSyncJobReusesDatasetVersionForSameContentHash(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	job, err := store.CreateSyncJob(context.Background(), &models.CreateSyncJobRequest{SourceID: store.connections[0].ID, OutputDatasetID: uuid.New()}, owner)
	require.NoError(t, err)
	payload := []byte(`stable-payload`)
	ingestion := &fakeIngestionPort{result: cmruntime.IngestionResult{Payload: payload, BytesWritten: int64(len(payload)), FilesWritten: 1}}
	versions := &fakeDatasetVersioningPort{id: uuid.New()}
	h := &handlers.Handlers{Repo: store, IngestionRuntime: ingestion, DatasetVersioning: versions}

	for range 2 {
		req := withRouteParam(authedReq(http.MethodPost, "/syncs/"+job.ID.String()+"/run", "", owner), "sync_id", job.ID.String())
		rec := httptest.NewRecorder()
		h.RunSyncJob(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	}
	require.Len(t, versions.requests, 1, "second run should reuse the recorded dataset version for the same content hash")
	runs := store.runs[job.ID]
	require.Len(t, runs, 2)
	require.NotNil(t, runs[0].DatasetVersionID)
	require.NotNil(t, runs[1].DatasetVersionID)
	assert.Equal(t, *runs[0].DatasetVersionID, *runs[1].DatasetVersionID)
}

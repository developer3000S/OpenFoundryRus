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
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func TestDatasetJSONShape(t *testing.T) {
	t.Parallel()
	d := models.Dataset{
		ID: uuid.New(), Name: "sales", Description: "fact",
		Format: "parquet", StoragePath: "s3://x/y",
		SizeBytes: 1024, RowCount: 100,
		OwnerID:        uuid.New(),
		Tags:           []string{"finance", "daily"},
		CurrentVersion: 1,
		CreatedAt:      time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(d)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "name", "description", "format", "storage_path", "size_bytes",
		"row_count", "owner_id", "tags", "current_version", "created_at", "updated_at",
	} {
		assert.Contains(t, view, k)
	}
	assert.Equal(t, "parquet", view["format"])
}

func TestCreateDatasetRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("POST", "/datasets",
		strings.NewReader(`{"name":"x","storage_path":"s3://y"}`))
	rec := httptest.NewRecorder()
	h.CreateDataset(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestCreateDatasetRejectsEmptyFields(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/datasets",
		strings.NewReader(`{"name":"","storage_path":""}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateDataset(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "name and storage_path required")
}

func TestListDatasetsRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("GET", "/datasets", nil)
	rec := httptest.NewRecorder()
	h.ListDatasets(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestListDatasetsRejectsBadOwnerID(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("GET", "/datasets?owner_id=not-uuid", nil)
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.ListDatasets(rec, req)
	assert.Equal(t, 400, rec.Code)
}

type fakeStore struct {
	datasets        []models.Dataset
	versions        map[uuid.UUID][]models.DatasetVersion
	branches        map[uuid.UUID][]models.DatasetBranch
	files           map[uuid.UUID][]models.DatasetFile
	transactions    map[uuid.UUID]string
	versionConflict bool
	branchConflict  bool
}

func newFakeStore(owner uuid.UUID) *fakeStore {
	ds := models.Dataset{
		ID: uuid.New(), Name: "sales", Description: "", Format: "parquet",
		StoragePath: "s3://bucket/sales", OwnerID: owner, CurrentVersion: 1,
		Tags: []string{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	return &fakeStore{datasets: []models.Dataset{ds}, versions: map[uuid.UUID][]models.DatasetVersion{}, branches: map[uuid.UUID][]models.DatasetBranch{}, files: map[uuid.UUID][]models.DatasetFile{}, transactions: map[uuid.UUID]string{}}
}

func (f *fakeStore) ListDatasets(_ context.Context, ownerID *uuid.UUID, _ int) ([]models.Dataset, error) {
	out := []models.Dataset{}
	for _, d := range f.datasets {
		if ownerID == nil || d.OwnerID == *ownerID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeStore) GetDataset(_ context.Context, id uuid.UUID) (*models.Dataset, error) {
	for i := range f.datasets {
		if f.datasets[i].ID == id {
			return &f.datasets[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) GetDatasetForOwner(_ context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Dataset, error) {
	for i := range f.datasets {
		if f.datasets[i].ID == id && f.datasets[i].OwnerID == ownerID {
			return &f.datasets[i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateDataset(_ context.Context, body *models.CreateDatasetRequest, ownerID uuid.UUID) (*models.Dataset, error) {
	d := models.Dataset{ID: uuid.New(), Name: body.Name, StoragePath: body.StoragePath, Format: "parquet", OwnerID: ownerID, Tags: []string{}, CurrentVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	f.datasets = append(f.datasets, d)
	return &d, nil
}
func (f *fakeStore) UpdateDataset(_ context.Context, id uuid.UUID, _ *models.UpdateDatasetRequest) (*models.Dataset, error) {
	return f.GetDataset(context.Background(), id)
}
func (f *fakeStore) DeleteDataset(_ context.Context, id uuid.UUID) (bool, error) {
	_, err := f.GetDataset(context.Background(), id)
	return err == nil, err
}
func (f *fakeStore) ListVersions(_ context.Context, datasetID uuid.UUID) ([]models.DatasetVersion, error) {
	return f.versions[datasetID], nil
}
func (f *fakeStore) GetVersion(_ context.Context, datasetID uuid.UUID, version int32) (*models.DatasetVersion, error) {
	for i := range f.versions[datasetID] {
		if f.versions[datasetID][i].Version == version {
			return &f.versions[datasetID][i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateVersion(_ context.Context, datasetID uuid.UUID, body *models.CreateDatasetVersionRequest) (*models.DatasetVersion, error) {
	if f.versionConflict {
		return nil, repo.ErrConflict
	}
	version := int32(len(f.versions[datasetID]) + 1)
	if body.Version != nil {
		version = *body.Version
	}
	v := models.DatasetVersion{ID: uuid.New(), DatasetID: datasetID, Version: version, Message: body.Message, SizeBytes: body.SizeBytes, RowCount: body.RowCount, StoragePath: body.StoragePath, CreatedAt: time.Now().UTC()}
	f.versions[datasetID] = append([]models.DatasetVersion{v}, f.versions[datasetID]...)
	return &v, nil
}
func (f *fakeStore) EnsureDefaultBranch(_ context.Context, dataset *models.Dataset) error {
	if len(f.branches[dataset.ID]) == 0 {
		b := models.DatasetBranch{ID: uuid.New(), RID: "ri.foundry.main.branch." + uuid.NewString(), DatasetID: dataset.ID, DatasetRID: "ri.foundry.main.dataset." + dataset.ID.String(), Name: "main", Labels: []byte(`{}`), FallbackChain: []string{}, Version: dataset.CurrentVersion, BaseVersion: dataset.CurrentVersion, Description: "Default branch", IsDefault: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}
		f.branches[dataset.ID] = append(f.branches[dataset.ID], b)
	}
	return nil
}
func (f *fakeStore) ListBranches(_ context.Context, datasetID uuid.UUID) ([]models.DatasetBranch, error) {
	return f.branches[datasetID], nil
}
func (f *fakeStore) GetBranch(_ context.Context, datasetID uuid.UUID, name string) (*models.DatasetBranch, error) {
	for i := range f.branches[datasetID] {
		if f.branches[datasetID][i].Name == name {
			return &f.branches[datasetID][i], nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CreateBranch(_ context.Context, dataset *models.Dataset, body *models.CreateDatasetBranchRequest) (*models.DatasetBranch, error) {
	if f.branchConflict {
		return nil, repo.ErrConflict
	}
	b := models.DatasetBranch{ID: uuid.New(), RID: "ri.foundry.main.branch." + uuid.NewString(), DatasetID: dataset.ID, DatasetRID: "ri.foundry.main.dataset." + dataset.ID.String(), Name: strings.TrimSpace(body.Name), Labels: []byte(`{}`), FallbackChain: []string{}, Version: dataset.CurrentVersion, BaseVersion: dataset.CurrentVersion, Description: body.Description, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}
	f.branches[dataset.ID] = append(f.branches[dataset.ID], b)
	return &b, nil
}

func (f *fakeStore) ListFiles(_ context.Context, datasetID uuid.UUID, _ string, prefix string) ([]models.DatasetFile, error) {
	out := []models.DatasetFile{}
	for _, file := range f.files[datasetID] {
		if prefix == "" || strings.HasPrefix(file.LogicalPath, prefix) {
			out = append(out, file)
		}
	}
	return out, nil
}
func (f *fakeStore) GetFile(_ context.Context, datasetID uuid.UUID, fileID uuid.UUID) (*models.DatasetFile, error) {
	for i := range f.files[datasetID] {
		if f.files[datasetID][i].ID == fileID {
			return &f.files[datasetID][i], nil
		}
	}
	return nil, nil
}

func (f *fakeStore) GetTransactionStatus(_ context.Context, _ uuid.UUID, transactionID uuid.UUID) (string, bool, error) {
	status, ok := f.transactions[transactionID]
	return status, ok, nil
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

func datasetReq(method string, store *fakeStore, owner uuid.UUID, body string) *http.Request {
	req := authedReq(method, "/datasets/"+store.datasets[0].ID.String(), body, owner)
	return withRouteParam(req, "id", store.datasets[0].ID.String())
}

func TestCreateListGetVersion(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	req := datasetReq("POST", store, owner, `{"version":2,"message":"snapshot","storage_path":"s3://v2","size_bytes":10,"row_count":1}`)
	rec := httptest.NewRecorder()
	h.CreateVersion(rec, req)
	require.Equal(t, 201, rec.Code)
	var created models.DatasetVersion
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, int32(2), created.Version)

	req = datasetReq("GET", store, owner, "")
	rec = httptest.NewRecorder()
	h.ListVersions(rec, req)
	require.Equal(t, 200, rec.Code)
	var page models.Page[models.DatasetVersion]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)

	req = withRouteParam(datasetReq("GET", store, owner, ""), "version", "2")
	rec = httptest.NewRecorder()
	h.GetVersion(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func TestCreateListGetBranch(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	req := datasetReq("POST", store, owner, `{"name":"feature","description":"work"}`)
	rec := httptest.NewRecorder()
	h.CreateBranch(rec, req)
	require.Equal(t, 201, rec.Code)
	var created models.DatasetBranch
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "feature", created.Name)

	req = datasetReq("GET", store, owner, "")
	rec = httptest.NewRecorder()
	h.ListBranches(rec, req)
	require.Equal(t, 200, rec.Code)
	var branches []models.DatasetBranch
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &branches))
	assert.GreaterOrEqual(t, len(branches), 2)

	req = withRouteParam(datasetReq("GET", store, owner, ""), "branch", "feature")
	rec = httptest.NewRecorder()
	h.GetBranch(rec, req)
	assert.Equal(t, 200, rec.Code)
}

func TestBranchNotFound(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	req := withRouteParam(datasetReq("GET", store, owner, ""), "branch", "missing")
	rec := httptest.NewRecorder()
	h.GetBranch(rec, req)
	assert.Equal(t, 404, rec.Code)
}

func TestVersionAndBranchConflicts(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	store.versionConflict = true
	req := datasetReq("POST", store, owner, `{"version":1,"storage_path":"s3://v1"}`)
	rec := httptest.NewRecorder()
	h.CreateVersion(rec, req)
	assert.Equal(t, 409, rec.Code)
	store.branchConflict = true
	req = datasetReq("POST", store, owner, `{"name":"feature"}`)
	rec = httptest.NewRecorder()
	h.CreateBranch(rec, req)
	assert.Equal(t, 409, rec.Code)
}

func TestTenantIsolationForNestedSurfaces(t *testing.T) {
	owner := uuid.New()
	intruder := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	req := datasetReq("GET", store, intruder, "")
	rec := httptest.NewRecorder()
	h.ListVersions(rec, req)
	assert.Equal(t, 404, rec.Code)
	req = datasetReq("GET", store, intruder, "")
	rec = httptest.NewRecorder()
	h.ListBranches(rec, req)
	assert.Equal(t, 404, rec.Code)
}

func TestListAndDownloadFiles(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	fileID := uuid.New()
	store.files[datasetID] = []models.DatasetFile{{
		ID: fileID, DatasetID: datasetID, TransactionID: uuid.New(), LogicalPath: "daily/part-000.parquet",
		PhysicalURI: "local:///datasets/sales/daily/part-000.parquet", SizeBytes: 42,
		CreatedAt: time.Now().UTC(), ModifiedAt: time.Now().UTC(), Status: "active",
	}}
	fs := storageabstraction.NewLocalBackingFS("http://files.local", "", []byte("test-secret"))
	h := &handlers.Handlers{Repo: store, BackingFS: fs, PresignTTL: time.Minute}

	req := datasetReq("GET", store, owner, "")
	req.URL.RawQuery = "prefix=daily/"
	rec := httptest.NewRecorder()
	h.ListFiles(rec, req)
	require.Equal(t, 200, rec.Code)
	var listed models.ListDatasetFilesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Files, 1)
	assert.Equal(t, "daily/part-000.parquet", listed.Files[0].LogicalPath)

	req = withRouteParam(datasetReq("GET", store, owner, ""), "file_id", fileID.String())
	rec = httptest.NewRecorder()
	h.DownloadFile(rec, req)
	require.Equal(t, 200, rec.Code)
	var downloaded models.DownloadDatasetFileResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &downloaded))
	assert.Equal(t, "GET", downloaded.Method)
	assert.Contains(t, downloaded.URL, "http://files.local/api/v1/_internal/local-fs/datasets/sales/daily/part-000.parquet")
}

func TestDownloadDeletedFileReturnsGone(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	fileID := uuid.New()
	deletedAt := time.Now().UTC()
	store.files[datasetID] = []models.DatasetFile{{ID: fileID, DatasetID: datasetID, TransactionID: uuid.New(), LogicalPath: "old.csv", PhysicalURI: "local:///old.csv", DeletedAt: &deletedAt, Status: "deleted"}}
	h := &handlers.Handlers{Repo: store, BackingFS: storageabstraction.NewLocalBackingFS("http://files.local", "", []byte("test-secret"))}

	req := withRouteParam(datasetReq("GET", store, owner, ""), "file_id", fileID.String())
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, req)
	assert.Equal(t, http.StatusGone, rec.Code)
}

func TestCreateFileUploadURL(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	txnID := uuid.New()
	store.transactions[txnID] = "OPEN"
	h := &handlers.Handlers{Repo: store, BackingFS: storageabstraction.NewLocalBackingFS("http://files.local", "dataset-root", []byte("secret")), PresignTTL: time.Minute}

	req := withRouteParam(datasetReq("POST", store, owner, `{"logical_path":"incoming/file.csv"}`), "txn", txnID.String())
	rec := httptest.NewRecorder()
	h.CreateFileUploadURL(rec, req)
	require.Equal(t, 200, rec.Code)
	var out models.CreateDatasetFileUploadURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "PUT", out.Method)
	assert.Equal(t, "local:///dataset-root/transactions/"+txnID.String()+"/incoming/file.csv", out.PhysicalURI)
}

func TestCreateFileUploadURLRejectsClosedTransaction(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	txnID := uuid.New()
	store.transactions[txnID] = "COMMITTED"
	h := &handlers.Handlers{Repo: store, BackingFS: storageabstraction.NewLocalBackingFS("http://files.local", "", []byte("secret"))}

	req := withRouteParam(datasetReq("POST", store, owner, `{"logical_path":"file.csv"}`), "txn", txnID.String())
	rec := httptest.NewRecorder()
	h.CreateFileUploadURL(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

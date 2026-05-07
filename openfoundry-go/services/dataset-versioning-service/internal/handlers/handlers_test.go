package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	datasets           []models.Dataset
	versions           map[uuid.UUID][]models.DatasetVersion
	branches           map[uuid.UUID][]models.DatasetBranch
	files              map[uuid.UUID][]models.DatasetFile
	transactions       map[uuid.UUID]string
	markings           map[uuid.UUID][]models.EffectiveMarking
	permissions        map[uuid.UUID][]models.DatasetPermissionEdge
	lineageLinks       map[uuid.UUID][]models.DatasetLineageLink
	fileIndex          map[uuid.UUID][]models.DatasetFileIndexEntry
	views              map[uuid.UUID][]models.DatasetView
	schemas            map[uuid.UUID]models.SchemaResponse
	versionConflict    bool
	branchConflict     bool
	permissionConflict bool
}

func newFakeStore(owner uuid.UUID) *fakeStore {
	ds := models.Dataset{
		ID: uuid.New(), Name: "sales", Description: "", Format: "parquet",
		StoragePath: "s3://bucket/sales", OwnerID: owner, CurrentVersion: 1,
		Tags: []string{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	return &fakeStore{datasets: []models.Dataset{ds}, versions: map[uuid.UUID][]models.DatasetVersion{}, branches: map[uuid.UUID][]models.DatasetBranch{}, files: map[uuid.UUID][]models.DatasetFile{}, transactions: map[uuid.UUID]string{}, markings: map[uuid.UUID][]models.EffectiveMarking{}, permissions: map[uuid.UUID][]models.DatasetPermissionEdge{}, lineageLinks: map[uuid.UUID][]models.DatasetLineageLink{}, fileIndex: map[uuid.UUID][]models.DatasetFileIndexEntry{}, views: map[uuid.UUID][]models.DatasetView{}, schemas: map[uuid.UUID]models.SchemaResponse{}}
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

func (f *fakeStore) StorageDetails(_ context.Context, datasetID uuid.UUID, fsID string, driver string, baseDir string, ttlSeconds uint64) (*models.StorageDetailsOut, error) {
	out := &models.StorageDetailsOut{FSID: fsID, Driver: driver, BaseDirectory: baseDir, PresignTTLSeconds: ttlSeconds}
	for _, file := range f.files[datasetID] {
		if file.DeletedAt == nil {
			out.TotalActiveFiles++
			out.TotalActiveBytes += file.SizeBytes
		} else {
			out.TotalDeletedFiles++
			out.TotalDeletedBytes += file.SizeBytes
		}
	}
	return out, nil
}

func (f *fakeStore) GetTransactionStatus(_ context.Context, _ uuid.UUID, transactionID uuid.UUID) (string, bool, error) {
	status, ok := f.transactions[transactionID]
	return status, ok, nil
}

func (f *fakeStore) ResolveDatasetID(_ context.Context, raw string) (uuid.UUID, error) {
	if id, err := uuid.Parse(raw); err == nil {
		for _, d := range f.datasets {
			if d.ID == id {
				return id, nil
			}
		}
		return uuid.Nil, repo.ErrNotFound
	}
	for _, d := range f.datasets {
		if raw == "ri.foundry.main.dataset."+d.ID.String() {
			return d.ID, nil
		}
	}
	return uuid.Nil, repo.ErrNotFound
}

func (f *fakeStore) DatasetExists(_ context.Context, datasetID uuid.UUID) (bool, error) {
	for _, d := range f.datasets {
		if d.ID == datasetID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) DatasetViewBelongsToDataset(_ context.Context, _ uuid.UUID, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeStore) GetCatalogDataset(_ context.Context, datasetID uuid.UUID) (*models.CatalogDataset, error) {
	d, err := f.GetDataset(context.Background(), datasetID)
	if err != nil || d == nil {
		return nil, err
	}
	return &models.CatalogDataset{ID: d.ID, Name: d.Name, Description: d.Description, Format: d.Format, StoragePath: d.StoragePath, SizeBytes: d.SizeBytes, RowCount: d.RowCount, OwnerID: d.OwnerID, Tags: d.Tags, CurrentVersion: d.CurrentVersion, ActiveBranch: "main", Metadata: []byte(`{}`), HealthStatus: "unknown", CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}, nil
}

func (f *fakeStore) GetDatasetRichModel(ctx context.Context, datasetID uuid.UUID) (*models.DatasetRichModel, error) {
	cat, err := f.GetCatalogDataset(ctx, datasetID)
	if err != nil || cat == nil {
		return nil, err
	}
	d := models.Dataset{ID: cat.ID, Name: cat.Name, Description: cat.Description, Format: cat.Format, StoragePath: cat.StoragePath, SizeBytes: cat.SizeBytes, RowCount: cat.RowCount, OwnerID: cat.OwnerID, Tags: cat.Tags, CurrentVersion: cat.CurrentVersion, CreatedAt: cat.CreatedAt, UpdatedAt: cat.UpdatedAt}
	return &models.DatasetRichModel{Dataset: d, Files: f.fileIndex[datasetID], Branches: f.branches[datasetID], Versions: f.versions[datasetID], Health: models.DatasetHealthSummary{Status: cat.HealthStatus}, Markings: f.markings[datasetID], Permissions: f.permissions[datasetID], LineageLinks: f.lineageLinks[datasetID]}, nil
}

func (f *fakeStore) PatchDatasetMetadata(_ context.Context, datasetID uuid.UUID, body *models.DatasetMetadataPatch) (*models.CatalogDataset, error) {
	for i := range f.datasets {
		if f.datasets[i].ID == datasetID {
			if body.Name != nil {
				f.datasets[i].Name = *body.Name
			}
			if body.Description != nil {
				f.datasets[i].Description = *body.Description
			}
			if body.Format != nil {
				f.datasets[i].Format = *body.Format
			}
			if body.Tags != nil {
				f.datasets[i].Tags = body.Tags
			}
			return f.GetCatalogDataset(context.Background(), datasetID)
		}
	}
	return nil, repo.ErrNotFound
}

func (f *fakeStore) ListDatasetMarkings(_ context.Context, datasetID uuid.UUID) ([]models.EffectiveMarking, error) {
	return f.markings[datasetID], nil
}
func (f *fakeStore) ReplaceDatasetMarkings(_ context.Context, datasetID uuid.UUID, markings []uuid.UUID, _ uuid.UUID) error {
	out := []models.EffectiveMarking{}
	for _, id := range markings {
		out = append(out, models.EffectiveMarking{ID: id, Source: models.MarkingReason{Kind: "direct"}})
	}
	f.markings[datasetID] = out
	return nil
}
func (f *fakeStore) ListDatasetPermissions(_ context.Context, datasetID uuid.UUID) ([]models.DatasetPermissionEdge, error) {
	return f.permissions[datasetID], nil
}
func (f *fakeStore) ReplaceDatasetPermissions(_ context.Context, datasetID uuid.UUID, permissions []models.PutDatasetPermissionEdge) error {
	if f.permissionConflict {
		return repo.ErrConflict
	}
	out := []models.DatasetPermissionEdge{}
	for _, p := range permissions {
		source := "direct"
		if p.Source != nil {
			source = *p.Source
		}
		out = append(out, models.DatasetPermissionEdge{ID: uuid.New(), DatasetID: datasetID, PrincipalKind: p.PrincipalKind, PrincipalID: p.PrincipalID, Role: p.Role, Actions: p.Actions, Source: source, InheritedFrom: p.InheritedFrom, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	}
	f.permissions[datasetID] = out
	return nil
}
func (f *fakeStore) ListDatasetLineageLinks(_ context.Context, datasetID uuid.UUID) ([]models.DatasetLineageLink, error) {
	return f.lineageLinks[datasetID], nil
}
func (f *fakeStore) ReplaceDatasetLineageLinks(_ context.Context, datasetID uuid.UUID, links []models.PutDatasetLineageLink) error {
	out := []models.DatasetLineageLink{}
	for _, l := range links {
		targetKind := "dataset"
		if l.TargetKind != nil {
			targetKind = *l.TargetKind
		}
		relationKind := "derives_from"
		if l.RelationKind != nil {
			relationKind = *l.RelationKind
		}
		out = append(out, models.DatasetLineageLink{ID: uuid.New(), DatasetID: datasetID, Direction: l.Direction, TargetRID: l.TargetRID, TargetKind: targetKind, RelationKind: relationKind, PipelineID: l.PipelineID, WorkflowID: l.WorkflowID, Metadata: l.Metadata, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	}
	f.lineageLinks[datasetID] = out
	return nil
}
func (f *fakeStore) ListDatasetFileIndex(_ context.Context, datasetID uuid.UUID) ([]models.DatasetFileIndexEntry, error) {
	return f.fileIndex[datasetID], nil
}
func (f *fakeStore) ReplaceDatasetFileIndex(_ context.Context, datasetID uuid.UUID, files []models.PutDatasetFileIndexEntry) error {
	out := []models.DatasetFileIndexEntry{}
	for _, file := range files {
		entryType := "file"
		if file.EntryType != nil {
			entryType = *file.EntryType
		}
		size := int64(0)
		if file.SizeBytes != nil {
			size = *file.SizeBytes
		}
		out = append(out, models.DatasetFileIndexEntry{ID: uuid.New(), DatasetID: datasetID, Path: file.Path, StoragePath: file.StoragePath, EntryType: entryType, SizeBytes: size, ContentType: file.ContentType, Metadata: file.Metadata, LastModified: file.LastModified, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	}
	f.fileIndex[datasetID] = out
	return nil
}

func (f *fakeStore) ListActiveRuntimeBranches(_ context.Context, datasetID uuid.UUID) ([]models.RuntimeBranch, error) {
	out := []models.RuntimeBranch{}
	for _, b := range f.branches[datasetID] {
		out = append(out, models.RuntimeBranch{ID: b.ID, RID: b.RID, DatasetID: b.DatasetID, DatasetRID: b.DatasetRID, Name: b.Name, ParentBranchID: b.ParentBranchID, HeadTransactionID: b.HeadTransactionID, CreatedFromTransactionID: b.CreatedFromTransactionID, LastActivityAt: b.LastActivityAt, Labels: b.Labels, FallbackChain: b.FallbackChain, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt})
	}
	return out, nil
}
func (f *fakeStore) CreateRuntimeBranch(_ context.Context, datasetID uuid.UUID, body *models.CreateBranchBody, _ uuid.UUID) (*models.RuntimeBranch, error) {
	if f.branchConflict {
		return nil, repo.ErrConflict
	}
	b := models.DatasetBranch{ID: uuid.New(), RID: "ri.foundry.main.branch." + uuid.NewString(), DatasetID: datasetID, DatasetRID: "ri.foundry.main.dataset." + datasetID.String(), Name: strings.TrimSpace(body.Name), Labels: []byte(`{}`), FallbackChain: body.FallbackChain, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}
	if body.ParentBranch != nil {
		for _, p := range f.branches[datasetID] {
			if p.Name == *body.ParentBranch {
				b.ParentBranchID = &p.ID
			}
		}
	}
	f.branches[datasetID] = append(f.branches[datasetID], b)
	return &models.RuntimeBranch{ID: b.ID, RID: b.RID, DatasetID: b.DatasetID, DatasetRID: b.DatasetRID, Name: b.Name, ParentBranchID: b.ParentBranchID, Labels: b.Labels, FallbackChain: b.FallbackChain, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, LastActivityAt: b.LastActivityAt}, nil
}
func (f *fakeStore) GetRuntimeBranch(_ context.Context, datasetID uuid.UUID, branch string) (*models.RuntimeBranch, error) {
	for _, b := range f.branches[datasetID] {
		if b.Name == branch {
			return &models.RuntimeBranch{ID: b.ID, RID: b.RID, DatasetID: b.DatasetID, DatasetRID: b.DatasetRID, Name: b.Name, ParentBranchID: b.ParentBranchID, HeadTransactionID: b.HeadTransactionID, CreatedFromTransactionID: b.CreatedFromTransactionID, LastActivityAt: b.LastActivityAt, Labels: b.Labels, FallbackChain: b.FallbackChain, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}, nil
		}
	}
	return nil, repo.ErrNotFound
}
func (f *fakeStore) PreviewDeleteBranch(_ context.Context, datasetID uuid.UUID, branch string) (*models.BranchDeletePreview, error) {
	b, err := f.GetRuntimeBranch(context.Background(), datasetID, branch)
	if err != nil {
		return nil, err
	}
	return &models.BranchDeletePreview{Branch: b.Name, BranchRID: b.RID, TransactionsPreserved: true, ChildrenToReparent: []models.BranchDeleteChildReparent{}}, nil
}
func (f *fakeStore) DeleteRuntimeBranch(_ context.Context, datasetID uuid.UUID, branch string) (*models.BranchDeleteResponse, error) {
	b, err := f.GetRuntimeBranch(context.Background(), datasetID, branch)
	if err != nil {
		return nil, err
	}
	return &models.BranchDeleteResponse{Branch: b.Name, BranchRID: b.RID, Reparented: []models.BranchDeleteChildReparent{}}, nil
}
func (f *fakeStore) ReparentRuntimeBranch(_ context.Context, datasetID uuid.UUID, branch string, newParent *string) (*models.RuntimeBranch, error) {
	b, err := f.GetRuntimeBranch(context.Background(), datasetID, branch)
	if err != nil {
		return nil, err
	}
	if newParent != nil {
		p, err := f.GetRuntimeBranch(context.Background(), datasetID, *newParent)
		if err != nil {
			return nil, err
		}
		b.ParentBranchID = &p.ID
	}
	return b, nil
}
func (f *fakeStore) BranchAncestry(_ context.Context, datasetID uuid.UUID, branch string) ([]models.RuntimeBranch, error) {
	b, err := f.GetRuntimeBranch(context.Background(), datasetID, branch)
	if err != nil {
		return nil, err
	}
	return []models.RuntimeBranch{*b}, nil
}
func (f *fakeStore) UpdateBranchRetention(_ context.Context, datasetID uuid.UUID, branch string, _ models.RetentionPolicy, _ *int32) (*models.RuntimeBranch, error) {
	return f.GetRuntimeBranch(context.Background(), datasetID, branch)
}
func (f *fakeStore) RestoreBranch(_ context.Context, datasetID uuid.UUID, branch string) (*models.RuntimeBranch, error) {
	return f.GetRuntimeBranch(context.Background(), datasetID, branch)
}
func (f *fakeStore) ListBranchMarkings(_ context.Context, branchID uuid.UUID) ([]models.BranchMarking, error) {
	return []models.BranchMarking{{BranchID: branchID, MarkingID: uuid.New(), Source: "EXPLICIT"}}, nil
}
func (f *fakeStore) CompareBranches(_ context.Context, _ uuid.UUID, base string, compare string) (*models.BranchCompareResponse, error) {
	return &models.BranchCompareResponse{BaseBranch: base, CompareBranch: compare, AOnlyTransactions: []models.TransactionSummary{}, BOnlyTransactions: []models.TransactionSummary{}, ConflictingFiles: []models.ConflictingFile{}}, nil
}
func (f *fakeStore) RollbackBranch(_ context.Context, _ uuid.UUID, branch string, _ *models.RollbackBody, _ uuid.UUID) (map[string]any, error) {
	return map[string]any{"view": map[string]any{"branch": branch}}, nil
}
func (f *fakeStore) ListFallbacks(_ context.Context, _ uuid.UUID) ([]models.RuntimeFallbackEntry, error) {
	return []models.RuntimeFallbackEntry{}, nil
}
func (f *fakeStore) ReplaceFallbacks(_ context.Context, _ uuid.UUID, _ []string) error { return nil }

func (f *fakeStore) ListViews(_ context.Context, datasetID uuid.UUID) ([]models.DatasetView, error) {
	return f.views[datasetID], nil
}
func (f *fakeStore) CreateView(_ context.Context, datasetID uuid.UUID, body *models.CreateDatasetViewRequest) (*models.DatasetView, error) {
	v := models.DatasetView{ID: uuid.New(), DatasetID: datasetID, Name: body.Name, Description: derefString(body.Description), SQLText: body.SQL, SourceBranch: body.SourceBranch, SourceVersion: body.SourceVersion, Format: "parquet", CurrentVersion: 1, SchemaFields: []byte(`[]`), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if body.Materialized != nil {
		v.Materialized = *body.Materialized
	}
	if body.RefreshOnSourceUpdate != nil {
		v.RefreshOnSourceUpdate = *body.RefreshOnSourceUpdate
	}
	f.views[datasetID] = append(f.views[datasetID], v)
	return &v, nil
}
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (f *fakeStore) GetDatasetView(_ context.Context, datasetID uuid.UUID, viewOrName string) (*models.DatasetView, error) {
	for i := range f.views[datasetID] {
		if f.views[datasetID][i].Name == viewOrName || f.views[datasetID][i].ID.String() == viewOrName {
			return &f.views[datasetID][i], nil
		}
	}
	return nil, repo.ErrNotFound
}
func (f *fakeStore) RefreshDatasetView(ctx context.Context, datasetID uuid.UUID, viewOrName string) (*models.DatasetView, error) {
	v, err := f.GetDatasetView(ctx, datasetID, viewOrName)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	v.LastRefreshedAt = &now
	return v, nil
}
func (f *fakeStore) GetCurrentView(ctx context.Context, datasetID uuid.UUID, branch string) (*models.ViewOut, error) {
	if len(f.views[datasetID]) == 0 {
		_, _ = f.CreateView(ctx, datasetID, &models.CreateDatasetViewRequest{Name: "current", SQL: "select *"})
	}
	v := f.views[datasetID][0]
	return &models.ViewOut{ID: v.ID, DatasetID: datasetID, BranchID: uuid.New(), HeadTransactionID: uuid.New(), RequestedBranch: branch, ResolvedBranch: branch, FallbackChain: []string{}, ComputedAt: time.Now().UTC(), Files: []models.RuntimeViewFile{}}, nil
}
func (f *fakeStore) GetViewAt(ctx context.Context, datasetID uuid.UUID, branch string, _ *time.Time, _ *uuid.UUID) (*models.ViewOut, error) {
	return f.GetCurrentView(ctx, datasetID, branch)
}
func (f *fakeStore) ListViewFiles(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]models.RuntimeViewFile, error) {
	return []models.RuntimeViewFile{{LogicalPath: "part.parquet", PhysicalPath: "s3://x/part.parquet", SizeBytes: 42}}, nil
}
func (f *fakeStore) GetViewSchema(_ context.Context, viewID uuid.UUID) (*models.SchemaResponse, error) {
	if s, ok := f.schemas[viewID]; ok {
		return &s, nil
	}
	return nil, nil
}
func (f *fakeStore) PutViewSchema(_ context.Context, viewID uuid.UUID, datasetID uuid.UUID, branch *string, schema models.DatasetSchema, contentHash string) (*models.SchemaResponse, error) {
	s := models.SchemaResponse{ViewID: viewID, DatasetID: datasetID, Branch: branch, Schema: schema, ContentHash: contentHash, CreatedAt: time.Now().UTC()}
	if old, ok := f.schemas[viewID]; ok && old.ContentHash == contentHash {
		s.Unchanged = true
		s.CreatedAt = old.CreatedAt
	}
	f.schemas[viewID] = s
	return &s, nil
}
func (f *fakeStore) GetCurrentSchema(ctx context.Context, datasetID uuid.UUID, branch string) (*models.SchemaResponse, error) {
	view, err := f.GetCurrentView(ctx, datasetID, branch)
	if err != nil {
		return nil, err
	}
	if s, ok := f.schemas[view.ID]; ok {
		return &s, nil
	}
	return nil, nil
}
func (f *fakeStore) PreviewData(_ context.Context, _ uuid.UUID, _ *uuid.UUID, q models.PreviewQuery) (*models.PreviewDataResponse, error) {
	limit := 100
	if q.Limit != nil {
		limit = *q.Limit
	}
	offset := 0
	if q.Offset != nil {
		offset = *q.Offset
	}
	return &models.PreviewDataResponse{Columns: []string{"id"}, Rows: [][]models.JSONValue{}, Format: "json", Limit: limit, Offset: offset}, nil
}
func (f *fakeStore) ValidateSchema(_ context.Context, _ uuid.UUID, schema models.DatasetSchema) (*models.ValidateResponse, error) {
	errs := []string{}
	for _, field := range schema.Fields {
		if strings.TrimSpace(field.Name) == "" {
			errs = append(errs, "field name is required")
		}
	}
	return &models.ValidateResponse{Conforms: len(errs) == 0, Files: []models.FileValidationReport{}, SchemaErrors: errs}, nil
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
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "http://files.local/v1/_internal/local-fs/datasets/sales/daily/part-000.parquet")
	assert.Equal(t, "private, max-age=0, must-revalidate", rec.Header().Get("Cache-Control"))
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

func catalogReq(method string, store *fakeStore, claims *authmw.Claims, body string) *http.Request {
	rid := "ri.foundry.main.dataset." + store.datasets[0].ID.String()
	req := httptest.NewRequest(method, "/v1/datasets/"+rid, strings.NewReader(body))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), claims))
	return withRouteParam(req, "rid", rid)
}

func TestDatasetModelCatalogHandlersReadAndPatch(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	store.markings[datasetID] = []models.EffectiveMarking{{ID: uuid.New(), Source: models.MarkingReason{Kind: "direct"}}}
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner}

	req := catalogReq(http.MethodGet, store, claims, "")
	rec := httptest.NewRecorder()
	h.GetDatasetModel(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var model models.DatasetRichModel
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &model))
	require.Equal(t, datasetID, model.ID)
	require.Len(t, model.Markings, 1)

	claims.Permissions = []string{"dataset.write"}
	req = catalogReq(http.MethodPatch, store, claims, `{"name":"sales_v2","format":"CSV"}`)
	rec = httptest.NewRecorder()
	h.PatchDatasetMetadata(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var patched models.CatalogDataset
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &patched))
	require.Equal(t, "sales_v2", patched.Name)
	require.Equal(t, "csv", patched.Format)
}

func TestDatasetCatalogHandlersRejectUnauthedForbiddenAndBadInput(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}

	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/x/markings", nil)
	req = withRouteParam(req, "rid", "ri.foundry.main.dataset."+store.datasets[0].ID.String())
	rec := httptest.NewRecorder()
	h.ListDatasetMarkings(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	claims := &authmw.Claims{Sub: owner}
	req = catalogReq(http.MethodPut, store, claims, `{"markings":[]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetMarkings(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	claims.Permissions = []string{"dataset.write"}
	req = catalogReq(http.MethodPut, store, claims, `{"files":[{"path":"","storage_path":"s3://x"}]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetFileIndex(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDatasetCatalogPutHandlersAreIdempotentAndReturnLists(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	admin := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.admin", "dataset.write"}}
	markingID := uuid.New()

	req := catalogReq(http.MethodPut, store, admin, `{"markings":["`+markingID.String()+`"]}`)
	rec := httptest.NewRecorder()
	h.PutDatasetMarkings(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var markings []models.EffectiveMarking
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &markings))
	require.Len(t, markings, 1)
	require.Equal(t, "direct", markings[0].Source.Kind)

	req = catalogReq(http.MethodPut, store, admin, `{"permissions":[{"principal_kind":"user","principal_id":"u1","role":"viewer","actions":["read"]}]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetPermissions(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var perms []models.DatasetPermissionEdge
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &perms))
	require.Len(t, perms, 1)

	req = catalogReq(http.MethodPut, store, admin, `{"links":[{"direction":"upstream","target_rid":"ri.parent"}]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetLineageLinks(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var links []models.DatasetLineageLink
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &links))
	require.Len(t, links, 1)

	size := int64(42)
	req = catalogReq(http.MethodPut, store, admin, `{"files":[{"path":"part-000.parquet","storage_path":"s3://x/part-000.parquet","size_bytes":`+strconv.FormatInt(size, 10)+`}]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetFileIndex(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var files []models.DatasetFileIndexEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &files))
	require.Len(t, files, 1)
}

func TestDatasetCatalogNotFoundAndConflict(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.admin"}}

	req := httptest.NewRequest(http.MethodGet, "/v1/datasets/ri.missing/permissions", nil)
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), claims))
	req = withRouteParam(req, "rid", "ri.missing")
	rec := httptest.NewRecorder()
	h.ListDatasetPermissions(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	store.permissionConflict = true
	req = catalogReq(http.MethodPut, store, claims, `{"permissions":[{"principal_kind":"user","principal_id":"u1","role":"viewer","actions":["read"]}]}`)
	rec = httptest.NewRecorder()
	h.PutDatasetPermissions(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestAdvancedBranchLifecycleHandlers(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	parent := models.DatasetBranch{ID: uuid.New(), RID: "ri.foundry.main.branch.parent", DatasetID: datasetID, DatasetRID: "ri.foundry.main.dataset." + datasetID.String(), Name: "master", Labels: []byte(`{}`), FallbackChain: []string{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}
	store.branches[datasetID] = []models.DatasetBranch{parent}
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.write"}}

	req := catalogReq(http.MethodPost, store, claims, `{"name":"feature","parent_branch":"master"}`)
	rec := httptest.NewRecorder()
	h.CreateBranch(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var created models.RuntimeBranch
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "feature", created.Name)

	req = catalogReq(http.MethodGet, store, claims, "")
	rec = httptest.NewRecorder()
	h.ListBranches(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var page models.Page[models.RuntimeBranch]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Data, 2)

	req = withRouteParam(catalogReq(http.MethodGet, store, claims, ""), "branch", "feature")
	rec = httptest.NewRecorder()
	h.BranchAncestry(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = withRouteParam(catalogReq(http.MethodGet, store, claims, ""), "branch", "feature")
	rec = httptest.NewRecorder()
	h.PreviewDeleteBranch(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = withRouteParam(catalogReq(http.MethodDelete, store, claims, ""), "branch", "feature")
	rec = httptest.NewRecorder()
	h.DeleteBranch(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAdvancedBranchRetentionCompareFallbacksAndValidation(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	store.branches[datasetID] = []models.DatasetBranch{{ID: uuid.New(), RID: "ri.branch.master", DatasetID: datasetID, DatasetRID: "ri.dataset", Name: "master", Labels: []byte(`{}`), FallbackChain: []string{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}, {ID: uuid.New(), RID: "ri.branch.feature", DatasetID: datasetID, DatasetRID: "ri.dataset", Name: "feature", Labels: []byte(`{}`), FallbackChain: []string{}, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), LastActivityAt: time.Now().UTC()}}
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.write"}}

	req := withRouteParam(catalogReq(http.MethodPatch, store, claims, `{"policy":"TTL_DAYS","ttl_days":7}`), "branch", "feature")
	rec := httptest.NewRecorder()
	h.UpdateRetention(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = withRouteParam(catalogReq(http.MethodPatch, store, claims, `{"policy":"TTL_DAYS","ttl_days":0}`), "branch", "feature")
	rec = httptest.NewRecorder()
	h.UpdateRetention(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	req.URL.RawQuery = "base=master&compare=feature"
	rec = httptest.NewRecorder()
	h.CompareBranches(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = withRouteParam(catalogReq(http.MethodPut, store, claims, `{"fallbacks":["master"]}`), "branch", "feature")
	rec = httptest.NewRecorder()
	h.PutFallbacks(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = withRouteParam(catalogReq(http.MethodGet, store, claims, ""), "branch", "feature")
	rec = httptest.NewRecorder()
	h.GetBranchMarkings(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestViewsSchemaPreviewHandlers(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.write"}}

	req := catalogReq(http.MethodPost, store, claims, `{"name":"latest_orders","sql":"select * from orders","materialized":true}`)
	rec := httptest.NewRecorder()
	h.CreateView(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var view models.DatasetView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))

	req = catalogReq(http.MethodGet, store, claims, "")
	rec = httptest.NewRecorder()
	h.ListViews(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	req = withRouteParam(req, "view_or_action", "latest_orders")
	rec = httptest.NewRecorder()
	h.GetView(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodPost, store, claims, "{}")
	req = withRouteParam(req, "view_or_action", "latest_orders:refresh")
	rec = httptest.NewRecorder()
	h.ViewAction(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	schemaJSON := `{"schema":{"file_format":"parquet","fields":[{"name":"id","field_type":"STRING","type":"STRING","nullable":false}]}}`
	req = catalogReq(http.MethodPost, store, claims, schemaJSON)
	req = withRouteParam(req, "view_id", view.ID.String())
	rec = httptest.NewRecorder()
	h.PutViewSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var schema models.SchemaResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &schema))
	require.Equal(t, view.ID, schema.ViewID)

	req = catalogReq(http.MethodGet, store, claims, "")
	req = withRouteParam(req, "view_id", view.ID.String())
	rec = httptest.NewRecorder()
	h.GetViewSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	rec = httptest.NewRecorder()
	h.GetCurrentView(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	req.URL.RawQuery = "ts=2026-05-07T00:00:00Z"
	rec = httptest.NewRecorder()
	h.GetViewAt(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	req = withRouteParam(req, "view_id", view.ID.String())
	rec = httptest.NewRecorder()
	h.ListViewFiles(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestPreviewCurrentSchemaAndValidateHandlers(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	h := &handlers.Handlers{Repo: store}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.write"}}
	view, err := store.CreateView(context.Background(), store.datasets[0].ID, &models.CreateDatasetViewRequest{Name: "v", SQL: "select *"})
	require.NoError(t, err)
	_, err = store.PutViewSchema(context.Background(), view.ID, store.datasets[0].ID, nil, models.DatasetSchema{FileFormat: "parquet", Fields: []models.Field{{Name: "id", Type: "STRING"}}}, "hash")
	require.NoError(t, err)

	req := catalogReq(http.MethodGet, store, claims, "")
	rec := httptest.NewRecorder()
	h.GetCurrentSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodGet, store, claims, "")
	req.URL.RawQuery = "limit=10&offset=2&format=csv"
	rec = httptest.NewRecorder()
	h.PreviewDataset(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var preview models.PreviewDataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.Equal(t, 10, preview.Limit)
	require.Equal(t, 2, preview.Offset)

	req = catalogReq(http.MethodGet, store, claims, "")
	req = withRouteParam(req, "view_id", view.ID.String())
	rec = httptest.NewRecorder()
	h.PreviewMaterializedView(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	req = catalogReq(http.MethodPost, store, claims, `{"schema":{"file_format":"parquet","fields":[{"name":"","field_type":"STRING","type":"STRING","nullable":false}]}}`)
	rec = httptest.NewRecorder()
	h.ValidateSchema(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var validation models.ValidateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &validation))
	require.False(t, validation.Conforms)
}

func TestLocalPresignProxyRoundTripAndSafety(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	root := t.TempDir()
	fixed := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	fs := storageabstraction.NewLocalBackingFS("http://files.local", "datasets", []byte("secret"))
	fs.RootDir = root
	fs.Now = func() time.Time { return fixed }
	require.NoError(t, fs.WriteLocalObject("datasets/sales/part-000.parquet", []byte("hello")))
	signed, err := fs.PresignedURL(storageabstraction.PhysicalLocation{FSID: "local", BaseDirectory: "datasets", RelativePath: "sales/part-000.parquet"}, time.Minute)
	require.NoError(t, err)
	h := &handlers.Handlers{Repo: store, BackingFS: fs}

	req := httptest.NewRequest(http.MethodGet, signed.URL, nil)
	req = withRouteParam(req, "*", "datasets/sales/part-000.parquet")
	rec := httptest.NewRecorder()
	h.LocalPresignProxy(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "hello", rec.Body.String())

	req = httptest.NewRequest(http.MethodGet, strings.Replace(signed.URL, "sig=", "sig=tampered", 1), nil)
	req = withRouteParam(req, "*", "datasets/sales/part-000.parquet")
	rec = httptest.NewRecorder()
	h.LocalPresignProxy(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	expired := fixed.Add(-time.Minute)
	expiredURL := "http://files.local/v1/_internal/local-fs/datasets/sales/part-000.parquet?expires=" + strconv.FormatInt(expired.Unix(), 10) + "&sig=" + fs.SignLocalKey("datasets/sales/part-000.parquet", expired.Unix())
	req = httptest.NewRequest(http.MethodGet, expiredURL, nil)
	req = withRouteParam(req, "*", "datasets/sales/part-000.parquet")
	rec = httptest.NewRecorder()
	h.LocalPresignProxy(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest(http.MethodGet, signed.URL, nil)
	req = withRouteParam(req, "*", "datasets/../secret.txt")
	rec = httptest.NewRecorder()
	h.LocalPresignProxy(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStorageDetailsAndMultipartUpload(t *testing.T) {
	owner := uuid.New()
	store := newFakeStore(owner)
	datasetID := store.datasets[0].ID
	deletedAt := time.Now().UTC()
	store.files[datasetID] = []models.DatasetFile{{ID: uuid.New(), DatasetID: datasetID, TransactionID: uuid.New(), LogicalPath: "active.csv", PhysicalURI: "local:///active.csv", SizeBytes: 5, Status: "active"}, {ID: uuid.New(), DatasetID: datasetID, TransactionID: uuid.New(), LogicalPath: "deleted.csv", PhysicalURI: "local:///deleted.csv", SizeBytes: 7, DeletedAt: &deletedAt, Status: "deleted"}}
	fs := storageabstraction.NewLocalBackingFS("http://files.local", "base", []byte("secret"))
	fs.RootDir = t.TempDir()
	h := &handlers.Handlers{Repo: store, BackingFS: fs, PresignTTL: 2 * time.Minute}
	claims := &authmw.Claims{Sub: owner, Permissions: []string{"dataset.write"}}

	req := catalogReq(http.MethodGet, store, claims, "")
	rec := httptest.NewRecorder()
	h.StorageDetails(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var details models.StorageDetailsOut
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &details))
	require.Equal(t, int64(5), details.TotalActiveBytes)
	require.Equal(t, int64(1), details.TotalDeletedFiles)
	require.Equal(t, uint64(120), details.PresignTTLSeconds)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("logical_path", "incoming/data.csv"))
	part, err := writer.CreateFormFile("file", "ignored.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte("a,b\n1,2\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req = catalogReq(http.MethodPost, store, claims, "")
	req.Body = io.NopCloser(&body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	h.UploadData(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	items := store.fileIndex[datasetID]
	require.Len(t, items, 1)
	require.Equal(t, "incoming/data.csv", items[0].Path)
	require.Equal(t, "local:///base/datasets/"+datasetID.String()+"/incoming/data.csv", items[0].StoragePath)
	got, err := fs.ReadLocalObject("base/datasets/" + datasetID.String() + "/incoming/data.csv")
	require.NoError(t, err)
	require.Equal(t, "a,b\n1,2\n", string(got))
}

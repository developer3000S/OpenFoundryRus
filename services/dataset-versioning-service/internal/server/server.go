// Package server wires the chi router for dataset-versioning-service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/capabilities"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/handlers"
)

func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, probes ...capabilities.DependencyProbe) *http.Server {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Method(http.MethodGet, "/metrics", m.Handler())

	// Capability registry — see docs/agent-automation/AGENT-CAPABILITIES-ROADMAP.md (M1.1).
	// M1.2: optional dependency probes wire into `/_meta/health`.
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/datasets", h.ListDatasets)
		api.Post("/datasets", h.CreateDataset)
		api.Get("/datasets/{id}", h.GetDataset)
		api.Patch("/datasets/{id}", h.UpdateDataset)
		api.Delete("/datasets/{id}", h.DeleteDataset)
		api.Post("/datasets/{id}:restore", h.RestoreDataset)
		api.Post("/datasets/{id}/restore", h.RestoreDataset)

		api.Get("/datasets/{id}/versions", h.ListVersions)
		api.Post("/datasets/{id}/versions", h.CreateVersion)
		api.Get("/datasets/{id}/versions/{version}", h.GetVersion)

		api.Get("/datasets/{id}/branches", h.ListBranches)
		api.Post("/datasets/{id}/branches", h.CreateBranch)
		api.Get("/datasets/{id}/branches/{branch}", h.GetBranch)
		api.Delete("/datasets/{id}/branches/{branch}", h.DeleteBranch)
		api.Get("/datasets/{id}/branches/{branch}/transactions", h.ListBranchTransactions)
		api.Post("/datasets/{id}/branches/{branch}/transactions", h.StartTransaction)
		api.Get("/datasets/{id}/branches/{branch}/transactions/{txn}", h.GetTransaction)
		api.Post("/datasets/{id}/branches/{branch}/transactions/{txn}", h.TransactionAction)
		api.Post("/datasets/{id}/branches/{branch}/transactions/{txn}:commit", h.CommitTransaction)
		api.Post("/datasets/{id}/branches/{branch}/transactions/{txn}:abort", h.AbortTransaction)
		api.Get("/datasets/{id}/transactions", h.ListTransactions)
		api.Post("/datasets/{id}/transactions:batchGet", h.BatchGetTransactions)
		api.Get("/datasets/{id}/incremental-readiness", h.GetIncrementalReadiness)

		api.Get("/datasets/{id}/files", h.ListFiles)
		api.Get("/datasets/{id}/files/metadata", h.GetFileMetadataByPath)
		api.Get("/datasets/{id}/files/content", h.DownloadFileContentByPath)
		api.Get("/datasets/{id}/files/{file_id}", h.GetFileMetadata)
		api.Get("/datasets/{id}/files/{file_id}/content", h.DownloadFile)
		api.Get("/datasets/{id}/files/{file_id}/download", h.DownloadFile)
		api.Post("/datasets/{id}/transactions/{txn}/files", h.CreateFileUploadURL)
		api.Post("/datasets/{id}/transactions/{txn}/files/content", h.UploadTransactionFileContent)
		api.Delete("/datasets/{id}/transactions/{txn}/files", h.DeleteTransactionFile)
		api.Post("/datasets/{id}/outputs:commit", h.CommitDatasetOutput)

		api.Get("/datasets/{id}/quality", h.GetDatasetQuality)
		api.Post("/datasets/{id}/quality/profile", h.RefreshDatasetQuality)
		api.Post("/datasets/{id}/quality/rules", h.CreateQualityRule)
		api.Patch("/datasets/{id}/quality/rules/{rule_id}", h.UpdateQualityRule)
		api.Delete("/datasets/{id}/quality/rules/{rule_id}", h.DeleteQualityRule)
		api.Get("/datasets/{id}/lint", h.GetDatasetLint)
		api.Get("/datasets/{rid}/health", h.GetDatasetHealth)
	})

	r.Route("/internal", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		api.Get("/datasets/{rid}/metadata", h.GetDatasetMetadata)
	})

	r.Route("/api/v2", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		api.Use(handlers.DatasetAPIScopeMiddleware)
		api.Get("/datasets", h.ListDatasets)
		api.Post("/datasets", h.CreateDataset)
		api.Post("/datasets/getSchemaBatch", h.GetFoundryDatasetSchemaBatch)
		api.Get("/datasets/{rid}", h.GetDataset)
		api.Patch("/datasets/{rid}", h.UpdateDataset)
		api.Delete("/datasets/{rid}", h.DeleteDataset)
		api.Post("/datasets/{rid}:restore", h.RestoreDataset)
		api.Post("/datasets/{rid}/restore", h.RestoreDataset)
		api.Get("/datasets/{rid}/readTable", h.ReadTableDataset)
		api.Get("/datasets/{rid}/preview", h.PreviewDataset)
		api.Get("/datasets/{rid}/schema", h.GetCurrentSchema)
		api.Get("/datasets/{rid}/getSchema", h.GetFoundryDatasetSchema)
		api.Put("/datasets/{rid}/schema", h.PutFoundryDatasetSchema)
		api.Put("/datasets/{rid}/putSchema", h.PutFoundryDatasetSchema)
		api.Post("/datasets/{rid}/schema:infer", h.InferDatasetSchema)
		api.Post("/datasets/{rid}/schema:validate", h.ValidateSchema)
		api.Get("/datasets/{rid}/schema/history", h.ListDatasetSchemaHistory)
		api.Get("/datasets/{rid}/branches", h.ListFoundryBranches)
		api.Post("/datasets/{rid}/branches", h.CreateFoundryBranch)
		api.Get("/datasets/{rid}/branches/{branch}", h.GetFoundryBranch)
		api.Delete("/datasets/{rid}/branches/{branch}", h.DeleteFoundryBranch)
		api.Get("/datasets/{rid}/branches/{branch}/transactions", h.ListFoundryBranchTransactions)
		api.Post("/datasets/{rid}/branches/{branch}/transactions", h.StartTransaction)
		api.Get("/datasets/{rid}/branches/{branch}/transactions/{txn}", h.GetTransaction)
		api.Post("/datasets/{rid}/branches/{branch}/transactions/{txn}:commit", h.CommitTransaction)
		api.Post("/datasets/{rid}/branches/{branch}/transactions/{txn}:abort", h.AbortTransaction)
		api.Get("/datasets/{rid}/transactions", h.ListTransactions)
		api.Post("/datasets/{rid}/transactions:batchGet", h.BatchGetTransactions)
		api.Get("/datasets/{rid}/incremental-readiness", h.GetIncrementalReadiness)
		api.Get("/datasets/{rid}/files", h.ListFiles)
		api.Get("/datasets/{rid}/files/metadata", h.GetFileMetadataByPath)
		api.Get("/datasets/{rid}/files/content", h.DownloadFileContentByPath)
		api.Get("/datasets/{rid}/files/{file_id}", h.GetFileMetadata)
		api.Get("/datasets/{rid}/files/{file_id}/content", h.DownloadFile)
		api.Get("/datasets/{rid}/files/{file_id}/download", h.DownloadFile)
		api.Post("/datasets/{rid}/transactions/{txn}/files", h.CreateFileUploadURL)
		api.Post("/datasets/{rid}/transactions/{txn}/files/content", h.UploadTransactionFileContent)
		api.Delete("/datasets/{rid}/transactions/{txn}/files", h.DeleteTransactionFile)
		api.Get("/datasets/{rid}/compare", h.CompareViews)
		api.Get("/datasets/{rid}/views", h.ListViews)
		api.Post("/datasets/{rid}/views", h.CreateView)
		api.Get("/datasets/{rid}/views/current", h.GetCurrentView)
		api.Get("/datasets/{rid}/views/at", h.GetViewAt)
		api.Get("/datasets/{rid}/views/{view_id}/files", h.ListViewFiles)
		api.Get("/datasets/{rid}/views/{view_id}/backing-datasets", h.ListViewBackingDatasets)
		api.Put("/datasets/{rid}/views/{view_id}/backing-datasets", h.ReplaceViewBackingDatasets)
		api.Post("/datasets/{rid}/views/{view_id}/backing-datasets:add", h.AddViewBackingDatasets)
		api.Post("/datasets/{rid}/views/{view_id}/backing-datasets:remove", h.RemoveViewBackingDatasets)
		api.Put("/datasets/{rid}/views/{view_id}/primary-key", h.PutViewPrimaryKey)
		api.Delete("/datasets/{rid}/views/{view_id}/primary-key", h.DeleteViewPrimaryKey)
		api.Get("/datasets/{rid}/views/{view_id}/schema", h.GetViewSchema)
		api.Post("/datasets/{rid}/views/{view_id}/schema", h.PutViewSchema)
		api.Get("/datasets/{rid}/views/{view_id}/data", h.PreviewViewData)
		api.Get("/datasets/{rid}/views/{view_id}/preview", h.PreviewMaterializedView)
		api.Get("/datasets/{rid}/views/{view_or_action}", h.GetView)
		api.Post("/datasets/{rid}/views/{view_or_action}", h.ViewAction)
	})

	r.Route("/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			protected := authmw.Middleware(jwt)(next)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/_internal/local-fs/") {
					next.ServeHTTP(w, r)
					return
				}
				protected.ServeHTTP(w, r)
			})
		})

		// Public Rust parity endpoint: the query-string HMAC is the authenticator.
		api.Get("/_internal/local-fs/*", h.LocalPresignProxy)
		api.Get("/_internal/local-fs/{key:.+}", h.LocalPresignProxy)

		api.Get("/catalog/facets", h.GetCatalogFacets)

		api.Get("/datasets", h.ListDatasets)
		api.Post("/datasets", h.CreateDataset)
		api.Get("/datasets/{rid}", h.GetDataset)
		api.Patch("/datasets/{rid}", h.UpdateDataset)
		api.Delete("/datasets/{rid}", h.DeleteDataset)
		api.Post("/datasets/{rid}:restore", h.RestoreDataset)
		api.Post("/datasets/{rid}/restore", h.RestoreDataset)

		api.Get("/datasets/{rid}/model", h.GetDatasetModel)
		api.Patch("/datasets/{rid}/metadata", h.PatchDatasetMetadata)
		api.Get("/datasets/{rid}/markings", h.ListDatasetMarkings)
		api.Put("/datasets/{rid}/markings", h.PutDatasetMarkings)
		api.Get("/datasets/{rid}/permissions", h.ListDatasetPermissions)
		api.Put("/datasets/{rid}/permissions", h.PutDatasetPermissions)
		api.Get("/datasets/{rid}/lineage-links", h.ListDatasetLineageLinks)
		api.Put("/datasets/{rid}/lineage-links", h.PutDatasetLineageLinks)
		api.Get("/datasets/{rid}/files/index", h.ListDatasetFileIndex)
		api.Put("/datasets/{rid}/files/index", h.PutDatasetFileIndex)

		api.Get("/datasets/{rid}/versions", h.ListVersions)

		api.Get("/datasets/{rid}/branches", h.ListBranches)
		api.Post("/datasets/{rid}/branches", h.CreateBranch)
		api.Get("/datasets/{rid}/branches/compare", h.CompareBranches)
		api.Get("/datasets/{rid}/branches/{branch}", h.GetBranch)
		api.Delete("/datasets/{rid}/branches/{branch}", h.DeleteBranch)
		api.Post("/datasets/{rid}/branches/{branch}", h.BranchAction)
		api.Post("/datasets/{rid}/branches/{branch}/checkout", h.CheckoutBranch)
		api.Get("/datasets/{rid}/branches/{branch}/ancestry", h.BranchAncestry)
		api.Get("/datasets/{rid}/branches/{branch}/preview-delete", h.PreviewDeleteBranch)
		api.Patch("/datasets/{rid}/branches/{branch}/retention", h.UpdateRetention)
		api.Get("/datasets/{rid}/branches/{branch}/markings", h.GetBranchMarkings)
		api.Post("/datasets/{rid}/branches/{branch}:restore", h.RestoreBranch)
		api.Post("/datasets/{rid}/branches/{branch}/rollback", h.RollbackBranch)
		api.Get("/datasets/{rid}/branches/{branch}/fallbacks", h.ListFallbacks)
		api.Put("/datasets/{rid}/branches/{branch}/fallbacks", h.PutFallbacks)

		api.Get("/datasets/{rid}/branches/{branch}/transactions", h.ListBranchTransactions)
		api.Post("/datasets/{rid}/branches/{branch}/transactions", h.StartTransaction)
		api.Get("/datasets/{rid}/branches/{branch}/transactions/{txn}", h.GetTransaction)
		api.Post("/datasets/{rid}/branches/{branch}/transactions/{txn}", h.TransactionAction)
		api.Post("/datasets/{rid}/branches/{branch}/transactions/{txn}:commit", h.CommitTransaction)
		api.Post("/datasets/{rid}/branches/{branch}/transactions/{txn}:abort", h.AbortTransaction)
		api.Get("/datasets/{rid}/transactions", h.ListTransactions)
		api.Post("/datasets/{rid}/transactions:batchGet", h.BatchGetTransactions)
		api.Get("/datasets/{rid}/incremental-readiness", h.GetIncrementalReadiness)

		api.Get("/datasets/{rid}/compare", h.CompareViews)

		api.Get("/datasets/{rid}/views", h.ListViews)
		api.Post("/datasets/{rid}/views", h.CreateView)
		api.Get("/datasets/{rid}/views/current", h.GetCurrentView)
		api.Get("/datasets/{rid}/views/at", h.GetViewAt)
		api.Get("/datasets/{rid}/views/{view_id}/files", h.ListViewFiles)
		api.Get("/datasets/{rid}/views/{view_id}/backing-datasets", h.ListViewBackingDatasets)
		api.Put("/datasets/{rid}/views/{view_id}/backing-datasets", h.ReplaceViewBackingDatasets)
		api.Post("/datasets/{rid}/views/{view_id}/backing-datasets:add", h.AddViewBackingDatasets)
		api.Post("/datasets/{rid}/views/{view_id}/backing-datasets:remove", h.RemoveViewBackingDatasets)
		api.Put("/datasets/{rid}/views/{view_id}/primary-key", h.PutViewPrimaryKey)
		api.Delete("/datasets/{rid}/views/{view_id}/primary-key", h.DeleteViewPrimaryKey)
		api.Get("/datasets/{rid}/views/{view_id}/schema", h.GetViewSchema)
		api.Post("/datasets/{rid}/views/{view_id}/schema", h.PutViewSchema)
		api.Get("/datasets/{rid}/views/{view_id}/data", h.PreviewViewData)
		api.Get("/datasets/{rid}/views/{view_id}/preview", h.PreviewMaterializedView)
		api.Get("/datasets/{rid}/views/{view_or_action}", h.GetView)
		api.Post("/datasets/{rid}/views/{view_or_action}", h.ViewAction)

		api.Get("/datasets/{rid}/files", h.ListFiles)
		api.Get("/datasets/{rid}/files/metadata", h.GetFileMetadataByPath)
		api.Get("/datasets/{rid}/files/content", h.DownloadFileContentByPath)
		api.Get("/datasets/{rid}/files/{file_id}", h.GetFileMetadata)
		api.Get("/datasets/{rid}/files/{file_id}/content", h.DownloadFile)
		api.Get("/datasets/{rid}/files/{file_id}/download", h.DownloadFile)
		api.Post("/datasets/{rid}/transactions/{txn_id}/files", h.CreateFileUploadURL)
		api.Post("/datasets/{rid}/transactions/{txn_id}/files/content", h.UploadTransactionFileContent)
		api.Delete("/datasets/{rid}/transactions/{txn_id}/files", h.DeleteTransactionFile)
		api.Post("/datasets/{rid}/outputs:commit", h.CommitDatasetOutput)
		api.Get("/datasets/{rid}/storage-details", h.StorageDetails)
		api.Post("/datasets/{rid}/upload", h.UploadData)

		api.Get("/datasets/{rid}/preview", h.PreviewDataset)
		api.Get("/datasets/{rid}/readTable", h.ReadTableDataset)
		api.Get("/datasets/{rid}/schema", h.GetCurrentSchema)
		api.Put("/datasets/{rid}/schema", h.PutFoundryDatasetSchema)
		api.Post("/datasets/{rid}/schema:infer", h.InferDatasetSchema)
		api.Get("/datasets/{rid}/schema/history", h.ListDatasetSchemaHistory)
		api.Post("/datasets/{rid}/schema:validate", h.ValidateSchema)
		api.Get("/datasets/{rid}/health", h.GetDatasetHealth)

	})

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix:  "dataset-versioning",
		AuthPaths: []string{"/api/v1", "/api/v2", "/v1"},
		Tags:      []string{"datasets"},
	}); err != nil {
		panic("dataset-versioning-service: capability ingest failed: " + err.Error())
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func Run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		log.Info("shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Package server wires the HTTP surface for pipeline-build-service.
//
// URL grid mirrors the Rust router 1:1:
//
//   - /api/v1/builds                          (builds_v1)
//   - /api/v1/builds/{id}                     (builds_v1)
//   - /api/v1/builds/{id}/abort               (builds_v1)
//   - /api/v1/builds/{id}/jobs                (job_logs)
//   - /api/v1/jobs/{id}                       (job_logs)
//   - /api/v1/jobs/{id}/logs                  (job_logs)
//   - /api/v1/jobs/{id}/logs/stream           (job_logs SSE)
//   - /api/v1/pipelines                       (legacy CRUD)
//   - /api/v1/pipelines/_validate             (draft graph validation)
//   - /api/v1/pipelines/_schema-guidance      (draft join/union schema guidance)
//   - /api/v1/pipelines/aip/generate          (AIP transform generation)
//   - /api/v1/pipelines/transforms/catalog    (transform catalog)
//   - /api/v1/pipelines/{id}                  (legacy CRUD)
//   - /api/v1/pipelines/{id}/_validate        (persisted graph validation)
//   - /api/v1/pipelines/{id}/schema-guidance  (persisted join/union schema guidance)
//   - /api/v1/pipelines/{id}/nodes/{node_id}/preview (local preview)
//   - /api/v1/pipelines/geospatial/gpx/parse (GPX upload parse helper)
//   - /api/v1/pipelines/{id}/runs             (runs)
//   - /api/v1/pipelines/{id}/runs/{run_id}    (runs)
//   - /api/v1/pipelines/{id}/runs/{run_id}/retry  (runs)
//   - /api/v1/pipelines/{id}/runs/{run_id}/cancel (runs)
//   - /api/v1/dry-run/resolve                 (dry_run)
//   - /api/v1/dry-run/validate                (dry_run)
//   - /api/v1/execute                         (execute)
//   - /api/v1/data-integration/pipelines/{id}/runs                 (runs)
//   - /api/v1/data-integration/pipelines/{id}/runs/{run_id}        (runs)
//   - /api/v1/data-integration/pipelines/{id}/runs/{run_id}/retry  (runs)
//   - /api/v1/data-integration/builds                              (builds queue)
//   - /api/v1/data-integration/builds/_summary                     (builds queue)
//   - /api/v1/data-integration/builds/{run_id}/abort               (builds queue)
//   - /api/v1/data-integration/pipelines/_scheduler/run-due        (scheduler)
//   - /api/v1/data-integration/pipelines/{pipeline_rid}/dry-run-resolve (dry_run)
//   - /api/v1/data-integration/spark-runs                          (spark_runs)
//   - /api/v1/data-integration/spark-runs/{id}                     (spark_runs)
//   - /api/v1/data-integration/pipelines/{id}/runs/{run_id}/spec   (spark_runs)
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/capabilities"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/handler"
)

func New(cfg *config.Config, m *observability.Metrics, probes ...capabilities.DependencyProbe) *http.Server {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           BuildRouter(cfg, m, probes...),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func BuildRouter(cfg *config.Config, m *observability.Metrics, probes ...capabilities.DependencyProbe) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	if m != nil {
		r.Method(http.MethodGet, "/metrics", m.Handler())
	}

	// Capability registry — see docs/agent-automation/AGENT-CAPABILITIES-ROADMAP.md (M1.1).
	caps := capabilities.New(cfg.Service.Name, cfg.Service.Version)
	for _, p := range probes {
		caps.RegisterDependency(p)
	}
	caps.Mount(r)

	jwt := authmw.NewJWTConfig(cfg.JWTSecret)
	// Rust-compatible SparkApplication submission surface from the Rust /api/v1/pipeline nest.
	// Rust mounted these routes outside the JWT-protected data-integration/v1 groups;
	// the gateway can still enforce auth upstream.
	r.Route("/api/v1/pipeline", func(pipeline chi.Router) {
		pipeline.Post("/builds/run", handler.SubmitPipelineBuildRun)
		pipeline.Get("/builds/{run_id}/status", handler.GetPipelineBuildRunStatus)
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		// Builds (v1).
		api.Get("/builds", handler.ListBuilds)
		api.Post("/builds", handler.CreateBuild)
		api.Get("/builds/{id}", handler.GetBuild)
		api.Post("/builds/{id}/abort", handler.AbortBuild)
		api.Get("/builds/{id}/jobs", handler.ListJobs)

		// Jobs + logs.
		api.Get("/jobs/{id}", handler.GetJob)
		api.Get("/jobs/{id}/logs", handler.ListJobLogs)
		api.Get("/jobs/{id}/logs/stream", handler.StreamJobLogs)

		// Dry-run + execute.
		api.Post("/dry-run/resolve", handler.DryRunResolve)
		api.Post("/dry-run/validate", handler.DryRunValidate)
		api.Post("/execute", handler.ExecutePipeline)

		// Pipelines (legacy CRUD + runs).
		api.Get("/pipelines", handler.ListPipelines)
		api.Post("/pipelines", handler.CreatePipeline)
		api.Post("/pipelines/_validate", handler.ValidatePipelineGraph)
		api.Post("/pipelines/_schema-guidance", handler.PipelineSchemaGuidance)
		api.Post("/pipelines/aip/generate", handler.PipelineAIPGenerate)
		api.Get("/pipelines/transforms/catalog", handler.ListPipelineTransformCatalog)
		api.Post("/pipelines/geospatial/gpx/parse", handler.ParseGPXUpload)
		api.Get("/pipelines/{id}", handler.GetPipeline)
		api.Patch("/pipelines/{id}", handler.UpdatePipeline)
		api.Put("/pipelines/{id}", handler.UpdatePipeline)
		api.Delete("/pipelines/{id}", handler.DeletePipeline)
		api.Get("/pipelines/{id}/versions", handler.ListPipelineVersions)
		api.Post("/pipelines/{id}/publish", handler.PublishPipeline)
		api.Post("/pipelines/{id}/proposals", handler.CreatePipelineProposal)
		api.Post("/pipelines/{id}/versions/{version_id}/restore", handler.RestorePipelineVersion)
		api.Post("/pipelines/{id}/_validate", handler.ValidatePipelineByID)
		api.Post("/pipelines/{id}/schema-guidance", handler.PipelineSchemaGuidanceByID)
		api.Post("/pipelines/{id}/aip/generate", handler.PipelineAIPGenerateByID)
		api.Get("/pipelines/{id}/nodes/{node_id}/preview", handler.PreviewPipelineNode)
		api.Post("/pipelines/{id}/nodes/{node_id}/preview", handler.PreviewPipelineNode)

		api.Get("/pipelines/{id}/runs", handler.ListPipelineRuns)
		api.Post("/pipelines/{id}/runs", handler.TriggerPipelineRun)
		api.Get("/pipelines/{id}/runs/{run_id}", handler.GetPipelineRun)
		api.Post("/pipelines/{id}/runs/{run_id}/retry", handler.RetryPipelineRun)
		api.Post("/pipelines/{id}/runs/{run_id}/cancel", handler.CancelPipelineRun)

		// Rust-compatible data-integration route group plus SparkApplication helpers.
		api.Route("/data-integration", func(di chi.Router) {
			di.Get("/pipelines/{id}/runs", handler.ListPipelineRuns)
			di.Post("/pipelines/{id}/runs", handler.TriggerPipelineRun)
			di.Get("/pipelines/{id}/runs/{run_id}", handler.GetPipelineRun)
			di.Post("/pipelines/{id}/runs/{run_id}/retry", handler.RetryPipelineRun)
			di.Get("/builds", handler.ListDataIntegrationBuilds)
			di.Get("/builds/_summary", handler.DataIntegrationQueueSummary)
			di.Post("/builds/{run_id}/abort", handler.AbortDataIntegrationBuild)
			di.Post("/pipelines/_scheduler/run-due", handler.RunDueScheduledPipelines)
			di.Post("/pipelines/{pipeline_rid}/dry-run-resolve", handler.DryRunResolve)

			di.Get("/spark-runs", handler.ListSparkRuns)
			di.Post("/spark-runs", handler.SubmitSparkRun)
			di.Get("/spark-runs/{id}", handler.GetSparkRun)
			di.Get("/pipelines/{id}/runs/{run_id}/spec", handler.GetSpecForRun)
		})
	})

	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(authmw.Middleware(jwt))
		v1.Get("/builds", handler.ListBuildsV1)
		v1.Post("/builds", handler.CreateBuildV1)
		v1.Get("/builds/{rid}", handler.GetBuildV1)
		v1.Post("/builds/{rid}:abort", handler.AbortBuildV1)
		v1.Get("/datasets/{rid}/builds", handler.ListDatasetBuildsV1)
		v1.Get("/jobs/{rid}/outputs", handler.GetJobOutputsV1)
		v1.Get("/jobs/{rid}/input-resolutions", handler.GetJobInputResolutionsV1)
		v1.Post("/job-specs/{kind}", handler.CreateJobSpecV1)
		v1.Get("/jobs/{rid}/logs", handler.ListJobLogsV1)
		v1.Post("/jobs/{rid}/logs", handler.EmitJobLogV1)
		v1.Get("/jobs/{rid}/logs/stream", handler.StreamJobLogsV1)
		v1.Get("/jobs/{rid}/logs/ws", handler.WSJobLogsV1)
	})

	if _, err := caps.IngestChiRoutes(r, capabilities.IngestOptions{
		IDPrefix:  "pipeline-build",
		AuthPaths: []string{"/api/v1/pipeline", "/api/v1", "/v1"},
		Tags:      []string{"pipelines"},
	}); err != nil {
		panic("pipeline-build-service: capability ingest failed: " + err.Error())
	}

	return r
}

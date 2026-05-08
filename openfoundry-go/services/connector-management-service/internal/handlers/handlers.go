// Package handlers wires the HTTP endpoints for connector-management-service.
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/domain"
	syncdomain "github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/domain/sync"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/repo"
	cmruntime "github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/runtime"
)

type Store interface {
	ListConnections(ctx context.Context, ownerID *uuid.UUID) ([]models.Connection, error)
	GetConnection(ctx context.Context, id uuid.UUID) (*models.Connection, error)
	GetConnectionForOwner(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Connection, error)
	CreateConnection(ctx context.Context, body *models.CreateConnectionRequest, ownerID uuid.UUID) (*models.Connection, error)
	UpdateConnection(ctx context.Context, id uuid.UUID, body *models.UpdateConnectionRequest) (*models.Connection, error)
	DeleteConnection(ctx context.Context, id uuid.UUID) (bool, error)
	ListSyncJobs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SyncJob, error)
	GetSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncJob, error)
	CreateSyncJob(ctx context.Context, body *models.CreateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error)
	UpdateSyncJob(ctx context.Context, id uuid.UUID, body *models.UpdateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error)
	RunSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncRun, error)
	ListSyncRuns(ctx context.Context, syncID uuid.UUID, ownerID uuid.UUID) ([]models.SyncRun, error)
	ListCredentials(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.CredentialResponse, error)
	SetCredential(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID, kind string, ciphertext []byte, fingerprint string) (*models.CredentialResponse, error)
	ListSourcePolicies(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SourcePolicyBindingResponse, error)
	AttachPolicy(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID, policyID uuid.UUID, kind string) (*models.SourcePolicyBindingResponse, error)
	DetachPolicy(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID, policyID uuid.UUID) (bool, error)
	ListMediaSetSyncs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.MediaSetSync, error)
	GetMediaSetSync(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.MediaSetSync, error)
	CreateMediaSetSync(ctx context.Context, sourceID uuid.UUID, body *models.CreateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error)
	UpdateMediaSetSync(ctx context.Context, id uuid.UUID, body *models.UpdateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error)
	EnableVirtualTableSource(ctx context.Context, sourceRID string, body *models.EnableVirtualTableSourceRequest) (*models.VirtualTableSourceLink, error)
	CreateVirtualTable(ctx context.Context, sourceRID string, actorID string, body *models.CreateVirtualTableRequest) (*models.VirtualTable, error)
	ListVirtualTables(ctx context.Context, ownerID string, project, source string, limit int) ([]models.VirtualTable, error)
	GetVirtualTable(ctx context.Context, rid string, ownerID string) (*models.VirtualTable, error)
	ListRegistrations(ctx context.Context, sourceID uuid.UUID) ([]models.ConnectionRegistration, error)
	UpsertRegistration(ctx context.Context, sourceID uuid.UUID, source models.DiscoveredSource, mode string, autoSync bool, updateDetection bool, targetDatasetID *uuid.UUID, metadata json.RawMessage) (*models.ConnectionRegistration, error)
	GetRegistration(ctx context.Context, sourceID uuid.UUID, registrationID uuid.UUID) (*models.ConnectionRegistration, error)
	DeleteRegistration(ctx context.Context, sourceID uuid.UUID, registrationID uuid.UUID) (bool, error)
	UpdateConnectionConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) (*models.Connection, error)
	ListIcebergNamespaces(ctx context.Context) ([]models.Connection, error)
	GetIcebergConnection(ctx context.Context, namespace string) (*models.Connection, error)
	ListIcebergTables(ctx context.Context, connectionID uuid.UUID) ([]models.ConnectionRegistration, error)
}

type syncRunCompleter interface {
	CompleteSyncRun(ctx context.Context, runID uuid.UUID, ownerID uuid.UUID, status string, bytesWritten int64, filesWritten int64, errMsg *string, ingestJobID *string, datasetVersionID *uuid.UUID, contentHash *string) (*models.SyncRun, error)
}

type previousDatasetVersionLookup interface {
	PreviousDatasetVersionForHash(ctx context.Context, syncDefID uuid.UUID, contentHash string) (*uuid.UUID, error)
}

type datasetVersionRecorder interface {
	RecordDatasetVersionOnRun(ctx context.Context, runID uuid.UUID, datasetVersionID uuid.UUID, contentHash string) error
}

type RuntimeConfig struct {
	DatasetServiceURL            string
	PipelineServiceURL           string
	OntologyServiceURL           string
	IngestionReplicationGRPCURL  string
	NetworkBoundaryServiceURL    string
	SyncPollIntervalSecs         uint64
	AllowPrivateNetworkEgress    bool
	AllowedEgressHosts           []string
	AgentStaleAfterSecs          uint64
	CredentialEncryptionKey      string
	CredentialKey                [32]byte
	SecretManagerURL             string
	OutboxEnabled                bool
	AutoRegistrationIntervalSecs uint64
	VendedCredentialsTTLSeconds  int64
}

type Handlers struct {
	Repo              Store
	AdapterRegistry   *adapters.Registry
	MediaSetRuntime   MediaSetRuntime
	IngestionRuntime  cmruntime.IngestionPort
	DatasetVersioning cmruntime.DatasetVersioningPort
	Config            RuntimeConfig
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type routeError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeRoutePending(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, routeError{Error: code, Code: code, Message: message})
}

func routeUUIDParam(r *http.Request, names ...string) (uuid.UUID, string, error) {
	for _, name := range names {
		if raw := chi.URLParam(r, name); raw != "" {
			id, err := uuid.Parse(raw)
			return id, name, err
		}
	}
	return uuid.Nil, names[0], errors.New("missing route parameter")
}

func (h *Handlers) notImplemented(w http.ResponseWriter, r *http.Request, code string, requireAuth bool) {
	if requireAuth {
		if _, ok := requireClaims(w, r); !ok {
			return
		}
	}
	writeRoutePending(w, http.StatusNotImplemented, code, "route mounted for Rust parity; implementation pending")
}

func (h *Handlers) GetConnectorCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.BuildGalleryCatalog())
}

func (h *Handlers) GetConnectorContracts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.BuildConnectorContractCatalog())
}

func (h *Handlers) ListStreamingSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]models.StreamingSourceContract{"data": models.StreamingSourceContracts()})
}

func (h *Handlers) GetConnectionCapabilities(w http.ResponseWriter, r *http.Request) {
	id, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.Repo == nil {
		writeRoutePending(w, http.StatusServiceUnavailable, "repository_unavailable", "connection repository is not configured")
		return
	}
	connection, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		slog.Error("connection capability lookup failed", "error", err)
		writeJSONErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if connection == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	contract, ok := models.ConnectorProfile(connection.ConnectorType)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no connector contract available for " + connection.ConnectorType})
		return
	}
	capabilities := h.connectionEffectiveCapabilities(*connection, contract)
	writeJSON(w, http.StatusOK, models.ConnectionCapabilityResponse{ConnectionID: connection.ID, ConnectorType: connection.ConnectorType, Status: connection.Status, Contract: contract, Capabilities: capabilities})
}

func (h *Handlers) connectionEffectiveCapabilities(connection models.Connection, contract models.ConnectorContractProfile) models.ConnectionEffectiveCapabilities {
	configKeys, inferred := models.ConfigKeys(connection.Config)
	workers := []string{"foundry"}
	if contract.Auth.SupportsPrivateNetworkAgent {
		workers = append(workers, "agent")
	}
	warnings := []string{}
	privateAllowed := h.Config.AllowPrivateNetworkEgress
	if len(h.Config.AllowedEgressHosts) > 0 {
		privateAllowed = true
	}
	requiresAgent := contract.Auth.SupportsPrivateNetworkAgent && !privateAllowed
	if requiresAgent {
		warnings = append(warnings, "private network egress is disabled; route through a connector agent or allow egress for this source")
	}
	return models.ConnectionEffectiveCapabilities{
		ConnectionID:                connection.ID,
		ConnectorType:               connection.ConnectorType,
		Status:                      connection.Status,
		Modes:                       append([]string(nil), contract.Sync.Modes...),
		Workers:                     workers,
		SupportsConnectionTesting:   contract.Testing.SupportsConnectionTesting,
		SupportsDiscovery:           contract.Testing.SupportsDiscovery,
		SupportsSchemaIntrospection: contract.Testing.SupportsSchemaIntrospection,
		SupportsIncremental:         contract.Sync.SupportsIncremental || inferred.HasIncrementalCursor,
		SupportsCDC:                 contract.Sync.SupportsCDC && (inferred.HasCDCSelector || connection.ConnectorType == "kafka" || connection.ConnectorType == "kinesis" || connection.ConnectorType == "iot"),
		SupportsZeroCopy:            contract.Sync.SupportsZeroCopy || inferred.RequestsZeroCopy,
		SupportsPrivateNetworkAgent: contract.Auth.SupportsPrivateNetworkAgent,
		RequiresPrivateNetworkAgent: requiresAgent,
		PrivateNetworkEgressAllowed: privateAllowed,
		AllowedEgressHosts:          append([]string(nil), h.Config.AllowedEgressHosts...),
		ConfigKeys:                  configKeys,
		ConfigInferred:              inferred,
		PolicyWarnings:              warnings,
		FoundryCompute:              models.FoundryCompute{PythonSingleNode: true, PythonSpark: true, PipelineBuilderSingleNode: true, PipelineBuilderSpark: true},
	}
}

var validCredentialKinds = map[string]bool{
	"password": true, "api_key": true, "oauth_token": true, "aws_keys": true, "service_account_json": true,
}

func (h *Handlers) ListCredentials(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	items, err := h.Repo.ListCredentials(r.Context(), sourceID, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) SetCredential(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	var body models.SetCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Kind = strings.TrimSpace(body.Kind)
	if !validCredentialKinds[body.Kind] {
		writeJSONErr(w, http.StatusBadRequest, fmt.Sprintf("unsupported credential kind: %s", body.Kind))
		return
	}
	if body.Value == "" {
		writeJSONErr(w, http.StatusBadRequest, "credential value required")
		return
	}
	digest := sha256.Sum256([]byte(body.Value))
	fingerprint := fmt.Sprintf("%x", digest[:])
	ciphertext, err := domain.EncryptCredential(h.Config.CredentialKey, []byte(body.Value))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "credential encryption failed")
		return
	}
	v, err := h.Repo.SetCredential(r.Context(), sourceID, claims.Sub, body.Kind, ciphertext, fingerprint)
	if errors.Is(err, pgx.ErrNoRows) || v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to store credential")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) ListSourcePolicies(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	items, err := h.Repo.ListSourcePolicies(r.Context(), sourceID, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list source policies")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) AttachPolicy(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, _, err := routeUUIDParam(r, "id", "source_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	var body models.AttachPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.PolicyID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "policy_id required")
		return
	}
	kind := strings.TrimSpace(body.Kind)
	if kind == "" {
		kind = "direct"
	}
	if kind != "direct" && kind != "agent_proxy" {
		writeJSONErr(w, http.StatusBadRequest, "kind must be 'direct' or 'agent_proxy'")
		return
	}
	v, err := h.Repo.AttachPolicy(r.Context(), sourceID, claims.Sub, body.PolicyID, kind)
	if errors.Is(err, pgx.ErrNoRows) || v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to attach policy")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) DetachPolicy(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, _, err := routeUUIDParam(r, "source_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid source_id")
		return
	}
	policyID, _, err := routeUUIDParam(r, "policy_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid policy_id")
		return
	}
	deleted, err := h.Repo.DetachPolicy(r.Context(), sourceID, claims.Sub, policyID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to detach policy")
		return
	}
	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	syncID, _, err := routeUUIDParam(r, "id", "sync_id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid sync id")
		return
	}
	items, err := h.Repo.ListSyncRuns(r.Context(), syncID, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list sync runs")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) DevAuthLogin(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, r, "dev_auth_pending", false)
}

func (h *Handlers) DevAuthRefresh(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, r, "dev_auth_pending", false)
}

func (h *Handlers) DevAuthBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, r, "dev_auth_pending", false)
}

func (h *Handlers) DevAuthMe(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, r, "dev_auth_me_pending", true)
}

func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var ownerID *uuid.UUID
	if raw := r.URL.Query().Get("owner_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid owner_id")
			return
		}
		ownerID = &id
	}
	items, err := h.Repo.ListConnections(r.Context(), ownerID)
	if err != nil {
		slog.Error("list connections", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list connections")
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.Connection]{Items: items})
}

func (h *Handlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.Repo.GetConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	caller, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.ConnectorType == "" {
		writeJSONErr(w, http.StatusBadRequest, "name and connector_type required")
		return
	}
	if len(body.Config) > 0 && !json.Valid(body.Config) {
		writeJSONErr(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}
	v, err := h.Repo.CreateConnection(r.Context(), &body, caller.Sub)
	if err != nil {
		slog.Error("create connection", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Config) > 0 && !json.Valid(body.Config) {
		writeJSONErr(w, http.StatusBadRequest, "config must be valid JSON")
		return
	}
	v, err := h.Repo.UpdateConnection(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.Repo.DeleteConnection(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeJSONErr(w, http.StatusNotFound, "connection not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireClaims(w http.ResponseWriter, r *http.Request) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return claims, true
}

func (h *Handlers) ListSyncJobs(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.Repo.ListSyncJobs(r.Context(), sourceID, claims.Sub)
	if err != nil {
		slog.Error("list sync jobs", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list sync jobs")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) GetSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	v, err := h.Repo.GetSyncJob(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	var body models.CreateSyncJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.SourceID == uuid.Nil || body.OutputDatasetID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "source_id and output_dataset_id required")
		return
	}
	v, err := h.Repo.CreateSyncJob(r.Context(), &body, claims.Sub)
	if err != nil {
		slog.Error("create sync job", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create sync job")
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	var body models.UpdateSyncJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.UpdateSyncJob(r.Context(), id, &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) RunSyncJob(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	job, err := h.Repo.GetSyncJob(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if job == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	conn, err := h.Repo.GetConnectionForOwner(r.Context(), job.SourceID, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conn == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	run, err := h.Repo.RunSyncJob(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		writeJSONErr(w, http.StatusNotFound, "sync job not found")
		return
	}
	ingestion := h.ingestionPort()
	if ingestion == nil {
		writeJSON(w, http.StatusAccepted, run)
		return
	}
	completed := h.dispatchSyncRun(r.Context(), claims.Sub, run, job, conn, ingestion)
	writeJSON(w, http.StatusAccepted, completed)
}

func (h *Handlers) ingestionPort() cmruntime.IngestionPort {
	if h.IngestionRuntime != nil {
		return h.IngestionRuntime
	}
	if strings.TrimSpace(h.Config.IngestionReplicationGRPCURL) == "" {
		return nil
	}
	return cmruntime.HTTPIngestionClient{BaseURL: h.Config.IngestionReplicationGRPCURL}
}

func (h *Handlers) datasetVersioningPort() cmruntime.DatasetVersioningPort {
	if h.DatasetVersioning != nil {
		return h.DatasetVersioning
	}
	if strings.TrimSpace(h.Config.DatasetServiceURL) == "" {
		return nil
	}
	return cmruntime.HTTPDatasetVersioningClient{BaseURL: h.Config.DatasetServiceURL}
}

func (h *Handlers) dispatchSyncRun(ctx context.Context, ownerID uuid.UUID, run *models.SyncRun, job *models.SyncJob, conn *models.Connection, ingestion cmruntime.IngestionPort) *models.SyncRun {
	materialized, err := cmruntime.Materialize(run.ID, job, conn)
	if err != nil {
		return h.completeRun(ctx, ownerID, run, syncdomain.RunStatusFailed, 0, 0, err, nil, nil, nil)
	}
	result, err := ingestion.Dispatch(ctx, materialized)
	if err != nil {
		return h.completeRun(ctx, ownerID, run, syncdomain.RunStatusFailed, 0, 0, err, nil, nil, &materialized.ContentHash)
	}
	contentHash := materialized.ContentHash
	if len(result.Payload) > 0 {
		digest := sha256.Sum256(result.Payload)
		contentHash = fmt.Sprintf("%x", digest[:])
	}
	datasetVersionID := h.registerDatasetVersion(ctx, run.ID, job, conn, result, contentHash)
	ingestJobID := strings.TrimSpace(result.IngestJobID)
	var ingestJobIDPtr *string
	if ingestJobID != "" {
		ingestJobIDPtr = &ingestJobID
	}
	return h.completeRun(ctx, ownerID, run, syncdomain.RunStatusSucceeded, result.BytesWritten, result.FilesWritten, nil, ingestJobIDPtr, datasetVersionID, &contentHash)
}

func (h *Handlers) registerDatasetVersion(ctx context.Context, runID uuid.UUID, job *models.SyncJob, conn *models.Connection, result *cmruntime.IngestionResult, contentHash string) *uuid.UUID {
	if lookup, ok := h.Repo.(previousDatasetVersionLookup); ok {
		previous, err := lookup.PreviousDatasetVersionForHash(ctx, job.ID, contentHash)
		if err == nil && previous != nil {
			if recorder, ok := h.Repo.(datasetVersionRecorder); ok {
				_ = recorder.RecordDatasetVersionOnRun(ctx, runID, *previous, contentHash)
			}
			return previous
		}
	}
	port := h.datasetVersioningPort()
	if port == nil {
		return nil
	}
	version, err := port.Register(ctx, cmruntime.DatasetVersionRequest{SyncDefID: job.ID, RunID: runID, SourceID: conn.ID, OutputDatasetID: job.OutputDatasetID, ContentHash: contentHash, RowCount: result.RowsWritten, SizeBytes: result.BytesWritten, Schema: json.RawMessage(`{}`), Message: "connector sync " + job.ID.String()})
	if err != nil || version == nil || version.DatasetVersionID == uuid.Nil {
		if err != nil {
			slog.Warn("dataset version registration failed", slog.String("error", err.Error()), slog.String("sync_run_id", runID.String()))
		}
		return nil
	}
	return &version.DatasetVersionID
}

func (h *Handlers) completeRun(ctx context.Context, ownerID uuid.UUID, run *models.SyncRun, status string, bytesWritten int64, filesWritten int64, runErr error, ingestJobID *string, datasetVersionID *uuid.UUID, contentHash *string) *models.SyncRun {
	var errMsg *string
	if runErr != nil {
		msg := runErr.Error()
		errMsg = &msg
	}
	if completer, ok := h.Repo.(syncRunCompleter); ok {
		updated, err := completer.CompleteSyncRun(ctx, run.ID, ownerID, status, bytesWritten, filesWritten, errMsg, ingestJobID, datasetVersionID, contentHash)
		if err == nil && updated != nil {
			return updated
		}
		if err != nil {
			slog.Error("complete sync run", slog.String("error", err.Error()), slog.String("sync_run_id", run.ID.String()))
		}
	}
	now := time.Now().UTC()
	copy := *run
	copy.Status = status
	copy.FinishedAt = &now
	copy.BytesWritten = bytesWritten
	copy.FilesWritten = filesWritten
	copy.Error = errMsg
	copy.IngestJobID = ingestJobID
	copy.DatasetVersionID = datasetVersionID
	copy.ContentHash = contentHash
	return &copy
}

func (h *Handlers) EnableVirtualTableSource(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireClaims(w, r); !ok {
		return
	}
	sourceRID := strings.TrimSpace(chi.URLParam(r, "source_rid"))
	if sourceRID == "" {
		writeJSONErr(w, http.StatusBadRequest, "source_rid required")
		return
	}
	var body models.EnableVirtualTableSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := h.Repo.EnableVirtualTableSource(r.Context(), sourceRID, &body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateVirtualTable(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceRID := strings.TrimSpace(chi.URLParam(r, "source_rid"))
	if sourceRID == "" {
		writeJSONErr(w, http.StatusBadRequest, "source_rid required")
		return
	}
	var body models.CreateVirtualTableRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.ProjectRID) == "" || strings.TrimSpace(body.TableType) == "" {
		writeJSONErr(w, http.StatusBadRequest, "project_rid and table_type required")
		return
	}
	v, err := h.Repo.CreateVirtualTable(r.Context(), sourceRID, claims.Sub.String(), &body)
	if errors.Is(err, repo.ErrConflict) {
		writeJSONErr(w, http.StatusConflict, "virtual table already registered")
		return
	}
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not enabled")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) ListVirtualTables(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.Repo.ListVirtualTables(r.Context(), claims.Sub.String(), r.URL.Query().Get("project"), r.URL.Query().Get("source"), limit)
	if err != nil {
		slog.Error("list virtual tables", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list virtual tables")
		return
	}
	writeJSON(w, http.StatusOK, models.ListVirtualTablesResponse{Items: items})
}

func (h *Handlers) GetVirtualTable(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	rid := strings.TrimSpace(chi.URLParam(r, "rid"))
	if rid == "" {
		writeJSONErr(w, http.StatusBadRequest, "rid required")
		return
	}
	v, err := h.Repo.GetVirtualTable(r.Context(), rid, claims.Sub.String())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "virtual table not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) ListMediaSetSyncs(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.Repo.ListMediaSetSyncs(r.Context(), sourceID, claims.Sub)
	if err != nil {
		slog.Error("list media set syncs", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list media set syncs")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) GetMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	v, err := h.Repo.GetMediaSetSync(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) CreateMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.CreateMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if errs := body.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": errs})
		return
	}
	v, err := h.Repo.CreateMediaSetSync(r.Context(), sourceID, &body, claims.Sub)
	if err != nil {
		slog.Error("create media set sync", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create media set sync")
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "source not found")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	var body models.UpdateMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Kind != nil && !body.Kind.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": {"kind must be MEDIA_SET_SYNC or VIRTUAL_MEDIA_SET_SYNC"}})
		return
	}
	if body.TargetMediaSetRID != nil && !strings.HasPrefix(strings.TrimSpace(*body.TargetMediaSetRID), "ri.foundry.main.media_set.") {
		writeJSON(w, http.StatusBadRequest, map[string][]string{"errors": {"target_media_set_rid must start with ri.foundry.main.media_set."}})
		return
	}
	v, err := h.Repo.UpdateMediaSetSync(r.Context(), id, &body, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if v == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handlers) RunMediaSetSync(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	id, param, err := routeUUIDParam(r, "sync_id", "id")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+param)
		return
	}
	var body models.RunMediaSetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	sync, err := h.Repo.GetMediaSetSync(r.Context(), id, claims.Sub)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sync == nil {
		writeJSONErr(w, http.StatusNotFound, "media set sync not found")
		return
	}
	if h.MediaSetRuntime == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "media set runtime not configured")
		return
	}
	report, err := h.MediaSetRuntime.ExecuteMediaSetSync(r.Context(), sync, &body, r.Header.Get("Authorization"))
	if err != nil {
		writeJSONErr(w, runtimeHTTPStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, report)
}

// Package handlers exposes the HTTP surface of code-repository-review-service.
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/outbox"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/codesecurity"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/repo"
)

// PromoteTopic is the canonical Kafka topic for the global-branch
// promote-requested event. Preserved verbatim from Rust.
const PromoteTopic = "foundry.global.branch.promote.requested.v1"

// PromoteEventType is the value placed on the `event_type` header
// AND inside the payload — both are part of the wire contract.
const PromoteEventType = "global.branch.promote.requested.v1"

// Handlers wires repo + pool together.
type Handlers struct {
	Repo         *repo.GlobalBranchRepo
	Pool         *pgxpool.Pool
	Actor        string
	CodeSecurity *codesecurity.Service
}

// --- helpers ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// --- handlers -----------------------------------------------------------

func (h *Handlers) ListGlobalBranches(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Repo.ListBranches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) CreateGlobalBranch(w http.ResponseWriter, r *http.Request) {
	var body models.CreateGlobalBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	branch, err := h.Repo.CreateBranch(r.Context(), body, h.Actor)
	if err != nil {
		if repo.IsUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "name already in use",
				"name":  body.Name,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("audit", slog.String("action", "global_branch.created"),
		slog.String("actor", h.Actor), slog.String("target", branch.RID),
		slog.String("name", branch.Name))
	writeJSON(w, http.StatusCreated, branch)
}

func (h *Handlers) GetGlobalBranch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	branch, err := h.Repo.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if branch == nil {
		writeError(w, http.StatusNotFound, "global branch not found")
		return
	}
	link, drifted, archived, err := h.Repo.LinkCounts(r.Context(), branch.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models.SummaryFromBranch(*branch, link, drifted, archived))
}

func (h *Handlers) AddLink(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.CreateGlobalBranchLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.ResourceType) == "" ||
		strings.TrimSpace(body.ResourceRID) == "" ||
		strings.TrimSpace(body.BranchRID) == "" {
		writeError(w, http.StatusBadRequest,
			"resource_type, resource_rid and branch_rid are required")
		return
	}
	branch, err := h.Repo.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if branch == nil {
		writeError(w, http.StatusNotFound, "global branch not found")
		return
	}
	link, err := h.Repo.AddLink(r.Context(), id, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("audit", slog.String("action", "global_branch.link.added"),
		slog.String("actor", h.Actor), slog.String("target", id.String()),
		slog.String("resource_type", body.ResourceType),
		slog.String("resource_rid", body.ResourceRID),
		slog.String("branch_rid", body.BranchRID))
	writeJSON(w, http.StatusCreated, link)
}

func (h *Handlers) ListResources(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	rows, err := h.Repo.ListLinks(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) Promote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	branch, err := h.Repo.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if branch == nil {
		writeError(w, http.StatusNotFound, "global branch not found")
		return
	}

	eventID := uuid.New()
	payload, err := buildPromotePayload(branch.ID, branch.Name, h.Actor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tx, err := h.Pool.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	event := outbox.New(eventID, "global_branch", branch.ID.String(), PromoteTopic, payload).
		WithHeader("event_type", PromoteEventType).
		WithHeader("global_branch_rid", branch.RID)
	if err := outbox.Enqueue(r.Context(), tx, event); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	committed = true

	slog.Info("audit", slog.String("action", "global_branch.promote.requested"),
		slog.String("actor", h.Actor), slog.String("target", branch.RID),
		slog.String("event_id", eventID.String()))

	writeJSON(w, http.StatusOK, models.PromoteResponse{
		EventID:        eventID,
		GlobalBranchID: branch.ID,
		Topic:          PromoteTopic,
	})
}

// buildPromotePayload returns the verbatim Rust payload shape:
//
//	{
//	  "event_type": "global.branch.promote.requested.v1",
//	  "global_branch_id": "<uuid>",
//	  "global_branch_name": "<name>",
//	  "actor": "<actor>",
//	  "occurred_at": "<RFC3339>"
//	}
func buildPromotePayload(globalID uuid.UUID, name, actor string) (json.RawMessage, error) {
	body := map[string]any{
		"event_type":         PromoteEventType,
		"global_branch_id":   globalID,
		"global_branch_name": name,
		"actor":              actor,
		"occurred_at":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(body)
}

// CreateCodeSecurityScan runs the configured scanner and persists the scan + findings.
func (h *Handlers) CreateCodeSecurityScan(w http.ResponseWriter, r *http.Request) {
	if h.CodeSecurity == nil {
		writeError(w, http.StatusServiceUnavailable, "code security scanner is not configured")
		return
	}
	var body codesecurity.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.CodeSecurity.ScanAndPersist(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

package marketplace

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
)

type Handlers struct{ Repo Repository }

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandlers(repo Repository) *Handlers { return &Handlers{Repo: repo} }

func (h *Handlers) ListListings(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := h.Repo.ListListings(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	writeJSON(w, http.StatusOK, models.PaginatedListResponse[models.ListingDefinition]{
		Items:      items,
		Pagination: models.Pagination{Limit: limit, Offset: offset, Total: total},
	})
}

func (h *Handlers) CreateListing(w http.ResponseWriter, r *http.Request) {
	var req models.CreateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	listing, err := h.Repo.CreateListing(r.Context(), req)
	if err != nil {
		handleRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (h *Handlers) GetListing(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")
	if ref == "" {
		ref = chi.URLParam(r, "slug")
	}
	if ref == "" {
		ref = chi.URLParam(r, "id")
	}
	detail, err := h.Repo.GetListing(r.Context(), ref)
	if err != nil {
		handleRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handlers) UpdateListing(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var req models.UpdateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	listing, err := h.Repo.UpdateListing(r.Context(), id, req)
	if err != nil {
		handleRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (h *Handlers) PublishVersion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var req models.PublishVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	version, err := h.Repo.PublishVersion(r.Context(), id, req)
	if err != nil {
		handleRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

func parsePagination(r *http.Request) (int, int, error) {
	limit := 50
	offset := 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, errors.New("limit must be between 1 and 100")
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be greater than or equal to 0")
		}
	}
	return limit, offset, nil
}

func handleRepoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "listing not found")
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusBadRequest, stringsAfterColon(err.Error()))
	default:
		writeError(w, http.StatusInternalServerError, "database operation failed")
	}
}

func stringsAfterColon(msg string) string {
	const prefix = "marketplace validation failed: "
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return msg
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

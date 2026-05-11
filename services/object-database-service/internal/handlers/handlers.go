// Package handlers wires HTTP requests to ObjectStore / LinkStore.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/storage"
)

type Handlers struct {
	Objects storage.ObjectStore
	Links   storage.LinkStore
	Backend config.BackendMode
}

// --- request / response wire types --------------------------------------

type writeObjectRequest struct {
	TypeID          string          `json:"type_id"`
	Version         uint64          `json:"version"`
	Payload         json.RawMessage `json:"payload"`
	ExpectedVersion *uint64         `json:"expected_version,omitempty"`
	Owner           *string         `json:"owner,omitempty"`
	Markings        []string        `json:"markings"`
	OrganizationID  *string         `json:"organization_id,omitempty"`
	CreatedAtMs     *int64          `json:"created_at_ms,omitempty"`
	UpdatedAtMs     *int64          `json:"updated_at_ms,omitempty"`
}

type writeObjectResponse struct {
	Outcome         string  `json:"outcome"`
	PreviousVersion *uint64 `json:"previous_version"`
	NewVersion      *uint64 `json:"new_version"`
	ExpectedVersion *uint64 `json:"expected_version"`
	ActualVersion   *uint64 `json:"actual_version"`
}

type objectListResponse struct {
	Items     []storage.Object `json:"items"`
	NextToken *string          `json:"next_token"`
}

type linkListResponse struct {
	Items     []storage.Link `json:"items"`
	NextToken *string        `json:"next_token"`
}

type writeLinkRequest struct {
	From        string          `json:"from"`
	To          string          `json:"to"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	CreatedAtMs *int64          `json:"created_at_ms,omitempty"`
}

type statusResponse struct {
	Service string             `json:"service"`
	Ready   bool               `json:"ready"`
	Backend config.BackendMode `json:"backend"`
}

// --- helpers ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, err error) {
	if re, ok := storage.AsRepoError(err); ok {
		switch re.Kind {
		case storage.ErrNotFound:
			http.Error(w, re.Error(), http.StatusNotFound)
		case storage.ErrInvalidArgument:
			http.Error(w, re.Error(), http.StatusBadRequest)
		case storage.ErrTenantScope:
			http.Error(w, re.Error(), http.StatusForbidden)
		default:
			http.Error(w, re.Error(), http.StatusInternalServerError)
		}
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func parseConsistency(v string) storage.ReadConsistency {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "eventual":
		return storage.ReadEventual
	default:
		return storage.ReadStrong
	}
}

func pageFromQuery(r *http.Request) storage.Page {
	q := r.URL.Query()
	size := uint32(100)
	if s := q.Get("size"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			size = uint32(n)
		}
	}
	if size < 1 {
		size = 1
	}
	if size > 5000 {
		size = 5000
	}
	var token *string
	if t := q.Get("token"); t != "" {
		t := t
		token = &t
	}
	return storage.Page{Size: size, Token: token}
}

func writeOutcomeResponse(w http.ResponseWriter, o storage.PutOutcome) {
	resp := writeObjectResponse{Outcome: string(o.Kind)}
	switch o.Kind {
	case storage.PutUpdated:
		pv := o.PreviousVersion
		nv := o.NewVersion
		resp.PreviousVersion = &pv
		resp.NewVersion = &nv
	case storage.PutVersionConflict:
		ev := o.ExpectedVersion
		av := o.ActualVersion
		resp.ExpectedVersion = &ev
		resp.ActualVersion = &av
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- handlers -----------------------------------------------------------

func (h *Handlers) Status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Service: "object-database-service",
		Ready:   true,
		Backend: h.Backend,
	})
}

func (h *Handlers) Readiness(w http.ResponseWriter, r *http.Request) { h.Status(w, r) }

func (h *Handlers) GetObject(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	id := storage.ObjectId(chi.URLParam(r, "object_id"))
	obj, err := h.Objects.Get(r.Context(), tenant, id, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	if obj == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

func (h *Handlers) PutObject(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	id := storage.ObjectId(chi.URLParam(r, "object_id"))

	var body writeObjectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	nowMs := time.Now().UnixMilli()
	createdAtMs := body.CreatedAtMs
	if createdAtMs == nil {
		createdAtMs = &nowMs
	}
	updatedAtMs := nowMs
	if body.UpdatedAtMs != nil {
		updatedAtMs = *body.UpdatedAtMs
	}

	var ownerPtr *storage.OwnerId
	if body.Owner != nil {
		o := storage.OwnerId(*body.Owner)
		ownerPtr = &o
	}
	markings := make([]storage.MarkingId, len(body.Markings))
	for i, m := range body.Markings {
		markings[i] = storage.MarkingId(m)
	}

	obj := storage.Object{
		Tenant:         tenant,
		ID:             id,
		TypeID:         storage.TypeId(body.TypeID),
		Version:        body.Version,
		Payload:        body.Payload,
		OrganizationID: body.OrganizationID,
		CreatedAtMs:    createdAtMs,
		UpdatedAtMs:    updatedAtMs,
		Owner:          ownerPtr,
		Markings:       markings,
	}

	outcome, err := h.Objects.Put(r.Context(), obj, body.ExpectedVersion)
	if err != nil {
		writeError(w, err)
		return
	}
	writeOutcomeResponse(w, outcome)
}

func (h *Handlers) DeleteObject(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	id := storage.ObjectId(chi.URLParam(r, "object_id"))
	deleted, err := h.Objects.Delete(r.Context(), tenant, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListByType(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	typeID := storage.TypeId(chi.URLParam(r, "type_id"))
	res, err := h.Objects.ListByType(r.Context(), tenant, typeID, pageFromQuery(r), parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectListResponse{Items: res.Items, NextToken: res.NextToken})
}

func (h *Handlers) ListByOwner(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	owner := storage.OwnerId(chi.URLParam(r, "owner_id"))
	res, err := h.Objects.ListByOwner(r.Context(), tenant, owner, pageFromQuery(r), parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectListResponse{Items: res.Items, NextToken: res.NextToken})
}

func (h *Handlers) ListByMarking(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	marking := storage.MarkingId(chi.URLParam(r, "marking_id"))
	res, err := h.Objects.ListByMarking(r.Context(), tenant, marking, pageFromQuery(r), parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objectListResponse{Items: res.Items, NextToken: res.NextToken})
}

func (h *Handlers) PutLink(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	lt := storage.LinkTypeId(chi.URLParam(r, "link_type"))
	var body writeLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.From) == "" || strings.TrimSpace(body.To) == "" {
		http.Error(w, "from and to required", http.StatusBadRequest)
		return
	}
	nowMs := time.Now().UnixMilli()
	if body.CreatedAtMs != nil {
		nowMs = *body.CreatedAtMs
	}
	var payload *json.RawMessage
	if len(body.Payload) > 0 {
		cp := append(json.RawMessage(nil), body.Payload...)
		payload = &cp
	}
	link := storage.Link{
		Tenant:      tenant,
		LinkType:    lt,
		From:        storage.ObjectId(body.From),
		To:          storage.ObjectId(body.To),
		Payload:     payload,
		CreatedAtMs: nowMs,
	}
	if err := h.Links.Put(r.Context(), link); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (h *Handlers) ListOutgoingLinks(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	lt := storage.LinkTypeId(chi.URLParam(r, "link_type"))
	from := storage.ObjectId(chi.URLParam(r, "from"))
	res, err := h.Links.ListOutgoing(r.Context(), tenant, lt, from, pageFromQuery(r), parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, linkListResponse{Items: res.Items, NextToken: res.NextToken})
}

func (h *Handlers) ListIncomingLinks(w http.ResponseWriter, r *http.Request) {
	tenant := storage.TenantId(chi.URLParam(r, "tenant"))
	lt := storage.LinkTypeId(chi.URLParam(r, "link_type"))
	to := storage.ObjectId(chi.URLParam(r, "to"))
	res, err := h.Links.ListIncoming(r.Context(), tenant, lt, to, pageFromQuery(r), parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, linkListResponse{Items: res.Items, NextToken: res.NextToken})
}

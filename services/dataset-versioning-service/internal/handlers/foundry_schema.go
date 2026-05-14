package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) GetFoundryDatasetSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := foundryBranchName(r.URL.Query().Get("branchName"))
	endTxn, ok := parseOptionalTransactionRIDParam(w, r.URL.Query().Get("endTransactionRid"))
	if !ok {
		return
	}
	var versionID *string
	if raw := strings.TrimSpace(r.URL.Query().Get("versionId")); raw != "" {
		versionID = &raw
	}
	out, err := h.Repo.GetDatasetSchema(r.Context(), datasetID, branch, endTxn, versionID)
	if err != nil {
		writeFoundrySchemaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) PutFoundryDatasetSchema(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	var body models.PutFoundryDatasetSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	reader := "PARQUET"
	if body.DataframeReader != nil {
		reader = *body.DataframeReader
	}
	schema := models.DatasetSchemaFromFoundry(body.Schema, reader)
	if body.CustomMetadata != nil {
		schema.CustomMetadata = body.CustomMetadata
	}
	if body.ParserOptions != nil {
		schema.CustomMetadata = &models.CustomMetadata{CSV: body.ParserOptions}
	}
	if errs := models.ValidateDatasetSchema(schema); len(errs) > 0 {
		writeSchemaParseError(w, strings.Join(errs, "; "))
		return
	}
	var endTxn *uuid.UUID
	if body.EndTransactionRID != nil {
		parsed, err := parseFoundryTransactionRID(*body.EndTransactionRID)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid endTransactionRid")
			return
		}
		endTxn = &parsed
	}
	branch := ""
	if body.BranchName != nil {
		branch = foundryBranchName(*body.BranchName)
	}
	out, err := h.Repo.PutDatasetSchema(r.Context(), datasetID, branch, endTxn, reader, schema)
	if err != nil {
		writeFoundrySchemaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) GetFoundryDatasetSchemaBatch(w http.ResponseWriter, r *http.Request) {
	var body []models.GetSchemaDatasetsBatchRequestElement
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body) > 1000 {
		writeJSONErr(w, http.StatusBadRequest, "batch size must be <= 1000")
		return
	}
	out := models.GetSchemaDatasetsBatchResponse{Data: map[string]models.FoundryDatasetSchemaResponse{}}
	for _, item := range body {
		rid := strings.TrimSpace(item.DatasetRID)
		if rid == "" {
			continue
		}
		datasetID, err := h.Repo.ResolveDatasetID(r.Context(), rid)
		if err != nil {
			continue
		}
		branch := ""
		if item.BranchName != nil {
			branch = foundryBranchName(*item.BranchName)
		}
		var endTxn *uuid.UUID
		if item.EndTransactionRID != nil && strings.TrimSpace(*item.EndTransactionRID) != "" {
			parsed, err := parseFoundryTransactionRID(*item.EndTransactionRID)
			if err != nil {
				continue
			}
			endTxn = &parsed
		}
		schema, err := h.Repo.GetDatasetSchema(r.Context(), datasetID, branch, endTxn, item.VersionID)
		if err != nil || schema == nil {
			continue
		}
		out.Data[rid] = *schema
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) ListDatasetSchemaHistory(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := foundryBranchName(r.URL.Query().Get("branchName"))
	if raw := strings.TrimSpace(r.URL.Query().Get("branch")); raw != "" {
		branch = foundryBranchName(raw)
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeJSONErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if n < limit {
			limit = n
		}
	}
	rows, err := h.Repo.ListDatasetSchemaHistory(r.Context(), datasetID, branch, limit)
	if err != nil {
		writeFoundrySchemaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.Page[models.SchemaEvolutionEntry]{Data: rows, HasMore: len(rows) == limit})
}

func foundryBranchName(raw string) string {
	return strings.TrimSpace(raw)
}

func parseOptionalTransactionRIDParam(w http.ResponseWriter, raw string) (*uuid.UUID, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, true
	}
	id, err := parseFoundryTransactionRID(raw)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid endTransactionRid")
		return nil, false
	}
	return &id, true
}

func parseFoundryTransactionRID(raw string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "ri.foundry.main.transaction.")
	return uuid.Parse(trimmed)
}

func writeFoundrySchemaError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, repo.ErrNotFound) {
		writeJSONErr(w, http.StatusNotFound, "schema not found")
		return
	}
	if errors.Is(err, repo.ErrValidation) {
		writeSchemaParseError(w, err.Error())
		return
	}
	if repo.IsConflict(err) {
		writeJSONErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONErr(w, http.StatusInternalServerError, err.Error())
}

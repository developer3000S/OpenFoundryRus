// Package geospatial wires the geospatial layer-management handlers
// ported 1:1 from
// services/ontology-exploratory-analysis-service/src/geospatial/geospatial_base/handlers/layers.rs
// (S8 / ADR-0030 — geospatial-intelligence-service absorbed). The
// handlers live as a typed library namespace; the Go binary does not
// mount them yet, mirroring the Rust `#[allow(dead_code)] mod geospatial`.
package geospatial

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// AppState carries the dependencies needed by every geospatial handler.
// Mirrors `crate::geospatial::AppState { db, jwt_config }` — the
// jwt_config field is intentionally omitted until OEA-3+ wires auth
// (matches Rust where the field is held but unused on these handlers).
type AppState struct {
	DB *pgxpool.Pool
}

// ErrorResponse is the `{ "error": "..." }` envelope every handler in the
// geospatial namespace returns on failure.
type ErrorResponse struct {
	Error string `json:"error"`
}

// SupportedOperations enumerates the operations advertised by the lazy
// indexer overview. Order matches `build_overview` in Rust — clients
// rely on stable indexes for capability negotiation.
var SupportedOperations = []string{
	"within",
	"intersects",
	"nearest",
	"buffer",
	"dbscan",
	"kmeans",
	"vector_tiles",
	"routing",
}

// BuildOverview mirrors `domain::indexer::build_overview` in Rust. It
// derives the lazy index summary from the in-memory layer slice — no
// I/O, suitable for tests against synthetic layers.
func BuildOverview(layers []models.LayerDefinition) models.GeospatialOverview {
	indexed := 0
	totalFeatures := 0
	tileReady := 0
	for _, layer := range layers {
		if layer.Indexed {
			indexed++
		}
		totalFeatures += len(layer.Features)
		if layer.Indexed && len(layer.Features) > 0 {
			tileReady++
		}
	}
	ops := make([]string, len(SupportedOperations))
	copy(ops, SupportedOperations)
	return models.GeospatialOverview{
		LayerCount:          len(layers),
		IndexedLayers:       indexed,
		TotalFeatures:       totalFeatures,
		TileReadyLayers:     tileReady,
		SupportedOperations: ops,
	}
}

const layerSelectColumns = `id, name, description, source_kind, source_dataset, geometry_type, style, features, tags, indexed, created_at, updated_at`

// LoadAllLayers mirrors `load_all_layers` in handlers/mod.rs.
func LoadAllLayers(ctx context.Context, db *pgxpool.Pool) ([]models.LayerDefinition, error) {
	rows, err := db.Query(ctx, `SELECT `+layerSelectColumns+` FROM geospatial_layers ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.LayerDefinition, 0)
	for rows.Next() {
		row, err := scanLayerRow(rows)
		if err != nil {
			return nil, err
		}
		def, err := row.ToDefinition()
		if err != nil {
			return nil, fmt.Errorf("decode layer row: %w", err)
		}
		out = append(out, def)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadLayerRow mirrors `load_layer_row` in handlers/mod.rs.
// Returns (definition, true, nil) on hit, (zero, false, nil) on miss.
func LoadLayerRow(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (models.LayerDefinition, bool, error) {
	row := db.QueryRow(ctx, `SELECT `+layerSelectColumns+` FROM geospatial_layers WHERE id = $1`, id)
	r, err := scanLayerRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.LayerDefinition{}, false, nil
	}
	if err != nil {
		return models.LayerDefinition{}, false, err
	}
	def, err := r.ToDefinition()
	if err != nil {
		return models.LayerDefinition{}, false, fmt.Errorf("decode layer row: %w", err)
	}
	return def, true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLayerRow(rs rowScanner) (models.LayerRow, error) {
	var r models.LayerRow
	err := rs.Scan(
		&r.ID,
		&r.Name,
		&r.Description,
		&r.SourceKind,
		&r.SourceDataset,
		&r.GeometryType,
		&r.Style,
		&r.Features,
		&r.Tags,
		&r.Indexed,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	return r, err
}

// GetOverview mirrors `handlers::layers::get_overview`.
func (s *AppState) GetOverview(w http.ResponseWriter, r *http.Request) {
	layers, err := LoadAllLayers(r.Context(), s.DB)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, BuildOverview(layers))
}

// ListLayers mirrors `handlers::layers::list_layers`.
func (s *AppState) ListLayers(w http.ResponseWriter, r *http.Request) {
	layers, err := LoadAllLayers(r.Context(), s.DB)
	if err != nil {
		s.dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, models.ListResponse[models.LayerDefinition]{Items: layers})
}

// CreateLayer mirrors `handlers::layers::create_layer`.
func (s *AppState) CreateLayer(w http.ResponseWriter, r *http.Request) {
	var req models.CreateLayerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "layer name is required")
		return
	}
	if len(req.Features) == 0 {
		writeError(w, http.StatusBadRequest, "layer requires at least one feature")
		return
	}
	if !req.SourceKind.Valid() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported layer source kind: %s", req.SourceKind))
		return
	}
	if !req.GeometryType.Valid() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported geometry type: %s", req.GeometryType))
		return
	}

	style := models.NewDefaultLayerStyle()
	if req.Style != nil {
		style = *req.Style
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	indexed := true
	if req.Indexed != nil {
		indexed = *req.Indexed
	}

	id, err := uuid.NewV7()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()

	styleBytes, err := json.Marshal(style)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	featuresBytes, err := json.Marshal(req.Features)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsBytes, err := json.Marshal(tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = s.DB.Exec(r.Context(),
		`INSERT INTO geospatial_layers (id, name, description, source_kind, source_dataset, geometry_type, style, features, tags, indexed, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9::jsonb, $10, $11, $12)`,
		id,
		req.Name,
		req.Description,
		req.SourceKind.String(),
		req.SourceDataset,
		req.GeometryType.String(),
		styleBytes,
		featuresBytes,
		tagsBytes,
		indexed,
		now,
		now,
	)
	if err != nil {
		s.dbError(w, err)
		return
	}

	def, found, err := LoadLayerRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusInternalServerError, "created layer could not be reloaded")
		return
	}
	writeJSON(w, http.StatusOK, def)
}

// UpdateLayer mirrors `handlers::layers::update_layer`.
func (s *AppState) UpdateLayer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid layer id")
		return
	}

	existing, found, err := LoadLayerRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "layer not found")
		return
	}

	var req models.UpdateLayerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	layer := existing
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeError(w, http.StatusBadRequest, "layer name cannot be empty")
			return
		}
		layer.Name = *req.Name
	}
	if req.Description != nil {
		layer.Description = *req.Description
	}
	if req.SourceKind != nil {
		if !req.SourceKind.Valid() {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported layer source kind: %s", *req.SourceKind))
			return
		}
		layer.SourceKind = *req.SourceKind
	}
	if req.SourceDataset != nil {
		layer.SourceDataset = *req.SourceDataset
	}
	if req.GeometryType != nil {
		if !req.GeometryType.Valid() {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported geometry type: %s", *req.GeometryType))
			return
		}
		layer.GeometryType = *req.GeometryType
	}
	if req.Style != nil {
		layer.Style = *req.Style
	}
	if req.Features != nil {
		if len(*req.Features) == 0 {
			writeError(w, http.StatusBadRequest, "layer requires at least one feature")
			return
		}
		layer.Features = *req.Features
	}
	if req.Tags != nil {
		layer.Tags = *req.Tags
	}
	if req.Indexed != nil {
		layer.Indexed = *req.Indexed
	}

	now := time.Now().UTC()
	styleBytes, err := json.Marshal(layer.Style)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	featuresBytes, err := json.Marshal(layer.Features)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tagsBytes, err := json.Marshal(layer.Tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = s.DB.Exec(r.Context(),
		`UPDATE geospatial_layers
		 SET name = $2,
		     description = $3,
		     source_kind = $4,
		     source_dataset = $5,
		     geometry_type = $6,
		     style = $7::jsonb,
		     features = $8::jsonb,
		     tags = $9::jsonb,
		     indexed = $10,
		     updated_at = $11
		 WHERE id = $1`,
		id,
		layer.Name,
		layer.Description,
		layer.SourceKind.String(),
		layer.SourceDataset,
		layer.GeometryType.String(),
		styleBytes,
		featuresBytes,
		tagsBytes,
		layer.Indexed,
		now,
	)
	if err != nil {
		s.dbError(w, err)
		return
	}

	updated, found, err := LoadLayerRow(r.Context(), s.DB, id)
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusInternalServerError, "updated layer could not be reloaded")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Routes returns a chi sub-router with the layer endpoints. Not yet
// mounted by the binary — exposed for the upcoming consolidation main.
func (s *AppState) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/overview", s.GetOverview)
	r.Get("/layers", s.ListLayers)
	r.Post("/layers", s.CreateLayer)
	r.Put("/layers/{id}", s.UpdateLayer)
	r.Post("/query", s.QueryFeatures)
	r.Post("/cluster", s.ClusterFeatures)
	r.Get("/tiles/{id}", s.GetVectorTile)
	r.Post("/geocode", s.ForwardGeocode)
	r.Post("/geocode/reverse", s.ReverseGeocode)
	return r
}

func (s *AppState) dbError(w http.ResponseWriter, cause error) {
	slog.Error("geospatial-service database error", slog.String("error", cause.Error()))
	writeError(w, http.StatusInternalServerError, "database operation failed")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

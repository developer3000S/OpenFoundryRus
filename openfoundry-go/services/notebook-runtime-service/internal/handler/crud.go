// Package handler — notebook + cell CRUD endpoints. 1:1 port of
// `services/notebook-runtime-service/src/handlers/crud.rs`.
//
// Wire format (paths / verbs / status codes / JSON shapes) matches
// the Rust router so the existing notebook frontends keep round-
// tripping. Behaviour highlights:
//
//   - `add_cell` reads `MAX(position)` to compute the next ordinal
//     when the caller doesn't pass one, mirroring the Rust path
//     exactly.
//   - `get_notebook` returns `{ "notebook": ..., "cells": [...] }`
//     so the client gets the cell list in one round-trip.
//   - Update endpoints implement the "COALESCE($n, column)" pattern
//     so PATCH semantics line up with the Rust handler (only fields
//     present in the body get rewritten).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

// ── Notebook CRUD ───────────────────────────────────────────────────

// CreateNotebook mirrors `pub async fn create_notebook`.
func (s *State) CreateNotebook(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	var body models.CreateNotebookRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	kernel := "python"
	if body.DefaultKernel != nil && *body.DefaultKernel != "" {
		kernel = *body.DefaultKernel
	}
	id, _ := uuid.NewV7()
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		// Explicit smoke-mode fallback: synthesise and keep the row in memory.
		now := time.Now().UTC()
		nb := models.Notebook{
			ID: id, Name: body.Name, Description: description,
			OwnerID: claims.Sub, DefaultKernel: kernel,
			CreatedAt: now, UpdatedAt: now,
		}
		s.memoryRepo().putNotebook(nb)
		writeJSON(w, http.StatusCreated, nb)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        INSERT INTO notebooks (id, name, description, owner_id, default_kernel)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, name, description, owner_id, default_kernel, created_at, updated_at`,
		id, body.Name, description, claims.Sub, kernel)
	nb, err := scanNotebook(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, nb)
}

// ListNotebooks mirrors `pub async fn list_notebooks`.
func (s *State) ListNotebooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseInt64(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	perPage := parseInt64(q.Get("per_page"), 20)
	switch {
	case perPage < 1:
		perPage = 1
	case perPage > 100:
		perPage = 100
	}
	repo := s.notebookListRepo()
	if repo == nil {
		s.databaseRequired(w)
		return
	}
	notebooks, total, err := repo.ListNotebooks(r.Context(), ListNotebooksParams{
		Search:  q.Get("search"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": notebooks, "total": total, "page": page, "per_page": perPage,
	})
}

// GetNotebook mirrors `pub async fn get_notebook`. Returns the
// notebook plus its cells in one round-trip.
func (s *State) GetNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		nb, ok := s.memoryRepo().loadNotebook(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		cells := s.memoryRepo().loadCells(id)
		writeJSON(w, http.StatusOK, map[string]any{"notebook": nb, "cells": cells})
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        SELECT id, name, description, owner_id, default_kernel, created_at, updated_at
        FROM notebooks WHERE id = $1`, id)
	nb, err := scanNotebook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	cells, err := loadCells(r.Context(), s.Pool, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"notebook": nb,
		"cells":    cells,
	})
}

// UpdateNotebook mirrors `pub async fn update_notebook`. PATCH +
// PUT route through the same handler — the Rust crate registers them
// against the same axum function.
func (s *State) UpdateNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	var body models.UpdateNotebookRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		nb, ok := s.memoryRepo().updateNotebook(id, body)
		if !ok {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		writeJSON(w, http.StatusOK, nb)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        UPDATE notebooks SET
            name = COALESCE($2, name),
            description = COALESCE($3, description),
            default_kernel = COALESCE($4, default_kernel),
            updated_at = NOW()
        WHERE id = $1
        RETURNING id, name, description, owner_id, default_kernel, created_at, updated_at`,
		id, body.Name, body.Description, body.DefaultKernel)
	nb, err := scanNotebook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, nb)
}

// DeleteNotebook mirrors `pub async fn delete_notebook`.
func (s *State) DeleteNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		if !s.memoryRepo().deleteNotebook(id) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM notebooks WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if tag.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Cell CRUD ───────────────────────────────────────────────────────

// AddCell mirrors `pub async fn add_cell`. When `position` is unset,
// inserts after the last cell — mirrors the Rust impl that reads
// `MAX(position)` first.
func (s *State) AddCell(w http.ResponseWriter, r *http.Request) {
	notebookID, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	var body models.CreateCellRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	cellType := "code"
	if body.CellType != nil && *body.CellType != "" {
		cellType = *body.CellType
	}
	kernel := "python"
	if body.Kernel != nil && *body.Kernel != "" {
		kernel = *body.Kernel
	}
	source := ""
	if body.Source != nil {
		source = *body.Source
	}
	id, _ := uuid.NewV7()
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		position := int32(1)
		if body.Position != nil {
			position = *body.Position
		}
		now := time.Now().UTC()
		cell := models.Cell{
			ID:         id,
			NotebookID: notebookID,
			CellType:   cellType,
			Kernel:     kernel,
			Source:     source,
			Position:   position,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		s.memoryRepo().putCell(cell)
		writeJSON(w, http.StatusCreated, cell)
		return
	}
	position := int32(0)
	if body.Position != nil {
		position = *body.Position
	} else {
		var maxPos *int32
		_ = s.Pool.QueryRow(r.Context(),
			`SELECT MAX(position) FROM cells WHERE notebook_id = $1`,
			notebookID).Scan(&maxPos)
		if maxPos == nil {
			position = 1
		} else {
			position = *maxPos + 1
		}
	}
	row := s.Pool.QueryRow(r.Context(), `
        INSERT INTO cells (id, notebook_id, cell_type, kernel, source, position)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, notebook_id, cell_type, kernel, source, position,
                  last_output, execution_count, created_at, updated_at`,
		id, notebookID, cellType, kernel, source, position)
	cell, err := scanCell(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, cell)
}

// UpdateCell mirrors `pub async fn update_cell`.
func (s *State) UpdateCell(w http.ResponseWriter, r *http.Request) {
	cellID, err := pathUUID(r, "cell_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid cell id"))
		return
	}
	var body models.UpdateCellRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		cell, ok := s.memoryRepo().updateCell(cellID, body)
		if !ok {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		writeJSON(w, http.StatusOK, cell)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        UPDATE cells SET
            source = COALESCE($2, source),
            cell_type = COALESCE($3, cell_type),
            kernel = COALESCE($4, kernel),
            position = COALESCE($5, position),
            updated_at = NOW()
        WHERE id = $1
        RETURNING id, notebook_id, cell_type, kernel, source, position,
                  last_output, execution_count, created_at, updated_at`,
		cellID, body.Source, body.CellType, body.Kernel, body.Position)
	cell, err := scanCell(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, cell)
}

// DeleteCell mirrors `pub async fn delete_cell`.
func (s *State) DeleteCell(w http.ResponseWriter, r *http.Request) {
	cellID, err := pathUUID(r, "cell_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid cell id"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		if !s.memoryRepo().deleteCell(cellID) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM cells WHERE id = $1`, cellID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if tag.RowsAffected() == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Repository helpers ──────────────────────────────────────────────

// rowScanner abstracts pgx.Row + pgx.Rows so scan helpers work for
// either a one-row QueryRow result or a streaming Query result.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanNotebook(s rowScanner) (models.Notebook, error) {
	var nb models.Notebook
	err := s.Scan(&nb.ID, &nb.Name, &nb.Description, &nb.OwnerID,
		&nb.DefaultKernel, &nb.CreatedAt, &nb.UpdatedAt)
	return nb, err
}

func scanCell(s rowScanner) (models.Cell, error) {
	var c models.Cell
	var lastOutput []byte
	err := s.Scan(&c.ID, &c.NotebookID, &c.CellType, &c.Kernel, &c.Source,
		&c.Position, &lastOutput, &c.ExecutionCount, &c.CreatedAt, &c.UpdatedAt)
	if len(lastOutput) > 0 {
		c.LastOutput = lastOutput
	}
	return c, err
}

func loadCells(ctx context.Context, pool *pgxpool.Pool, notebookID uuid.UUID) ([]models.Cell, error) {
	rows, err := pool.Query(ctx, `
        SELECT id, notebook_id, cell_type, kernel, source, position,
               last_output, execution_count, created_at, updated_at
        FROM cells WHERE notebook_id = $1 ORDER BY position ASC`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Cell{}
	for rows.Next() {
		c, err := scanCell(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ── Misc helpers shared with sessions.go ────────────────────────────

func parseInt64(s string, fallback int64) int64 {
	if s == "" {
		return fallback
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/llm"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// PromptsHandlers is the prompt-templates surface (list / create /
// update / render). Mirrors libs/ai-kernel/src/handlers/prompts.rs.
type PromptsHandlers struct {
	Pool *pgxpool.Pool
}

const promptColumns = `id, name, description, category, status, tags,
                       latest_version_number, versions, created_at, updated_at`

// scanPromptRow scans + builds the PromptTemplate, deriving
// current_version from the last entry in versions (or a placeholder
// when versions is empty — matches Rust From impl).
func scanPromptRow(s toolScanner) (models.PromptTemplate, error) {
	var p models.PromptTemplate
	var tagsRaw, versionsRaw []byte
	if err := s.Scan(
		&p.ID, &p.Name, &p.Description, &p.Category, &p.Status,
		&tagsRaw, &p.LatestVersionNumber, &versionsRaw,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return p, err
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &p.Tags)
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if len(versionsRaw) > 0 {
		_ = json.Unmarshal(versionsRaw, &p.Versions)
	}
	if p.Versions == nil {
		p.Versions = []models.PromptVersion{}
	}
	if n := len(p.Versions); n > 0 {
		p.CurrentVersion = p.Versions[n-1]
	} else {
		p.CurrentVersion = models.PromptVersion{
			VersionNumber:  p.LatestVersionNumber,
			InputVariables: []string{},
			CreatedAt:      p.CreatedAt,
		}
	}
	return p, nil
}

func (h *PromptsHandlers) loadPromptRow(ctx context.Context, id uuid.UUID) (*models.PromptTemplate, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+promptColumns+` FROM ai_prompt_templates WHERE id = $1`, id)
	p, err := scanPromptRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPrompts handles `GET /api/v1/prompts`.
func (h *PromptsHandlers) ListPrompts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+promptColumns+` FROM ai_prompt_templates
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.PromptTemplate, 0)
	for rows.Next() {
		p, err := scanPromptRow(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, models.ListPromptTemplatesResponse{Data: out})
}

// CreatePrompt handles `POST /api/v1/prompts`. Validates name +
// content. Inserts version 1 with notes defaulting to "Initial version".
func (h *PromptsHandlers) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	var body models.CreatePromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Content) == "" {
		writeError(w, http.StatusBadRequest, "prompt name and content are required")
		return
	}

	description := derefString(body.Description, "")
	category := derefString(body.Category, models.DefaultPromptCategory)
	notes := derefString(body.Notes, "Initial version")

	inputVars := body.InputVariables
	if inputVars == nil {
		inputVars = []string{}
	}
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}

	version := models.PromptVersion{
		VersionNumber:  1,
		Content:        strings.TrimSpace(body.Content),
		InputVariables: inputVars,
		Notes:          notes,
		CreatedAt:      time.Now().UTC(),
	}

	tagsJSON, _ := json.Marshal(tags)
	versionsJSON, _ := json.Marshal([]models.PromptVersion{version})

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ai_prompt_templates
              (id, name, description, category, status, tags,
               latest_version_number, versions)
            VALUES ($1, $2, $3, $4, 'active', $5, 1, $6)
            RETURNING `+promptColumns,
		uuid.New(), strings.TrimSpace(body.Name), description, category,
		tagsJSON, versionsJSON)
	p, err := scanPromptRow(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// UpdatePrompt handles `PATCH /api/v1/prompts/{id}`. When content,
// input_variables, OR notes is supplied, appends a new version
// (latest+1) and bumps latest_version_number; otherwise just
// updates the metadata.
func (h *PromptsHandlers) UpdatePrompt(w http.ResponseWriter, r *http.Request, promptID uuid.UUID) {
	current, err := h.loadPromptRow(r.Context(), promptID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "prompt template not found")
		return
	}

	var body models.UpdatePromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	versions := append([]models.PromptVersion(nil), current.Versions...)
	if body.Content != nil || body.InputVariables != nil || body.Notes != nil {
		base := current.CurrentVersion
		nextNumber := int32(1)
		if n := len(versions); n > 0 {
			nextNumber = versions[n-1].VersionNumber + 1
		}
		content := base.Content
		if body.Content != nil {
			content = *body.Content
		}
		inputVars := base.InputVariables
		if body.InputVariables != nil {
			inputVars = *body.InputVariables
		}
		var notes string
		if body.Notes != nil {
			notes = *body.Notes
		} else {
			notes = "Version " + itoa32(nextNumber)
		}
		versions = append(versions, models.PromptVersion{
			VersionNumber:  nextNumber,
			Content:        strings.TrimSpace(content),
			InputVariables: inputVars,
			Notes:          notes,
			CreatedAt:      time.Now().UTC(),
		})
	}

	latestVersionNumber := current.LatestVersionNumber
	if n := len(versions); n > 0 {
		latestVersionNumber = versions[n-1].VersionNumber
	}

	name := derefString(body.Name, current.Name)
	desc := derefString(body.Description, current.Description)
	category := derefString(body.Category, current.Category)
	status := derefString(body.Status, current.Status)
	tags := current.Tags
	if body.Tags != nil {
		tags = *body.Tags
	}
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, _ := json.Marshal(tags)
	versionsJSON, _ := json.Marshal(versions)

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ai_prompt_templates SET
            name = $2, description = $3, category = $4, status = $5,
            tags = $6, latest_version_number = $7, versions = $8,
            updated_at = NOW()
          WHERE id = $1
          RETURNING `+promptColumns,
		promptID, name, desc, category, status, tagsJSON,
		latestVersionNumber, versionsJSON)
	p, err := scanPromptRow(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// RenderPrompt handles `POST /api/v1/prompts/{id}/render`. Loads the
// template's current_version content, runs llm.InterpolateTemplate
// against the request variables, and returns the rendered content +
// missing variable list. In strict mode missing variables → 400.
func (h *PromptsHandlers) RenderPrompt(w http.ResponseWriter, r *http.Request, promptID uuid.UUID) {
	current, err := h.loadPromptRow(r.Context(), promptID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "prompt template not found")
		return
	}

	var body models.RenderPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	rendered, missing := llm.InterpolateTemplate(current.CurrentVersion.Content, body.Variables, body.Strict)
	if missing == nil {
		missing = []string{}
	}
	if body.Strict && len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "missing prompt variables")
		return
	}

	writeJSON(w, http.StatusOK, models.RenderPromptResponse{
		PromptID:         promptID,
		VersionNumber:    current.CurrentVersion.VersionNumber,
		RenderedContent:  rendered,
		MissingVariables: missing,
	})
}

// itoa32 is a small inline integer-to-string helper to avoid pulling
// strconv just for the version-number formatting.
func itoa32(n int32) string {
	if n == 0 {
		return "0"
	}
	var b [11]byte
	i := len(b)
	negative := false
	x := n
	if x < 0 {
		negative = true
		x = -x
	}
	for x > 0 {
		i--
		b[i] = byte('0' + x%10)
		x /= 10
	}
	if negative {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

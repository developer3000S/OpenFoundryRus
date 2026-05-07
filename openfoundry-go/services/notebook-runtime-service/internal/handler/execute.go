package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/domain/environment"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

const defaultNotebookCellTimeoutSeconds uint32 = 60

// notebookPythonKernel is the injectable runtime boundary used by
// ExecuteCell/ExecuteAllCells. Production wires it to python-sidecar;
// tests replace it with fakes without spawning a subprocess.
type NotebookPythonKernel interface {
	EnsureSession(ctx context.Context, sessionID uuid.UUID) error
	ExecuteCell(ctx context.Context, sessionID, notebookID uuid.UUID, source, workspaceDir string, timeoutSeconds uint32) (*pythonsidecar.NotebookCellResult, error)
	DropSession(ctx context.Context, sessionID uuid.UUID) error
}

type NotebookSQLKernel interface {
	ExecuteSQL(ctx context.Context, claims *authmw.Claims, source string) (*pythonsidecar.NotebookCellResult, error)
}

type NotebookRKernel interface {
	ExecuteR(ctx context.Context, source, workspaceDir string) (*pythonsidecar.NotebookCellResult, error)
}

type NotebookLLMKernel interface {
	ExecuteLLM(ctx context.Context, sessionID *uuid.UUID, notebookID uuid.UUID, source, workspaceDir string, workspaceFiles []models.NotebookWorkspaceFile, claims *authmw.Claims) (*pythonsidecar.NotebookCellResult, error)
	DropSession(ctx context.Context, sessionID uuid.UUID) error
}

type httpSQLKernel struct {
	Client          *http.Client
	QueryServiceURL string
	JWTConfig       *authmw.JWTConfig
}

type rscriptKernel struct{}

type httpLLMKernel struct {
	Client        *http.Client
	AIServiceURL  string
	JWTConfig     *authmw.JWTConfig
	mu            sync.Mutex
	conversations map[uuid.UUID]uuid.UUID
}

// memoryNotebookRepo is the minimal no-DB repository slice used by unit
// tests and smoke clusters. The Postgres path remains the source of
// truth when Pool is non-nil.
type MemoryNotebookRepo struct {
	mu        sync.Mutex
	notebooks map[uuid.UUID]models.Notebook
	cells     map[uuid.UUID]models.Cell
	sessions  map[uuid.UUID]models.Session
}

func NewMemoryNotebookRepo() *MemoryNotebookRepo {
	return &MemoryNotebookRepo{
		notebooks: map[uuid.UUID]models.Notebook{},
		cells:     map[uuid.UUID]models.Cell{},
		sessions:  map[uuid.UUID]models.Session{},
	}
}

func (m *MemoryNotebookRepo) putNotebook(nb models.Notebook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notebooks == nil {
		m.notebooks = map[uuid.UUID]models.Notebook{}
	}
	m.notebooks[nb.ID] = nb
}

func (m *MemoryNotebookRepo) listNotebooks() []models.Notebook {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.Notebook, 0, len(m.notebooks))
	for _, nb := range m.notebooks {
		out = append(out, nb)
	}
	sortNotebooksByUpdatedAt(out)
	return out
}

func (m *MemoryNotebookRepo) ListNotebooks(_ context.Context, params ListNotebooksParams) ([]models.Notebook, int64, error) {
	notebooks := m.listNotebooks()
	if params.Search != "" {
		filtered := notebooks[:0]
		needle := strings.ToLower(params.Search)
		for _, nb := range notebooks {
			if strings.Contains(strings.ToLower(nb.Name), needle) {
				filtered = append(filtered, nb)
			}
		}
		notebooks = filtered
	}
	total := int64(len(notebooks))
	start := (params.Page - 1) * params.PerPage
	if start >= int64(len(notebooks)) {
		return []models.Notebook{}, total, nil
	}
	end := start + params.PerPage
	if end > int64(len(notebooks)) {
		end = int64(len(notebooks))
	}
	return notebooks[start:end], total, nil
}

func (m *MemoryNotebookRepo) loadNotebook(id uuid.UUID) (models.Notebook, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nb, ok := m.notebooks[id]
	return nb, ok
}

func (m *MemoryNotebookRepo) updateNotebook(id uuid.UUID, body models.UpdateNotebookRequest) (models.Notebook, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nb, ok := m.notebooks[id]
	if !ok {
		return models.Notebook{}, false
	}
	if body.Name != nil {
		nb.Name = *body.Name
	}
	if body.Description != nil {
		nb.Description = *body.Description
	}
	if body.DefaultKernel != nil {
		nb.DefaultKernel = *body.DefaultKernel
	}
	nb.UpdatedAt = time.Now().UTC()
	m.notebooks[id] = nb
	return nb, true
}

func (m *MemoryNotebookRepo) deleteNotebook(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.notebooks[id]; !ok {
		return false
	}
	delete(m.notebooks, id)
	for cellID, cell := range m.cells {
		if cell.NotebookID == id {
			delete(m.cells, cellID)
		}
	}
	for sessionID, session := range m.sessions {
		if session.NotebookID == id {
			delete(m.sessions, sessionID)
		}
	}
	return true
}

func (m *MemoryNotebookRepo) putCell(c models.Cell) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cells == nil {
		m.cells = map[uuid.UUID]models.Cell{}
	}
	m.cells[c.ID] = c
}

func (m *MemoryNotebookRepo) updateCell(id uuid.UUID, body models.UpdateCellRequest) (models.Cell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cells[id]
	if !ok {
		return models.Cell{}, false
	}
	if body.Source != nil {
		c.Source = *body.Source
	}
	if body.CellType != nil {
		c.CellType = *body.CellType
	}
	if body.Kernel != nil {
		c.Kernel = *body.Kernel
	}
	if body.Position != nil {
		c.Position = *body.Position
	}
	c.UpdatedAt = time.Now().UTC()
	m.cells[id] = c
	return c, true
}

func (m *MemoryNotebookRepo) deleteCell(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cells[id]; !ok {
		return false
	}
	delete(m.cells, id)
	return true
}

func (m *MemoryNotebookRepo) putSession(sess models.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = map[uuid.UUID]models.Session{}
	}
	m.sessions[sess.ID] = sess
}

func (m *MemoryNotebookRepo) loadCell(id uuid.UUID) (models.Cell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cells[id]
	return c, ok
}

func (m *MemoryNotebookRepo) loadCells(notebookID uuid.UUID) []models.Cell {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.Cell{}
	for _, c := range m.cells {
		if c.NotebookID == notebookID {
			out = append(out, c)
		}
	}
	sortCellsByPosition(out)
	return out
}

func (m *MemoryNotebookRepo) loadCodeCells(notebookID uuid.UUID) []models.Cell {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.Cell{}
	for _, c := range m.cells {
		if c.NotebookID == notebookID && c.CellType == "code" {
			out = append(out, c)
		}
	}
	sortCellsByPosition(out)
	return out
}

func (m *MemoryNotebookRepo) loadSession(id uuid.UUID) (models.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

func (m *MemoryNotebookRepo) listSessions(notebookID uuid.UUID) []models.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.Session{}
	for _, sess := range m.sessions {
		if sess.NotebookID == notebookID {
			out = append(out, sess)
		}
	}
	sortSessionsByCreatedAt(out)
	return out
}

func (m *MemoryNotebookRepo) ListSessions(_ context.Context, notebookID uuid.UUID) ([]models.Session, error) {
	return m.listSessions(notebookID), nil
}

func (m *MemoryNotebookRepo) stopSession(id uuid.UUID) (models.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return models.Session{}, false
	}
	sess.Status = "dead"
	sess.LastActivity = time.Now().UTC()
	m.sessions[id] = sess
	return sess, true
}

func (m *MemoryNotebookRepo) updateSessionStatus(id uuid.UUID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return
	}
	sess.Status = status
	sess.LastActivity = time.Now().UTC()
	m.sessions[id] = sess
}

func (m *MemoryNotebookRepo) persistOutput(cellID uuid.UUID, output models.CellOutput, count int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cells[cellID]
	if !ok {
		return
	}
	raw, _ := json.Marshal(output)
	c.LastOutput = raw
	c.ExecutionCount = &count
	c.UpdatedAt = time.Now().UTC()
	m.cells[cellID] = c
}

type executeQueryRequest struct {
	SQL   string `json:"sql"`
	Limit *int   `json:"limit,omitempty"`
}

type queryColumn struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

type executeQueryResponse struct {
	Columns         []queryColumn `json:"columns"`
	Rows            [][]string    `json:"rows"`
	TotalRows       int           `json:"total_rows"`
	ExecutionTimeMs uint64        `json:"execution_time_ms"`
}

func (k *httpSQLKernel) ExecuteSQL(ctx context.Context, claims *authmw.Claims, source string) (*pythonsidecar.NotebookCellResult, error) {
	if k == nil || strings.TrimSpace(k.QueryServiceURL) == "" {
		return nil, errors.New("query-service URL is not configured")
	}
	if k.JWTConfig == nil {
		return nil, errors.New("query-service token requires JWT config")
	}
	token, err := authmw.EncodeToken(k.JWTConfig, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign query-service token: %w", err)
	}
	limit := 1000
	body, _ := json.Marshal(executeQueryRequest{SQL: source, Limit: &limit})
	url := strings.TrimRight(k.QueryServiceURL, "/") + "/api/v1/queries/execute"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("content-type", "application/json")
	client := k.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query-service request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, errors.New(resp.Status)
		}
		if raw, ok := payload["error"]; ok {
			var msg string
			if json.Unmarshal(raw, &msg) == nil && msg != "" {
				return nil, errors.New(msg)
			}
		}
		return nil, errors.New("query execution failed")
	}
	var payload executeQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid query-service response: %w", err)
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize SQL result: %w", err)
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "table", ContentJSON: content}, nil
}

func (rscriptKernel) ExecuteR(ctx context.Context, source, workspaceDir string) (*pythonsidecar.NotebookCellResult, error) {
	cmd := exec.CommandContext(ctx, "Rscript", "-e", buildRScript(source, workspaceDir))
	if workspaceDir != "" {
		cmd.Dir = workspaceDir
	}
	out, err := cmd.Output()
	if err == nil {
		content, _ := json.Marshal(string(out))
		return &pythonsidecar.NotebookCellResult{OutputType: "text", ContentJSON: content, Stdout: string(out)}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := string(exitErr.Stderr)
		if strings.TrimSpace(stderr) == "" {
			return nil, errors.New("R execution failed")
		}
		return nil, errors.New(stderr)
	}
	return nil, fmt.Errorf("failed to start Rscript: %w", err)
}

func buildRScript(source, workspaceDir string) string {
	workspaceDir = strings.ReplaceAll(strings.ReplaceAll(workspaceDir, `\`, "/"), `'`, `\'`)
	return fmt.Sprintf("workspace_dir <- '%s'\nif (nzchar(workspace_dir)) { setwd(workspace_dir) }\n%s\n", workspaceDir, source)
}

type chatCompletionRequest struct {
	ConversationID  *uuid.UUID `json:"conversation_id,omitempty"`
	UserMessage     string     `json:"user_message"`
	SystemPrompt    string     `json:"system_prompt"`
	FallbackEnabled bool       `json:"fallback_enabled"`
	MaxTokens       int        `json:"max_tokens"`
}

type chatCompletionResponse struct {
	ConversationID uuid.UUID       `json:"conversation_id"`
	ProviderName   string          `json:"provider_name"`
	Reply          string          `json:"reply"`
	Citations      json.RawMessage `json:"citations"`
	Usage          json.RawMessage `json:"usage"`
	CreatedAt      string          `json:"created_at"`
}

func (k *httpLLMKernel) ExecuteLLM(ctx context.Context, sessionID *uuid.UUID, notebookID uuid.UUID, source, workspaceDir string, workspaceFiles []models.NotebookWorkspaceFile, claims *authmw.Claims) (*pythonsidecar.NotebookCellResult, error) {
	if k == nil || strings.TrimSpace(k.AIServiceURL) == "" {
		return nil, errors.New("AI service URL is not configured")
	}
	if k.JWTConfig == nil {
		return nil, errors.New("AI service token requires JWT config")
	}
	token, err := authmw.EncodeToken(k.JWTConfig, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign AI service token: %w", err)
	}
	var conversationID *uuid.UUID
	if sessionID != nil {
		k.mu.Lock()
		if k.conversations != nil {
			if existing, ok := k.conversations[*sessionID]; ok {
				copy := existing
				conversationID = &copy
			}
		}
		k.mu.Unlock()
	}
	requestPayload := chatCompletionRequest{
		ConversationID:  conversationID,
		UserMessage:     source,
		SystemPrompt:    buildLLMSystemPrompt(notebookID, workspaceDir, workspaceFiles),
		FallbackEnabled: true,
		MaxTokens:       900,
	}
	body, _ := json.Marshal(requestPayload)
	url := strings.TrimRight(k.AIServiceURL, "/") + "/api/v1/ai/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("content-type", "application/json")
	client := k.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai-service request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, errors.New(resp.Status)
		}
		if raw, ok := payload["error"]; ok {
			var msg string
			if json.Unmarshal(raw, &msg) == nil && msg != "" {
				return nil, errors.New(msg)
			}
		}
		return nil, errors.New("LLM completion failed")
	}
	var payload chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid ai-service response: %w", err)
	}
	if sessionID != nil {
		k.mu.Lock()
		if k.conversations == nil {
			k.conversations = map[uuid.UUID]uuid.UUID{}
		}
		k.conversations[*sessionID] = payload.ConversationID
		k.mu.Unlock()
	}
	if len(payload.Citations) == 0 {
		payload.Citations = json.RawMessage(`[]`)
	}
	if len(payload.Usage) == 0 {
		payload.Usage = json.RawMessage(`{}`)
	}
	content, err := json.Marshal(map[string]json.RawMessage{
		"reply":           mustJSON(payload.Reply),
		"provider_name":   mustJSON(payload.ProviderName),
		"conversation_id": mustJSON(payload.ConversationID),
		"citations":       payload.Citations,
		"usage":           payload.Usage,
		"created_at":      mustJSON(payload.CreatedAt),
	})
	if err != nil {
		return nil, err
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "llm", ContentJSON: content}, nil
}

func (k *httpLLMKernel) DropSession(_ context.Context, sessionID uuid.UUID) error {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.conversations, sessionID)
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func buildLLMSystemPrompt(notebookID uuid.UUID, workspaceDir string, workspaceFiles []models.NotebookWorkspaceFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are assisting inside an OpenFoundry notebook cell.\nNotebook ID: %s\n", notebookID)
	if workspaceDir != "" {
		fmt.Fprintf(&b, "Workspace directory: %s\n", workspaceDir)
	}
	if len(workspaceFiles) > 0 {
		b.WriteString("Workspace files in scope:\n")
		for i, file := range workspaceFiles {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "--- %s ---\n%s\n", file.Path, truncateString(file.Content, 900))
		}
	}
	b.WriteString("\nBe concise, technical, and notebook-friendly. When the user asks for code, return runnable code or precise operational guidance.")
	return b.String()
}

func truncateString(value string, maxChars int) string {
	if len([]rune(value)) <= maxChars {
		return value
	}
	return string([]rune(value)[:maxChars]) + "\n...[truncated]"
}

func (s *State) ExecuteCell(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	notebookID, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	cellID, err := pathUUID(r, "cell_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid cell id"))
		return
	}
	var body models.ExecuteCellRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}

	cell, ok, err := s.loadCell(r.Context(), cellID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok || cell.NotebookID != notebookID {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if cell.CellType == "markdown" {
		content, _ := json.Marshal(cell.Source)
		writeJSON(w, http.StatusOK, models.CellOutput{OutputType: "text", Content: content, ExecutionCount: 0})
		return
	}

	if body.SessionID != nil {
		sess, ok, err := s.loadSession(r.Context(), *body.SessionID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if sess.Status == "dead" {
			writeJSON(w, http.StatusConflict, "session is stopped")
			return
		}
		if sess.Kernel != cell.Kernel {
			writeJSON(w, http.StatusBadRequest, "session kernel does not match cell kernel")
			return
		}
	}

	output, count := s.executeCodeCell(r.Context(), notebookID, cell, body.SessionID, claims)
	writeJSON(w, http.StatusOK, output)
	_ = count
}

func (s *State) ExecuteAllCells(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	notebookID, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	var body models.ExecuteCellRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}

	var sharedSession *models.Session
	if body.SessionID != nil {
		sess, ok, err := s.loadSession(r.Context(), *body.SessionID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		if ok {
			sharedSession = &sess
			s.updateSessionStatus(r.Context(), sess.ID, "busy")
		}
	}

	cells, err := s.loadCodeCells(r.Context(), notebookID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}

	results := make([]map[string]any, 0, len(cells))
	for _, cell := range cells {
		var sid *uuid.UUID
		if sharedSession != nil && sharedSession.Kernel == cell.Kernel && sharedSession.Status != "dead" {
			sid = &sharedSession.ID
		}
		output, _ := s.executeCodeCell(r.Context(), notebookID, cell, sid, claims)
		results = append(results, map[string]any{"cell_id": cell.ID, "output": output})
	}

	if sharedSession != nil {
		s.updateSessionStatus(r.Context(), sharedSession.ID, "idle")
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *State) executeCodeCell(ctx context.Context, notebookID uuid.UUID, cell models.Cell, sessionID *uuid.UUID, claims *authmw.Claims) (models.CellOutput, int32) {
	if sessionID != nil {
		sess, ok, err := s.loadSession(ctx, *sessionID)
		if err != nil {
			return s.errorOutput(cell, err.Error())
		}
		if !ok {
			return s.errorOutput(cell, "session not found")
		}
		if sess.Status == "dead" {
			return s.errorOutput(cell, "session is stopped")
		}
		if sess.Kernel != cell.Kernel {
			return s.errorOutput(cell, "session kernel does not match cell kernel")
		}
		s.updateSessionStatus(ctx, sess.ID, "busy")
	}

	result, err := s.executeKernel(ctx, notebookID, cell, sessionID, claims)
	count := executionCount(cell)
	output := outputFromKernelResult(result, err, count)
	s.persistCellOutput(ctx, cell.ID, output, count)

	if sessionID != nil {
		s.updateSessionStatus(ctx, *sessionID, "idle")
	}
	return output, count
}

func (s *State) executeKernel(ctx context.Context, notebookID uuid.UUID, cell models.Cell, sessionID *uuid.UUID, claims *authmw.Claims) (*pythonsidecar.NotebookCellResult, error) {
	switch strings.ToLower(cell.Kernel) {
	case "python":
		if s.PythonKernel == nil {
			return nil, errors.New("python kernel sidecar is not configured")
		}
		sid := uuid.Nil
		if sessionID != nil {
			sid = *sessionID
			if err := s.PythonKernel.EnsureSession(ctx, sid); err != nil {
				return nil, err
			}
		}
		dataDir := ""
		if s.Cfg != nil {
			dataDir = s.Cfg.DataDir
		}
		workspaceDir := environment.WorkspaceRoot(dataDir, notebookID)
		if err := environment.EnsureSeed(dataDir, notebookID); err != nil {
			return nil, err
		}
		return s.PythonKernel.ExecuteCell(ctx, sid, notebookID, cell.Source, workspaceDir, s.notebookCellTimeoutSeconds())
	case "sql":
		return s.sqlKernel().ExecuteSQL(ctx, claims, cell.Source)
	case "r":
		dataDir := ""
		if s.Cfg != nil {
			dataDir = s.Cfg.DataDir
		}
		workspaceDir := environment.WorkspaceRoot(dataDir, notebookID)
		if err := environment.EnsureSeed(dataDir, notebookID); err != nil {
			return nil, err
		}
		return s.rKernel().ExecuteR(ctx, cell.Source, workspaceDir)
	case "llm":
		dataDir := ""
		if s.Cfg != nil {
			dataDir = s.Cfg.DataDir
		}
		workspaceDir := environment.WorkspaceRoot(dataDir, notebookID)
		_ = environment.EnsureSeed(dataDir, notebookID)
		workspaceFiles, _ := environment.ListWorkspaceFiles(dataDir, notebookID)
		return s.llmKernel().ExecuteLLM(ctx, sessionID, notebookID, cell.Source, workspaceDir, workspaceFiles, claims)
	default:
		return nil, fmt.Errorf("unsupported kernel: %s", cell.Kernel)
	}
}

func (s *State) sqlKernel() NotebookSQLKernel {
	if s.SQLKernel != nil {
		return s.SQLKernel
	}
	queryURL := ""
	jwtSecret := ""
	if s.Cfg != nil {
		queryURL = s.Cfg.QueryServiceURL
		jwtSecret = s.Cfg.JWTSecret
	}
	return &httpSQLKernel{Client: http.DefaultClient, QueryServiceURL: queryURL, JWTConfig: authmw.NewJWTConfig(jwtSecret)}
}

func (s *State) rKernel() NotebookRKernel {
	if s.RKernel != nil {
		return s.RKernel
	}
	return rscriptKernel{}
}

func (s *State) llmKernel() NotebookLLMKernel {
	if s.LLMKernel != nil {
		return s.LLMKernel
	}
	aiURL := ""
	jwtSecret := ""
	if s.Cfg != nil {
		aiURL = s.Cfg.AIServiceURL
		jwtSecret = s.Cfg.JWTSecret
	}
	s.LLMKernel = &httpLLMKernel{Client: http.DefaultClient, AIServiceURL: aiURL, JWTConfig: authmw.NewJWTConfig(jwtSecret)}
	return s.LLMKernel
}

func (s *State) errorOutput(cell models.Cell, message string) (models.CellOutput, int32) {
	count := executionCount(cell)
	content, _ := json.Marshal(map[string]string{"error": message})
	output := models.CellOutput{OutputType: "error", Content: content, ExecutionCount: count}
	s.persistCellOutput(context.Background(), cell.ID, output, count)
	return output, count
}

func outputFromKernelResult(result *pythonsidecar.NotebookCellResult, err error, count int32) models.CellOutput {
	if err != nil {
		content, _ := json.Marshal(map[string]string{"error": err.Error()})
		return models.CellOutput{OutputType: "error", Content: content, ExecutionCount: count}
	}
	outputType := result.OutputType
	if outputType == "" {
		outputType = "text"
	}
	content := json.RawMessage(result.ContentJSON)
	if !json.Valid(content) {
		content, _ = json.Marshal(string(result.ContentJSON))
	}
	if len(content) == 0 {
		content, _ = json.Marshal("")
	}
	return models.CellOutput{OutputType: outputType, Content: content, ExecutionCount: count}
}

func executionCount(cell models.Cell) int32 {
	if cell.ExecutionCount == nil {
		return 1
	}
	return *cell.ExecutionCount + 1
}

func (s *State) loadCell(ctx context.Context, cellID uuid.UUID) (models.Cell, bool, error) {
	if s.Pool == nil {
		if s.MemoryRepo == nil {
			return models.Cell{}, false, nil
		}
		c, ok := s.MemoryRepo.loadCell(cellID)
		return c, ok, nil
	}
	row := s.Pool.QueryRow(ctx, `
        SELECT id, notebook_id, cell_type, kernel, source, position,
               last_output, execution_count, created_at, updated_at
        FROM cells WHERE id = $1`, cellID)
	cell, err := scanCell(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Cell{}, false, nil
	}
	return cell, err == nil, err
}

func (s *State) loadCodeCells(ctx context.Context, notebookID uuid.UUID) ([]models.Cell, error) {
	if s.Pool == nil {
		if s.MemoryRepo == nil {
			return []models.Cell{}, nil
		}
		return s.MemoryRepo.loadCodeCells(notebookID), nil
	}
	rows, err := s.Pool.Query(ctx, `
        SELECT id, notebook_id, cell_type, kernel, source, position,
               last_output, execution_count, created_at, updated_at
        FROM cells WHERE notebook_id = $1 AND cell_type = 'code' ORDER BY position ASC`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cells := []models.Cell{}
	for rows.Next() {
		cell, err := scanCell(rows)
		if err != nil {
			return nil, err
		}
		cells = append(cells, cell)
	}
	return cells, rows.Err()
}

func (s *State) loadSession(ctx context.Context, sessionID uuid.UUID) (models.Session, bool, error) {
	if s.Pool == nil {
		if s.MemoryRepo == nil {
			return models.Session{}, false, nil
		}
		sess, ok := s.MemoryRepo.loadSession(sessionID)
		return sess, ok, nil
	}
	row := s.Pool.QueryRow(ctx, `
        SELECT id, notebook_id, kernel, status, started_by, created_at, last_activity
        FROM sessions WHERE id = $1`, sessionID)
	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Session{}, false, nil
	}
	return sess, err == nil, err
}

func (s *State) updateSessionStatus(ctx context.Context, sessionID uuid.UUID, status string) {
	if s.Pool == nil {
		if s.MemoryRepo != nil {
			s.MemoryRepo.updateSessionStatus(sessionID, status)
		}
		return
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE sessions SET status = $2, last_activity = NOW() WHERE id = $1`, sessionID, status)
}

func (s *State) persistCellOutput(ctx context.Context, cellID uuid.UUID, output models.CellOutput, count int32) {
	if s.Pool == nil {
		if s.MemoryRepo != nil {
			s.MemoryRepo.persistOutput(cellID, output, count)
		}
		return
	}
	raw, _ := json.Marshal(output)
	_, _ = s.Pool.Exec(ctx,
		`UPDATE cells SET last_output = $2, execution_count = $3, updated_at = NOW() WHERE id = $1`,
		cellID, raw, count)
}

func (s *State) notebookCellTimeoutSeconds() uint32 {
	if s.Cfg != nil && s.Cfg.PythonSidecarTimeoutSeconds != 0 {
		return s.Cfg.PythonSidecarTimeoutSeconds
	}
	return defaultNotebookCellTimeoutSeconds
}

func sortNotebooksByUpdatedAt(notebooks []models.Notebook) {
	for i := 1; i < len(notebooks); i++ {
		j := i
		for j > 0 && notebooks[j-1].UpdatedAt.Before(notebooks[j].UpdatedAt) {
			notebooks[j-1], notebooks[j] = notebooks[j], notebooks[j-1]
			j--
		}
	}
}

func sortSessionsByCreatedAt(sessions []models.Session) {
	for i := 1; i < len(sessions); i++ {
		j := i
		for j > 0 && sessions[j-1].CreatedAt.Before(sessions[j].CreatedAt) {
			sessions[j-1], sessions[j] = sessions[j], sessions[j-1]
			j--
		}
	}
}

func sortCellsByPosition(cells []models.Cell) {
	for i := 1; i < len(cells); i++ {
		j := i
		for j > 0 && cells[j-1].Position > cells[j].Position {
			cells[j-1], cells[j] = cells[j], cells[j-1]
			j--
		}
	}
}

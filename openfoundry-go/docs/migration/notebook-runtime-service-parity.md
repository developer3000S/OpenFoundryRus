# Notebook runtime service parity (Go)

Date: 2026-05-07

## Scope

This note reconciles the Go `notebook-runtime-service` against the Rust source
under `services/notebook-runtime-service/**` for the notebook surfaces requested
in this pass: notebook CRUD, cell CRUD, session CRUD, workspace files, notepad
export, notepad presence, and Python sidecar configuration.

## Current state

| Surface | Go state | Test coverage |
|---|---|---|
| Notebook CRUD | Production path is pgx-backed. No-DB operation is only allowed when `NOTEBOOK_RUNTIME_SMOKE_MODE=true`, where an in-memory repository returns concrete notebook resources. | `TestNotebookCellSessionSmokeCRUDRoundTrip`, `TestListNotebooksRequiresDatabaseOutsideSmokeMode` |
| Cell CRUD | Production path is pgx-backed. Explicit smoke mode persists cells in memory and returns concrete resources. | `TestNotebookCellSessionSmokeCRUDRoundTrip`, `TestAddCellNoDBDefaultsKernelAndType`, `TestUpdateCellNoDBReturns404` |
| Session CRUD | Production path is pgx-backed. Explicit smoke mode persists create/list/stop lifecycle in memory. | `TestNotebookCellSessionSmokeCRUDRoundTrip`, `TestCreateSessionNoDBReturnsIdleRow`, `TestListSessionsNoDB`, `TestStopSessionNoDBReturns404` |
| Workspace files | File CRUD is ported through `internal/domain/environment`; paths are normalized and traversal/absolute path escapes are rejected. | `internal/domain/environment` tests |
| Notepad document CRUD | Repository-backed through Postgres with in-memory fallback. | `TestNotepadDocumentCRUDComplete`, owner/auth tests |
| Notepad presence | Repository-backed through Postgres with in-memory fallback. | `TestNotepadPresenceUpsertAndList` |
| Notepad export | Persisted document export uses the Go notepad renderer and returns an HTML payload. | `TestExportDocumentUsesPersistedDocument`, `internal/domain/notepad` tests |
| Python sidecar unset | Service starts; Python cell execution returns explicit `python kernel sidecar is not configured` output. | `TestSidecarBinaryUnsetIsSkipped`, handler execution tests |
| Python sidecar set | `libs/python-sidecar.Manager` starts a binary, waits for gRPC health, and drives inline/pipeline/notebook/session calls. A fake sidecar binary validates the configured path without requiring `openfoundry-pyruntime` in CI. | `TestSidecarEndToEndWithFakeBinary`; real sidecar smoke remains opt-in via `PYTHON_SIDECAR_BINARY` |

## Remaining non-goals in this pass

- No pipeline sidecar work was implemented.
- No ontology sidecar work was implemented.
- SQL/R/LLM kernel adapters were not expanded beyond the existing Go adapters.

## Empty-envelope policy

Notebook, cell, and session routes no longer use implicit empty-envelope behavior
when `DATABASE_URL` is absent. Empty/no-data responses are only valid when they
represent the concrete state of the explicit smoke-mode in-memory repository.
With smoke mode disabled and no database, CRUD routes return `503` with a
configuration error.

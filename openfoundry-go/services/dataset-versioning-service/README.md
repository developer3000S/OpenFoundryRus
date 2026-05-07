# dataset-versioning-service (Go)

This Go service is the incremental port of the Rust `dataset-versioning-service`.

## Port status

Implemented end-to-end verticals:

- Dataset CRUD (`/api/v1/datasets`).
- Dataset versions (`/api/v1/datasets/{id}/versions`).
- Dataset branches (`/api/v1/datasets/{id}/branches`).
- Files API + backing filesystem presigning:
  - `GET /api/v1/datasets/{id}/files` lists the persisted logical path to physical URI mappings from `dataset_files`, with branch cutoff and prefix filtering.
  - `GET /api/v1/datasets/{id}/files/{file_id}/download` returns a temporary backing-filesystem URL for active files and rejects soft-deleted files with `410 Gone`.
  - `POST /api/v1/datasets/{id}/transactions/{txn}/files` validates an OPEN transaction and returns a transaction-scoped presigned upload URL plus the canonical `physical_uri`.
  - The default production wiring uses the local backing filesystem presigner in `libs/storage-abstraction`; set `DATASET_FILES_BASE_URL` and `DATASET_FILES_BASE_DIR` to control generated URLs.

Still pending after this slice:

- Dataset quality persistence/API.
- Views API and view-specific preview/schema endpoints.
- Retention worker physical purge loop.
- Full Iceberg writer/catalog integration beyond local backing-file presigning.

# proto/

Reserved for future Go-only proto extensions (none today).

The canonical `.proto` source of truth lives at `../proto/` and is
consumed directly by `buf.gen.yaml` (`inputs: - directory: ../proto`).
Generated Go code lands in `libs/proto-gen/`.

Do **not** copy proto files here — keep one source of truth and
regenerate via `make gen`.

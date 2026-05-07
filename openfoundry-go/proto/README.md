# proto/

Reserved for future Go-only proto extensions (none today).

The canonical `.proto` source of truth lives at `../proto/` and is
consumed directly by `buf.gen.yaml` (`inputs: - directory: ../proto`).
Generated Go code lands in `libs/proto-gen/`.

Do **not** copy proto files here — keep one source of truth.

## Regenerating Go protobuf code

Generated protobuf and gRPC files under `libs/proto-gen/` are committed so
fresh checkouts and CI can run `go test ./...` without first producing local
artifacts.

Regenerate after editing files in `../proto/`:

```sh
cd openfoundry-go
make tools      # first time, installs buf/sqlc/etc. into ./bin
make gen-proto  # or make gen to also run sqlc
```

The Makefile prepends `./bin` to `PATH`, so CI can use the same workflow:

```sh
cd openfoundry-go
make tools
make gen
# optional guard for CI: fail if generation changed tracked files
git diff --exit-code -- libs/proto-gen
```

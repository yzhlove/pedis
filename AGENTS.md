# Repository Guidelines

## Project Structure & Module Organization
- `cmd/pedis/`: application entry point (`main.go`), DI wiring only.
- `internal/`: core packages (`cipher`, `codec`, `conn`, `config`, `resp`, `service`, `text`, etc.). Keep business logic here.
- `internal/service/client/`: client state machine and bridge workers.
- `proto/`: protobuf source (`msg.proto`); generated code in `proto/pb/`.
- Root config/build files: `config.json`, `Makefile`, `go.mod`.

Use `internal/*` for new implementation packages; avoid adding non-entrypoint logic under `cmd/`.

## Build, Test, and Development Commands
- `go build ./...`: compile all packages.
- `go test ./...`: run full test suite.
- `go test ./internal/resp/...`: run tests for a single package while iterating.
- `go run cmd/pedis/main.go`: start the service locally.
- `make proto`: regenerate protobuf stubs after editing `proto/msg.proto`.

If installed, run `golangci-lint run` before opening a PR.

## Coding Style & Naming Conventions
- Follow standard Go formatting (`gofmt`) and idioms; tabs are used by Go formatters.
- Package names are short, lowercase (`resp`, `codec`, `conn`).
- Exported identifiers use `PascalCase`; unexported use `camelCase`.
- Do not hand-edit generated files in `proto/pb/*.pb.go`.
- Prefer repository logging wrapper (`internal/log`) over ad-hoc prints.
- Error strings should be lowercase and without trailing punctuation.

## Testing Guidelines
- Place tests next to code as `*_test.go` (current pattern: `internal/*/*_test.go`).
- Prefer table-driven tests for protocol/codec parsing paths.
- Cover happy path + malformed input path for parsers and wire codecs.
- Run `go test ./...` before commits; add focused package tests for touched modules.

## Commit & Pull Request Guidelines
- Follow Conventional Commit style seen in history, e.g.:
  - `feat(client): implement codec manager`
  - `refactor(cipher): simplify session init`
  - `docs(readme): update module notes`
- Keep commits scoped to one concern.
- PRs should include: purpose, key changes, test commands run, and config/proto impact.
- If `proto/msg.proto` changes, include regenerated `proto/pb/msg.pb.go` in the same PR.

## Security & Configuration Tips
- Runtime config loads from `config.json` with `PEDIS_*` env overrides.
- Never commit real secrets/keys; use local-only values for `server_private_key` and related fields.

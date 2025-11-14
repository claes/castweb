## General requirements

The entire web interface should be keyboard navigatable. 

It should use htmx web framework. 
 

# Repository Guidelines

## Project Structure & Module Organization
- Web service module: `github.com/claes/ytplv`.
- Layout:
  - `cmd/server/main.go` – HTTP entrypoint.
  - `internal/http/` – handlers, middleware, routing.
  - `internal/service/` – business logic; pure where possible.
  - `internal/store/` – persistence (DB, external APIs).
  - `pkg/` – reusable public packages (if any).
  - `api/` – OpenAPI/specs; `testdata/` for fixtures; `flake.nix` for Nix builds.

## Build, Test, and Development Commands
- Run locally: `PORT=8080 go run ./cmd/server` (example: `curl :8080/health`).
- Build binary: `go build -o bin/server ./cmd/server`.
- Always run tests: `go test ./...` (required for every PR).
- Extras: `go test -race ./...`, `go vet ./...`, `gofmt -s -w .`.
- Nix dev shell: `nix develop` (pins Go/tooling). Build with `nix build` → `result/bin/server`.
- Nix checks: `nix flake check` should pass before merging.

## Coding Style & Naming Conventions
- Follow Effective Go; format with `gofmt`. Keep packages short, lower-case (`http`, `service`).
- Files `snake_case.go`; tests `*_test.go`. Exported names use `CamelCase` and have doc comments.
- Errors: wrap with context `fmt.Errorf("...: %w", err)`; don’t log-and-return.
- HTTP: use `context.Context`, set timeouts, validate inputs, return proper status codes.

## Testing Guidelines
- Use the standard `testing` package with table-driven tests and `net/http/httptest`.
- Keep unit tests deterministic; place fixtures under `testdata/`.
- Mark external/integration tests with `//go:build integration`; default CI runs `go test ./...` only.
- Aim for meaningful coverage on handlers, services, and error paths.

## Commit & Pull Request Guidelines
- Commits: imperative and scoped (e.g., `http: add health endpoint`) or Conventional Commits.
- PRs must: pass `go test ./...` and `nix flake check`, describe changes and risks, link issues, and include endpoint examples (e.g., `curl :8080/health`).

## Nix Notes
- Keep `flake.nix` at the repo root with `packages.default` (server build), `devShells.default` (Go + tools), and `checks` (tests/linters).
- Update `flake.lock` when changing dependencies; don’t commit secrets.

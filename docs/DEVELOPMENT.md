# Development Guide

Everything you need to start working on DiscoPanel.

## Prerequisites

- **Go** 1.24+ (backend)
- **Node.js** 20+ with npm (frontend, `web/discopanel`)
- **buf** CLI ([buf.build](https://buf.build)) — regenerates protobuf/Connect code. If you
  don't have it locally: `go install github.com/bufbuild/buf/cmd/buf@latest` or run it via
  Docker (`make proto`).
- **Docker** — only needed to actually run servers/modules, not for building.

## Getting started

```bash
# 1. Backend
go build ./...     # generated proto code is committed; no generation step required
go test ./...

# 2. Frontend
cd web/discopanel
npm ci
npm run dev        # SvelteKit dev server
npm run build      # production build
npm run check      # svelte-check type checking

# 3. Run the panel locally
go run ./cmd/discopanel --config config.yaml
```

## Proto workflow

The ConnectRPC API is defined in `proto/discopanel/v1/*.proto`. Generated code lives in:

- `pkg/proto/...` (Go)
- `web/discopanel/src/lib/proto/...` (TypeScript)

**The generated code is committed** so a fresh clone builds immediately. If you change a
`.proto` file, regenerate and commit the result:

```bash
buf generate          # or: make proto (uses Docker)
go build ./...        # the build breaks until new service methods are implemented
cd web/discopanel && npm run check
```

Checklist when adding an RPC:

1. Define the RPC + request/response messages in the relevant `.proto`.
2. `buf generate`.
3. Implement the handler in `internal/rpc/services/` (there is a compile-time
   `var _ ...Handler = (*Service)(nil)` check that will fail the build otherwise).
4. Add the procedure to `internal/rbac/mapping.go` (`ProcedurePermissions`) — procedures
   without a mapping are denied for non-admin roles. Use `ObjectIDField` for per-server
   scoping and `ScopedList: true` for collection endpoints that must filter results for
   scoped users.
5. Regenerate + commit.

## Project layout

```
cmd/discopanel       panel entrypoint and startup wiring (proxy, scheduler, event bus)
cmd/status           built-in status page module
internal/config      YAML configuration (config.example.yaml documents every option)
internal/db          SQLite store, models (GORM), migrations are auto-run
internal/docker      Docker container lifecycle + env building from ServerConfig
internal/proxy       hostname-routing Minecraft/TCP/UDP/HTTP proxies
internal/rpc         ConnectRPC server, auth interceptor, RBAC, HTTP handlers
internal/rpc/services  one file per service (server.go, task.go, backups.go, ...)
internal/scheduler   cron/interval/event task scheduler + backup executor
internal/module      module system (builtin templates in builtin_templates.go)
internal/rbac        Casbin enforcer + procedure permission mapping
pkg/files            safe filesystem helpers (zip extract, disk space, dir size)
pkg/upload           chunked upload sessions (used by modpack import, server import)
web/discopanel       SvelteKit frontend
```

## Conventions

- Backend tests live next to the code (`*_test.go`); run `go test ./...` before pushing.
- Frontend components use Svelte 5 runes (`$state`, `$derived`, `$effect`); call RPCs via
  `rpcClient.<service>.<method>` from `$lib/api/rpc-client` and build requests with
  `create(<Request>Schema, {...})`.
- Docker overrides applied to servers: `internal/docker/client.go` (`ApplyOverrides`,
  `buildEnvFromConfig` — env vars come from struct `env:` tags on `ServerConfig`).
- Config categories in the server settings UI are derived from `ServerConfig` struct tags
  plus `getCategoryIndex` in `internal/rpc/services/config.go` — add new fields there too.
- Backups are written to `storage.backup_dir/<server-dir-basename>/<name>_<timestamp>.zip`;
  the backup management RPCs live in `internal/rpc/services/backups.go`.

## Security notes

Never commit credentials. `scripts/deploy-proxmox.ps1` currently contains example
credentials — treat any real values as secrets and pass them via environment variables
instead.

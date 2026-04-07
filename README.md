# Supabase CLI (Fork)

Fork of the [Supabase CLI](https://github.com/supabase/cli) with a full native sandbox mode, dependency optimizations, and targeted bug fixes.

## Fork Improvements

### Full Native Sandbox (`--sandbox`)

Expands the upstream `--sandbox` mode from 4 services to **all 8 core services** — no Docker Compose at runtime.

```bash
supabase start --sandbox
```

| Service | Binary Source | Description |
|---------|-------------|-------------|
| **postgres** | GitHub release | PostgreSQL with Supabase extensions |
| **auth** (GoTrue) | GitHub release | Authentication and JWT management |
| **postgrest** | GitHub release | Auto-generated REST API |
| **realtime** | Docker extraction | WebSocket-based real-time subscriptions |
| **logflare** | Docker extraction | Analytics and log aggregation |
| **storage-api** | Docker extraction | S3-compatible file storage |
| **postgres-meta** | Docker extraction | Database introspection API |
| **studio** | Docker extraction | Web-based admin dashboard |

#### Built-in Reverse Proxy

All traffic routes through a single API URL with automatic `apikey` header to JWT transformation for publishable keys. Realtime WebSocket connections pass through without transformation.

#### Process-Compose Orchestration

Services are managed by [process-compose](https://github.com/F1bonacc1/process-compose) with:
- Dynamic port allocation avoiding conflicts
- Health checks and dependency ordering
- Hot-reload via `--reload` flag
- Full REST API for service introspection

#### Docker Extraction (Build-Time Only)

Docker is used once to extract service binaries (`docker create` + `docker cp`), then never again at runtime. Image tags are read from `pkg/config/templates/Dockerfile` via `config.Images`, staying in sync with upstream automatically. Binaries are cached at `~/.supabase/bin/` and reused across projects.

#### Sandbox Commands

```bash
supabase start --sandbox          # Start all services
supabase start --sandbox --reload # Hot-reload config without restart
supabase stop                     # Stop all services
supabase status                   # Show service URLs and ports
supabase logs [service]           # Stream logs (--follow for live streaming)
supabase restart [service]        # Restart a specific service
```

#### Prerequisites

- **Docker** (build-time only — for extracting service binaries)
- **Node.js** (for storage, postgres-meta, and studio)
- **Linux** (Docker-extracted services contain Linux binaries)

### Live Log Streaming (`supabase logs`)

New `supabase logs` command with `--follow` flag for real-time log streaming via WebSocket. Supports filtering by service name (`db`, `auth`, `rest`, `realtime`, `storage`, `analytics`, `meta`, `studio`).

### Dependency Optimizations

Lightweight stub replacements that eliminate heavy transitive dependencies, significantly reducing build times and binary size:

- **go-ethereum stub** — Replaces the full `go-ethereum` dependency (pulled transitively by `docker/compose/v2`) with a minimal `decred/dcrd/dcrec/secp256k1` stub that satisfies the `crypto/secp256k1` interface.
- **containers/common stub** — Replaces the full `containers/common` package with a minimal `libnetwork/types` stub providing only the type definitions actually used.
- **Direct Docker API** — Replaces `docker/compose/v2` with direct `docker/docker` client API calls, removing the Compose dependency entirely.

### Bug Fixes (Not Yet Upstream)

- **Kong worker exhaustion** — Removed hardcoded `KONG_NGINX_WORKER_PROCESSES=1` that caused Kong to stop responding under load. Kong now auto-detects the appropriate worker count.
- **MCP Kong route matching** — Fixed route path matching in Kong configuration for MCP endpoints.
- **`sslmode=disable` honored** — Database connections now correctly respect `sslmode=disable` in connection strings.
- **Snippets pagination** — `supabase snippets list` now handles paginated responses from the API instead of returning only the first page.
- **Masked secrets input** — `supabase secrets set` supports interactive masked input with proper non-ASCII byte handling.

## Building from Source

```bash
go build -o supabase .
./supabase start --sandbox
```

Requires Go >= 1.22.

## Upstream

This fork tracks the upstream `develop` branch.

- Upstream: https://github.com/supabase/cli
- Docs: https://supabase.com/docs/reference/cli/about

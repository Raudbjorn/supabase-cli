# Supabase CLI (Fork — Full Native Sandbox)

Fork of the [Supabase CLI](https://github.com/supabase/cli) with an expanded `--sandbox` mode that runs **all 8 core services** natively — no Docker Compose at runtime.

## Sandbox Mode

```bash
supabase start --sandbox
```

Starts a full local Supabase stack using native binaries and [process-compose](https://github.com/F1bonacc1/process-compose):

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

Plus a built-in **reverse proxy** that routes all traffic through a single API URL.

### Prerequisites

- **Docker** (build-time only — used to extract service binaries, not at runtime)
- **Node.js** (for storage, postgres-meta, and studio)
- Standard Supabase project with `config.toml`

### How It Works

1. Downloads auth, postgrest, postgres, and process-compose from GitHub releases
2. Extracts realtime, logflare, storage, pgmeta, and studio from official Docker images via `docker create` + `docker cp`
3. Generates a `process-compose.yaml` from your project's `config.toml`
4. Orchestrates all services with dynamic port allocation and health checks

Binaries are cached at `~/.supabase/bin/` and reused across projects.

### Commands

```bash
supabase start --sandbox          # Start all services
supabase start --sandbox --reload # Hot-reload config without restart
supabase stop                     # Stop all services
supabase status                   # Show service URLs and ports
supabase logs [service]           # Stream logs (db, auth, rest, realtime, storage, analytics, meta, studio)
supabase restart [service]        # Restart a specific service
```

### Output

```text
APIs:
  API URL:     http://127.0.0.1:{api}
  REST:        http://127.0.0.1:{api}/rest/v1/
  Auth:        http://127.0.0.1:{api}/auth/v1/
  Realtime:    http://127.0.0.1:{api}/realtime/v1/
  Storage:     http://127.0.0.1:{api}/storage/v1/
  Studio:      http://127.0.0.1:{studio}

Database:
  URL:         postgresql://postgres:postgres@127.0.0.1:{db}/postgres
```

## Building from Source

```bash
go build -o supabase .
./supabase start --sandbox
```

Requires Go >= 1.22.

## Upstream

This fork tracks the upstream `develop` branch. The sandbox expansion lives on the `feature/full-sandbox` branch.

- Upstream: https://github.com/supabase/cli
- Docs: https://supabase.com/docs/reference/cli/about

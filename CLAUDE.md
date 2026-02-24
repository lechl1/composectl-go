# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

dc (`dc`) is a Go HTTP server that provides a REST API and web UI for managing Docker Compose stacks. It handles stack lifecycle (up/down/list/get/put/delete), container inspection, YAML enrichment, JWT authentication, and real-time WebSocket broadcasting.

The Go module name is `dcapi` (not `dc`).

## Build & Development Commands

```bash
make build      # go build -ldflags="-s -w" -o dc
make test       # go test -v ./...
make clean      # remove binary

# Run directly (no install)
go build -o dc && ./dc

# Command-line flags
./dc --stacks-dir=/path/to/stacks --env-path=/path/to/prod.env --port=8882 --addr=0.0.0.0
```

## Service Management (systemd user service)

```bash
make install    # build + install to ~/.local/bin + create systemd user service
make update     # rebuild + restart service in place
make start / stop / restart / status / logs
make setup-auth # interactive credential setup into ~/.local/containers/prod.env
```

## Architecture

All logic lives in a single `main` package with no subdirectories. Files map to concerns:

| File | Responsibility |
|------|----------------|
| `main.go` | Route registration, server startup |
| `config.go` | `InitPaths`, credential loading with 6-level priority cascade |
| `auth.go` | JWT generation/validation, in-memory `SessionStore`, login/logout handlers |
| `server.go` | `HandleRoot` (template serving), `handleStackAPI` router, script execution |
| `stack.go` | All Docker Compose operations: list, get, put, delete, start, stop, enrich YAML |
| `container.go` | `docker inspect` wrapper, container start/stop/delete handlers |
| `websocket.go` | WebSocket upgrade, client map (mutex-protected), broadcast channel |
| `watch.go` | `fsnotify`-based recursive file watching |
| `thumbnail.go` | Docker Hub thumbnail scraping with local cache |
| `yaml.go` | YAML encoding helpers: multiline strings, sorted map keys |

## Key Patterns

### Configuration Cascade (config.go)
`GetConfig(key, args, default)` resolves values in this priority order:
1. `--key=value` CLI arg
2. `KEY_FILE` env var (path to file containing the value)
3. `KEY` env var directly
4. `prod.env` file (key=value, case-insensitive)
5. `/run/secrets/KEY` Docker secrets directory
6. Default value

Conflicts between sources 4 and 5 panic at startup. `prod.env` lives at `~/.local/containers/prod.env` (chmod 600, auto-created).

### Authentication
- `POST /api/auth/login` accepts HTTP Basic Auth → returns JWT Bearer token
- All other endpoints wrapped with `BasicAuthMiddleware` which validates Bearer tokens
- Sessions stored in `SessionStore` (`sync.RWMutex`-protected map); expiration extended 12h on each request
- Background `SessionCleanup()` goroutine runs every hour

### YAML Enrichment Pipeline (stack.go)
`HandleEnrichYAML` / `enrichStack` transforms user YAML:
1. Auto-set `container_name` if missing
2. Add resource defaults (CPU/memory limits)
3. Attach `homelab` network to all services
4. Auto-detect HTTP ports and inject Traefik routing labels
5. Write result to `{name}.effective.yml` alongside the source

### Working Directory Layout
```
~/.local/containers/
├── prod.env          # Credentials: ADMIN_USERNAME, ADMIN_PASSWORD, SECRET_KEY
├── stacks/           # User's Docker Compose YAMLs
│   ├── app.yml
│   └── app.effective.yml  # Auto-generated enriched version
└── thumbnails/       # Cached Docker Hub images
```

### Concurrency
- `SessionStore`: `sync.RWMutex`
- WebSocket clients map: `sync.Mutex`
- Real-time updates via unbuffered broadcast channel → all connected WS clients
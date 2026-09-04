# TiBrain Technical Documentation

Technical specifications, API endpoints, configuration details, and development workflows for TiBrain.

---

## 1. Architecture Overview

### System Architecture

```
CLI / Agent ──► TiRouter :3004 ──► CLIProxyAPI :3004 ──► Provider executors
                       │
                       ▼
                      ┌──────────────────────────────────────┐
       HTTP :3005 ───►│            TiBrain Server            │
                      │  (single-port HTTP, main.go)          │
                      ├──────────────────────────────────────┤
        /api/*  (REST)──►│  REST API layer                       │
        /mcp    (SSE) ───►│  MCP hub (tool registry)              │
        /  (Browser UI)──►│  Web UI                               │
        /health /ready ───►│  Health & readiness                   │
                      ├──────────────────────────────────────┤
                      │  Core subsystems:                     │
                      │   • CLI registry & handoff            │
                      │   • Agent orchestration               │
                      │   • Cognitive memory                  │
                      │     (episodic/semantic/procedural)    │
                      │   • RAG (vector store, reranker,      │
                      │     embeddings, adaptive retrieval)   │
                      │   • Knowledge indexing                │
                      │   • Learning / quality (BEADS LEARN)  │
                      │   • Cloudflare integration, RTK       │
                      └──────────────────────────────────────┘
                                       │            │
                                       ▼            ▼
                                  Redis (cache,   Vector store /
                                   optional)      knowledge index
```

### Core Subsystems

| Subsystem | Description | Package |
|-----------|-------------|---------|
| CLI Registry & Handoff | Register and manage CLI instances, handle handoff between agents | `internal/cli/` |
| Agent Orchestration | Multi-agent coordination, dispatch, monitoring | `internal/orchestration/` |
| Cognitive Memory | Episodic, semantic, procedural memory systems | `internal/memory/` |
| RAG | Vector store, reranker, embeddings, adaptive retrieval | `internal/rag/` |
| Knowledge Indexing | Document ingestion, indexing, search | `internal/knowledge/` |
| Learning / Quality | BEADS LEARN model for continuous improvement | `internal/learning/` |
| Cloudflare Integration | Tunnel management, RTK integration | `internal/cloudflare/` |

---

## 2. API Endpoints

### Health & Metrics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check (`{"status":"ok"}`) |
| GET | `/metrics` | Prometheus metrics |
| GET | `/ready` | Readiness probe |

### MCP Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/mcp/protocol` | Protocol version & capabilities |
| POST | `/mcp` | Streamable HTTP MCP endpoint |
| GET | `/mcp/sse` | SSE MCP endpoint |
| POST | `/mcp/message` | Legacy message endpoint |

### REST API - Prompts

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v2/runtime/prompts` | Prompt runtime presets/config JSON |
| POST | `/api/v1/prompt/preflight` | Prompt validation (preflight) |
| POST | `/api/v1/prompt/feedback` | Prompt feedback collection |
| GET | `/api/prompts` | List prompt capsules |
| GET | `/api/prompts/metrics` | Prompt statistics |
| GET | `/api/prompts/{id}` | Prompt detail |
| GET | `/api/prompts/{id}/versions` | Prompt versions |
| POST | `/api/prompts/{id}/canary` | Deploy canary prompt |
| POST | `/api/prompts/{id}/promote` | Promote canary to production |
| POST | `/api/prompts/{id}/rollback` | Rollback to draft |
| POST | `/api/prompts/observe` | Observe prompt decision |
| POST | `/api/prompts/{id}/approve` | Approve ingested prompt |
| POST | `/api/prompts/{id}/reject` | Reject ingested prompt |

### Secret Vault

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/secrets/upsert` | Upsert secret (requires bearer `TIBRAIN_MCP_BEARER_TOKEN`) |
| GET | `/v1/secrets/resolve` | Resolve secret (requires bearer `TIBRAIN_MCP_BEARER_TOKEN`) |

### Deprecated Endpoints (404)

The following endpoints are deprecated and return 404:
- `/api/status`
- `/api/tools`
- `/api/agents`
- `/api/knowledge`
- `/api/rag/query`
- `/api/v2/retrieve`
- `/api/v2/runtime/registry`

> **Note:** All inter-component communication must go through **TiRouter Gateway** at port `:3004`. Do not connect directly to OpenClaw Gateway (`:1807`) or Router Agent (`:1806`).

---

## 3. MCP Tools

### Filesystem Tools (fs.*)

| Tool | Description |
|------|-------------|
| `fs.read_file` | Read file content |
| `fs.write_file` | Write file content |
| `fs.list_files` | List directory contents |
| `fs.delete_file` | Delete file |
| `fs.file_info` | Get file metadata |

### Shell Tools (shell.*)

| Tool | Description |
|------|-------------|
| `shell.exec` | Execute shell command |
| `shell.read_output` | Read process output |
| `shell.kill` | Kill process |

### Process Tools (process.*)

| Tool | Description |
|------|-------------|
| `process.start` | Start new process |
| `process.list` | List running processes |
| `process.kill` | Kill process by PID |

### Git Tools (git.*)

| Tool | Description |
|------|-------------|
| `git.status` | Git status |
| `git.diff` | Git diff |
| `git.log` | Git log |
| `git.commit` | Git commit |

### HTTP Tools (http.*)

| Tool | Description |
|------|-------------|
| `http.get` | HTTP GET request |
| `http.post` | HTTP POST request |
| `http.put` | HTTP PUT request |
| `http.delete` | HTTP DELETE request |

### Database Tools (db.*)

| Tool | Description |
|------|-------------|
| `db.query` | Execute SQL query |
| `db.execute` | Execute SQL statement |
| `db.migrate` | Run migrations |

### Browser Tools (browser.*)

| Tool | Description |
|------|-------------|
| `browser.navigate` | Navigate to URL |
| `browser.snapshot` | Take accessibility snapshot |
| `browser.click` | Click element |
| `browser.fill` | Fill form field |

### Workflow Tools (workflow.*)

| Tool | Description |
|------|-------------|
| `workflow.run` | Execute workflow |
| `workflow.status` | Get workflow status |

---

## 4. Configuration (config.yaml)

### Tibrain Section

```yaml
tibrain:
  port: 3005                    # MCP Hub port
  data_dir: "Z:\03_DATA\tibrain-database"
```

### Server Section

```yaml
server:
  host: "0.0.0.0"
  port: 3005
  public_base_url: "http://localhost:3005"
```

### MCP Section

```yaml
mcp:
  public_name: "tibrain"
  public_mode: "sse"
  expose_upstream_tools: true
  compatibility: true
  streamable_http_path: "/mcp"
  legacy_sse_path: "/mcp/sse"
  legacy_message_path: "/mcp/message"
  session_ttl: "30m"
  max_concurrent_calls_per_session: 4
  max_request_bytes: 1048576
  max_output_bytes: 4194304
  servers:
    - name: "android-agent"
      transport: "stdio"
      command: "python"
      args: ["-m", "mcp_android"]
      enabled: true
```

### Auth Section

```yaml
auth:
  mode: "bearer"
  bearer_token_env: "TIBRAIN_MCP_BEARER_TOKEN"
  allowed_origins:
    - "http://localhost:3005"
    - "http://127.0.0.1:*"
  rate_limit_per_minute: 60
```

### Permissions Section

```yaml
permissions:
  active_profile: "operator"
  trusted_full:
    enabled: false
```

### Audit Section

```yaml
audit:
  enabled: true
  path: ".runtime/logs/audit.jsonl"
  redact_secrets: true
```

### Allowed Roots

```yaml
allowed_roots:
  - "$HOME/.config/tibrain"
  - "$PWD"
```

### Environment Variable Overrides

All parameters can be overridden via environment variables or command-line flags:

```bash
# Override port
tibrain --port 8080

# Override config file
tibrain --config ./custom-config.yaml

# Override data dir
TIBRAIN_DATA_DIR=Z:\data tibrain
```

---

## 5. Build & Run

### Build Commands

```powershell
# Standard build
go build -o tibrain.exe .

# Optimized build
go build -ldflags="-s -w" -o tibrain.exe .

# Debug build (no optimizations)
go build -o tibrain.exe .

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o tibrain-linux .

# Using Makefile
make build            # Build with optimizations
make build-debug      # Build without optimizations
make build-all        # Build for linux, windows, darwin
```

### Run Commands

```powershell
# Direct run (default port 3005)
.\tibrain.exe

# Custom port
.\tibrain.exe --port 8080

# Custom host
.\tibrain.exe --host 0.0.0.0 --port 3005

# With tunnel (Windows)
.\start-tunnel.bat

# With tunnel (Linux/macOS)
./tibrain & cloudflared tunnel run tibrain
```

### Using Scripts

```powershell
# Windows scripts
.\build.ps1              # Build
.\start-tibrain.ps1      # Start with tunnel
.\restart-tunnel.bat     # Restart
.\stop-tunnel.bat        # Stop

# Makefile targets
make run                # Run server
make start              # Run with tunnel
make test               # Run all tests
make verify             # vet + lint + test-short
make clean              # Clean build artifacts
```

---

## 6. Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3005` | Server port |
| `TIBRAIN_DATA_DIR` | `Z:\03_DATA\tibrain-database` | SQLite data directory |
| `TIBRAIN_DB_DRIVER` | `sqlite3` | Database driver (sqlite3/postgres) |
| `MCP_SERVER_NAME` | `tibrain` | MCP server name for Claude Code |
| `MCP_TIMEOUT_MS` | `10000` | MCP request timeout (ms) |
| `ECC_HOOK_PROFILE` | `standard` | Hook profile (minimal/standard/strict) |
| `TIBRAIN_MCP_BEARER_TOKEN` | (required) | Bearer token for secret vault |

---

## 7. Development Workflow

### Go Development Workflow

```bash
# Full workflow (review + build + test)
node cli/workflows/go-dev.js --targetDir=.

# Individual steps
node cli/workflows/go-dev.js --step=review --targetDir=.
node cli/workflows/go-dev.js --step=build --targetDir=.
node cli/workflows/go-dev.js --step=test --targetDir=.
```

### Handoff Logging

Each step logs to `.mimocode/handoff/`:
- `plan-{timestamp}.md` — Execution plan
- `review-{timestamp}.md` — Review results (vet, lint, format)
- `build-{timestamp}.md` — Build results
- `test-{timestamp}.md` — Test results

### Auto-skip Logic

- Build skipped if review finds errors
- Test skipped if build fails

### Quality Gates (Pre-commit)

```bash
# Must pass all:
gofmt -w .
go vet ./...
go test ./... -race -count=1
go build ./...
golangci-lint run ./...
```

---

## 8. Project Structure

```
tibrain/
├── main.go                     # Entry point
├── config.yaml                 # Configuration
├── go.mod                      # Go module
├── go.sum                      # Dependencies checksum
├── Makefile                    # Build targets
├── build.ps1                   # PowerShell build script
├── start-tibrain.bat           # Windows start script
├── restart-tibrain.bat         # Windows restart script
├── stop-tibrain.bat            # Windows stop script
├── start-tunnel.bat            # Cloudflare tunnel start
├── restart-tunnel.bat          # Tunnel restart
├── stop-tunnel.bat             # Tunnel stop
├── AGENTS.md                   # Agent registry definitions
├── README.md                   # Project overview
├── tibrain-tech.md             # This file
├── docs/
│   └── TOOLS.md               # MCP tools documentation
├── .claude/
│   ├── settings.json          # Claude Code settings
│   ├── CLAUDE.md              # Project instructions
│   ├── skills/                # Skills
│   ├── hooks/                 # Hook scripts
│   └── worktrees/             # Worktree isolation
├── internal/
│   ├── api/                   # REST API handlers
│   ├── mcp/                   # MCP gateway, registry, tools
│   ├── prompt/                # Prompt intelligence
│   ├── security/              # Auth, guard, audit, secrets
│   ├── db/                    # Database hub, migrations
│   ├── orchestration/         # Agent orchestration
│   ├── memory/                # Memory systems
│   ├── rag/                   # RAG retrieval
│   ├── knowledge/             # Knowledge indexing
│   ├── learning/              # BEADS LEARN
│   ├── cloudflare/            # Cloudflare integration
│   ├── async/                 # Async writer
│   ├── config/                # Config loading
│   ├── execution/             # Execution engine
│   ├── lazyrouter/            # Lazy router
│   ├── notionprovider/        # Notion provider
│   ├── storage/               # TTL cache
│   ├── trace/                 # Tracing
│   └── verification/          # Verification
├── bin/
│   ├── tibrain.exe            # Built binary
│   └── releases/              # Release binaries
└── test/
    └── ...                    # Test files
```

---

## 9. MCP Server Configuration

### Local MCP Servers (settings.json)

```json
{
  "mcpServers": {
    "tibrain": {
      "type": "sse",
      "url": "http://localhost:3005/mcp/sse",
      "description": "TiBrain MCP Hub"
    },
    "tirouter": {
      "type": "http",
      "url": "http://localhost:3004/mcp",
      "description": "TiRouter Gateway"
    }
  }
}
```

### Cloudflare Tunnel

```json
{
  "tunnel": "tibrain",
  "hostname": "tibrain.trepremium.online",
  "target": "http://localhost:3005"
}
```

Config location: `.runtime/config/tunnel_config.json`

---

## 10. Database Schema

### SQLite Schema (default)

```sql
-- Memory tables
CREATE TABLE episodic_memory (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    content TEXT NOT NULL,
    metadata JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tiered_memory (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    pattern TEXT NOT NULL,
    confidence REAL,
    usage_count INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE global_memory (
    id TEXT PRIMARY KEY,
    key TEXT UNIQUE NOT NULL,
    value JSON NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Knowledge tables
CREATE TABLE documents (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    metadata JSON,
    embedding BLOB,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_documents_embedding ON documents(embedding);
```

---

## 11. Contributing

### Development Process

1. **Fork** repository and create feature branch (`git checkout -b feature/name`)
2. Follow **Conventional Commits** for commit messages
3. Write **unit tests** for all new functions (`go test ./...`)
4. Run **lint** (`golangci-lint run`) before PR
5. Ensure **docstrings** and **godoc** are updated
6. Submit PR and wait for maintainer review

### Code Standards

- **Package comments**: Vietnamese for internal packages
- **Function comments**: English for public APIs
- **Error messages**: Vietnamese (user-facing)
- **Log messages**: Vietnamese (internal)
- **Format**: `gofmt -w .`
- **Vet**: `go vet ./...`
- **Tests**: `go test ./... -race -count=1`

---

## 12. License

MIT License — see `LICENSE` file for details.

---

## 13. Related Files

- [`README.md`](./README.md) — Project overview & getting started
- [`AGENTS.md`](./AGENTS.md) — Agent registry definitions
- [`config.yaml`](./config.yaml) — Server configuration
- [`Makefile`](./Makefile) — Build targets
- [`docs/TOOLS.md`](./docs/TOOLS.md) — MCP tools list
- [`internal/`](./internal/) — Source code packages
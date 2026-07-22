# AGENTS.md — TiBrain Central Intelligence Hub

> **Phiên bản:** 2.3.0 | **Cập nhật:** 2026-07-22 | **Công nghệ:** Go 1.25+ | **Cổng:** `3005` (MCP Hub: unified)

---

## 1. Tổng quan & Vai trò

TiBrain là **internal intelligence service** của hệ sinh thái Ti — một HTTP server duy nhất tại port `3005` gồm REST API, MCP (SSE) và Browser UI. TiBrain quản lý knowledge, memory, retrieval, agent orchestration và learning; TiBrain không phải model ingress.

```
CLI / Agent ──► TiRouter front door (:3004) ──► CLIProxyAPI (:3004) ──► Provider executors
                              │
                              ▼
                       TiBrain (:3005)
                       ├─ MCP Hub
                       ├─ SQLite / Vector Store
                       ├─ RAG / Memory / Learning
                       └─ Prompt Intelligence
```

**Vai trò:** Intelligence/Knowledge Plane nội bộ, Agent Orchestrator, MCP Hub và Cognitive Memory. **TiRouter `:3004` là public model ingress; CLIProxyAPI `:3004` là runtime áp dụng policy/prompt bắt buộc.**

---

## 2. Kiến trúc & Thành phần

### 2.1 Single-Port

```
:3005 → main.go
 ├─ /health, /ready          → Health probes
 ├─ /register-cli, ...        → CLI Registry & Handoff
 ├─ /chat                     → Chat / NL
 ├─ /mcp/sse                  → MCP Hub (SSE)
 ├─ /api/*                    → APIServer (RAG, agents, tools, RTK, v1/v2)
 └─ /                         → Browser UI
```

**Luồng dữ liệu:**
```
TiRouter → TiBrain → RAGSystemManager  → Embedding API
                  → PromptIntelligence → Registry / Retrieval / Feedback [TARGET]
                  → MCPHubClient       → MCP Servers (mcp/)
                  → LocalToolExecutor  → Filesystem / Shell
                  → CognitiveMemory    → SQLite / Vector Store
```

### 2.2 Core subsystems

| Subsystem | File | Chức năng |
|---|---|---|
| RAGSystemManager | `rag_system.go` | FTS5 + keyword + vector search |
| AdaptiveRetrievalRuntime | `adaptive_retrieval.go` | Confidence scoring, brain pattern tiers |
| MCPHubClient | `mcp_hub_client.go` | MCP proxy client (integrated with TiBrain) |
| MCPServerManager | `mcp_server.go` | Embedded MCP server - acts as main MCP hub for TiBrain |
| MCPSubServers | `.runtime/config/mcp_config.json` | Sub-MCPs: chrome-devtools-mcp, github-mcp, obsidian-mcp |
| APIServer | `api_server.go` | REST API layer (12 nhóm, ~100 routes) |
| KnowledgeIndexer | `knowledge_indexer.go` | Auto-scan apps/, docs/ |
| CodeGraphService | `code_graph.go` | AST parsing → code graph |
| CloudflareService | `cloudflare_service.go` | Token management (AES-GCM) |
| IntegrationManager | `integration.go` | Agent orchestration, cross-brain |
| TiAgentOrchestrator | `ti_agent.go` | Multi-agent orchestration |
| RetrievalRouter | `retrieval_router.go` | Intelligent retrieval routing |
| LocalToolExecutor | `local_tool_executor.go` | File/command/git/build (sandbox) |
| AutoLearningMechanism | `auto_learning_mechanism.go` | Query patterns, quality scoring |
| CrossReferenceIntelligence | `cross_reference_intelligence.go` | Document linking, version tracking |
| PredictiveMaintenance | `predictive_maintenance.go` | Performance metrics, predictions |
| Prompt Intelligence | `internal/prompt/*` [TARGET] | Registry, preflight, ranking, feedback, evaluation |

### 2.3 MCP Tools

**Registry Tools (mcp_registry_tools.go):**

| Tool | Chức năng |
|---|---|
| `brain_list_clis` | List registered CLIs |
| `brain_register_cli` | Register CLI |
| `brain_unregister_cli` | Unregister CLI |
| `brain_heartbeat_cli` | Send heartbeat |
| `brain_create_handoff` | Create handoff |
| `brain_recall_handoff` | Recall handoff |
| `brain_list_handoffs` | List handoffs |
| `brain_register_mcp` | Register MCP server |
| `brain_list_mcp` | List MCP servers |
| `brain_get_mcp` | Get MCP server details |
| `brain_sync_mcp_tools` | Sync MCP tools |
| `brain_register_tool` | Register tool |
| `brain_list_tools_registry` | List tools |
| `brain_get_tool` | Get tool details |
| `brain_execute_tool` | Execute tool |
| `brain_update_model_stats` | Update BEADS stats |
| `brain_get_model_stats` | Get model stats |
| `brain_get_best_model` | Best model for task |
| `brain_mcp_proxy_servers` | List upstream MCP servers |
| `brain_mcp_proxy_tools` | List upstream MCP tools |
| `brain_mcp_proxy_call` | Call MCP tool |
| `brain_mcp_proxy_batch` | Batch call MCP tools |
| `brain_sync_mcp_hub` | Sync MCP hub |

**Surface Tools (mcp_surface_tools.go):** `brain_search`, `brain_index_knowledge`, `brain_memory_store/query/stats`, `brain_feedback`, Cloudflare management tools

**Ops Tools (tools_ops.go):**

| Tool | Package | Mô tả |
|---|---|---|
| `ops.qualitygate` | `internal/qualitygate` | Chạy pipeline gofmt → vet → build → test |
| `ops.audit` | `internal/audit` | 扫 repo tìm secret, binary artifacts |
| `ops.handoff` | `internal/tracker` | Ghi handoff entry |
| `ops.handoffs` | `internal/tracker` | Đọc recent handoffs |
| `ops.errors` | `internal/tracker` | Đọc recent errors (deduped) |

### 2.4 Sub-MCP Servers

TiBrain là **MCP server chính**, các MCP servers khác đăng ký làm sub-MCP:

| MCP Server | Loại | File cấu hình |
|---|---|---|
| `chrome-devtools-mcp` | External (stdio) | `.runtime/config/mcp_config.json` |
| `github-mcp` | External (stdio) | `.runtime/config/mcp_config.json` |
| `obsidian-mcp-server` | External (stdio) | `.runtime/config/mcp_config.json` |

**Luồng MCP:**
```
Agent/CLI → brain_mcp_proxy_call → TiBrain MCP Hub (port 3005)
                                    ├─► chrome-devtools-mcp (upstream)
                                    ├─► github-mcp (upstream)
                                    └─► obsidian-mcp-server (upstream)
```

---

## 3. Cấu hình

### 3.1 `config.yaml`

```yaml
tibrain:
  port: 3005
  data_dir: "data"
  api_keys: ["test-key-123", "dev-key-456"]

cli_registry: { enabled: true }
handoff_tracking: { enabled: true }
skill_sync: { enabled: false }

mcp:
  public_name: "tibrain"
  public_mode: "sse"
  expose_upstream_tools: true
  compatibility: true
  # Unified mode: TiBrain acts as both control plane AND MCP proxy hub
  # All MCP servers (chrome-devtools-mcp, github-mcp, etc.) register as sub-MCPs
  internal_hub: { enabled: true, url: "http://localhost:3005" }
  targets:
    filesystem: "1mcp-local-bridge"
    fs: "1mcp-local-bridge"
    git: "1mcp-local-bridge"
    sqlite: "1mcp-local-bridge"
    postgres: "1mcp-local-bridge"
    github: "github-mcp"
    obsidian: "obsidian-mcp-server"
    browser: "browser-extension"
    chrome-devtools: "chrome-devtools-mcp"
```

### 3.2 Environment Variables

| Variable | Default | Mô tả |
|---|---|---|
| `TIBRAIN_INDEX_KNOWLEDGE` | `false` | Auto-index at startup |
| `TIBRAIN_ALLOWED_ROOTS` | `""` | Override allowed roots (JSON array) |
| `MCP_HUB_API_KEY` | `""` | MCP Hub API key |
| `MCPPROXY_API_KEY` | `""` | MCP Proxy API key |
| `MCP_HUB_API_KEY_FILE` | `""` | Path to API key file |
| `MCP_HUB_ENV_FILE` | `""` | Path to .env file |
| `MCP_HUB_CONFIG` | `""` | Path to MCP config JSON |
| `MCPPROXY_CONFIG` | `""` | Path to MCP proxy config JSON |
| `MCP_RUNTIME_ROOT` | `""` | Root dir for MCP runtime config |
| `MCP_PROXY_BIN` | `""` | MCP proxy binary path |
| `TIBRAIN_CLOUDFLARE_SECRET` | `""` | Cloudflare encryption key |
| `TIBRAIN_SECRET_KEY` | `""` | Fallback encryption key |

**CLI Flags:** `--port` (default 3005), `--index-knowledge`

---

## 4. Build, Run & Development

### 4.1 Commands

```bash
# Build
make build                  # Optimized build
make build-debug            # Debug build
make build-all              # Cross-platform: linux + windows + darwin
go build -o tibrain.exe .   # Direct

# Test
make test                   # All tests + coverage
make test-unit              # Unit tests (fast)
make test-integration       # Integration tests
make test-short             # Skip slow tests
make bench                  # Benchmarks

# Lint & Quality
make lint | vet | fmt | tidy
make verify                 # Pre-commit: vet + lint + test-short

# Run
make run                    # Build + run
make run-debug              # Build debug + run
make run-index              # Build + run --index-knowledge
./tibrain.exe --port 3005

# Docker
docker build -t tibrain .
docker run -p 3005:3005 -v tibrain_data:/app/data tibrain
docker-compose up -d

# Tunnel & Service`r`nmake tunnel                 # Cloudflare tunnel (foreground)`r`nmake start                  # Build + run + tunnel (all-in-one)`r`r`n# Public tunnel endpoint:`r`nhttps://tibrain.trepremium.online -> port 3005 (MCP Hub)

# Windows Batch Scripts (Port cleanup included)
start-tibrain.bat           # Start TiBrain (kills old process on port 3005 first)
stop-tibrain.bat            # Stop TiBrain and cleanup processes
restart-tibrain.bat         # Restart TiBrain service
bootstrap-mcp.js            # Register sub-MCPs (chrome-devtools-mcp, etc.)
```

### 4.2 Health Check

```bash
curl http://localhost:3005/health    # Liveness
curl http://localhost:3005/ready     # Readiness
curl http://localhost:3005/status    # Service status
curl http://localhost:3005/list-mcps      # List MCP servers (via tool)`r`ncurl http://localhost:3005/mcp/sse          # MCP SSE endpoint
```

### 4.3 Agent Workflow

```
1. Đọc docs   → AGENTS.md, SPEC.md, DESIGN.md, API.md, TASKS.md
2. Build thử  → go build ./...
3. Tests      → make verify (vet + lint + test-short)
4. Implement  → Tuân thủ Code Rules
5. Test lại   → make verify → make test
6. Clean      → make clean
7. Ghi handoff → handoff.json (xem mục 7)
```

### 4.4 Code Rules

| Rule | Mô tả |
|---|---|
| **DB** | Dùng `*sql.DB` từ Hub qua DI — không mở connection riêng |
| **DDL** | Qua migration file (`internal/db/migrations/`) — không hardcode trong .go |
| **Driver** | Chỉ `modernc.org/sqlite` (driver name: `"sqlite"`) |
| **Async write** | Dùng AsyncWriter cho log, analytics |
| **Sync write** | Dùng DB trực tiếp cho transactional ops |
| **MCP tool** | Thêm trong `mcp_registry_tools.go` / `mcp_surface_tools.go` |
| **Handoff log** | Ghi `Z:\\02_CORE\\_cli\\.config\\handoff.json` |
| **Error ledger** | Lỗi lặp lại → `Z:\\02_CORE\\_cli\\.config\\shared\\memory\\error-ledger.ndjson` |

---

## 5. API & Database

### 5.1 Core REST Endpoints

| Method | Path | Mô tả |
|---|---|---|
| GET | `/health` | Liveness probe |
| GET | `/ready` | Readiness probe |
| GET | `/status` | Service status |
| POST | `/register-cli` | Register CLI/agent |
| POST | `/unregister-cli` | Unregister CLI |
| GET | `/list-clis` | List registered CLIs |
| GET | `/heartbeat` | CLI heartbeat |
| POST | `/create-handoff` | Create handoff |
| GET | `/recall-handoff` | Recall handoff |
| GET | `/list-handoffs` | List handoffs |
| POST | `/register-mcp` | Register MCP server |
| GET | `/list-mcps` | List MCP servers |
| POST | `/register-tool` | Register tool |
| GET | `/list-tools` | List tools |
| POST | `/execute-tool` | Execute tool |
| GET | `/tool-usage-logs` | Tool usage logs |
| POST | `/update-model-stats` | BEADS LEARN stats |
| GET | `/get-model-stats` | Get model stats |
| GET | `/get-best-model` | Best model for task |
| POST | `/chat` | Chat interface |
| POST | `/store-memory` | Store memory |
| POST | `/query-memory` | Query memory |
| GET | `/memory-stats` | Memory stats |
| POST | `/api/v1/prompt/preflight` | Chọn PromptEnvelope cho TiRouter |
| POST | `/api/v1/prompt/feedback` | Ghi outcome prompt |
| GET | `/api/v1/prompt/catalog/version` | Version/ETag của catalog |
| POST | `/api/v1/feedback/model-stats` | Cập nhật model performance stats |

**MCP SSE:** `GET /mcp/sse`, `POST /mcp/message`

**APIServer nhánh:** Xem [API.md](./API.md) cho ~60 endpoints (Health, Knowledge, Agents, RAG, v2 Retrieval, v1 Open-WebUI, RTK, Brain, Tools, Browser Runtime, Docs).

### 5.2 Database

- **Driver:** `modernc.org/sqlite` (pure-Go, no CGO)
- **Connection:** single `*sql.DB` với `SetMaxOpenConns(1)`
- **PRAGMA:** WAL, busy_timeout=5000, synchronous=NORMAL, cache_size=-64000
- **AsyncWriter:** batch writes (flush 5s / 100 jobs)

**Migrations (`internal/db/migrations/`):**

```
0001_hub_init.up.sql    → cli_registry, handoffs, mcp + tool registry
0002_rag.up.sql         → rag_documents, knowledge_bases, code_graph
0003_agents.up.sql      → agent_registry, orchestration_log, task_memory
0004_learning.up.sql    → query_patterns, content_gaps, metrics
0005_ecosystem.up.sql   → brain_systems, sync_status, api_endpoints
0006_predictive.up.sql  → performance + health metrics, predictions
0007_indexing.up.sql    → content_priority, content_classification
0008_crossref.up.sql    → document_links, dependencies, impact_analysis
0009_cloudflare.up.sql  → cloudflare_tokens
0010_ensure_columns.up.sql  → ALTER TABLE ADD COLUMN IF NOT EXISTS
```

**Bảng dữ liệu:**

| Nhóm | Bảng |
|---|---|
| Registry | `cli_registry`, `mcp_registry`, `tool_registry` |
| Handoff | `global_handoffs` |
| RAG | `rag_documents`, `rag_documents_fts`, `rag_knowledge_bases`, `rag_query_history`, `rag_runtime_traces` |
| Learning | `brain_pattern_candidates`, `learned_corrections`, `model_performance_stats` |
| Code Graph | `code_graph_nodes`, `code_graph_edges` |
| Cloudflare | `cloudflare_tokens` |
| Logs | `tool_usage_log` |

---

## 6. Cấu trúc dự án

### 6.1 Source vs Runtime

```
Source:  Z:\01_PROJECTS\apps\tibrain       # Mã nguồn Go
Runtime: Z:\04_RUNTIME\tibrain             # Binary + runtime config
Data:    Z:\01_PROJECTS\apps\tibrain\data   # SQLite DB, WAL, Bleve index
Build:   tibrain/build/                     # Local build output
```

### 6.2 File tree

```
tibrain/
├── main.go                  # Entry point, Hub, HTTP handlers
├── config.yaml              # Service config
├── Makefile                 # 20+ targets
├── Dockerfile / docker-compose.yml
├── go.mod / go.sum / .golangci.yml / .gitignore / .gitmodules
├── AGENTS.md / README.md / API.md / DESIGN.md / SPEC.md / TASKS.md
├── *.go                     # ~45 flat Go files (RAG, MCP, embeddings, etc.)
│
├── internal/
│   ├── core/                # AsyncWriter
│   ├── crypto/              # AES-GCM encryption
│   ├── db/                  # DB layer + migrations (0001-0010)
│   │   └── migrations/
│   ├── memory/              # 3-tier cognitive memory [BLOCKER]
│   └── tools/               # Tool definitions [BLOCKER]
│
├── mcp/ obsidian-headless/ qdrant/  # Git submodules
├── data/                     # Runtime data (gitignored)
├── docs/ scripts/ skills/ knowledge/
├── build/ bin/ logs/ tmp/
├── agent/ providers/ config/ cmd/ cli/
└── lan-ai-api/ termux-mcp/ mimocode-auth/   # Legacy
```

### 6.3 Git Submodules

| Submodule | Path | URL |
|---|---|---|
| MCP Proxy | `mcp/` | `https://github.com/smart-mcp-proxy/mcpproxy-go.git` |
| Obsidian Headless | `obsidian-headless/` | `https://github.com/obsidianmd/obsidian-headless.git` |
| Qdrant | `qdrant/` | `https://github.com/qdrant/qdrant.git` |

```bash
git submodule update --init --recursive
```

### 6.4 File inventory (key files)

| File | Vai trò |
|---|---|
| `main.go` | Entry point, Hub, HTTP handlers |
| `api_server.go` | REST API (12 nhóm, ~100 routes) |
| `rag_system.go` | RAG core (FTS5, keyword, vector) |
| `adaptive_retrieval.go` | Adaptive retrieval + brain patterns |
| `mcp_hub_client.go` | MCP Hub client (circuit breaker) |
| `mcp_server.go` | Embedded MCP server |
| `mcp_registry_tools.go` | Registry MCP tools (20+) |
| `local_tool_executor.go` | Local tool executor (sandboxed) |
| `knowledge_indexer.go` | Knowledge indexing |
| `embeddings.go` | Embedding API client |
| `llm_client.go` | LLM client |
| `code_graph.go` | Go AST parser → graph |
| `cloudflare_service.go` | Cloudflare token management |
| `integration.go` | Agent orchestration |
| `ti_agent.go` | Ti Agent orchestrator |
| `retrieval_router.go` | Retrieval routing |
| `rtk_handler.go` | Runtime Toolkit |
| `cloudflare_db.go` | Cloudflare DB operations |
| `cross_reference_intelligence.go` | Document linking, version tracking |
| `auto_learning_mechanism.go` | Query patterns, quality scoring |
| `mcp_surface_tools.go` | Surface MCP tools |
| `internal/qualitygate` | Built-in quality gate pipeline (gofmt, vet, build, test) |
| `internal/audit` | Built-in secret and binary artifact scanner |
| `internal/tracker` | Built-in handoff/error JSONL logger |
| `docs/PROMPT_INTELLIGENCE_CONTRACT.md` | Target API và data contract với TiRouter |

---

## 7. Operations

### 7.1 Troubleshooting

| Vấn đề | Giải pháp |
|---|---|
| Build lỗi `internal/*` | Implement `internal/db`, `internal/memory`, `internal/tools` |
| `database is locked` | Kiểm tra process khác, xóa `data/tibrain.db-wal` + `.db-shm` |
| `malformed database` | Restore backup hoặc xóa `data/tibrain.db` |
| MCP Hub lỗi | Kiểm tra `MCP_HUB_API_KEY`, `mcp/.runtime/config/mcp_config.json` |
| Circuit breaker open | Restart service (reset breaker) |
| CGO/mattn error | Thay `mattn/go-sqlite3` = `modernc.org/sqlite`, set `CGO_ENABLED=0` |

### 7.2 Security

| Lĩnh vực | Mô tả |
|---|---|
| API Keys | Env vars (`MCP_HUB_API_KEY`, `MCPPROXY_API_KEY`) — không hardcode |
| Cloudflare | Token mã hóa AES-GCM trước khi lưu DB |
| Path traversal | `resolveToolPath()` kiểm tra allowed roots |
| Tool access | 3 levels: `read`, `write`, `destructive` (inferred from name) |
| Port exposure | Mặc định localhost; tunnel qua `mcp.trepremium.online` |

### 7.3 Known Issues & Blockers

| Issue | Mức | Tình trạng |
|---|---|---|
| `internal/memory/` chưa implement | 🔴 Blocker | Package chưa tồn tại |
| `internal/db/` thiếu file | 🔴 Blocker | `db.go`, `db_interface.go`, `migrate.go` chưa đủ |
| `internal/tools/` chưa implement | 🟡 Blocker | Package chưa tồn tại |
| Module cũ tự mở SQLite connection (6+ file) | 🟡 Medium | Cần refactor DI |
| Mixed SQLite drivers (`mattn` vs `modernc`) | 🟡 Medium | Cần thống nhất |
| Bảng trùng tên (`query_patterns`, `learning_metrics`) | 🟡 Medium | Định nghĩa ở nhiều file |
| Schema drift detection | 🟢 Low | Checksum chưa active |
| Prompt Intelligence API | 🟡 Planned | Contract đã chốt; code/migration chưa triển khai |
| `internal/qualitygate` | 🟢 New | Built-in lint/vet/build/test pipeline |
| `internal/audit` | 🟢 New | Secret and artifact scanner |
| `internal/tracker` | 🟢 New | Handoff/error JSONL logging |

---

## 8. Phụ lục & Tham chiếu

### 8.1 T4 Service & Termux (Legacy)

> Chạy trên Termux (Android), không phải phần của TiBrain core.

**OmniRoute CLI:** `omniroute t4:start|status|stop|restart|logs|metrics|one-shot|load-test|health-monitor`

**Mimocode-Auth plugin:**
```bash
git clone https://github.com/yinianhuakai000/mimocode-auth.git
cd mimocode-auth && npm install && npm run build
```

### 8.2 Freebuff2API Proxy

```bash
set FREEBUFF_TOKEN=your_token_here
node freebuff2api.js --port 8080
```

> Hoạt động ở DEMO mode. Freebuff không có public REST API chính thức.

### 8.3 Handoff & Error Logging

```json
// handoff.json (JSON Lines) @ Z:\02_CORE\_cli\.config\handoff.json
{"agent":"tibrain","action":"description","timestamp":"ISO_UTC","details":{...}}

// error-ledger.ndjson @ Z:\02_CORE\_cli\.config\shared\memory\error-ledger.ndjson
```

### 8.4 Documents

| Document | Nội dung |
|---|---|
| `DESIGN.md` | DB layer design, migration framework, conflict resolution |
| `SPEC.md` | Implementation plan — Phase 1 (CGO removal), Phase 2 (package split), Phase 3 (metrics) |
| `API.md` | API reference với curl examples (~60 endpoints) |
| `TASKS.md` | Task tracking |
| `docs/PROMPT_INTELLIGENCE_CONTRACT.md` | Contract Prompt Intelligence với TiRouter |

---

*Kế thừa policy từ `Z:\AGENTS.md`, `Z:\01_PROJECTS\AGENTS.md`, `Z:\01_PROJECTS\apps\AGENTS.md`.*
*Nếu có mơ hồ, dừng lại và hỏi người dùng trước khi thay đổi kiến trúc.*





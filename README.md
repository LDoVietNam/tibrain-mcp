# TiBrain

> Dịch vụ **internal intelligence / knowledge hub** viết bằng Go, chạy trên một cổng HTTP duy nhất, gom REST API, MCP (SSE) và Browser UI vào cùng một tiến trình.

Repo: [`github.com/ti/router/tibrain`](https://github.com/ti/router/tibrain)

---

## Mục đích (Purpose)

TiBrain là **bộ não knowledge, memory, retrieval và learning** phía sau TiRouter trong hệ sinh thái Ti. Nó cung cấp:

- **Điều phối agent (agent orchestration)** và bàn giao công việc (handoff) giữa các CLI.
- **Bộ nhớ nhận thức (cognitive memory)**: episodic, semantic, procedural.
- **RAG retrieval**: vector store, reranker, embeddings, adaptive retrieval, cache.
- **Knowledge indexing**: nạp và đánh chỉ mục tri thức phục vụ truy hồi.
- **MCP hub**: đăng ký và phục vụ tool qua Model Context Protocol (SSE).
- **Learning & quality**: thống kê theo mô hình BEADS LEARN.

**Port mặc định:** `3005`
**Giao thức:** REST tại `/api/*`, MCP SSE tại `/mcp`, Browser UI tại `/gui`.

---

## Cách sử dụng (Usage)

### Windows (khuyến nghị)

```powershell
# Start tunnel (dọn dẹp port trước, khởi động server, start tunnel)
.\start-tunnel.bat

# Hoặc restart nếu đang chạy
.\restart-tunnel.bat

# Dừng tất cả
.\stop-tunnel.bat

# Build và chạy trực tiếp
.\start-tibrain.bat

# Hoặc dùng script
.\build.ps1
.\run_tibrain.ps1
```

### Linux/macOS

```bash
# Kill existing processes
pkill -f tibrain || true
pkill -f cloudflared || true

# Start server
./tibrain &

# Start tunnel
cloudflared tunnel run tibrain
```

### Docker

```bash
# Build Docker image
docker build -t tibrain:latest .

# Run container
docker run -p 3005:3005 tibrain:latest
```

### Makefile

```bash
# Build
make build

# Run server
make run

# Run with tunnel
make start

# Test
make test

# Clean
make clean
```

---

## Ví dụ (Examples)

### Ví dụ 1: Khởi động TiBrain với tunnel

```powershell
cd Z:\01_PROJECTS\apps\products\tibrain
.\start-tunnel.bat
```

**Output mong đợi:**
```
[TiBrain] Đang don port 3005...
[TiBrain] Build thành công: tibrain.exe
[TiBrain] MCP server đang chay tại http://localhost:3005/mcp
[TiBrain] REST API tại http://localhost:3005/api/*
```

### Ví dụ 2: Health check

```powershell
curl -s http://localhost:3005/health
```

**Output mong đợi:**
```json
{"status":"ok"}
```

### Ví dụ 3: Danh sách MCP tools

```powershell
curl -s http://localhost:3005/mcp
```

### Ví dụ 4: Build với tối ưu

```powershell
go build -ldflags="-s -w" -o tibrain.exe .
```

### Ví dụ 5: Khởi động với port tùy chỉnh

```powershell
.\tibrain.exe --port 8080 --host 0.0.0.0
```

### Ví dụ 6: Kiểm tra các endpoints MCP

```powershell
# Health check
curl http://localhost:3005/health

# MCP SSE
curl http://localhost:3005/mcp/sse

# Metrics
curl http://localhost:3005/metrics

# Prompt config
curl http://localhost:3005/api/v2/runtime/prompts
```

---

## Cấu trúc dự án

```
tibrain/
├── main.go                     # Điểm vào ứng dụng
├── config.yaml                 # Cấu hình server/MCP/auth/audit
├── go.mod                      # Go module dependencies
├── Makefile                    # Build targets
├── build.ps1                   # Script biên dịch PowerShell
├── start-tibrain.bat           # Script khởi động Windows
├── restart-tibrain.bat         # Script restart Windows
├── stop-tibrain.bat            # Script dừng Windows
├── start-tunnel.bat            # Script start Cloudflare tunnel
├── AGENTS.md                   # Agent registry definitions
├── README.md                   # File này
├── docs/                       # Documentation
│   └── TOOLS.md               # MCP tools danh sách đầy đủ
├── internal/                   # Internal packages
│   ├── api/                    # REST API server + prompt handlers
│   ├── mcp/                    # MCP gateway, transports, registry, tools
│   ├── prompt/                 # Prompt intelligence, ingestion, registry
│   ├── security/               # Auth, guard, audit, secret vault
│   ├── db/                     # Database hub, migrations, sync
│   └── ...
└── bin/                        # Build artifacts
    ├── tibrain.exe             # Binary đã build
    └── releases/               # Release binaries
```

---

## Environment Variables

| Variable | Default | Mô tả |
| --- | --- | --- |
| `PORT` | `3005` | Port server chạy |
| `TIBRAIN_DATA_DIR` | `Z:\03_DATA\tibrain-database` | Thư mục dữ liệu SQLite |
| `TIBRAIN_DB_DRIVER` | `sqlite3` | Driver database (sqlite3/postgres) |
| `MCP_SERVER_NAME` | `tibrain` | Tên MCP server cho Claude Code |
| `MCP_TIMEOUT_MS` | `10000` | Timeout cho MCP requests (ms) |
| `ECC_HOOK_PROFILE` | `standard` | Hook profile (minimal/standard/strict) |

---

## Endpoints API

| Phương thức | Endpoint | Mô tả |
|------------|----------|-------|
| **GET** | `/health` | Health check |
| **GET** | `/metrics` | Prometheus metrics |
| **GET** | `/gui` | Web UI |
| **GET** | `/mcp/sse` | MCP SSE endpoint |
| **POST** | `/mcp` | MCP HTTP streamable endpoint |
| **GET** | `/api/v2/runtime/prompts` | Prompt runtime config |
| **GET** | `/api/prompts` | Danh sách prompt capsules |
| **POST** | `/api/v1/prompt/ingest` | Nạp tài liệu vào prompt intelligence |
| **POST** | `/api/v1/prompt/feedback` | Ghi nhận kết quả sử dụng prompt |
| **POST** | `/v1/secrets/upsert` | Upsert secret (cần bearer token) |
| **GET** | `/v1/secrets/resolve` | Resolve secret (cần bearer token) |

---

## Related files

- [`AGENTS.md`](./AGENTS.md) — Agent registry, skill definitions
- [`config.yaml`](./config.yaml) — Server configuration
- [`Makefile`](./Makefile) — Build targets, test commands, tunnel management
- [`docs/TOOLS.md`](./docs/TOOLS.md) — MCP tools danh sách đầy đủ
- [`internal/api/`](./internal/api/) — REST API handlers
- [`internal/mcp/`](./internal/mcp/) — MCP gateway, registry, tools
- [`internal/prompt/`](./internal/prompt/) — Prompt intelligence system
- [`internal/security/`](./internal/security/) — Auth, guard, audit, secret vault
- [`internal/db/`](./internal/db/) — Database hub, migrations

---

## Documentation standards

Mọi file `.md` trong project phải có:
- [ ] Title in Vietnamese
- [ ] Mục đích (Purpose) section
- [ ] Cách sử dụng (Usage) section
- [ ] Ví dụ (Examples) section
- [ ] Related files link

Mọi file `.go` phải có:
- [ ] Package comment in Vietnamese
- [ ] Function comments cho public APIs
- [ ] Error messages in Vietnamese (user-facing)
- [ ] Log messages in Vietnamese

---

## License

MIT License
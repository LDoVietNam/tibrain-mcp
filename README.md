# TiBrain

> Dịch vụ **internal intelligence / knowledge hub** viết bằng Go, chạy trên một cổng HTTP duy nhất, gom REST API, MCP (SSE) và Browser UI vào cùng một tiến trình.

Repo: [`github.com/ti/router/tibrain`](https://github.com/ti/router/tibrain)

---

## Quick Start

### Windows

```powershell
# Start tunnel (dọn dẹp port trước, khởi động server, start tunnel)
.\start-tunnel.bat

# Hoặc restart nếu đang chạy
.\restart-tunnel.bat

# Dừng tất cả
.\stop-tunnel.bat
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

---

## Overview

TiBrain là bộ não knowledge, memory, retrieval và learning phía sau TiRouter. Nó cung cấp
dịch vụ intelligence nội bộ cho:

- **Điều phối agent (agent orchestration)** và bàn giao công việc (handoff) giữa các CLI.
- **Bộ nhớ nhận thức (cognitive memory)**: episodic, semantic, procedural.
- **RAG retrieval**: vector store, reranker, embeddings, adaptive retrieval, cache.
- **Knowledge indexing**: nạp và đánh chỉ mục tri thức phục vụ truy hồi.
- **MCP hub**: đăng ký và phục vụ tool qua Model Context Protocol (SSE).
- **Learning & quality**: thống kê theo mô hình BEADS LEARN.

Toàn bộ chạy trên **một cổng duy nhất** (mặc định `3005`): REST tại `/api/*`,
MCP SSE tại `/mcp`, và Browser UI phục vụ trực tiếp từ cùng server.

---

## MCP File Server

TiBrain cung cấp các tool filesystem qua MCP SSE:

- `read_file` - đọc file
- `write_file` - ghi file
- `list_files` - liệt kê thư mục
- `delete_file` - xóa file
- `file_info` - thông tin file

### Endpoints

| Endpoint | Mô tả |
| --- | --- |
| `http://localhost:3005/health` | Health check |
| `http://localhost:3005/gui` | Web UI |
| `http://localhost:3005/mcp` | MCP SSE endpoint |
| `https://tibrain.trepremium.online/mcp` | Public MCP endpoint |

### Test Tools

```powershell
# Health check
curl http://localhost:3005/health

# List tools via SSE
curl http://localhost:3005/mcp
```

---

## Configuration

Port được cấu hình trong `config.yaml`:

```yaml
tibrain:
  port: 3005  # MCP Hub port
  data_dir: "data"
```

Tunnel config tại `.runtime/config/tunnel_config.json`:

```json
{
  "tunnel": "tibrain",
  "hostname": "tibrain.trepremium.online",
  "target": "http://localhost:3005"
}
```

---

## Tunnel Scripts

| File | Mô tả |
| --- | --- |
| `start-tunnel.bat` | Start server + tunnel, dọn dẹp port trước |
| `restart-tunnel.bat` | Restart tunnel |
| `stop-tunnel.bat` | Dừng mọi process |

---

## Architecture

Kiến trúc single-port: một HTTP server duy nhất định tuyến tới nhiều subsystem.

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

---

## Build & Run

```bash
# Build
go build -o tibrain.exe .

# Run
.\tibrain.exe

# Or with tunnel
.\start-tunnel.bat
```

---

## Development

```bash
make build    # build binary
make run      # chạy dịch vụ
make verify   # vet + lint + test-short
```

---

## Handoff Logging

Agents must log actions to: `Z:\02_CORE\_cli\.config\handoff.json`

```json
{"agent":"tibrain","action":"updated README with tunnel scripts","timestamp":"2026-07-16T14:30:00+07:00","details":{"files":["README.md"]}}
```

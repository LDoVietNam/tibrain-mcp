# TiBrain API Reference

> **Lưu ý về Prompt Intelligence:** các endpoint `/api/v1/prompt/*` đang là target contract, chưa phải API runtime đã xác minh. Xem [`docs/PROMPT_INTELLIGENCE_CONTRACT.md`](./docs/PROMPT_INTELLIGENCE_CONTRACT.md) và `TASKS.md` để theo dõi triển khai. Không tích hợp production vào các endpoint này trước khi contract test chạy xanh.

TiBrain là một Go service chạy trên **port `3005`**, gom chung một HTTP server duy nhất phục vụ đồng thời:

- **REST API** (root mux — định nghĩa trong `main.go` + `api_server.go`)
- **REST API mở rộng** dưới prefix `/api/*` (`APIServer` trong `api_server.go`)
- **MCP SSE** (Model Context Protocol qua Server-Sent Events, embedded)

> **Ghi chú:** Toàn bộ payload JSON trong tài liệu này là **ví dụ minh họa** dựa trên tên trường phổ biến. Cấu trúc thực tế có thể khác đôi chút tùy phiên bản; hãy kiểm tra response thực tế của service để biết schema chính xác.

## Thông tin chung

| Mục | Giá trị |
|-----|---------|
| Base URL | `http://localhost:3005` |
| Content-Type | `application/json` (cho các request có body) |
| MCP SSE endpoint | `GET /mcp/sse`, `POST /mcp/message` |

Quy ước ví dụ:

```bash
export TIBRAIN="http://localhost:3005"
```

---

## Mục lục

1. [Health / Status](#1-health--status)
2. [CLI Registry & Handoff](#2-cli-registry--handoff)
3. [MCP Registry & Hub](#3-mcp-registry--hub)
4. [Tools & Execution](#4-tools--execution)
5. [Cognitive Memory](#5-cognitive-memory)
6. [RAG & Retrieval](#6-rag--retrieval)
7. [Knowledge](#7-knowledge)
8. [Agents & Orchestration](#8-agents--orchestration)
9. [RTK (Runtime Toolkit)](#9-rtk-runtime-toolkit)
10. [Misc (Chat, Model Stats, Docs, Browser Runtime, v1 Open-WebUI)](#10-misc)
11. [MCP Tools (SSE)](#11-mcp-tools-sse)

---

## 1. Health / Status

Nhóm endpoint kiểm tra tình trạng service, độ sẵn sàng và tổng quan hệ thống.

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/health` | Health check cơ bản, trả về trạng thống sống của service |
| GET | `/status` | Trạng thái tổng hợp của TiBrain |
| GET | `/v1/tibrain/status` | Trạng thái theo namespace `v1/tibrain` |
| GET | `/ready` | Readiness probe (đã sẵn sàng nhận traffic hay chưa) |
| GET | `/api/health` | Health check của lớp `APIServer` |
| GET | `/api/status` | Trạng thái của lớp `APIServer` |
| GET | `/api/overview` | Tổng quan toàn hệ thống (số agent, CLI, tool, memory...) |

### Ví dụ: GET /health

```bash
curl -s "$TIBRAIN/health"
```

Response (minh họa):

```json
{
  "status": "ok",
  "service": "tibrain",
  "version": "1.0.0",
  "uptime_seconds": 12045
}
```

### Ví dụ: GET /api/overview

```bash
curl -s "$TIBRAIN/api/overview"
```

Response (minh họa):

```json
{
  "agents": 4,
  "clis": 3,
  "mcp_servers": 7,
  "tools": 42,
  "memory_items": 1580,
  "knowledge_documents": 320,
  "healthy": true
}
```

---

## 2. CLI Registry & Handoff

Đăng ký các CLI/agent client, gửi heartbeat, và trao đổi ngữ cảnh (handoff) giữa các phiên làm việc.

### CLI Registry

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/register-cli` | Đăng ký một CLI/agent client mới |
| GET | `/unregister-cli` | Hủy đăng ký một CLI (theo query param, ví dụ `?id=...`) |
| GET | `/list-clis` | Liệt kê các CLI đang đăng ký |
| GET | `/heartbeat` | Gửi/cập nhật heartbeat cho một CLI |

#### Ví dụ: POST /register-cli

```bash
curl -s -X POST "$TIBRAIN/register-cli" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "kilo-code",
    "name": "Kilo Code Agent",
    "version": "1.2.0",
    "capabilities": ["chat", "tools", "memory"]
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "cli_id": "kilo-code",
  "registered_at": "2026-07-10T04:44:00+07:00"
}
```

#### Ví dụ: GET /list-clis

```bash
curl -s "$TIBRAIN/list-clis"
```

Response (minh họa):

```json
{
  "clis": [
    {
      "id": "kilo-code",
      "name": "Kilo Code Agent",
      "status": "online",
      "last_heartbeat": "2026-07-10T04:43:55+07:00"
    }
  ],
  "count": 1
}
```

#### Ví dụ: GET /heartbeat

```bash
curl -s "$TIBRAIN/heartbeat?id=kilo-code"
```

### Handoff

Cho phép một agent lưu ("create") ngữ cảnh bàn giao và một agent khác lấy lại ("recall").

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/create-handoff` | Tạo một bản handoff (bàn giao ngữ cảnh) |
| GET | `/recall-handoff` | Lấy lại handoff (theo query param, ví dụ `?id=...`) |
| GET | `/list-handoffs` | Liệt kê các handoff hiện có |

#### Ví dụ: POST /create-handoff

```bash
curl -s -X POST "$TIBRAIN/create-handoff" \
  -H "Content-Type: application/json" \
  -d '{
    "from_cli": "kilo-code",
    "to_cli": "claude-code",
    "context": "Đang refactor module auth, còn lại việc viết test.",
    "tags": ["auth", "refactor"]
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "handoff_id": "ho_20260710_001",
  "created_at": "2026-07-10T04:44:10+07:00"
}
```

#### Ví dụ: GET /recall-handoff

```bash
curl -s "$TIBRAIN/recall-handoff?id=ho_20260710_001"
```

Response (minh họa):

```json
{
  "handoff_id": "ho_20260710_001",
  "from_cli": "kilo-code",
  "to_cli": "claude-code",
  "context": "Đang refactor module auth, còn lại việc viết test.",
  "tags": ["auth", "refactor"],
  "created_at": "2026-07-10T04:44:10+07:00"
}
```

---

## 3. MCP Registry & Hub

Quản lý danh sách MCP server đã đăng ký, đồng bộ tool của chúng, và proxy gọi tool thông qua MCP Hub.

### MCP Registry

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/register-mcp` | Đăng ký một MCP server |
| GET | `/list-mcps` | Liệt kê các MCP server đã đăng ký |
| GET | `/get-mcp` | Lấy thông tin một MCP server (query `?id=...`) |
| POST | `/sync-mcp-tools` | Đồng bộ danh sách tool từ MCP server vào registry |

#### Ví dụ: POST /register-mcp

```bash
curl -s -X POST "$TIBRAIN/register-mcp" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "filesystem",
    "name": "Filesystem MCP",
    "transport": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "mcp_id": "filesystem",
  "tools_discovered": 8
}
```

### MCP Hub proxy

Proxy tập trung để liệt kê server/tool và gọi tool qua Hub.

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/mcp-hub/servers` | Liệt kê server do MCP Hub quản lý |
| GET | `/mcp-hub/tools` | Liệt kê tool khả dụng qua Hub |
| POST | `/mcp-hub/call` | Gọi một tool qua Hub |
| POST | `/mcp-hub/batch-call` | Gọi nhiều tool trong một request |
| POST | `/sync-mcp-hub` | Đồng bộ registry với MCP Hub |

#### Ví dụ: POST /mcp-hub/call

```bash
curl -s -X POST "$TIBRAIN/mcp-hub/call" \
  -H "Content-Type: application/json" \
  -d '{
    "server": "filesystem",
    "tool": "read_file",
    "arguments": { "path": "/data/notes.txt" }
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "result": {
    "content": "Nội dung file...",
    "mime_type": "text/plain"
  },
  "elapsed_ms": 34
}
```

#### Ví dụ: POST /mcp-hub/batch-call

```bash
curl -s -X POST "$TIBRAIN/mcp-hub/batch-call" \
  -H "Content-Type: application/json" \
  -d '{
    "calls": [
      { "server": "filesystem", "tool": "list_dir", "arguments": { "path": "/data" } },
      { "server": "git", "tool": "status", "arguments": {} }
    ]
  }'
```

---

## 4. Tools & Execution

Đăng ký, tra cứu và thực thi tool; đồng thời xem log sử dụng tool.

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/register-tool` | Đăng ký một tool mới vào registry |
| GET | `/list-tools` | Liệt kê tool đã đăng ký |
| GET | `/get-tool` | Lấy chi tiết một tool (query `?name=...`) |
| POST | `/execute-tool` | Thực thi một tool |
| GET | `/tool-usage-logs` | Xem log sử dụng tool |
| GET | `/api/tools` | Danh sách tool ở lớp `APIServer` |

#### Ví dụ: POST /register-tool

```bash
curl -s -X POST "$TIBRAIN/register-tool" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "summarize",
    "description": "Tóm tắt văn bản dài",
    "input_schema": {
      "type": "object",
      "properties": { "text": { "type": "string" } },
      "required": ["text"]
    }
  }'
```

Response (minh họa):

```json
{ "success": true, "tool": "summarize" }
```

#### Ví dụ: POST /execute-tool

```bash
curl -s -X POST "$TIBRAIN/execute-tool" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "summarize",
    "arguments": { "text": "Đoạn văn bản rất dài cần tóm tắt..." }
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "result": { "summary": "Bản tóm tắt ngắn gọn." },
  "elapsed_ms": 512
}
```

#### Ví dụ: GET /tool-usage-logs

```bash
curl -s "$TIBRAIN/tool-usage-logs?limit=20"
```

Response (minh họa):

```json
{
  "logs": [
    {
      "tool": "summarize",
      "cli_id": "kilo-code",
      "success": true,
      "elapsed_ms": 512,
      "timestamp": "2026-07-10T04:40:00+07:00"
    }
  ],
  "count": 1
}
```

---

## 5. Cognitive Memory

Bộ nhớ nhận thức (cognitive memory) để lưu/truy vấn kinh nghiệm và xem thống kê.

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/store-memory` | Lưu một mẩu ký ức/kinh nghiệm |
| POST | `/query-memory` | Truy vấn ký ức theo ngữ nghĩa |
| GET | `/memory-stats` | Thống kê bộ nhớ (số lượng, dung lượng...) |
| GET | `/recent-experience` | Lấy các trải nghiệm gần đây |
| GET | `/api/agent/memory` | Bộ nhớ của agent (lớp `APIServer`) |

#### Ví dụ: POST /store-memory

```bash
curl -s -X POST "$TIBRAIN/store-memory" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Khi build lỗi CGO, cần set CGO_ENABLED=1.",
    "type": "insight",
    "tags": ["build", "go", "cgo"],
    "source": "kilo-code"
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "memory_id": "mem_0001",
  "embedding_dim": 768
}
```

#### Ví dụ: POST /query-memory

```bash
curl -s -X POST "$TIBRAIN/query-memory" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "lỗi build CGO trên Go",
    "top_k": 5
  }'
```

Response (minh họa):

```json
{
  "results": [
    {
      "memory_id": "mem_0001",
      "content": "Khi build lỗi CGO, cần set CGO_ENABLED=1.",
      "score": 0.91,
      "tags": ["build", "go", "cgo"]
    }
  ],
  "count": 1
}
```

#### Ví dụ: GET /memory-stats

```bash
curl -s "$TIBRAIN/memory-stats"
```

Response (minh họa):

```json
{
  "total_items": 1580,
  "by_type": { "insight": 420, "event": 900, "fact": 260 },
  "storage_bytes": 10485760
}
```

---

## 6. RAG & Retrieval

Truy xuất tăng cường sinh (RAG), ingest tài liệu, gửi feedback, và các endpoint retrieval v2 nâng cao (traces, brain patterns).

### RAG (v1 lớp APIServer)

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/rag/query` | Truy vấn RAG (retrieval + tổng hợp câu trả lời) |
| GET | `/api/rag/status` | Trạng thái pipeline RAG |
| POST | `/api/rag/feedback` | Gửi feedback về kết quả RAG |
| POST | `/api/rag/ingest` | Ingest tài liệu vào kho RAG |

#### Ví dụ: POST /api/rag/query

```bash
curl -s -X POST "$TIBRAIN/api/rag/query" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Cách cấu hình port cho TiBrain?",
    "top_k": 4,
    "include_sources": true
  }'
```

Response (minh họa):

```json
{
  "answer": "TiBrain mặc định chạy trên port 3005...",
  "sources": [
    { "doc_id": "doc_12", "title": "Config", "score": 0.88 }
  ],
  "elapsed_ms": 640
}
```

#### Ví dụ: POST /api/rag/feedback

```bash
curl -s -X POST "$TIBRAIN/api/rag/feedback" \
  -H "Content-Type: application/json" \
  -d '{ "query_id": "q_001", "rating": "up", "comment": "Chính xác" }'
```

### Retrieval v2

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/v2/retrieve` | Retrieval nâng cao (v2) |
| GET | `/api/v2/retrieve/traces` | Xem trace của quá trình retrieval |
| GET | `/api/v2/brain/patterns` | Liệt kê pattern đã học của "brain" |
| POST | `/api/v2/brain/patterns/promote` | Nâng cấp (promote) một pattern |
| GET | `/api/v2/runtime/registry` | Registry runtime v2 |

#### Ví dụ: POST /api/v2/retrieve

```bash
curl -s -X POST "$TIBRAIN/api/v2/retrieve" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "cấu hình embedding model",
    "top_k": 6,
    "with_traces": true
  }'
```

Response (minh họa):

```json
{
  "documents": [
    { "doc_id": "doc_31", "chunk": "Embedding model được set qua...", "score": 0.86 }
  ],
  "trace_id": "trace_abc",
  "count": 1
}
```

---

## 7. Knowledge

Quản lý kho tri thức: liệt kê, index, và kiểm tra trạng thái.

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/knowledge` | Liệt kê tài liệu/knowledge item |
| POST | `/api/knowledge/index` | Index (nạp & vector hóa) tài liệu |
| GET | `/api/knowledge/status` | Trạng thái tiến trình index |

#### Ví dụ: POST /api/knowledge/index

```bash
curl -s -X POST "$TIBRAIN/api/knowledge/index" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/data/docs",
    "recursive": true,
    "chunk_size": 512
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "job_id": "idx_001",
  "queued_documents": 128
}
```

#### Ví dụ: GET /api/knowledge/status

```bash
curl -s "$TIBRAIN/api/knowledge/status?job_id=idx_001"
```

Response (minh họa):

```json
{
  "job_id": "idx_001",
  "state": "running",
  "processed": 64,
  "total": 128
}
```

---

## 8. Agents & Orchestration

Đăng ký agent, gửi yêu cầu, điều phối (orchestrate) đa agent, và định tuyến cross-brain.

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/agents` | Liệt kê agent |
| POST | `/api/agents/register` | Đăng ký agent mới |
| POST | `/api/orchestrate` | Điều phối một tác vụ qua nhiều agent |
| POST | `/api/agent/request` | Gửi request tới một agent cụ thể |
| GET | `/api/agent/memory` | Bộ nhớ của agent |
| GET | `/api/brain/router` | Router cross-brain (định tuyến giữa các brain) |
| GET | `/api/brain/ti` | Endpoint brain "ti" (cross-brain) |

#### Ví dụ: POST /api/agents/register

```bash
curl -s -X POST "$TIBRAIN/api/agents/register" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "researcher",
    "name": "Research Agent",
    "role": "research",
    "capabilities": ["web_search", "summarize"]
  }'
```

Response (minh họa):

```json
{ "success": true, "agent_id": "researcher" }
```

#### Ví dụ: POST /api/orchestrate

```bash
curl -s -X POST "$TIBRAIN/api/orchestrate" \
  -H "Content-Type: application/json" \
  -d '{
    "task": "Nghiên cứu và viết tóm tắt về MCP",
    "agents": ["researcher", "writer"],
    "strategy": "sequential"
  }'
```

Response (minh họa):

```json
{
  "success": true,
  "run_id": "run_007",
  "steps": [
    { "agent": "researcher", "status": "done" },
    { "agent": "writer", "status": "running" }
  ]
}
```

#### Ví dụ: POST /api/agent/request

```bash
curl -s -X POST "$TIBRAIN/api/agent/request" \
  -H "Content-Type: application/json" \
  -d '{ "agent_id": "researcher", "input": "Tìm 3 nguồn về MCP" }'
```

---

## 9. RTK (Runtime Toolkit)

Nhóm RTK phục vụ tối ưu ngữ cảnh: đo "gain", khám phá tool, nén (compress) nội dung/messages, log và quản lý rule.

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/rtk/gain` | Đo "gain" (lợi ích) của một thao tác/context |
| GET | `/api/rtk/discover` | Khám phá tool/khả năng phù hợp |
| POST | `/api/rtk/compress` | Nén một khối nội dung |
| POST | `/api/rtk/compress-messages` | Nén lịch sử hội thoại (messages) |
| GET | `/api/rtk/log` | Xem log RTK |
| GET | `/api/rtk/rules` | Liệt kê rule RTK |
| POST | `/api/rtk/rules/sync` | Đồng bộ rule RTK |

#### Ví dụ: POST /api/rtk/compress-messages

```bash
curl -s -X POST "$TIBRAIN/api/rtk/compress-messages" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      { "role": "user", "content": "Câu hỏi dài..." },
      { "role": "assistant", "content": "Trả lời dài..." }
    ],
    "target_tokens": 500
  }'
```

Response (minh họa):

```json
{
  "compressed": "Tóm tắt hội thoại rút gọn...",
  "original_tokens": 2200,
  "compressed_tokens": 480,
  "ratio": 0.22
}
```

#### Ví dụ: GET /api/rtk/gain

```bash
curl -s "$TIBRAIN/api/rtk/gain?context_id=ctx_01"
```

Response (minh họa):

```json
{ "context_id": "ctx_01", "gain": 0.73, "tokens_saved": 1720 }
```

---

## 10. Misc

Các endpoint còn lại: Chat/NL, Model Stats (BEADS LEARN), Docs, Browser Runtime, và API tương thích Open-WebUI (v1).

### Chat / Natural Language

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/chat` | Chat hội thoại (NL) với TiBrain |
| POST | `/agent-process` | Xử lý yêu cầu qua pipeline agent |
| POST | `/notion-sync` | Đồng bộ dữ liệu với Notion |

#### Ví dụ: POST /chat

```bash
curl -s -X POST "$TIBRAIN/chat" \
  -H "Content-Type: application/json" \
  -d '{ "message": "TiBrain là gì?", "session_id": "s1" }'
```

Response (minh họa):

```json
{
  "reply": "TiBrain là một Go service tổng hợp REST + MCP...",
  "session_id": "s1",
  "used_rag": true
}
```

### Model Stats (BEADS LEARN)

Theo dõi chất lượng model để chọn model tốt nhất và xem xu hướng.

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/update-model-stats` | Cập nhật thống kê chất lượng của model |
| GET | `/get-model-stats` | Lấy thống kê một model |
| GET | `/get-best-model` | Lấy model tốt nhất theo tiêu chí |
| GET | `/get-quality-trend` | Xu hướng chất lượng theo thời gian |
| GET | `/get-all-model-stats` | Thống kê của tất cả model |

#### Ví dụ: POST /update-model-stats

```bash
curl -s -X POST "$TIBRAIN/update-model-stats" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "task": "summarize",
    "quality": 0.92,
    "latency_ms": 850,
    "success": true
  }'
```

Response (minh họa):

```json
{ "success": true, "model": "gpt-4o", "samples": 128 }
```

#### Ví dụ: GET /get-best-model

```bash
curl -s "$TIBRAIN/get-best-model?task=summarize"
```

Response (minh họa):

```json
{ "task": "summarize", "best_model": "gpt-4o", "avg_quality": 0.9 }
```

### Docs

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/docs/search` | Tìm kiếm trong tài liệu |
| GET | `/api/docs/status` | Trạng thái index tài liệu |

#### Ví dụ: GET /api/docs/search

```bash
curl -s "$TIBRAIN/api/docs/search?q=port+3005"
```

### Browser Runtime

Đăng ký browser runtime và quản lý task tự động hóa trình duyệt (có hỗ trợ streaming).

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/browser-runtime/register` | Đăng ký một browser runtime |
| GET | `/api/browser-runtime/tasks` | Liệt kê task của browser runtime |
| GET | `/api/browser-runtime/tasks/stream` | Stream cập nhật task (SSE) |

#### Ví dụ: GET /api/browser-runtime/tasks

```bash
curl -s "$TIBRAIN/api/browser-runtime/tasks"
```

### v1 (Open-WebUI compatible)

Nhóm API tương thích Open-WebUI cho RAG, knowledge, graph, router, memory và analytics.

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/v1/rag/query` | Truy vấn RAG (chuẩn v1) |
| GET | `/api/v1/knowledge/documents` | Liệt kê tài liệu knowledge |
| POST | `/api/v1/knowledge/ingest` | Ingest tài liệu |
| GET | `/api/v1/graph/nodes` | Liệt kê node trong knowledge graph |
| GET | `/api/v1/router/routes` | Liệt kê route định tuyến |
| POST | `/api/v1/memory/store` | Lưu memory (chuẩn v1) |
| GET | `/api/v1/analytics` | Số liệu analytics |

#### Ví dụ: POST /api/v1/rag/query

```bash
curl -s -X POST "$TIBRAIN/api/v1/rag/query" \
  -H "Content-Type: application/json" \
  -d '{ "query": "TiBrain hỗ trợ MCP không?", "top_k": 3 }'
```

Response (minh họa):

```json
{
  "answer": "Có, TiBrain hỗ trợ MCP qua SSE...",
  "sources": [{ "doc_id": "doc_5", "score": 0.87 }]
}
```

---

## 11. MCP Tools (SSE)

TiBrain nhúng một MCP server truy cập qua SSE:

- **`GET /mcp/sse`** — mở kết nối SSE để nhận sự kiện/kết quả tool.
- **`POST /mcp/message`** — gửi message/tool-call theo giao thức MCP (JSON-RPC).

Kết nối theo chuẩn MCP: client mở SSE tại `/mcp/sse`, sau đó gửi các request (initialize, tools/list, tools/call) tới `/mcp/message`. Dưới đây là các tool được cung cấp, chia theo nhóm chức năng.

### Nhóm Brain (bộ nhớ & RAG)

| Tool | Mô tả |
|------|-------|
| `query_memory` | Truy vấn bộ nhớ nhận thức theo ngữ nghĩa |
| `store_memory` | Lưu một mẩu ký ức/kinh nghiệm |
| `chat_rag` | Hỏi–đáp có RAG (retrieval + sinh câu trả lời) |

### Nhóm Control-plane (thực thi & điều phối)

| Tool | Mô tả |
|------|-------|
| `brain_execute` | Thực thi một thao tác/tool đơn |
| `brain_batch_execute` | Thực thi nhiều thao tác theo lô |
| `brain_transaction_*` | Nhóm tool giao dịch (begin/commit/rollback...) cho thao tác nguyên tử |
| `brain_preflight` | Kiểm tra trước (preflight) tính khả thi của thao tác |
| `brain_tool_search` | Tìm tool phù hợp theo mô tả nhu cầu |
| `brain_health` | Kiểm tra tình trạng brain |
| `brain_capabilities` | Liệt kê khả năng/năng lực hiện có |

### Nhóm Registry (danh bạ tài nguyên)

| Tool | Mô tả |
|------|-------|
| `brain_agents` | Liệt kê/tra cứu agent |
| `brain_clis` | Liệt kê/tra cứu CLI đã đăng ký |
| `brain_mcp_registry` | Truy vấn registry MCP server |
| `brain_tools_registry` | Truy vấn registry tool |
| `brain_runtime_registry` | Truy vấn registry runtime |

### Nhóm Hub (proxy MCP Hub)

| Tool | Mô tả |
|------|-------|
| `hub_call_tool` | Gọi một tool qua MCP Hub |
| `hub_batch_call` | Gọi nhiều tool qua Hub trong một lần |
| `hub_servers` | Liệt kê server do Hub quản lý |
| `hub_server_status` | Trạng thái của một server trong Hub |
| `hub_tools` | Liệt kê tool khả dụng qua Hub |
| `hub_sync_registry` | Đồng bộ registry với Hub |

#### Ví dụ: gọi tool MCP (minh họa JSON-RPC qua /mcp/message)

```bash
curl -s -X POST "$TIBRAIN/mcp/message" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "query_memory",
      "arguments": { "query": "lỗi build CGO", "top_k": 5 }
    }
  }'
```

> Trong luồng MCP đầy đủ, client thường đọc kết quả qua kênh SSE (`/mcp/sse`) sau khi gửi tool-call tới `/mcp/message`.

---

## Ghi chú cuối

- Tất cả path là tương đối so với base URL `http://localhost:3005`.
- Với các endpoint `GET` có tham số, hãy dùng query string (ví dụ `?id=...`, `?name=...`, `?q=...`); tên tham số cụ thể có thể khác — tham chiếu implement nếu cần chính xác.
- Payload/response minh họa trong tài liệu này nhằm mục đích tham khảo cấu trúc, không phải hợp đồng API chính thức.

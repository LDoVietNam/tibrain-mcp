# Troubleshooting Guide (TiBrain)

**Phiên bản:** 2.3.0  
**Cập nhật:** 2026-09-12

## MCP Auth Issues

### MCP Auth Bypass (InvalidAuth test return 200 thay vì 401)

| Error | Cause | Fix |
|-------|-------|-----|
| `InvalidAuth` trả về 200 | Hai nguyên nhân: (1) `main.go` dùng `config.Default()` mà không gọi `ApplyEnv()` → `TIBRAIN_MCP_BEARER_TOKEN` env không load → token="" → server fail-closed nhưng config sai; (2) Zombie process chiếm IPv6 `[::]:3005` → curl resolve `localhost`→`::1` → request trúng server cũ bypass auth | (1) Thêm `cfg.ApplyEnv()` sau `config.Default()` trong `main.go`; (2) Kill zombie processes bằng `netstat -tlnp \| grep 3005` → identify PID → `kill -9 <pid>` |

**Verify:**
```bash
# Missing token → 401
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:3005/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
# Expected: 401

# Valid token → 200
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:3005/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <valid_token>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
# Expected: 200
```

**Root cause analysis:**
1. `internal/config/config.go:183` — method `ApplyEnv()` export từ `applyEnv()`, load env vars (`TIBRAIN_PORT`, `TIBRAIN_HOST`, `TIBRAIN_MCP_BEARER_TOKEN`)
2. `main.go:31-32` — `cfg := config.Default()` + `cfg.ApplyEnv()` 
3. `internal/mcp/gateway.go:220-226` — `HandleStreamableHTTP` wrap handler với `m.auth.Wrap()`, 401 fail-closed
4. Zombie processes (PIDs 43612, 32884, 39201) chiếm IPv6 `[::]:3005` — kill trước khi start server mới

Liên quan: [[tibrain-auth-bypass-fix-2026-09]] [[tibrain-port-3005-conflicts]]

---

## MCP Server Issues

### Connection Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `ECONNREFUSED 3005` | Server not running | `npm run start` trong TiBrain directory |
| SSE connection drops | Network/timeout | Check firewall, increase timeout |
| `401 Unauthorized` | Auth fail-closed | Set `TIBRAIN_MCP_BEARER_TOKEN` env, verify token |
| `-32001 missing bearer token` | Request thiếu Authorization header | Thêm `-H "Authorization: Bearer <token>"` |
| `-32001 invalid bearer token` | Token sai | Verify token trong `router.env` hoặc process env |

### MCP Tools Not Found

| Error | Cause | Fix |
|-------|-------|-----|
| `-32601 Method not found` | Tool name sai | Verify tool name qua MCP `tools/list` |
| Session ID missing | SDK stateful yêu cầu `Mcp-Session-Id` header | Thêm header sau handshake |

---

## Build Issues (Go)

### Go 1.26.6 + modernc.org/sqlite Compiler Bug

| Error | Cause | Fix |
|-------|-------|-----|
| `signal 0xc000001d` / `SIGILL` / `unexpected return pc` | Go 1.26.x + modernc.org/sqlite SSA/DSE pass race with GC | `GOGC=off GOWORK=off go build -p=1` — disable GC, workspace, force single-threaded compile |

**Verify:**
```bash
export GOWORK=off && GOGC=off go build -p=1 ./...
```

### go.work Interference

| Error | Cause | Fix |
|-------|-------|-----|
| `directory prefix . does not contain modules listed in go.work` | Workspace mode active | `export GOWORK=off` trước khi build/test |

---

## Health Check

```bash
# TiBrain health
curl -s http://localhost:3005/health

# MCP endpoint (Streamable HTTP)
curl -s -X POST http://localhost:3005/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
```

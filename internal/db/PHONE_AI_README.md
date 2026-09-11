# 📱 Phone AI — TiBrain MCP Integration

Chạy AI models trên điện thoại Android (Termux) và tích hợp với TiBrain MCP.

## 🏗️ Kiến trúc

```
┌─────────────────────────────────────────────────────────────┐
│                        MẠNG LAN                            │
│                                                             │
│  PC Windows (TiBrain)          Android Phone (Termux)       │
│  ──────────────────────        ────────────────────         │
│                                                             │
│  TiBrain Core :1810            Ollama :11434                │
│    ├── REST API                ├── AI Service :5000         │
│    ├── MCP SSE :1810/mcp       │   (FastAPI: REST + WS)     │
│    ├── MCP Hub :1840           └── MCP Server :5100         │
│    │     └──► phone-ai-mcp ─────────► (SSH/HTTP)           │
│    ├── /phone-ai/* proxy       SSH :8022                    │
│    └── CLI Registry                                        │
│                                                             │
│  CLI Clients (LAN)                                          │
│    ├── curl ──► http://phone:5000/v1/chat/completions       │
│    ├── ws    ──► ws://phone:5000/ws/chat                    │
│    └── phone_ai_cli.py                                      │
└─────────────────────────────────────────────────────────────┘
```

## 📁 Cấu trúc Files

| File | Mô tả |
|------|-------|
| `scripts/phone_ai_service.py` | FastAPI server trên Termux (REST + WebSocket) |
| `scripts/phone_ai_mcp_server.py` | MCP server trên Termux (SSE + JSON-RPC) |
| `scripts/phone_ai_cli.py` | CLI client gọi phone AI từ LAN |
| `scripts/termux_setup.sh` | Script cài đặt tự động trên Termux |
| `phone_ai_integration.go` | Go handler tích hợp phone AI vào TiBrain |
| `mcp/sub-mcp-android.json` | MCP Hub config cho phone AI server |

---

## 🚀 Quick Start

### Bước 1: Trên điện thoại (Termux)

```bash
# Cài đặt từ F-Droid, sau đó:
pkg update && pkg upgrade
pkg install python python-pip git openssh
pip install fastapi uvicorn httpx websockets psutil

# Copy scripts vào ~/phone-ai/
mkdir -p ~/phone-ai
# (Copy scripts/phone_ai_service.py và phone_ai_mcp_server.py từ PC)

# Chạy AI service
cd ~/phone-ai
python phone_ai_service.py --port 5000 --ollama-url http://localhost:11434
```

### Bước 2: Cài Ollama

```bash
# Cách 1: Dùng proot-distro (khuyến nghị)
pkg install proot-distro
proot-distro install ubuntu
proot-distro login ubuntu
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3.2:1b
ollama serve &

# Cách 2: Dùng termux-ollama (nếu có)
```

### Bước 3: Trên PC (TiBrain)

```bash
# Khởi động TiBrain
cd Z:\01_PROJECTS\apps\products\tibrain
.\tibrain.exe --port 1810

# Kết nối đến phone AI
curl -X POST http://localhost:1810/phone-ai/connect \
  -H "Content-Type: application/json" \
  -d '{"url":"http://192.168.1.100:5000"}'

# Kiểm tra kết nối
curl http://localhost:1810/phone-ai/status
```

### Bước 4: Chat thử

```bash
# Trực tiếp đến phone
curl -X POST http://192.168.1.100:5000/api/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Xin chào, bạn là ai?"}'

# Qua TiBrain proxy
curl -X POST http://localhost:1810/phone-ai/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Hello!","model":"llama3.2:1b"}'

# Dùng CLI tool
python scripts/phone_ai_cli.py chat "Viết Python function tính Fibonacci" \
  --phone http://192.168.1.100:5000 --stream
```

---

## 🌐 API Reference

### Phone AI Service (`:5000`)

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| GET | `/health` | Health check + battery, CPU, memory |
| GET | `/v1/models` | Danh sách AI models |
| POST | `/v1/chat/completions` | OpenAI-compatible chat (stream/non-stream) |
| POST | `/api/chat` | Chat đơn giản (prompt → response) |
| WS | `/ws/chat` | WebSocket chat streaming |
| POST | `/api/v1/register` | Đăng ký với TiBrain |

### MCP Server (`:5100`)

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| GET | `/health` | MCP server health |
| POST | `/mcp` | JSON-RPC: initialize, tools/list, tools/call |
| GET | `/sse` | SSE transport cho MCP |

### TiBrain Proxy (`:1810`)

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| GET | `/phone-ai/status` | Trạng thái phone AI |
| POST | `/phone-ai/chat` | Chat qua phone AI |
| GET | `/phone-ai/models` | Danh sách models |
| POST | `/phone-ai/connect` | Kết nối/refresh phone AI |

---

## 🛠️ CLI Client Usage

```bash
# Kiểm tra sức khỏe
python scripts/phone_ai_cli.py health --phone http://192.168.1.100:5000

# Liệt kê models
python scripts/phone_ai_cli.py models --phone http://192.168.1.100:5000

# Chat
python scripts/phone_ai_cli.py chat "Xin chào" --phone http://192.168.1.100:5000

# Chat với stream
python scripts/phone_ai_cli.py chat "Kể chuyện cười" --phone http://192.168.1.100:5000 --stream

# Chat với system prompt
python scripts/phone_ai_cli.py chat "Viết code" -s "Bạn là expert Go" -m llama3.2:3b

# Xem trạng thái chi tiết
python scripts/phone_ai_cli.py status --phone http://192.168.1.100:5000

# Quét mạng tìm phone
python scripts/phone_ai_cli.py scan

# Đăng ký với TiBrain
python scripts/phone_ai_cli.py register --phone http://192.168.1.100:5000 --tibrain http://localhost:1810

# Chat qua MCP Hub
python scripts/phone_ai_cli.py mcp-chat "Hello" --hub http://localhost:1840

# Liệt kê MCP tools
python scripts/phone_ai_cli.py mcp-tools --hub http://localhost:1840

# Lưu cấu hình
python scripts/phone_ai_cli.py set --phone http://192.168.1.100:5000
```

---

## ⚙️ Environment Variables

```bash
# Trên PC
export PHONE_AI_URL=http://192.168.1.100:5000
export TIBRAIN_URL=http://localhost:1810
export MCP_HUB_URL=http://localhost:1840
export AI_DEFAULT_MODEL=llama3.2:1b

# Trên điện thoại (Termux)
export OLLAMA_URL=http://localhost:11434
export AI_SERVICE_PORT=5000
export AI_DEFAULT_MODEL=llama3.2:1b
```

---

## 🔧 TiBrain MCP Target Aliases

Sau khi kết nối, có thể dùng các alias sau trong MCP tools:

| Alias | Server Name | Mô tả |
|-------|------------|-------|
| `phone` | `phone-ai-mcp` | Phone AI |
| `phone-ai` | `phone-ai-mcp` | Phone AI |
| `android-ai` | `phone-ai-mcp` | Phone AI |
| `termux` | `phone-ai-mcp` | Phone AI |

Ví dụ gọi từ MCP:
```json
{
  "target": "phone-ai",
  "name": "chat_completion",
  "arguments": {
    "messages": [{"role": "user", "content": "Hello"}]
  }
}
```

---

## 📊 Models Khuyến Nghị Cho Android

| Model | Tham số | RAM | Tốc độ | Ghi chú |
|-------|---------|-----|--------|---------|
| `llama3.2:1b` | 1.1B | ~1GB | ⚡ Nhanh | Khuyến nghị |
| `llama3.2:3b` | 3.2B | ~2.5GB | 🐢 Trung bình | Phone 8GB RAM+ |
| `phi3:mini` | 3.8B | ~2.5GB | 🐢 Trung bình | Tốt cho code |
| `qwen2.5:1.5b` | 1.5B | ~1.2GB | ⚡ Nhanh | Đa ngôn ngữ |
| `gemma2:2b` | 2.6B | ~2GB | 🐢 Trung bình | Google |

---

## 🔒 Security Notes

1. **Chỉ dùng trong LAN tin cậy** — không expose phone AI ra internet
2. **SSH key authentication** — dùng key pair thay vì password
3. **Termux từ F-Droid** — không dùng Google Play (deprecated)
4. **Thermal protection** — phone sẽ nóng khi chạy LLM, monitor nhiệt độ
5. **Auto-start** — dùng Termux:Boot app để tự động start service

---

## 🧪 Testing

```bash
# 1. Kiểm tra kết nối cơ bản
curl http://192.168.1.100:5000/health

# 2. Kiểm tra Ollama
curl http://192.168.1.100:11434/api/tags

# 3. Kiểm tra WebSocket
pip install websocket-client
python -c "
import websocket
ws = websocket.create_connection('ws://192.168.1.100:5000/ws/chat')
ws.send('{\"messages\":[{\"role\":\"user\",\"content\":\"Hi\"}]}')
print(ws.recv())
ws.close()
"

# 4. Kiểm tra MCP
curl -X POST http://192.168.1.100:5100/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# 5. Kiểm tra qua TiBrain
curl http://localhost:1810/phone-ai/status
```

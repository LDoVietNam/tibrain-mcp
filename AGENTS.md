# TiBrain - Knowledge Service & Control Plane

**Phiên bản:** 1.0.0  
**Cập nhật:** 2026-07-27  

---

## 1. Giới thiệu

TiBrain là dịch vụ **knowledge service** và **control plane** trung tâm trong hệ sinh thái Ti. Nó cung cấp:

- **REST API** để truy xuất kiến thức, công cụ và sổ đăng ký.
- **MCP endpoint** làm cầu nối giao thức Model Context Protocol.
- **RAG (Retrieval-Augmented Generation)** để truy xuấtKnowledge Base thông minh.
- **Agent registry** đăng ký và quản lý các agent trong hệ thống.

TiBrain hoạt động như trung tâm điều phối, cho phép các thành phần khác (OmniRoute, Router Agent, v.v.) tương tác qua một giao diện thống nhất.

---

## 2. Kết nối

| Phương thức | Endpoint                               | Mô tả                                     |
|------------|----------------------------------------|-------------------------------------------|
| **REST API**   | `http://localhost:3005/api/*`         | Truy cập knowledge, RAG, registry, tools. |
| **MCP**        | `http://localhost:3005/mcp`           | Cầu nối giao thức Model Context Protocol. |
| **Health Check**| `http://localhost:3005/api/health`    | Kiểm tra tình trạng hoạt động của dịch vụ. |

---

## 3. Xây dựng (Build)

```powershell
# Chuyển vào thư mục dự án
cd Z:\01_PROJECTS\apps\tibrain

# Biên dịch bằng Go
go build -o tibrain.exe main.go

# Hoặc sử dụng script hỗ trợ
.\build.ps1
```

---

## 4. Khởi động (Run)

```powershell
# Sử dụng script khởi động
.\start-tibrain.ps1

# Hoặc chạy trực tiếp chỉ định port (mặc định 3005)
.\tibrain.exe --port 3005
```

---

## 5. Cấu trúc dự án

```
tibrain/
├── main.go              # Điểm vào ứng dụng
├── api_server.go        # Máy chủ REST API
├── mcp_hub_client.go    # MCP client (kết nối tới hub)
├── config.yaml          # Tập tin cấu hình (cổng, kết nối DB, v.v.)
├── build.ps1            # Script biên dịch tự động
├── start-tibrain.ps1    # Script khởi động dịch vụ
└── AGENTS.md            # Tài liệu này

cli/
├── commands/
│   ├── go-review.md     # Review code (vet, lint, format)
│   ├── go-build.md      # Build binary
│   └── go-test.md       # Run tests với race detection
├── workflows/
│   └── go-dev.js        # Workflow gộp review + build + test
└── skills/
    └── go-dev-workflows.md  # Skill tích hợp cho Go development
```

---

## 6. Go Development Workflow

Workflow `go-dev` tự động thực hiện 3 bước:

```bash
# Chạy workflow đầy đủ
node cli/workflows/go-dev.js --targetDir=.

# Hoặc chạy từng bước riêng
node cli/workflows/go-dev.js --step=review --targetDir=.
node cli/workflows/go-dev.js --step=build --targetDir=.
node cli/workflows/go-dev.js --step=test --targetDir=.
```

**Handoff Logging**: Mỗi step ghi log vào `.mimocode/handoff/`:
- `plan-{timestamp}.md`: Kế hoạch thực hiện
- `review-{timestamp}.md`: Kết quả review (vet, lint, format)
- `build-{timestamp}.md`: Kết quả build
- `test-{timestamp}.md`: Kết quả test

**Auto-skip Logic**:
- Build sẽ bị bỏ qua nếu review phát hiện lỗi
- Test sẽ bị bỏ qua nếu build thất bại

---

## 7. API Endpoints chi tiết

| Endpoint                           | Phương thức | Mô tả                                                                 |
|------------------------------------|------------|-----------------------------------------------------------------------|
| `/api/health`                      | GET        | Trả về trạng thái sức khỏe (OK / lỗi).                                 |
| `/api/status`                      | GET        | Cung cấp thông tin hệ thống: version, uptime, resource usage.          |
| `/api/tools`                       | GET        | Danh sách công cụ đã đăng ký (name, description, version).             |
| `/api/agents`                      | GET        | Danh sách agent đã đăng ký (id, name, type, endpoint, status).         |
| `/api/knowledge`                   | GET        | Thông tin tổng quan về Knowledge Base (số lượng document, chỉ mục).   |
| `/api/rag/query`                   | POST       | Truy vấn RAG: body `{ "query": "<câu hỏi>", "top_k": 5 }`. Trả về các đoạn văn bản liên quan. |
| `/api/v2/retrieve`                 | POST       | Truy vấn trực tiếp vào Knowledge Store (tương tự RAG nhưng không có bước generation). |
| `/api/v2/runtime/registry`         | GET        | Registry thời gian chạy: danh sách các instance agent đang hoạt động.   |

> **Lưu ý:** Không kết nối trực tiếp tới OmniRoute (`:1807`) hoặc Router Agent (`:1806`). Todas các tương tác giữa các thành phần phải qua **Tirouter Gateway** tại cổng `:3004`.

---

## 8. Cấu hình (Configuration)

Tập tin `config.yaml` chứa cáckhóa chính:

```yaml
server:
  port: 3005               # Port REST API
  host: "0.0.0.0"

mcp:
  endpoint: "http://localhost:3005/mcp"
  timeout: "10s"

knowledge:
  path: "./data/kb"        # Thư mục lưu trữ các tài liệu knowledge
  index_type: "sqlite"     # Hoặc "bolt", "memory"

logging:
  level: "info"
  format: "json"
```

Các tham số có thể được ghi đè qua biến môi trường hoặc tham số dòng lệnh (`--port`, `--config`, …).

---

## 9. Quy trình phát triển

1. **Fork** repository và tạo nhánh tính năng (`git checkout -b feature/ten-feature`).
2. Tuân thủ **Conventional Commits** cho thông điệp commit.
3. Viết **unit test** cho mọi hàm mới (`go test ./...`).
4. Chạy **lint** (`golangci-lint run`) trước khi tạo pull request.
5. Đảm bảo **docstrings** và **godoc** được cập nhật.
6. Gửi pull request và chờ review từ đội ngũ maintainer.

---

## 10. Đóng góp (Contributing)

Chúng tôi chào mừng sự đóng góp từ cộng đồng. Để đóng góp:

- Báo cáo vấn đề qua **Issues**.
- Đề xuất tính năng hoặc cải tiến qua **Pull Request**.
- Tuân thủ [CODE_OF_CONDUCT.md] (nếu có) và [CONTRIBUTING.md].

---

## 11. Giấy phép (License)

Dự án được phát hành dưới giấy phép **MIT** – xem tệp `LICENSE` để biết chi tiết.

---

*Tài liệu này được tạo tự động và có thể được cập nhật khi có thay đổi về phiên bản hoặc cấu trúc dự án.*
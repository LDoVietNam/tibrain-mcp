# DESIGN.md — Kiến trúc DB Layer tập trung & Migration Framework (TiBrain)

> Prompt Intelligence dùng chung DB layer và migration framework trong tài liệu này. Contract chức năng nằm tại [`docs/PROMPT_INTELLIGENCE_CONTRACT.md`](./docs/PROMPT_INTELLIGENCE_CONTRACT.md); TiRouter chịu trách nhiệm request mutation, TiBrain chịu trách nhiệm registry/retrieval/feedback.

> Tác giả: ArchitectAgent (Kilo Code Agent Team)
> Phạm vi: Thiết kế interface / structure / quyết định thiết kế. **KHÔNG viết code Go implement.**
> Trạng thái: Dự thảo để review trước khi implement.

---

## 0. Tóm tắt vấn đề (tl;dr)

Hiện tại TiBrain (single binary, port 1810) tự hào là "Hub", nhưng thực tế **~10 file tự mở connection SQLite riêng** và tự chạy `CREATE TABLE IF NOT EXISTS`. Điều này sinh ra 4 rủi ro hệ thống:

1. **Phân tán schema** — schema nằm rải rác ở `main.go`, `rag_system.go`, `schema_update.go`, `auto_learning_mechanism.go`, `ecosystem_integration.go`, `predictive_maintenance.go`, `intelligent_indexing_system.go`, `cross_reference_intelligence.go`, `deep_data_structure_analysis.go`, `cloudflare_db.go`.
2. **Nhiều connection riêng** — `auto_learning_mechanism.go`, `ecosystem_integration.go`, `predictive_maintenance.go`, `intelligent_indexing_system.go`, `cross_reference_intelligence.go`, `deep_data_structure_analysis.go` gọi `sql.Open("sqlite3", dbPath)` riêng → race condition / lock contention trên cùng 1 file `.db`.
3. **2 driver song song** — Hub dùng `modernc.org/sqlite` (pure-Go), còn các subsystem mở `"sqlite3"` (CGO `mattn/go-sqlite3`). Cùng 1 file mà 2 driver khác nhau là **undefined behavior**.
4. **Không có migration system** — chỉ `CREATE TABLE IF NOT EXISTS` lúc startup + `rag_system.go` có `ensureRAGSchemaColumns()` ALTER thủ công. Không có version, không rollback, không audit.

**Mục tiêu thiết kế:** một connection `*sql.DB` duy nhất, một driver duy nhất, schema tập trung trong module `internal/db`, migration versioned, và một interface `DB` cho mọi module thay vì tự mở connection.

---

## 1. Kiến trúc DB Layer

### 1.1 Nguyên tắc cốt lõi

- **Single shared `*sql.DB`**: Hub sở hữu duy nhất một connection pool tới 1 file SQLite. Mọi subsystem nhận `*sql.DB` (hoặc interface `DB` — xem §5) qua dependency injection từ Hub. Không ai được gọi `sql.Open` thêm lần nào nữa (trừ subsystem thực sự dùng DB file riêng biệt, ví dụ cache analytics read-only — lúc đó phải là file khác, không phải Hub db).
- **Single driver**: Chọn **`modernc.org/sqlite`** (pure-Go, không CGO) làm driver duy nhất. Bỏ `mattn/go-sqlite3` khỏi dependency chính.
  - *Lý do*: tránh CGO (build cross-platform dễ, không cần GCC), và đồng nhất với Hub hiện tại. Các subsystem đang mở `"sqlite3"` sẽ chuyển sang dùng driver name `"sqlite"` (tên register của modernc) hoặc truyền `*sql.DB` đã mở sẵn.
- **Schema tập trung**: toàn bộ DDL nằm trong `internal/db/migrations/*.sql`. Các file `*.go` cũ **không còn chứa DDL** — chúng gọi `db.ApplyMigrations(...)` tại startup.

### 1.2 Sơ đồ module (text)

```
tibrain/
├── main.go                      # Hub: khởi tạo DB, ApplyMigrations, inject *sql.DB vào subsystems
├── api_server.go
├── rag_system.go                # KHÔNG còn initRAGSchema(); dùng db.Schema().RAGDocuments...
├── schema_update.go             # logic schema cũ → chuyển thành data/seed, KHÔNG còn CREATE TABLE
├── async_writer.go              # refactor thành db.AsyncWriter (§5.4)
├── ...
└── internal/
    ├── db/                      # ★ MỚI — DB layer tập trung
    │   ├── db.go                # NewHubDB(path) → *sql.DB + set PRAGMA (§6), OpenShared
    │   ├── db_interface.go       # interface DB (§5)
    │   ├── migrate.go           # ApplyMigrations(db, fsys), version tracking (§2, §4)
    │   ├── migrations/          # ★ DDL versioned (§2.2)
    │   │   ├── 0001_hub_init.up.sql
    │   │   ├── 0001_hub_init.down.sql
    │   │   ├── 0002_rag.up.sql
    │   │   ├── 0002_rag.down.sql
    │   │   ├── 0003_agents.up.sql
    │   │   ├── ...
    │   │   └── 0010_ensure_columns.up.sql   # phần còn lại của ensureRAGSchemaColumns (§2.3)
    │   └── embedded.go          # //go:embed migrations → embed.FS (§4)
    └── memory/                  # ★ MỚI (đang là blocker build) — CognitiveMemoryManager
        └── memory.go            # implement interface DB của internal/db
```

> Lưu ý: `main.go` hiện `import "github.com/ti/router/tibrain/internal/db"` và `internal/memory` (dòng 26-28). Hai package này **chưa tồn tại** → blocker build. Thiết kế này định nghĩa contract để implement sau.

### 1.3 Quy tắc kết nối (connection contract)

- Hub gọi `db.OpenHub(path)` **đúng 1 lần** lúc startup.
- `OpenHub` trả về `*sql.DB` đã `SetMaxOpenConns(1)` (SQLite single-writer — quan trọng) + `SetConnMaxLifetime(0)`.
- Mọi subsystem nhận `*sql.DB` qua constructor hoặc field, KHÔNG tự mở. Ví dụ: `NewAutoLearningMechanism(db *sql.DB)` thay vì `sql.Open` bên trong.
- `AsyncWriter` (§5.4) cũng nhận cùng `*sql.DB` đó.

---

## 2. Migration Framework

### 2.1 Nguyên tắc

- **Versioned migrations** với bảng theo dõi `schema_migrations`.
- **Hỗ trợ up/down** (khuyên), nhưng vận hành production mặc định chạy up-only. Down dùng cho dev/test rollback.
- Mỗi migration = 1 cặp file `.up.sql` + `.down.sql` (hoặc gộp vào 1 file với comment phân tách — khuyên tách riêng cho rõ).
- `ApplyMigrations` chạy trong 1 transaction, idempotent (dựa vào `schema_migrations` đã áp dụng).

### 2.2 Cấu trúc thư mục & đặt tên

```
internal/db/migrations/
├── 0001_hub_init.up.sql       # cli_registry, global_handoffs, mcp_registry, tool_registry,
│                               #   tool_usage_log, model_performance_stats
├── 0001_hub_init.down.sql
├── 0002_rag.up.sql            # rag_documents + FTS5 rag_documents_fts + triggers,
│                               #   rag_knowledge_bases (DUY NHẤT), rag_query_history,
│                               #   rag_vector_index, rag_analytics, rag_feedback,
│                               #   code_graph_nodes, code_graph_edges, rag_runtime_traces,
│                               #   brain_pattern_candidates, cli_context, cli_query_cache,
│                               #   cli_help_index, cli_preferences
├── 0002_rag.down.sql
├── 0003_agents.up.sql         # agent_registry, cross_brain_communication, orchestration_log,
│                               #   agent_performance, task_memory, decision_memory,
│                               #   user_preferences, agent_performance_enhanced,
│                               #   routing_decision_log, rtk_rules, rtk_savings_log
├── 0003_agents.down.sql
├── 0004_learning.up.sql       # query_patterns (DUY NHẤT - xem §3), content_gaps,
│                               #   quality_scores, learning_metrics (DUY NHẤT), recommendations,
│                               #   user_interactions, learning_feedback
├── 0004_learning.down.sql
├── 0005_ecosystem.up.sql      # brain_systems, sync_status, sync_events, api_endpoints,
│                               #   external_sources, federation_rules, active_users,
│                               #   collaboration_rooms, change_events
├── 0005_ecosystem.down.sql
├── 0006_predictive.up.sql     # performance_metrics, storage_metrics, health_metrics,
│                               #   predictions, maintenance_tasks, alerts
├── 0006_predictive.down.sql
├── 0007_indexing.up.sql       # content_priority, content_classification, reindexing_tasks
├── 0007_indexing.down.sql
├── 0008_crossref.up.sql       # document_links, document_versions, dependencies,
│                               #   impact_analysis, cross_reference_metrics
├── 0008_crossref.down.sql
├── 0009_cloudflare.up.sql     # cloudflare_tokens
├── 0009_cloudflare.down.sql
└── 0010_ensure_columns.up.sql # các cột新增 từ ensureRAGSchemaColumns() cũ (ALTER TABLE ADD COLUMN)
    └── 0010_ensure_columns.down.sql
```

### 2.3 Xử lý backward-compat với `init*Schema` cũ

Chiến lược **consolidate + keep-ensure**:

1. **Consolidate**: Mọi `CREATE TABLE IF NOT EXISTS` cũ được gom vào các migration khởi tạo (0001–0009). Khi refactor, xóa DDL khỏi `main.go` / `rag_system.go` / `schema_update.go` / v.v., thay bằng gọi `db.ApplyMigrations`.
2. **Keep-ensure cho cột新增**: `rag_system.go:ensureRAGSchemaColumns()` (ALTER thủ công thêm cột) → chuyển thành migration `0010_ensure_columns.up.sql` (toàn `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`). Giữ pattern "add column nếu thiếu" vì nó an toàn cho upgrade sau này.
3. **Data preservation**: vì hiện tại production đã có bảng (tạo bởi `CREATE TABLE IF NOT EXISTS` cũ), migration **phải dùng `CREATE TABLE IF NOT EXISTS`** trong các file `.up.sql` thay vì `CREATE TABLE` tuyệt đối — để không drop dữ liệu cũ khi chạy lần đầu. Sau khi ổn định, các migration mới (0011+) dùng `CREATE TABLE` nghiêm ngặt.
4. **Drift detection**: `ApplyMigrations` so sánh checksum của mỗi file `.up.sql` với `schema_migrations.checksum`; nếu nội dung thay đổi mà version không tăng → báo lỗi (cảnh báo schema drift).

### 2.4 Bảng theo dõi `schema_migrations`

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    checksum    TEXT NOT NULL,        -- hash của file .up.sql để phát hiện drift
    direction   TEXT NOT NULL DEFAULT 'up'
);
```

`ApplyMigrations`:
- Đọc tất cả `*.up.sql` từ `embed.FS`, sort theo version.
- Với mỗi version chưa có trong `schema_migrations`: mở transaction, chạy DDL, insert `schema_migrations`, commit.
- Nếu lỗi: rollback transaction, dừng (fail-fast startup).

---

## 3. Giải quyết xung đột trùng tên bảng

Khảo sát thực tế xác nhận **xung đột thật**, không chỉ trùng tên:

| Tên bảng | Nơi định nghĩa | Schema khác biệt |
|---|---|---|
| `rag_knowledge_bases` | `rag_system.go:248` & `schema_update.go:31` | Gần giống nhau → chỉ là **trùng lặp ownership** (cả 2 đều `IF NOT EXISTS` → 1 cái vô hiệu). Giữ 1 bản tại `0002_rag.up.sql`. |
| `query_patterns` | `auto_learning_mechanism.go:152` / `predictive_maintenance.go:297` / `intelligent_indexing_system.go:142` | **3 schema KHÁC NHAU** (cột `pattern` vs `query`, `response_time` vs `avg_time`). Hiện tại ai chạy trước thắng, 2 còn lại silently no-op → **mất data/sai cột**. |
| `learning_metrics` | `auto_learning_mechanism.go:178` / `intelligent_indexing_system.go:118` | 2 schema khác nhau. |

### 3.1 Quyết định: namespace/prefix (KHÔNG gộp mù quáng)

Gộp mù quáng 3 `query_patterns` thành 1 sẽ phá logic của từng subsystem. Thay vào đó **đổi tên thành prefix theo subsystem**:

| Tên cũ | Tên mới (chuẩn hóa) | Module sở hữu |
|---|---|---|
| `query_patterns` (auto_learning) | `learning_query_patterns` | `0004_learning` |
| `query_patterns` (predictive) | `predictive_query_patterns` | `0006_predictive` |
| `query_patterns` (indexing) | `indexing_query_patterns` | `0007_indexing` |
| `learning_metrics` (auto_learning) | `learning_metrics` (giữ) | `0004_learning` |
| `learning_metrics` (indexing) | `indexing_learning_metrics` | `0007_indexing` |
| `rag_knowledge_bases` | `rag_knowledge_bases` (giữ 1 bản) | `0002_rag` |

**Quy tắc đặt tên tương lai:** `<subsystem>_<entity>` (snake_case, prefix = tên module). Áp dụng retroactive cho các bảng đã trùng. Các bảng chưa trùng (vd `rag_documents`, `agent_registry`) giữ nguyên vì prefix đã ngầm đúng.

> Lưu ý migration: vì bảng cũ (`query_patterns`) đã tồn tại trên prod, bước chuyển đổi an toàn là: tạo bảng mới có prefix, sau đó `INSERT INTO <mới> SELECT ... FROM query_patterns WHERE ...` (map cột tương thích) trong 1 migration riêng `0011_rename_collisions.up.sql`, rồi để bảng cũ lại (deprecated) hoặc `DROP` sau vài version khi confirm không còn reader. **Không DROP ngay** để tránh break subsystem chưa refactor xong.

---

## 4. Lựa chọn Migration Approach (so sánh)

| # | Cách | Ưu điểm | Nhược điểm | Phù hợp? |
|---|---|---|---|---|
| A | **Tự viết lightweight + `embed.FS`** | Không thêm dependency nặng; full control; DDL nằm trong binary (deploy đơn giản); checksum drift detection dễ thêm | Phải tự maintain (~150 dòng Go); không có CLI migrate riêng cho ngoài-code | ✅ **Khuyên dùng** |
| B | **golang-migrate** | Chuẩn industry; CLI mạnh; hỗ trợ nhiều DB; community lớn | Thêm dependency lớn; cú pháp `%Down` trong comment; cần driver source适配 modernc; overkill cho 1 SQLite file | Khả dĩ nếu muốn chuẩn hóa team |
| C | **Atlas (ariga)** | Schema-as-code, diff tự động, versioned | Nặng; HCL hoặc SQL; phức tạp hơn mức cần; CGO/build weight | Không cần thiết cho SQLite đơn |
| D | **Giữ `CREATE TABLE IF NOT EXISTS` + ALTER thủ công** | Không thay đổi code nhiều | Chính là vấn đề hiện tại → **loại** | ❌ |

### 4.1 Đề xuất: **Cách A (tự viết lightweight dùng `embed.FS`)**

- Lý do: TiBrain là single binary, muốn giữ dependency tối giản và pure-Go (đã chọn modernc). `embed.FS` đóng gói `migrations/*.sql` vào binary → deploy không cần mang theo thư mục SQL.
- Signature hàm (interface mức design):

```
ApplyMigrations(db *sql.DB, fsys fs.FS, opts Options) error
// Options{ Dir string, AllowDown bool, Timeout time.Duration }
```

- `fsys` mặc định là `embed.FS` chứa `migrations/`. Có thể truyền `os.DirFS` khi dev muốn load từ disk (hot-reload SQL).

---

## 5. Interface `DB` (repository/query contract)

Thiết kế interface để các module dùng **thay vì tự mở connection**. Đồng thời giải quyết blocker `internal/memory` (Hub implement `memory.DB`).

### 5.1 Interface cốt lõi

```go
// internal/db/db_interface.go (thiết kế — không implement)
type DB interface {
    BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
    ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
    PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
    Close() error
}
```

> `*sql.DB` của stdlib **đã implement sẵn** interface này → Hub chỉ cần pass `*sql.DB` trực tiếp. `main.go` hiện đã viết `Hub.BeginTx/ExecContext/...` delegate sang `h.db` (dòng 291-310) → `*Hub` cũng thỏa interface. Giữ nguyên để `internal/memory` nhận `DB`.

### 5.2 Interface mở rộng (repository per domain — optional)

Khuyên tách repository theo domain để module không truyền raw SQL lung tung:

```go
type RAGRepository interface {
    DB // embed core
    SaveDocument(ctx, doc RAGDocument) error
    SearchFTS(ctx, query string, limit int) ([]RAGDocument, error)
    // ...
}
type AgentRepository interface { DB; /* agent_registry, task_memory... */ }
```

Mỗi subsystem định nghĩa repository riêng trong `internal/db/repo/`. Hub compose các repo từ 1 `*sql.DB`.

### 5.3 Async write queue (refactor `async_writer.go`)

`async_writer.go` hiện là `AsyncWriter{db *sql.DB, jobQueue chan, ...}`. Đưa vào `internal/db` và cho nó implement một interface write-only:

```go
type AsyncWriter interface {
    Enqueue(query string, args ...interface{})   // non-blocking, backpressure khi buffer đầy
    Flush() error                                  // đợi tất cả job xong
    Close() error                                   // flush + stop worker
}
```

- `AsyncWriter` nhận **cùng `*sql.DB` của Hub** (không `sql.Open` riêng).
- Dùng cho các write không cần kết quả ngay (analytics, logs, traces): `rag_analytics`, `rag_query_history`, `orchestration_log`, `user_interactions`, `performance_metrics`...
- `Flush()` gọi tại shutdown (`SIGINT/SIGTERM` handler trong `main.go`) để không mất log.

### 5.4 Quy tắc dùng sync vs async

| Loại write | Dùng |
|---|---|
| Cần kết quả ngay / transactional (tạo agent, register CLI) | `DB.ExecContext` (sync) |
| Log, metrics, analytics, traces | `AsyncWriter.Enqueue` (async) |
| Batch ingest tài liệu RAG | `DB` trong transaction hoặc `AsyncWriter` nếu容忍 delay |

---

## 6. PRAGMA chuẩn (đặt tại 1 chỗ duy nhất)

Tất cả PRAGMA đặt trong `db.OpenHub()` — **không** đặt rải rác. Hiện tại chỉ subsystem set WAL, Hub không set → sửa ngay.

```sql
-- internal/db/db.go: OpenHub thiết lập 1 lần qua Exec (mỗi connection)
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;      -- 5s thay vì default 0 (tránh SQLITE_BUSY)
PRAGMA foreign_keys=ON;        -- bật FK constraint (hiện tại tắt mặc định)
PRAGMA synchronous=NORMAL;     -- WAL-safe, nhanh hơn FULL
PRAGMA cache_size=-64000;      -- ~64MB cache (negative = KB)
PRAGMA temp_store=MEMORY;
```

- Vì modernc `SetMaxOpenConns(1)`, busy_timeout vẫn giữ để an toàn khi nhiều goroutine qua pool.
- `foreign_keys=ON` yêu cầu mọi migration dùng FK phải tự nhất quán (hiện đa số bảng chưa có FK → giữ OFF nếu chưa sẵn sàng, bật dần).
- Đặt PRAGMA trong hàm `func() (driver.Conn, error)` qua `sql.OpenDB(connector)` nếu cần áp dụng mỗi connection — nhưng với MaxOpenConns(1) set 1 lần sau Open là đủ.

---

## 7. Incremental Rollout (không break runtime)

Mục tiêu: refactor từng bước, giữ binary chạy được sau mỗi bước (continuous deployable).

| Bước | Việc làm | Rủi ro | Rollback |
|---|---|---|---|
| **1. Tạo `internal/db` + `OpenHub`** | Viết `OpenHub` (single connection, modernc, set PRAGMA §6). Chưa đổi subsystem. | Thấp | Revert package |
| **2. Viết `embed.FS` migrations 0001–0009** | Chép DDL cũ vào `.up.sql` dùng `CREATE TABLE IF NOT EXISTS`. Chưa xóa DDL cũ. | Thấp (idempotent) | Bảng đã tồn tại → no-op |
| **3. Gọi `ApplyMigrations` tại startup** | Trong `main.go`, sau `OpenHub` gọi `ApplyMigrations`. Giữ DDL cũ tạm thời (trùng → no-op). | Thấp | Tắt gọi migrate |
| **4. Xóa DDL khỏi các file `*.go`** | Xóa `CREATE TABLE` trong `main.go`, `rag_system.go`, `schema_update.go`... sau khi confirm migrate chạy ok (log version). | Trung bình | Git revert |
| **5. Đóng các `sql.Open` riêng** | `auto_learning_mechanism.go`, `ecosystem_integration.go`, `predictive_maintenance.go`, `intelligent_indexing_system.go`, `cross_reference_intelligence.go`, `deep_data_structure_analysis.go` → nhận `*sql.DB` từ Hub qua constructor. | Trung bình (cần sửa signature) | Giữ connection cũ tạm qua wrapper |
| **6. Giải xung đột trùng tên (§3)** | Tạo migration `0011_rename_collisions`: bảng prefix mới + copy data từ bảng cũ. | Cao (data) | Chưa DROP bảng cũ |
| **7. Refactor `async_writer.go` → `internal/db`** | `AsyncWriter` nhận `*sql.DB` Hub. | Thấp | Giữ file cũ |
| **8. Thống nhất driver** | Bỏ `mattn/go-sqlite3` khỏi go.mod; đảm bảo mọi nơi dùng driver name `"sqlite"`. | Trung bình (CGO build) | Re-add dependency |
| **9. Bật `foreign_keys` + checksum drift** | Sau ổn định, bật FK và drift detection trong `ApplyMigrations`. | Thấp | Tắt flag |

> **Quy tắc vàng**: mỗi bước deploy được 1 lần, quan sát log `schema_migrations` trước khi sang bước sau. Bước 6 (data rename) phải chạy trên bản backup DB.

---

## 8. Checklist quyết định thiết kế (Design Decisions)

- [x] **Single shared `*sql.DB`**, `SetMaxOpenConns(1)`, inject vào subsystem.
- [x] **Driver duy nhất: `modernc.org/sqlite`** (pure-Go), bỏ `mattn/go-sqlite3`.
- [x] **Schema tập trung** trong `internal/db/migrations/*.sql`, không còn DDL trong logic file.
- [x] **Migration versioned** + bảng `schema_migrations` (version, name, applied_at, checksum, direction).
- [x] **Up/down** support, vận hành mặc định up-only; checksum drift detection.
- [x] **Xung đột trùng tên** → prefix `<subsystem>_<entity>`; `query_patterns` tách thành 3 bảng; không DROP bảng cũ ngay.
- [x] **Interface `DB`** (BeginTx/ExecContext/QueryContext/QueryRowContext/PrepareContext/Close) cho `internal/memory` và repository.
- [x] **`AsyncWriter`** refactor vào `internal/db`, dùng chung connection, `Flush()` lúc shutdown.
- [x] **PRAGMA chuẩn** (WAL, busy_timeout=5000, foreign_keys, synchronous=NORMAL) tại `OpenHub` duy nhất.
- [x] **Approach: tự viết lightweight + `embed.FS`** (không dependency nặng).
- [x] **Rollout 9 bước** incremental, mỗi bước deploy được.

---

## 9. Mở rộng tương lai (extensibility)

- **Multi-tenant / sharding**: `OpenHub(path)` có thể param `path` từ config → dễ tách DB per-CLI sau này.
- **Read replica**: hiện SQLite đơn; nếu cần scale, `rag_cache.go` (Redis) đã là cache layer — giữ truy vấn nặng qua cache, không mở thêm connection.
- **Migration CLI**: từ `embed.FS` dễ thêm subcommand `tibrain migrate [up|down|status]` đọc cùng `fsys`.
- **Schema lint**: thêm bước CI kiểm tra tên bảng có prefix, không trùng, FK tham chiếu tồn tại.

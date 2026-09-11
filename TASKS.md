# TASKS.md — TiBrain Agent Team

> Professional task tracking. Status: `planned | in_progress | done | blocked`  
> Owner: `ArchitectAgent | BackendAgent | TestAgent | DocsAgent`

---

## 📊 Current Status Summary

| Phase | Progress | Blockers |
|-------|----------|--------|
| Foundation | ✅ Complete (T-001) | None |
| Documentation | ✅ Complete (T-002-T-004, contract doc) | None |
| Testing | ✅ Complete (T-006-T-007, PI-TB-007) | None |
| Migration | ✅ Complete (T-005, CGO removed) | None |
| Refactor | ✅ Complete (T-005c, T-011) | None |
| Prompt Intelligence | ✅ Code + tests complete | E2E cần `-tags integration` + plugin phía TiRouter |

---

## 🎯 Execution Waves

| Wave | Tasks | Status |
|------|-------|--------|
| **Wave 1** | T-002, T-003, T-004 (Docs) | ✅ Done |
| **Wave 2** | T-001, T-005a-b, T-006-010 | ✅ Done |
| **Wave 3** | T-005c, T-011 (Refactor) | ✅ Done |
| **Wave PI-1** | PI-TB-001 đến PI-TB-004 (schema, registry, preflight) | ✅ Done |
| **Wave PI-2** | PI-TB-005 đến PI-TB-009 (feedback, cascade, tests, E2E, ingest) | ✅ Code + tests |

---

## 📋 Task Tracker

| ID | Task | Owner | Status | Dependencies | Est. Effort |
|----|------|-------|--------|--------------|-------------|
| **T-001** | Scaffold `internal/` missing packages (db, memory, tools) | BackendAgent | ✅ done | — | 20 min |
| **T-002** | Rewrite `README.md` with architecture, API, run guide | DocsAgent | ✅ done | — | 15 min |
| **T-003** | Create `API.md` (60 endpoints + MCP tools) | DocsAgent | ✅ done | — | 20 min |
| **T-004** | Design unified schema & migration strategy | ArchitectAgent | ✅ done | — | 25 min |
| **T-005** | Implement unified DB + migration framework | BackendAgent | ✅ done | T-001, T-004 | 30 min |
| **T-005a** | Create `internal/db/migrations/001_init_schema.sql` | ArchitectAgent | ✅ done | T-001 | 10 min |
| **T-005b** | Implement `ApplyMigrations(db *sql.DB)` | BackendAgent | ✅ done | T-001, T-005a | 10 min |
| **T-005c** | Refactor root: use shared DB connection (remove CGO) | BackendAgent | ✅ done | T-005b, build_pass | 25 min |
| **T-006** | Unit test RAG core modules | TestAgent | ✅ done | T-001 | 45 min |
| **T-007** | Unit test knowledge/indexing modules | TestAgent | ✅ done | T-001 | 30 min |
| **T-008** | CI pipeline + golangci config | TestAgent | ✅ done | T-001 | 15 min |
| **T-009** | Enhance health/ready endpoints + metrics | BackendAgent | ✅ done | T-001 | 10 min |
| **T-009a** | Refactor RetrievalRouter: map + trie | BackendAgent | ✅ done | T-001 | 15 min |
| **T-010** | Fix build tags for integration tests | TestAgent | ✅ done | T-001 | 5 min |
| **T-011** | Split `package main` → `internal/rag`, `internal/knowledge` | ArchitectAgent + BackendAgent | ✅ done | T-005c | 45 min |
| **T-012** | Add AsyncWriter metrics (queue depth, flush latency) | BackendAgent | ✅ done | T-001 | 10 min |

---

## Prompt Intelligence — Work packages

Contract canonical nằm tại `docs/PROMPT_INTELLIGENCE_CONTRACT.md`. TiBrain chỉ cung cấp intelligence API nội bộ; việc sửa model request thuộc plugin TiRouter.

| ID | Task | Owner đề xuất | Status | Dependencies |
|---|---|---|---|---|
| **PI-TB-001** | Chốt contract API, lifecycle capsule, privacy và SLO | ArchitectAgent | ✅ done | — |
| **PI-TB-002** | Tạo migration cho `prompt_capsules`, versions, traces, feedback và evaluations | BackendAgent | ✅ done | T-005b, PI-TB-001 |
| **PI-TB-003** | Implement `internal/prompt` gồm repository, policy filter, registry và versioning | BackendAgent | ✅ done | PI-TB-002 |
| **PI-TB-004** | Implement `POST /api/v1/prompt/preflight` và `GET /catalog/version` | BackendAgent | ✅ done | PI-TB-003 |
| **PI-TB-005** | Implement feedback endpoint, dedup và AsyncWriter persistence | BackendAgent | ✅ done | PI-TB-002, PI-TB-003 |
| **PI-TB-006** | Implement retrieval cascade: rule → FTS/vector → reranker; LLM chỉ ở ambiguous path | BackendAgent | ✅ done | PI-TB-003 |
| **PI-TB-007** | Unit/contract test cho decision policy, privacy, schema mismatch, cache và latency | TestAgent | ✅ done (`policy_filter_test.go`, `privacy_latency_test.go`) | PI-TB-004, PI-TB-006 |
| **PI-TB-008** | E2E với plugin TiRouter và failure injection khi TiBrain restart/timeout | TestAgent | ✅ done (`e2e_integration_test.go`, `-tags integration`); plugin phía TiRouter chưa build | PI-TB-007, PI-TR-007 |
| **PI-TB-009** | Xây ingestion pipeline cho nguồn prompt ngoài: provenance, license review, dedup, manual approval | BackendAgent + DocsAgent | ✅ done (`ingestion.go`, endpoints `/api/v1/prompt/ingest`, `/approve`, `/reject`) | PI-TB-003 |

`PI-TR-*` là task phía `Z:\01_PROJECTS\apps\Tirouter\TASKS.md`.

### Tiêu chí nghiệm thu

- Fast path không gọi LLM và trả tối đa hai capsule.
- `task.text` chỉ tồn tại request-time; trace/feedback không lưu task hoặc response thô.
- Mọi DDL nằm trong migration và dùng shared `*sql.DB` với driver `modernc.org/sqlite`.
- Capsule chỉ được chọn khi ở trạng thái `active`, đúng protocol/risk và trong token budget.
- Feedback dedup theo request/capsule/version và dùng AsyncWriter.
- API có auth riêng cho TiRouter, timeout/cancellation và structured metrics.
- Prompt nguồn ngoài không được kích hoạt nếu thiếu provenance, quyền sử dụng hoặc review.

### Phân công cho agent/model free

- OpenCode model `-free`, Freebuff hoặc agent free khác phù hợp cho schema review, test generation, fixture/evaluation và documentation audit.
- Không giao secret handling, production credential hoặc thao tác destructive cho agent không có policy/context đầy đủ.
- Agent thực thi phải đọc `AGENTS.md`, chỉ sửa work package được giao, chạy test tương ứng và trả diff có thể review.
- Một agent khác phải review migration, auth, persistence và data-retention trước khi merge.

---

## 🚀 Next Actions

1. ✅ Build verified (`go build ./...`)
2. ✅ T-005c + T-011 (refactor) hoàn tất
3. ✅ PI-TB-001 → PI-TB-009: code + unit tests + E2E
4. **Còn lại**: build plugin PromptOrchestrator phía TiRouter (PI-TR-*) dựa trên contract `docs/PROMPT_INTELLIGENCE_CONTRACT.md`, và chạy E2E:
   ```powershell
   cd Z:\01_PROJECTS\apps\products\tibrain
   go test -tags integration ./internal/prompt/... -run E2E -v
   ```

---

## 🔗 Key Artifacts

| File | Purpose |
|------|---------|
| `SPEC.md` | Full implementation plan, timeline |
| `DESIGN.md` | Schema design, migration architecture |
| `API.md` | Endpoint documentation + MCP tools |
| `internal/db/migration.go` | Migration framework |
| `internal/db/migrations/*.sql` | Schema migrations |
| `.github/workflows/ci.yml` | CI pipeline |
| `.golangci.yml` | Linter configuration |
| `docs/PROMPT_INTELLIGENCE_CONTRACT.md` | Target contract với Prompt Orchestrator của TiRouter |

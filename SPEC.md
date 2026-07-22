# TiBrain v2.0 Implementation Plan

> **Generated**: 2026-07-10  
> **Status**: Ready for execution (pending build verification)  
> **Team**: tibrain Agent Team (Go Backend Service)

---

## Current State Assessment

| Phase | Status | Notes |
|-------|--------|-------|
| **T-001** Scaffold internal/ packages | ✅ Code written | `internal/db`, `internal/memory`, `internal/tools` — 12 symbols aligned |
| **T-002** README rewrite | ✅ Complete | Clear overview, install, run, API summary |
| **T-003** API documentation | ✅ Complete | `API.md` with ~60 endpoints + MCP tools |
| **T-004** Schema design | ✅ Complete | `DESIGN.md` - unified connection, migration framework |
| **T-005** Migration framework | ✅ Code written | `internal/db/migration.go`, `migrations/0001.sql` |
| **T-006-007** Test coverage | ✅ Written | 10 test files for RAG + knowledge modules |
| **T-008** CI setup | ✅ Complete | `.github/workflows/ci.yml`, `.golangci.yml` |
| **T-009-010** Health + build tags | ✅ Complete | Enhanced health/ready, fixed integration tags |
| **T-011-012** Package refactor + metrics | ⏳ Blocked | Awaiting build success |

---

## Immediate Action Required

**Verify Build** — Run on local machine:
```powershell
cd Z:\01_PROJECTS\apps\tibrain
go build ./...
```

Or use the created script:
```powershell
.\build.ps1
```

**If build succeeds** → Proceed to Refactor Phase (T-011)

**If build fails** → Report errors for immediate correction

---

## Phase 1: Foundation Refactor (High Priority)

### T-005c — Eliminate CGO Drivers
**Goal**: Single SQLite driver (`modernc.org/sqlite`), remove `mattn/go-sqlite3` CGO dependency

| File | Action | Lines |
|------|--------|-------|
| `auto_learning_mechanism.go` | Remove `sql.Open("sqlite3",)`, use shared `h.db` | ~20 |
| `ecosystem_integration.go` | Remove CGO open, use shared DB | ~10 |
| `predictive_maintenance.go` | Remove CGO open, use shared DB | ~10 |
| `intelligent_indexing_system.go` | Remove CGO open, use shared DB | ~10 |
| `cross_reference_intelligence.go` | Remove CGO open, use shared DB | ~10 |
| `deep_data_structure_analysis.go` | Remove CGO open, use shared DB | ~10 |

**Impact**: Reduces build complexity, enables cross-compilation

---

## Phase 2: Package Modularization (Medium Priority)

### T-011 — Split `package main` into Domain Packages

#### 2.1 Target Package Structure
```
internal/
├── rag/           ← 45 files: vector, cache, rerank, embed, retrieval
├── knowledge/     ← 12 files: indexer, ingestion, schema
├── orchestration/ ← 3 files: ti_agent, ecosystem
├── cloudflare/    ← 2 files: service, db
└── predictive/    ← 1 file: maintenance
```

#### 2.2 Implementation Sequence
| Step | Action | Owner | Duration |
|------|--------|-------|----------|
| 1 | Create package skeletons (`rag/`, `knowledge/`, `orchestration/`) | ArchitectAgent | 2 min |
| 2 | Move `vector_store.go`, `reranker.go`, `rag_cache.go` → `internal/rag/` | BackendAgent | 10 min |
| 3 | Move `knowledge_indexer.go`, `ingestion.go` → `internal/knowledge/` | BackendAgent | 8 min |
| 4 | Move `ti_agent.go`, `integration.go` → `internal/orchestration/` | BackendAgent | 5 min |
| 5 | Update imports in `main.go` | BackendAgent | 5 min |
| 6 | Run `go fmt ./...`, fix compilation errors | BackendAgent | 5 min |
| 7 | Update test imports to use new packages | TestAgent | 10 min |

**Estimated Total**: ~45 minutes

---

## Phase 3: Performance Optimizations (Low Priority)

### T-012 — AsyncWriter Metrics Complete
- [x] Add `totalEnqueued`, `totalFlushed` counters
- [x] Add `flushLatencySum`, `flushLatencyCount`
- [ ] Add Prometheus `/metrics` endpoint (optional)

### Future Enhancements
| Feature | Priority | Effort |
|---------|----------|--------|
| Index optimization (`index_tool_id`, `index_timestamp`) | Medium | 5 min |
| Health check caching (`sync.Once` init) | Medium | 5 min |
| RAG query caching TTL tuning | Low | 10 min |

---

## Phase 4: Prompt Intelligence cho TiRouter

### Mục tiêu

TiBrain cung cấp registry, retrieval, ranking, versioning, feedback và evaluation cho plugin Prompt Orchestrator của TiRouter. TiRouter vẫn là model ingress duy nhất tại `:3004`; request mutation chạy trong CLIProxyAPI runtime `:3004`.

Contract canonical: [`docs/PROMPT_INTELLIGENCE_CONTRACT.md`](./docs/PROMPT_INTELLIGENCE_CONTRACT.md).

### Phạm vi triển khai

1. Migration và repository cho capsule/version/trace/feedback/evaluation.
2. `POST /api/v1/prompt/preflight`, `POST /api/v1/prompt/feedback` và `GET /api/v1/prompt/catalog/version`.
3. Retrieval cascade deterministic-first, giới hạn tối đa hai capsule.
4. Không persist task text/response text mặc định; feedback chỉ chứa outcome rút gọn.
5. Contract test và E2E với CLIProxyAPI plugin.
6. Ingestion pipeline có provenance, license/ToS review, dedup và manual activation.

### Ngoài phạm vi

- TiBrain không sửa payload model và không gọi provider thay TiRouter.
- Không xây plugin đầy đủ cho từng CLI.
- Không public lại prompt collection bên thứ ba khi chưa có quyền.
- Không dùng LLM trong fast path mặc định.

### Điều kiện bắt đầu

- Build foundation và shared DB migration framework chạy xanh.
- ADR TiRouter `docs/adr/0001-prompt-intelligence-boundary.md` vẫn ở trạng thái Accepted.
- Schema version `1.0` có contract test chung giữa hai repo.

### Điều kiện hoàn tất

- Preflight p95 cache-warm đạt mục tiêu ban đầu dưới `150 ms` trong môi trường local test.
- TiRouter timeout `250 ms` luôn fail-open.
- Không có task/response raw hoặc credential trong DB/log/feedback fixtures.
- Canary hoàn thành theo `observe → suggest → apply allowlist` và có rollback.

---

## Milestone Tracking

| Milestone | Criteria | Target Date |
|-----------|----------|-------------|
| **M1: Build Verified** | `go build ./...` passes, tests compile | ASAP (user run) |
| **M2: Package Modularized** | All domain packages under `internal/` | +1 day |
| **M3: Test Coverage ≥30%** | `go test -cover` shows ≥30% overall | +2 days |
| **M4: CI Green** | GitHub Actions all jobs pass | +2 days |
| **M5: Prompt Observe** | Preflight contract + plugin observe mode chạy E2E | Sau M1 |
| **M6: Prompt Canary** | Apply allowlist có metrics, rollback và privacy audit | Sau M5 |

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| **CGO removal breaks existing queries** | Keep backward-compatible DB interface, run integration tests |
| **Package split introduces import cycles** | Use `internal/memory` as dependency layer, avoid cross-imports |
| **Migration drift from production data** | Version-controlled migrations, optional `--skip-migration` flag |

---

## Next Actions

1. **User**: Run `.\build.ps1` or `go build ./...`
2. **On build pass**: I execute T-005c (CGO removal) + T-011 (package split)
3. **On build fail**: I analyze stderr and apply fixes

---

*Plan generated by tibrain Team Lead*

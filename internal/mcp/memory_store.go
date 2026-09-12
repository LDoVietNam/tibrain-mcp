// memory_store.go — One Store: SQLite FTS5 làm kho kiến thức duy nhất cho
// memory.flush / memory.search / memory.list_domains.
//
// Nguyên tắc thiết kế (plan chốt 2026-09-12):
//   - SQLite FTS5 là index tìm kiếm; MEMORY.md append-log và memory_index.yaml
//     vẫn là nguồn "con người đọc được" (human-readable audit trail). DB là
//     derived store — mất DB chỉ cần re-import, không mất kiến thức.
//   - Importer one-shot lúc mở DB lần đầu trong process: đọc index yaml +
//     append-log, dedupe theo (domain, normalized content).
//   - FTS5 unicode61 remove_diacritics 0: giữ nguyên dấu tiếng Việt
//     (verify bằng probe test TestProbeFTS5Support 2026-09-12).
//   - embedding BLOB NULL: cột sẵn cho tương lai vector/RRF hybrid search.
package mcp

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// memoryStoreDSN trả về DSN cho memory store SQLite. Resolve theo thứ tự:
// env TIBRAIN_MEMORY_STORE_DSN > data/memory_store.db CẠNH BINARY (luôn dùng
// vị trí này kể cả khi file chưa tồn tại — khác binaryDataPath vốn fallback
// theo cwd khi candidate chưa có, sẽ tái tạo bug ghi nhầm theo cwd cho file
// tạo mới).
var memoryStoreDSN = resolveMemoryStoreDSN()

func resolveMemoryStoreDSN() string {
	if p := os.Getenv("TIBRAIN_MEMORY_STORE_DSN"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), defaultConfigDir, "memory_store.db")
	}
	return filepath.Join(defaultConfigDir, "memory_store.db")
}

// memoryStoreHolder giữ singleton *sql.DB cho One Store — mọi MCP handler
// đều đi qua đây để tránh mở nhiều connection vào cùng DB file.
type memoryStoreHolder struct {
	mu sync.Mutex
	db *sql.DB
}

var memStoreHolder = &memoryStoreHolder{}

// memoryStoreDB mở (hoặc tạo lần đầu) DB One Store, chạy migration và
// importer one-shot. Trả về *sql.DB dùng chung — KHÔNG Close ở caller.
func memoryStoreDB() (*sql.DB, error) {
	memStoreHolder.mu.Lock()
	defer memStoreHolder.mu.Unlock()

	if memStoreHolder.db != nil {
		return memStoreHolder.db, nil
	}

	dsn := "file:" + filepath.ToSlash(memoryStoreDSN) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open memory store: %w", err)
	}
	db.SetMaxOpenConns(1)    // modernc sqlite an toàn nhất khi single-writer
	db.SetConnMaxLifetime(0) // giữ connection dài hạn, WAL chỉ init 1 lần

	if err := migrateMemoryStore(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate memory store: %w", err)
	}
	if err := importMemorySources(db); err != nil {
		// Importer fail không chặn mở DB: store cũ (nếu có) vẫn search được,
		// flush vẫn ghi được. Ghi warning vào stderr, không crash server.
		fmt.Fprintf(os.Stderr, "[memory_store] import warning: %v\n", err)
	}

	memStoreHolder.db = db
	return db, nil
}

const memSchema = `
CREATE TABLE IF NOT EXISTS memories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	domain TEXT NOT NULL,
	content TEXT NOT NULL,
	confidence REAL NOT NULL DEFAULT 0.9,
	verified INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT 'flush',
	ts TEXT NOT NULL,
	embedding BLOB,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%S+00:00','now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_domain_content ON memories(domain, content);
CREATE INDEX IF NOT EXISTS idx_memories_domain ON memories(domain);
CREATE INDEX IF NOT EXISTS idx_memories_ts ON memories(ts);
`

const memFTSSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
	content,
	tokenize = "unicode61 remove_diacritics 0"
);
CREATE TRIGGER IF NOT EXISTS memories_fts_ins AFTER INSERT ON memories BEGIN
	INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
END;
`

// migrateMemoryStore tạo schema nếu chưa có. FTS table là bảng thường có
// content (không contentless) — rowid của memories_fts được set = id của
// memories lúc insert để JOIN đơn giản, deterministic.
func migrateMemoryStore(db *sql.DB) error {
	if _, err := db.Exec(memSchema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	if _, err := db.Exec(memFTSSchema); err != nil {
		return fmt.Errorf("fts schema: %w", err)
	}
	return nil
}

// memInsert là dữ liệu 1 entry chuẩn hóa trước khi vào store.
type memInsert struct {
	Domain     string
	Content    string // đã normalize (collapse whitespace) trước khi gọi
	Confidence float64
	Verified   bool
	Source     string // "index" | "log" | "flush"
	TS         string // RFC3339; rỗng = dùng now
}

// importMemorySources chạy importer one-shot: đọc memory_index.yaml (entries
// tĩnh theo domain) và MEMORY.md append-log (entries do flush ghi), insert
// những entry chưa có — dedupe theo (domain, normalized content).
func importMemorySources(db *sql.DB) error {
	// 1. memory_index.yaml — entries tĩnh của từng domain
	if idx, err := readMemoryIndex(); err == nil {
		for _, dom := range idx.Domains {
			for _, entry := range dom.Entries {
				if strings.TrimSpace(entry) == "" {
					continue
				}
				if _, err := insertMemIfAbsent(db, memInsert{
					Domain: dom.Name, Content: entry,
					Confidence: dom.Confidence, Verified: dom.Verified,
					Source: "index",
				}); err != nil {
					return fmt.Errorf("import index domain %s: %w", dom.Name, err)
				}
			}
		}
	}

	// 2. MEMORY.md append-log — entries do memory.flush ghi
	if data, err := os.ReadFile(memoryLogPath); err == nil {
		for _, e := range parseMemoryLog(data) {
			if strings.TrimSpace(e.Content) == "" {
				continue
			}
			if _, err := insertMemIfAbsent(db, memInsert{
				Domain: e.Domain, Content: e.Content,
				Confidence: e.Confidence,
				Source:     "log", TS: e.Timestamp,
			}); err != nil {
				return fmt.Errorf("import log entry: %w", err)
			}
		}
	}
	return nil
}

// insertMemIfAbsent insert entry nếu (domain, normalized content) chưa tồn tại.
// Dedupe write-time: UNIQUE INDEX (domain, content) + INSERT OR IGNORE — an
// toàn race giữa các goroutine, không cần check-then-insert. FTS row được
// trigger memories_fts_ins populate atomically cùng transaction với insert.
// Trả về (inserted, error).
func insertMemIfAbsent(db *sql.DB, in memInsert) (bool, error) {
	// Normalize: trim + collapse whitespace để bắt duplicate gần đúng.
	normalized := strings.Join(strings.Fields(in.Content), " ")
	if normalized == "" {
		return false, nil
	}

	ts := in.TS
	if ts == "" {
		ts = time.Now().Format(time.RFC3339)
	}

	res, err := db.Exec(
		`INSERT OR IGNORE INTO memories(domain, content, confidence, verified, source, ts)
		 VALUES (?,?,?,?,?,?)`,
		in.Domain, normalized, in.Confidence, boolToSQLite(in.Verified), in.Source, ts,
	)
	if err != nil {
		return false, fmt.Errorf("insert: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return affected > 0, nil
}

func boolToSQLite(b bool) int {
	if b {
		return 1
	}
	return 0
}

// storeMemoryFlush là write path cho handleMemoryFlush: insert vào One Store.
// MEMORY.md append-log vẫn được handler ghi song song (audit human-readable) —
// DB và log không cần đồng bộ transaction: DB là store chính cho search, log
// là audit trail cho người đọc.
func storeMemoryFlush(domain, content string, confidence float64) (bool, error) {
	db, err := memoryStoreDB()
	if err != nil {
		return false, err
	}
	return insertMemIfAbsent(db, memInsert{
		Domain: domain, Content: content, Confidence: confidence,
		Source: "flush", TS: time.Now().Format(time.RFC3339),
	})
}

// storeMemorySearch là read path cho searchMemory: FTS5 MATCH ranked bm25.
// Query rỗng → liệt kê theo domain filter (giữ behavior cũ). Multi-word query
// match không cần contiguous (fix limitation TEST 6).
func storeMemorySearch(params searchParams) ([]string, error) {
	db, err := memoryStoreDB()
	if err != nil {
		return nil, err
	}

	minConf := params.MinConfidence
	if minConf == 0 {
		minConf = 0.8
	}
	limit := params.Limit
	if limit == 0 {
		limit = 10
	}

	var rows *sql.Rows
	if strings.TrimSpace(params.Query) == "" {
		// Query rỗng: danh sách theo domain, mới nhất trước. Verified entries
		// bypass confidence filter (giữ semantics của search cũ theo index).
		if params.Domain != "" {
			rows, err = db.Query(`SELECT domain, confidence, content FROM memories
				WHERE lower(domain) = lower(?) AND (confidence >= ? OR verified = 1)
				ORDER BY ts DESC LIMIT ?`, params.Domain, minConf, limit)
		} else {
			rows, err = db.Query(`SELECT domain, confidence, content FROM memories
				WHERE (confidence >= ? OR verified = 1)
				ORDER BY ts DESC LIMIT ?`, minConf, limit)
		}
	} else {
		matchExpr, merr := buildFTSMatch(params.Query)
		if merr != nil {
			return nil, merr
		}
		// Subquery pattern (FTS5 docs): MATCH + bm25 chạy trực tiếp trên fts
		// table bằng TÊN GỐC — alias trong JOIN bị parser từ chối
		// ("no such column" trên modernc sqlite).
		if params.Domain != "" {
			rows, err = db.Query(`SELECT m.domain, m.confidence, m.content FROM memories m
				JOIN (SELECT rowid AS rid, bm25(memories_fts) AS rnk
					FROM memories_fts WHERE memories_fts MATCH ?) f ON f.rid = m.id
				WHERE lower(m.domain) = lower(?) AND (m.confidence >= ? OR m.verified = 1)
				ORDER BY f.rnk LIMIT ?`,
				matchExpr, params.Domain, minConf, limit)
		} else {
			rows, err = db.Query(`SELECT m.domain, m.confidence, m.content FROM memories m
				JOIN (SELECT rowid AS rid, bm25(memories_fts) AS rnk
					FROM memories_fts WHERE memories_fts MATCH ?) f ON f.rid = m.id
				WHERE (m.confidence >= ? OR m.verified = 1)
				ORDER BY f.rnk LIMIT ?`,
				matchExpr, minConf, limit)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var domain, content string
		var confidence float64
		if err := rows.Scan(&domain, &confidence, &content); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, fmt.Sprintf("[%s] (%d%%) %s",
			domain, int(confidence*100), content))
		if len(results) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// storeMemoryListDomains là read path cho handleMemoryListDomains: aggregate
// từ SQLite — stats phản ánh One Store thực tế (bao gồm cả entries flushed
// sau khi index tĩnh sinh ra).
func storeMemoryListDomains() ([]string, error) {
	db, err := memoryStoreDB()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT domain,
			COUNT(1),
			CAST(ROUND(AVG(confidence) * 100) AS INTEGER),
			MAX(verified)
		FROM memories
		GROUP BY domain
		ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var domain string
		var count, avgConf, maxVerified int
		if err := rows.Scan(&domain, &count, &avgConf, &maxVerified); err != nil {
			return nil, fmt.Errorf("scan domain stats: %w", err)
		}
		verified := maxVerified == 1
		out = append(out, fmt.Sprintf("- %s (confidence: %d%%, verified: %t, entries: %d)",
			domain, avgConf, verified, count))
	}
	return out, nil
}

// buildFTSMatch chuyển free-text query thành FTS5 MATCH expression an toàn:
// split whitespace → quote từng token (escape double quote) → nối AND.
// "cleanup misplaced" → `"cleanup" AND "misplaced"` — match multi-word không
// cần contiguous, miễn cả 2 token cùng xuất hiện trong content.
func buildFTSMatch(query string) (string, error) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty query")
	}
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " AND "), nil
}

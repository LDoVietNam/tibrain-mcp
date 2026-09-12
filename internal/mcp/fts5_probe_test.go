// Package mcp probe FTS5 support trong modernc.org/sqlite v1.54.0
package mcp

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProbeFTS5Support(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/probe.db?vfs=memdb")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// 1. FTS5 virtual table với unicode61 tokenizer
	if _, err := db.Exec(`CREATE VIRTUAL TABLE probe_fts USING fts5(text, tokenize="unicode61 remove_diacritics 0")`); err != nil {
		t.Fatalf("FTS5 KHÔNG được hỗ trợ: %v", err)
	}

	// 2. Insert tiếng Việt + search multi-word
	rows_text := []string{
		"misplaced Z:/03_DATA/bin/memory đã dọn, không tái tạo cleanup 2026",
		"Build workaround: GOGC=off go build -p=1 cho Go 1.26.6",
	}
	for _, s := range rows_text {
		if _, err := db.Exec("INSERT INTO probe_fts(text) VALUES (?)", s); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// 3. MATCH multi-word — TEST 6 scenario
	rows, err := db.Query(`SELECT text FROM probe_fts WHERE probe_fts MATCH ? ORDER BY bm25(probe_fts)`, `"cleanup" AND "misplaced"`)
	if err != nil {
		t.Fatalf("MATCH query lỗi: %v", err)
	}
	defer rows.Close()
	var hits []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		hits = append(hits, s)
	}
	if len(hits) == 0 {
		t.Fatal("TEST 6 scenario KHÔNG match — FTS5 không hoạt động như mong đợi")
	}
	fmt.Printf("PROBE OK: %d hits, first: %s\n", len(hits), hits[0])

	// 4. Tokenize tiếng Việt: "dọn" như 1 token
	rows2, err := db.Query(`SELECT text FROM probe_fts WHERE probe_fts MATCH ?`, `"dọn"`)
	if err != nil {
		t.Fatalf("tiếng Việt MATCH lỗi: %v", err)
	}
	defer rows2.Close()
	vnHits := 0
	for rows2.Next() {
		vnHits++
		_ = rows2.Scan(new(string))
	}
	if vnHits == 0 {
		t.Fatal("query từ tiếng Việt 'dọn' không match")
	}
	fmt.Printf("PROBE OK: tiếng Việt 'dọn' match %d rows\n", vnHits)
}

// Package mcp provides unit tests for the database MCP tools. The handlers are
// exercised directly (bypassing the HTTP layer) against an in-memory SQLite
// database created with the modernc.org/sqlite driver.
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	_ "modernc.org/sqlite"
)

// textContent returns the concatenated text of all TextContent blocks in a
// CallToolResult. Returns "" when there is no text content.
func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var out strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			out.WriteString(tc.Text)
		}
	}
	return out.String()
}

// newTestManager builds a Manager whose dbManager already holds an open
// in-memory SQLite database registered under the "sqlite" connection name.
// Each call gets its own isolated in-memory DB so tests never share state.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	// Use a *private* in-memory database (no cache=shared) so every test runs
	// against an isolated DB. Cap the pool to one connection so the in-memory
	// DB is not lost between pool acquisitions (SQLite in-memory is tied to the
	// connection), and enable foreign-key enforcement for FK constraint tests.
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable fkeys: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := &Manager{dbm: newDBManager()}
	m.dbm.conns["sqlite"] = db
	return m
}

// seededManager returns a manager whose "sqlite" connection already contains a
// users table with a few rows plus a views table entry.
func seededManager(t *testing.T) *Manager {
	t.Helper()
	m := newTestManager(t)
	db := m.dbm.conns["sqlite"]
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE users (
		id   INTEGER PRIMARY KEY,
		name TEXT    NOT NULL,
		email TEXT,
		age   INTEGER
	);`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE orders (
		id        INTEGER PRIMARY KEY,
		user_id   INTEGER NOT NULL,
		amount    REAL,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);`); err != nil {
		t.Fatalf("create orders: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE VIEW user_names AS SELECT name FROM users;`); err != nil {
		t.Fatalf("create view: %v", err)
	}
	seed := `INSERT INTO users (name, email, age) VALUES
		('Alice', 'alice@example.com', 30),
		('Bob',   'bob@example.com',   25),
		('Carol', 'carol@example.com', NULL);`
	if _, err := db.ExecContext(ctx, seed); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orders (user_id, amount) VALUES (1, 9.99), (1, 14.50), (2, 3.25);`); err != nil {
		t.Fatalf("seed orders: %v", err)
	}
	return m
}

// callTool builds a CallToolRequest with the given string arguments.
func callTool(t *testing.T, name string, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// ---------------------------------------------------------------------------
// isSelectLike / normalizeVal (pure helpers)
// ---------------------------------------------------------------------------

func TestIsSelectLike(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"SELECT * FROM users", true},
		{"  select 1", true},
		{"PRAGMA table_info(users)", true},
		{"EXPLAIN QUERY PLAN SELECT 1", true},
		{"INSERT INTO users VALUES (1)", false},
		{"UPDATE users SET name='x'", false},
		{"DELETE FROM users", false},
		{"DROP TABLE users", false},
		{"  ", false},
		{"BEGIN", false},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			if got := isSelectLike(tc.query); got != tc.want {
				t.Errorf("isSelectLike(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestNormalizeVal(t *testing.T) {
	if got := normalizeVal([]byte("bytes")); got != "bytes" {
		t.Errorf("normalizeVal([]byte) = %v, want %q", got, "bytes")
	}
	if got := normalizeVal(int64(7)); got != int64(7) {
		t.Errorf("normalizeVal(int64) = %v, want 7", got)
	}
	if got := normalizeVal(nil); got != nil {
		t.Errorf("normalizeVal(nil) = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// handleDBQuery
// ---------------------------------------------------------------------------

func TestHandleDBQuery(t *testing.T) {
	m := seededManager(t)
	ctx := context.Background()

	t.Run("ok select", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "sqlite",
			"query":      "SELECT id, name, email FROM users WHERE age > 24 ORDER BY id",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("handleDBQuery err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error result: %s", textContent(t, res))
		}
		txt := textContent(t, res)
		var rows []map[string]interface{}
		if err := json.Unmarshal([]byte(txt), &rows); err != nil {
			t.Fatalf("unmarshal result %q: %v", txt, err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		if rows[0]["name"] != "Alice" {
			t.Errorf("expected first row name Alice, got %v", rows[0]["name"])
		}
		// NULL handling: carol has NULL age, but we filter above so verify email present.
		if rows[0]["email"] != "alice@example.com" {
			t.Errorf("unexpected email: %v", rows[0]["email"])
		}
	})

	t.Run("pragma permitted as read", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "sqlite",
			"query":      "PRAGMA table_info(users)",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("handleDBQuery err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		var rows []map[string]interface{}
		if err := json.Unmarshal([]byte(textContent(t, res)), &rows); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(rows) != 4 {
			t.Errorf("expected 4 pragma rows, got %d", len(rows))
		}
	})

	t.Run("missing connection param", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"query": "SELECT 1",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing connection")
		}
	})

	t.Run("missing query param", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "sqlite",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing query")
		}
	})

	t.Run("write statement rejected", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "sqlite",
			"query":      "DELETE FROM users",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for write statement")
		}
		if !strings.Contains(textContent(t, res), "forbidden") {
			t.Errorf("expected forbidden message, got: %s", textContent(t, res))
		}
	})

	t.Run("invalid sql returns error", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "sqlite",
			"query":      "SELECT FROM WHERE",
		})
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for invalid SQL")
		}
		if !strings.Contains(textContent(t, res), "query failed") {
			t.Errorf("expected query failed message, got: %s", textContent(t, res))
		}
	})

	t.Run("unknown connection", func(t *testing.T) {
		req := callTool(t, "db.query", map[string]any{
			"connection": "does_not_exist",
			"query":      "SELECT 1",
		})
		// open() for non-sqlite requires env vars; expect an error result (not panic).
		res, err := m.handleDBQuery(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for unknown connection")
		}
	})
}

// ---------------------------------------------------------------------------
// handleDBExecute
// ---------------------------------------------------------------------------

func TestHandleDBExecute(t *testing.T) {
	ctx := context.Background()

	t.Run("ok insert", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.execute", map[string]any{
			"connection": "sqlite",
			"statement":  "INSERT INTO users (name, email, age) VALUES ('Dave', 'dave@example.com', 40)",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("handleDBExecute err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "rows_affected=1") {
			t.Errorf("expected rows_affected=1, got: %s", textContent(t, res))
		}
		// Verify persisted.
		var n int
		if err := m.dbm.conns["sqlite"].QueryRowContext(ctx,
			"SELECT COUNT(*) FROM users WHERE name='Dave'").Scan(&n); err != nil {
			t.Fatalf("verify: %v", err)
		}
		if n != 1 {
			t.Errorf("expected Dave inserted, got count %d", n)
		}
	})

	t.Run("ok update", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.execute", map[string]any{
			"connection": "sqlite",
			"statement":  "UPDATE users SET age = 31 WHERE name = 'Alice'",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("handleDBExecute err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "rows_affected=1") {
			t.Errorf("expected rows_affected=1, got: %s", textContent(t, res))
		}
	})

	t.Run("ok delete", func(t *testing.T) {
		m := seededManager(t)
		// Delete Carol (id=3) who has no orders, so the FK on orders is not violated.
		req := callTool(t, "db.execute", map[string]any{
			"connection": "sqlite",
			"statement":  "DELETE FROM users WHERE name = 'Carol'",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("handleDBExecute err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "rows_affected=1") {
			t.Errorf("expected rows_affected=1, got: %s", textContent(t, res))
		}
		// Verify row count dropped from 3 to 2.
		var n int
		if err := m.dbm.conns["sqlite"].QueryRowContext(ctx,
			"SELECT COUNT(*) FROM users").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 2 {
			t.Errorf("expected 2 users after delete, got %d", n)
		}
	})

	t.Run("missing connection param", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.execute", map[string]any{
			"statement": "INSERT INTO users VALUES (1)",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("missing statement param", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.execute", map[string]any{
			"connection": "sqlite",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result")
		}
	})

	t.Run("invalid sql returns error", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.execute", map[string]any{
			"connection": "sqlite",
			"statement":  "INSERT INTO non_existent_table VALUES (1)",
		})
		res, err := m.handleDBExecute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for invalid SQL")
		}
		if !strings.Contains(textContent(t, res), "execute failed") {
			t.Errorf("expected execute failed message, got: %s", textContent(t, res))
		}
	})
}

// ---------------------------------------------------------------------------
// handleDBSchema
// ---------------------------------------------------------------------------

func TestHandleDBSchema(t *testing.T) {
	m := seededManager(t)
	ctx := context.Background()

	t.Run("lists tables and views", func(t *testing.T) {
		req := callTool(t, "db.schema", map[string]any{
			"connection": "sqlite",
		})
		res, err := m.handleDBSchema(ctx, req)
		if err != nil {
			t.Fatalf("handleDBSchema err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		txt := textContent(t, res)
		// Order: orders, user_names, users (alphabetical by name).
		lines := strings.Split(strings.TrimSpace(txt), "\n")
		var got []string
		for _, l := range lines {
			got = append(got, l)
		}
		wantTable := "orders (table)"
		wantView := "user_names (view)"
		wantUsers := "users (table)"
		if !contains(got, wantTable) {
			t.Errorf("expected %q in %v", wantTable, got)
		}
		if !contains(got, wantView) {
			t.Errorf("expected %q in %v", wantView, got)
		}
		if !contains(got, wantUsers) {
			t.Errorf("expected %q in %v", wantUsers, got)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 schema entries, got %d: %v", len(got), got)
		}
	})

	t.Run("empty database", func(t *testing.T) {
		m := newTestManager(t)
		req := callTool(t, "db.schema", map[string]any{
			"connection": "sqlite",
		})
		res, err := m.handleDBSchema(ctx, req)
		if err != nil {
			t.Fatalf("handleDBSchema err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		// No tables -> empty output (strings.Join of empty slice produces "").
		if textContent(t, res) != "" {
			t.Errorf("expected empty schema, got: %q", textContent(t, res))
		}
	})

	t.Run("missing connection param", func(t *testing.T) {
		req := callTool(t, "db.schema", map[string]any{})
		res, err := m.handleDBSchema(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing connection")
		}
	})

	t.Run("unknown connection", func(t *testing.T) {
		req := callTool(t, "db.schema", map[string]any{
			"connection": "does_not_exist",
		})
		res, err := m.handleDBSchema(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for unknown connection")
		}
	})
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// handleDBTransaction
// ---------------------------------------------------------------------------

func TestHandleDBTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("commit on success", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
			"statements": []string{
				"INSERT INTO users (name, email, age) VALUES ('Dave', 'dave@example.com', 41)",
				"INSERT INTO orders (user_id, amount) VALUES (4, 50.0)",
			},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("handleDBTransaction err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "ok statements=2") {
			t.Errorf("expected ok statements=2, got: %s", textContent(t, res))
		}
		// Verify both committed.
		var uid, oid int
		if err := m.dbm.conns["sqlite"].QueryRowContext(ctx,
			"SELECT id FROM users WHERE name='Dave'").Scan(&uid); err != nil {
			t.Fatalf("find user: %v", err)
		}
		if err := m.dbm.conns["sqlite"].QueryRowContext(ctx,
			"SELECT id FROM orders WHERE user_id=? AND amount=50.0", uid).Scan(&oid); err != nil {
			t.Fatalf("find order: %v", err)
		}
		if oid == 0 {
			t.Error("expected order to be committed")
		}
	})

	t.Run("rollback on error", func(t *testing.T) {
		m := seededManager(t)
		// Insert a valid row first, then a failing statement; expect rollback
		// so neither the valid nor invalid row is persisted.
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
			"statements": []string{
				"INSERT INTO users (name, email, age) VALUES ('Eve', 'eve@example.com', 22)",
				"INSERT INTO non_existent_table VALUES (1)",
			},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("handleDBTransaction err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for failing transaction")
		}
		if !strings.Contains(textContent(t, res), "transaction failed") {
			t.Errorf("expected transaction failed message, got: %s", textContent(t, res))
		}
		// Eve must not exist because the transaction rolled back.
		var n int
		if err := m.dbm.conns["sqlite"].QueryRowContext(ctx,
			"SELECT COUNT(*) FROM users WHERE name='Eve'").Scan(&n); err != nil {
			t.Fatalf("verify: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 rows (rollback), got %d", n)
		}
	})

	t.Run("foreign key constraint aborts tx", func(t *testing.T) {
		m := seededManager(t)
		// orders.user_id must exist; inserting a dangling fk fails the tx.
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
			"statements": []string{
				"INSERT INTO orders (user_id, amount) VALUES (999, 1.0)",
			},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("handleDBTransaction err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error for fk violation")
		}
	})

	t.Run("single statement transaction succeeds", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
			"statements": []string{
				"UPDATE users SET age = 31 WHERE name = 'Alice'",
			},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("handleDBTransaction err: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", textContent(t, res))
		}
		if !strings.Contains(textContent(t, res), "ok statements=1") {
			t.Errorf("unexpected result: %s", textContent(t, res))
		}
	})

	t.Run("missing connection param", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.transaction", map[string]any{
			"statements": []string{"SELECT 1"},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing connection")
		}
	})

	t.Run("missing statements param", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for missing statements")
		}
	})

	t.Run("empty statements slice rejected", func(t *testing.T) {
		m := seededManager(t)
		req := callTool(t, "db.transaction", map[string]any{
			"connection": "sqlite",
			"statements": []string{},
		})
		res, err := m.handleDBTransaction(ctx, req)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.IsError {
			t.Fatal("expected error result for empty statements")
		}
	})
}

// ---------------------------------------------------------------------------
// dbManager.open env-driven paths (covers the non-injected error branches)
// ---------------------------------------------------------------------------

func TestDBManagerOpen(t *testing.T) {
	t.Run("caches connection", func(t *testing.T) {
		d := newDBManager()
		db1 := &sql.DB{}
		d.conns["sqlite"] = db1
		got, err := d.open("sqlite")
		if err != nil {
			t.Fatalf("open err: %v", err)
		}
		if got != db1 {
			t.Error("expected cached connection")
		}
	})

	t.Run("unknown driver missing env returns error", func(t *testing.T) {
		t.Setenv("TIBRAIN_DB_DRIVER", "")
		t.Setenv("TIBRAIN_DB_DSN", "")
		d := newDBManager()
		_, err := d.open("postgres")
		if err == nil {
			t.Fatal("expected error for unconfigured driver")
		}
		if !strings.Contains(err.Error(), "not configured") {
			t.Errorf("unexpected err: %v", err)
		}
	})
}

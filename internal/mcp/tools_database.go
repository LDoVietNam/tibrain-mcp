package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mark3labs/mcp-go/mcp"
)

// dbManager holds opened *sql.DB connections keyed by name.
type dbManager struct {
	conns map[string]*sql.DB
}

func newDBManager() *dbManager {
	return &dbManager{conns: make(map[string]*sql.DB)}
}

// open resolves a database connection by name from environment configuration.
// SQLite uses the pure-Go modernc driver; other drivers must be registered at
// runtime. If unavailable, an explicit error is returned (never a mock).
func (d *dbManager) open(name string) (*sql.DB, error) {
	if db, ok := d.conns[name]; ok {
		return db, nil
	}
	var driver, dsn string
	switch name {
	case "sqlite", "sqlite_main":
		driver = "sqlite"
		dsn = lookupEnv("TIBRAIN_SQLITE_DSN")
		if dsn == "" {
			// Resolve data/tibrain.db cạnh binary rồi mới tới cwd fallback —
			// tránh tạo DB mới theo working directory khi binary chạy từ
			// thư mục khác (deploy ở Z:/03_DATA/bin).
			dsn = binaryDataPath(filepath.Join(defaultConfigDir, "tibrain.db"))
		}
	default:
		driver = lookupEnv("TIBRAIN_DB_DRIVER")
		dsn = lookupEnv("TIBRAIN_DB_DSN")
		if driver == "" || dsn == "" {
			return nil, fmt.Errorf("database %q not configured (set TIBRAIN_DB_DRIVER/TIBRAIN_DB_DSN)", name)
		}
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	d.conns[name] = db
	return db, nil
}

func (m *Manager) handleDBQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conn, err := req.RequireString("connection")
	if err != nil {
		return errInvalidParams("connection is required"), nil
	}
	query, err := req.RequireString("query")
	if err != nil {
		return errInvalidParams("query is required"), nil
	}
	if !isSelectLike(query) {
		return errForbidden("db.query only permits read statements"), nil
	}
	db, err := m.dbm.open(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.query] open %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("query failed: cannot open database"), nil
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.query] query %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("query failed"), nil
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.query] columns %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("query failed"), nil
	}
	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Fprintf(os.Stderr, "[db.query] scan %s failed: %v\n", conn, err)
			return mcp.NewToolResultError("query failed"), nil
		}
		row := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			row[c] = normalizeVal(vals[i])
		}
		result = append(result, row)
	}
	b, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleDBExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conn, err := req.RequireString("connection")
	if err != nil {
		return errInvalidParams("connection is required"), nil
	}
	stmt, err := req.RequireString("statement")
	if err != nil {
		return errInvalidParams("statement is required"), nil
	}
	db, err := m.dbm.open(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.execute] open %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("execute failed: cannot open database"), nil
	}
	res, err := db.ExecContext(ctx, stmt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.execute] stmt %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("execute failed"), nil
	}
	aff, _ := res.RowsAffected()
	return mcp.NewToolResultText(fmt.Sprintf("ok rows_affected=%d", aff)), nil
}

func (m *Manager) handleDBSchema(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conn, err := req.RequireString("connection")
	if err != nil {
		return errInvalidParams("connection is required"), nil
	}
	db, err := m.dbm.open(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.schema] open %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("schema failed: cannot open database"), nil
	}
	rows, err := db.QueryContext(ctx, "SELECT name, type FROM sqlite_master WHERE type IN ('table','view') ORDER BY name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.schema] query %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("schema failed"), nil
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			fmt.Fprintf(os.Stderr, "[db.schema] scan %s failed: %v\n", conn, err)
			return mcp.NewToolResultError("schema failed"), nil
		}
		tables = append(tables, fmt.Sprintf("%s (%s)", name, typ))
	}
	return mcp.NewToolResultText(strings.Join(tables, "\n")), nil
}

func (m *Manager) handleDBTransaction(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	conn, err := req.RequireString("connection")
	if err != nil {
		return errInvalidParams("connection is required"), nil
	}
	statements := req.GetStringSlice("statements", nil)
	if len(statements) == 0 {
		return errInvalidParams("statements is required"), nil
	}
	db, err := m.dbm.open(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.transaction] open %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("transaction failed: cannot open database"), nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db.transaction] begin %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("transaction failed"), nil
	}
	for _, s := range statements {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			_ = tx.Rollback()
			fmt.Fprintf(os.Stderr, "[db.transaction] exec %s failed: %v\n", conn, err)
			return mcp.NewToolResultError("transaction failed"), nil
		}
	}
	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "[db.transaction] commit %s failed: %v\n", conn, err)
		return mcp.NewToolResultError("transaction failed"), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("ok statements=%d", len(statements))), nil
}

func isSelectLike(q string) bool {
	t := strings.TrimSpace(strings.ToLower(q))
	return strings.HasPrefix(t, "select") || strings.HasPrefix(t, "pragma") || strings.HasPrefix(t, "explain")
}

func normalizeVal(v interface{}) interface{} {
	switch x := v.(type) {
	case []byte:
		return string(x)
	default:
		return x
	}
}

// DB returns the default sqlite connection for tool implementations that
// operate against the primary secrets/metadata store.
func (d *dbManager) DB() (*sql.DB, error) {
	return d.open("sqlite_main")
}

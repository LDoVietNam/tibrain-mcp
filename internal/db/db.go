package db

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
	"strconv"
	"time"
)

// Helper function to check if string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Hub represents a database hub with connection and utility methods
type Hub struct {
	db *sql.DB
}

// NewHub creates a new database hub with the given data directory
func NewHub(dataDir string) (*Hub, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// Open SQLite database
	dbPath := filepath.Join(dataDir, "tibrain.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool for SQLite (single writer)
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	// Apply PRAGMA settings (per DESIGN.md §6)
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",  // 5s instead of default 0
		"PRAGMA foreign_keys=ON;",    // enable FK constraints
		"PRAGMA synchronous=NORMAL;", // WAL-safe, faster than FULL
		"PRAGMA cache_size=-64000;",  // ~64MB cache (negative = KB)
		"PRAGMA temp_store=MEMORY;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to apply pragma %q: %w", pragma, err)
		}
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Hub{db: db}, nil
}

// DB returns the underlying database connection
func (h *Hub) DB() *sql.DB {
	return h.db
}

// Close closes the database connection
func (h *Hub) Close() error {
	if h.db != nil {
		return h.db.Close()
	}
	return nil
}

// ApplyMigrations runs all migration files in the migrations directory
func ApplyMigrations(db *sql.DB) error {
	fmt.Println("ApplyMigrations called!")
	migrationsDir := "migrations"
	cwd, _ := os.Getwd()
	// Ensure schema_migrations table exists before processing migrations
	schemaTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum    TEXT NOT NULL,
			direction   TEXT NOT NULL DEFAULT 'up'
		)`
	if _, err := db.Exec(schemaTableSQL); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table exists: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		// No migrations directory - nothing to apply
		fmt.Printf("Migrations directory does not exist! (looking for %s in cwd: %s)\n", migrationsDir, cwd)
		return nil
	}
	fmt.Printf("Migrations directory exists! (found %s in cwd: %s)\n", migrationsDir, cwd)

	// Get all .sql files in migrations directory
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d files in migrations directory\n", len(files))

	// Sort files to ensure consistent order
	var fileInfos []os.FileInfo
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") && !strings.HasSuffix(file.Name(), ".down.sql") {
			info, err := file.Info()
			if err != nil {
				return err
			}
			fileInfos = append(fileInfos, info)
		}
	}

	// Sort by filename to ensure proper order (0001_, 0002_, etc.)
	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].Name() < fileInfos[j].Name()
	})

	// Apply each migration file
	for _, file := range fileInfos {
		// Extract version from filename (e.g., 0001_init_schema.sql -> 0001)
		versionStr := file.Name()[:4]
		if !isNumeric(versionStr) {
			continue // Skip files that don't start with a number
		}

		// Check if migration already applied
		var applied bool
		var existingChecksum string
		err := db.QueryRow("SELECT 1, checksum FROM schema_migrations WHERE version = ?", versionStr).Scan(&applied, &existingChecksum)
		if err == nil && applied {
			// Drift detection: compare checksum
			content, _ := os.ReadFile(filepath.Join(migrationsDir, file.Name()))
			newChecksum := fmt.Sprintf("%x", md5.Sum(content))
			if existingChecksum != newChecksum {
				return fmt.Errorf("migration drift detected for version %s: checksum mismatch (expected %s, got %s)", versionStr, existingChecksum, newChecksum)
			}
			continue // Skip already applied migration
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, file.Name()))
		if err != nil {
			return err
		}

		fmt.Printf("Processing migration %s (%d bytes)\n", file.Name(), len(content))
		// Log first 200 chars of SQL to see what we're running
		sqlPreview := string(content)
		if len(sqlPreview) > 200 {
			sqlPreview = sqlPreview[:200] + "..."
		}
		fmt.Printf("SQL preview: %q\n", sqlPreview)

		// Execute migration
		result, err := db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", file.Name(), err)
		}
		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("Executed migration %s successfully, rows affected: %d\n", file.Name(), rowsAffected)

		// Record migration as applied
		name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		checksum := fmt.Sprintf("%x", md5.Sum(content))
		_, err = db.Exec(
			"INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)",
			versionStr, name, checksum, time.Now().Unix(),
		)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", versionStr, err)
		}
	}

	return nil
}

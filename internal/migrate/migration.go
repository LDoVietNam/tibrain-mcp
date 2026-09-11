package migrate

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Migration represents a single database migration
type Migration struct {
	Version   string
	Name      string
	UpSQL     string
	DownSQL   string
	Checksum  string
	AppliedAt int64
	Direction string
}

// MigrationConfig holds configuration for migrations
type MigrationConfig struct {
	MigrationsDir string
	DBPath        string
	TableName     string
}

// DefaultMigrationConfig returns default configuration
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		MigrationsDir: "migrations",
		DBPath:        "tibrain.db",
		TableName:     "schema_migrations",
	}
}

// MigrationRunner manages migration execution
type MigrationRunner struct {
	db     *sql.DB
	config *MigrationConfig
}

// NewMigrationRunner creates a new migration runner
func NewMigrationRunner(config *MigrationConfig) (*MigrationRunner, error) {
	if config == nil {
		config = DefaultMigrationConfig()
	}

	db, err := sql.Open("sqlite", config.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure SQLite for migrations
	db.SetMaxOpenConns(1)
	pragmas := []string{
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA journal_mode=WAL;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	runner := &MigrationRunner{
		db:     db,
		config: config,
	}

	// Ensure schema_migrations table exists
	if err := runner.ensureSchemaTable(); err != nil {
		return nil, err
	}

	return runner, nil
}

// Close closes the database connection
func (r *MigrationRunner) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// DB returns the underlying database connection
func (r *MigrationRunner) DB() *sql.DB {
	return r.db
}

// ensureSchemaTable creates the schema_migrations table if it doesn't exist
func (r *MigrationRunner) ensureSchemaTable() error {
	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  INTEGER NOT NULL,
			checksum    TEXT NOT NULL,
			direction   TEXT NOT NULL DEFAULT 'up'
		)
	`, r.config.TableName)

	_, err := r.db.Exec(sql)
	return err
}

// calculateChecksum computes SHA256 checksum of migration content
func calculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// LoadMigrations loads all migration files from the migrations directory
func (r *MigrationRunner) LoadMigrations() ([]*Migration, error) {
	var migrations []*Migration

	entries, err := os.ReadDir(r.config.MigrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return migrations, nil // No migrations directory
		}
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect .up.sql files
	type migFile struct {
		version  string
		name     string
		upPath   string
		downPath string
	}

	var files []migFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			version := name[:4]
			base := strings.TrimSuffix(name, ".up.sql")
			files = append(files, migFile{
				version:  version,
				name:     base[5:], // Remove "0001_" prefix
				upPath:   filepath.Join(r.config.MigrationsDir, name),
				downPath: filepath.Join(r.config.MigrationsDir, version+"_"+base[5:]+".down.sql"),
			})
		}
	}

	// Sort by version
	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	// Read each migration
	for _, f := range files {
		upContent, err := os.ReadFile(f.upPath)
		if err != nil {
			return nil, fmt.Errorf("read up migration %s: %w", f.upPath, err)
		}

		var downContent []byte
		if _, err := os.Stat(f.downPath); err == nil {
			downContent, _ = os.ReadFile(f.downPath)
		}

		upSQL := string(upContent)
		downSQL := string(downContent)
		checksum := calculateChecksum(upSQL)

		migrations = append(migrations, &Migration{
			Version:  f.version,
			Name:     f.name,
			UpSQL:    upSQL,
			DownSQL:  downSQL,
			Checksum: checksum,
		})
	}

	return migrations, nil
}

// GetAppliedMigrations returns all applied migrations from the database
func (r *MigrationRunner) GetAppliedMigrations() (map[string]*Migration, error) {
	query := fmt.Sprintf("SELECT version, name, applied_at, checksum, direction FROM %s ORDER BY version", r.config.TableName)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]*Migration)
	for rows.Next() {
		var m Migration
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt, &m.Checksum, &m.Direction); err != nil {
			return nil, err
		}
		applied[m.Version] = &m
	}
	return applied, rows.Err()
}

// Status returns migration status (pending, applied, drift)
func (r *MigrationRunner) Status() ([]MigrationStatus, error) {
	migrations, err := r.LoadMigrations()
	if err != nil {
		return nil, err
	}

	applied, err := r.GetAppliedMigrations()
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, m := range migrations {
		s := MigrationStatus{
			Migration: m,
			State:     "pending",
		}

		if a, ok := applied[m.Version]; ok {
			if a.Checksum != m.Checksum {
				s.State = "drift"
				s.DriftDetail = fmt.Sprintf("checksum mismatch: db=%s file=%s", a.Checksum[:16], m.Checksum[:16])
			} else {
				s.State = "applied"
				s.AppliedAt = a.AppliedAt
			}
		}
		status = append(status, s)
	}

	return status, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	*Migration
	State       string // pending, applied, drift
	AppliedAt   int64
	DriftDetail string
}

// Up runs all pending migrations
func (r *MigrationRunner) Up() error {
	migrations, err := r.LoadMigrations()
	if err != nil {
		return err
	}

	applied, err := r.GetAppliedMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			// Check for drift
			if applied[m.Version].Checksum != m.Checksum {
				return fmt.Errorf("drift detected for version %s: cannot apply (use --force to override)", m.Version)
			}
			continue // Already applied
		}

		fmt.Printf("Applying migration %s: %s\n", m.Version, m.Name)
		if err := r.executeMigration(m, "up"); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.Version, err)
		}
		fmt.Printf("  ✓ Applied %s\n", m.Version)
	}

	return nil
}

// Down rolls back the last N migrations (default 1)
func (r *MigrationRunner) Down(steps int) error {
	applied, err := r.GetAppliedMigrations()
	if err != nil {
		return err
	}

	// Get applied migrations in reverse order
	var versions []string
	for v := range applied {
		if applied[v].Direction == "up" {
			versions = append(versions, v)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	if len(versions) == 0 {
		fmt.Println("No migrations to roll back")
		return nil
	}

	if steps > len(versions) {
		steps = len(versions)
	}

	// Load migration files to get down SQL
	migrations, err := r.LoadMigrations()
	if err != nil {
		return err
	}
	migMap := make(map[string]*Migration)
	for _, m := range migrations {
		migMap[m.Version] = m
	}

	for i := 0; i < steps; i++ {
		v := versions[i]
		m := migMap[v]
		if m == nil || m.DownSQL == "" {
			return fmt.Errorf("no down migration for version %s", v)
		}

		fmt.Printf("Rolling back migration %s: %s\n", v, m.Name)
		if err := r.executeMigration(m, "down"); err != nil {
			return fmt.Errorf("rollback %s failed: %w", v, err)
		}
		fmt.Printf("  ✓ Rolled back %s\n", v)
	}

	return nil
}

// executeMigration runs a single migration (up or down)
func (r *MigrationRunner) executeMigration(m *Migration, direction string) error {
	sql := m.UpSQL
	if direction == "down" {
		sql = m.DownSQL
	}

	// Execute in transaction
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if _, err := tx.Exec(sql); err != nil {
		tx.Rollback()
		return fmt.Errorf("execute SQL: %w", err)
	}

	// Record migration
	appliedAt := time.Now().Unix()
	if direction == "up" {
		_, err = tx.Exec(
			fmt.Sprintf("INSERT INTO %s (version, name, applied_at, checksum, direction) VALUES (?, ?, ?, ?, ?)", r.config.TableName),
			m.Version, m.Name, appliedAt, m.Checksum, "up",
		)
	} else {
		_, err = tx.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE version = ?", r.config.TableName),
			m.Version,
		)
	}

	if err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit()
}

// Create creates a new migration file pair
func (r *MigrationRunner) Create(name string) error {
	// Get next version number
	migrations, err := r.LoadMigrations()
	if err != nil {
		return err
	}

	nextVersion := 1
	if len(migrations) > 0 {
		lastVersion := migrations[len(migrations)-1].Version
		if v, err := fmt.Sscanf(lastVersion, "%d", &nextVersion); err == nil && v == 1 {
			nextVersion++
		}
	}

	versionStr := fmt.Sprintf("%04d", nextVersion)
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "_")
	baseName := versionStr + "_" + sanitizedName

	upPath := filepath.Join(r.config.MigrationsDir, baseName+".up.sql")
	downPath := filepath.Join(r.config.MigrationsDir, baseName+".down.sql")

	// Create migrations directory if not exists
	if err := os.MkdirAll(r.config.MigrationsDir, 0755); err != nil {
		return fmt.Errorf("create migrations dir: %w", err)
	}

	// Template for up migration
	upTemplate := fmt.Sprintf(`-- Migration %s: %s
-- Created at: %s

-- TODO: Add UP migration SQL here

`, versionStr, name, time.Now().Format(time.RFC3339))

	// Template for down migration
	downTemplate := fmt.Sprintf(`-- Migration %s: %s (rollback)
-- Created at: %s

-- TODO: Add DOWN migration SQL here (reverse of up)

`, versionStr, name, time.Now().Format(time.RFC3339))

	if err := os.WriteFile(upPath, []byte(upTemplate), 0644); err != nil {
		return fmt.Errorf("write up migration: %w", err)
	}
	if err := os.WriteFile(downPath, []byte(downTemplate), 0644); err != nil {
		return fmt.Errorf("write down migration: %w", err)
	}

	fmt.Printf("Created migration pair:\n  %s\n  %s\n", upPath, downPath)
	return nil
}

// Verify checks all applied migrations for drift
func (r *MigrationRunner) Verify() error {
	status, err := r.Status()
	if err != nil {
		return err
	}

	hasDrift := false
	for _, s := range status {
		if s.State == "drift" {
			fmt.Printf("DRIFT: %s (%s) - %s\n", s.Version, s.Name, s.DriftDetail)
			hasDrift = true
		}
	}

	if !hasDrift {
		fmt.Println("All migrations verified - no drift detected")
	}
	return nil
}

// GetDB returns the database connection for custom operations
func (r *MigrationRunner) GetDB() *sql.DB {
	return r.db
}

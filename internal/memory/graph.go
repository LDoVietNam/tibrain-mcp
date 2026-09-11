package memory

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Graph manages memory entry relationships using SQLite.
type Graph struct {
	db      *sql.DB
	dbPath  string
	metrics *MetricsCollector
}

// NewGraph opens (or creates) a SQLite database for memory relationships.
func NewGraph(memoryBasePath string, metrics *MetricsCollector) (*Graph, error) {
	dbPath := filepath.Join(memoryBasePath, "graph.db")
	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open graph db: %w", err)
	}
	g := &Graph{db: db, dbPath: dbPath, metrics: metrics}
	if err := g.initSchema(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Graph) initSchema() error {
	_, err := g.db.Exec(`
		CREATE TABLE IF NOT EXISTS edges (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			source        TEXT NOT NULL,
			target        TEXT NOT NULL,
			rel_type      TEXT NOT NULL,
			created_at    TEXT NOT NULL,
			confidence    REAL DEFAULT 1.0,
			UNIQUE(source, target, rel_type)
		);
		CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source);
		CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target);
	`)
	return err
}

// Link creates or updates a relationship between two memory entries.
func (g *Graph) Link(source, target, relType string, confidence float64) error {
	start := time.Now()
	_, err := g.db.Exec(`
		INSERT INTO edges (source, target, rel_type, created_at, confidence)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source, target, rel_type) DO UPDATE SET
			confidence = excluded.confidence,
			created_at = excluded.created_at
	`, source, target, relType, time.Now().Format(time.RFC3339), confidence)
	if err != nil {
		if g.metrics != nil {
			g.metrics.RecordError("graph", "link_failed")
			g.metrics.RecordGraphQuery("link", time.Since(start))
		}
		return fmt.Errorf("link entries: %w", err)
	}
	if g.metrics != nil {
		g.metrics.RecordGraphQuery("link", time.Since(start))
	}
	return nil
}

// Related returns all entries related to the given source entry.
func (g *Graph) Related(source string) ([]string, error) {
	start := time.Now()
	rows, err := g.db.Query(`
		SELECT target FROM edges WHERE source = ?
		UNION
		SELECT source FROM edges WHERE target = ?
	`, source, source)
	if err != nil {
		if g.metrics != nil {
			g.metrics.RecordError("graph", "query_failed")
			g.metrics.RecordGraphQuery("related", time.Since(start))
		}
		return nil, fmt.Errorf("query related: %w", err)
	}
	defer rows.Close()

	var related []string
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			if g.metrics != nil {
				g.metrics.RecordError("graph", "scan_failed")
				g.metrics.RecordGraphQuery("related", time.Since(start))
			}
			return nil, err
		}
		related = append(related, entry)
	}

	if g.metrics != nil {
		g.metrics.RecordGraphQuery("related", time.Since(start))
	}
	return related, nil
}

// Close closes the underlying database connection.
func (g *Graph) Close() error {
	return g.db.Close()
}

// RemoveNode removes all edges associated with a memory entry.
func (g *Graph) RemoveNode(entryName string) error {
	start := time.Now()
	_, err := g.db.Exec(`DELETE FROM edges WHERE source = ? OR target = ?`, entryName, entryName)
	if err != nil {
		if g.metrics != nil {
			g.metrics.RecordError("graph", "remove_node_failed")
			g.metrics.RecordGraphQuery("remove_node", time.Since(start))
		}
		return fmt.Errorf("remove node edges: %w", err)
	}
	if g.metrics != nil {
		g.metrics.RecordGraphQuery("remove_node", time.Since(start))
	}
	return nil
}

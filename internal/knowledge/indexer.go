package knowledge

import "github.com/ti/router/tibrain/internal/db"

// KnowledgeIndexer handles knowledge indexing
type KnowledgeIndexer struct {
	hub *db.Hub
}

// KnowledgeIndexOptions configures indexing
type KnowledgeIndexOptions struct {
	Sources []KnowledgeIndexSource
}

// KnowledgeIndexSource represents a source to index
type KnowledgeIndexSource struct {
	Path        string
	Category    string
	Description string
}

// KnowledgeIndexResult represents indexing results
type KnowledgeIndexResult struct {
	Indexed int
	Skipped int
	Errors  []string
	Sources int
}

// NewKnowledgeIndexer creates a new knowledge indexer
func NewKnowledgeIndexer(hub *db.Hub) *KnowledgeIndexer {
	return &KnowledgeIndexer{hub: hub}
}

// Index performs knowledge indexing
func (k *KnowledgeIndexer) Index(opts KnowledgeIndexOptions) (*KnowledgeIndexResult, error) {
	return &KnowledgeIndexResult{
		Indexed: 0,
		Skipped: 0,
		Errors:  []string{},
		Sources: len(opts.Sources),
	}, nil
}

func categoryFromPath(path string) string {
	return "general"
}

package knowledge

import (
	"context"
	"fmt"
	"time"
)

// KnowledgeStore manages knowledge storage and retrieval
type KnowledgeStore struct {
	documents []Document
}

// Document represents a knowledge document
type Document struct {
	ID        string                 `json:"id"`
	Title     string                 `json:"title"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// NewKnowledgeStore creates a new knowledge store
func NewKnowledgeStore() *KnowledgeStore {
	return &KnowledgeStore{
		documents: make([]Document, 0),
	}
}

// Store stores a document in the knowledge base
func (k *KnowledgeStore) Store(ctx context.Context, doc Document) error {
	doc.UpdatedAt = time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}
	k.documents = append(k.documents, doc)
	return nil
}

// Query searches for documents by keyword
func (k *KnowledgeStore) Query(ctx context.Context, query string) ([]Document, error) {
	var results []Document
	query = fmt.Sprintf("%s", query) // normalize
	for _, doc := range k.documents {
		if containsKeyword(doc.Content, query) || containsKeyword(doc.Title, query) {
			results = append(results, doc)
		}
	}
	return results, nil
}

// Get retrieves a document by ID
func (k *KnowledgeStore) Get(ctx context.Context, id string) (*Document, error) {
	for _, doc := range k.documents {
		if doc.ID == id {
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("document not found: %s", id)
}

// Delete removes a document from the knowledge base
func (k *KnowledgeStore) Delete(ctx context.Context, id string) error {
	for i, doc := range k.documents {
		if doc.ID == id {
			k.documents = append(k.documents[:i], k.documents[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("document not found: %s", id)
}

// List lists all documents
func (k *KnowledgeStore) List(ctx context.Context) ([]Document, error) {
	return k.documents, nil
}

// UpdateMetadata updates document metadata
func (k *KnowledgeStore) UpdateMetadata(ctx context.Context, id string, metadata map[string]interface{}) error {
	for i, doc := range k.documents {
		if doc.ID == id {
			if doc.Metadata == nil {
				doc.Metadata = make(map[string]interface{})
			}
			for k, v := range metadata {
				doc.Metadata[k] = v
			}
			doc.UpdatedAt = time.Now()
			k.documents[i] = doc
			return nil
		}
	}
	return fmt.Errorf("document not found: %s", id)
}

func containsKeyword(s, keyword string) bool {
	return len(s) > 0 && len(keyword) > 0 &&
		(len(s) >= len(keyword) && (s == keyword || len(s) < 1000 && contains(s, keyword)))
}

func contains(s, keyword string) bool {
	for i := 0; i <= len(s)-len(keyword); i++ {
		if s[i:i+len(keyword)] == keyword {
			return true
		}
	}
	return false
}

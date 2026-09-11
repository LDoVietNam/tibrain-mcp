//go:build !integration

package notionprovider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ti/router/tibrain/internal/knowledge"
)

func TestSyncToKnowledge_StoresPages(t *testing.T) {
	t.Parallel()
	store := knowledge.NewKnowledgeStore()

	pages := []Page{
		{ID: "page-1", Title: "Alpha", Content: map[string]interface{}{"body": "alpha body"}},
		{ID: "page-2", Title: "Beta", Content: map[string]interface{}{"body": "beta body"}},
	}

	err := SyncPagesToKnowledge(context.Background(), store, pages)
	if err != nil {
		t.Fatalf("SyncPagesToKnowledge error: %v", err)
	}

	docs, err := store.Query(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("knowledge query error: %v", err)
	}
	if len(docs) == 0 {
		t.Error("expected 'alpha' document to be stored")
	}
}

func TestNotionClient_SyncToKnowledge(t *testing.T) {
	t.Parallel()

	store := knowledge.NewKnowledgeStore()
	client := NewNotionClient("fake-token")
	client.httpClient = &http.Client{
		Transport: &notionMockTransport{},
		Timeout:   5 * time.Second,
	}

	err := client.SyncToKnowledge(context.Background(), "db-1", store)
	if err != nil {
		t.Fatalf("SyncToKnowledge error: %v", err)
	}

	docs, err := store.Query(context.Background(), "Test")
	if err != nil {
		t.Fatalf("knowledge query error: %v", err)
	}
	if len(docs) == 0 {
		t.Error("expected documents to be stored from SyncToKnowledge")
	}
}

type notionMockTransport struct{}

func (m *notionMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"results": [{"id": "page-1", "title": "Test Page", "content": {"body": "test content"}}]}`
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

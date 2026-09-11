package notionprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ti/router/tibrain/internal/knowledge"
)

// NotionClient implements Notion API client
type NotionClient struct {
	apiToken   string
	httpClient *http.Client
}

// NewNotionClient creates a new Notion client
func NewNotionClient(apiToken string) *NotionClient {
	return &NotionClient{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Database represents a Notion database
type Database struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Page represents a Notion page
type Page struct {
	ID      string                 `json:"id"`
	Title   string                 `json:"title"`
	Content map[string]interface{} `json:"content"`
}

// QueryDatabase queries a Notion database
func (c *NotionClient) QueryDatabase(ctx context.Context, databaseID string) ([]Page, error) {
	reqBody := map[string]interface{}{
		"page_size": 100,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", databaseID),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []Page `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// CreatePage creates a new Notion page
func (c *NotionClient) CreatePage(ctx context.Context, databaseID string, content map[string]interface{}) (*Page, error) {
	reqBody := map[string]interface{}{
		"parent": map[string]interface{}{
			"database_id": databaseID,
		},
		"properties": content,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.notion.com/v1/pages",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	return &page, nil
}

// GetDatabase retrieves a Notion database
func (c *NotionClient) GetDatabase(ctx context.Context, databaseID string) (*Database, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.notion.com/v1/databases/%s", databaseID),
		nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Notion-Version", "2022-06-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var db Database
	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		return nil, err
	}

	return &db, nil
}

// SyncToKnowledge syncs Notion database pages into the provided knowledge store.
func (c *NotionClient) SyncToKnowledge(ctx context.Context, databaseID string, store interface {
	Store(context.Context, knowledge.Document) error
}) error {
	pages, err := c.QueryDatabase(ctx, databaseID)
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}

	return SyncPagesToKnowledge(ctx, store, pages)
}

// SyncPagesToKnowledge stores Notion pages into a knowledge store.
func SyncPagesToKnowledge(ctx context.Context, store interface {
	Store(context.Context, knowledge.Document) error
}, pages []Page) error {
	if store == nil {
		return fmt.Errorf("knowledge store is nil")
	}

	var lastErr error
	for _, page := range pages {
		content, _ := json.Marshal(page.Content)
		doc := knowledge.Document{
			ID:        page.ID,
			Title:     page.Title,
			Content:   string(content),
			Metadata:  map[string]interface{}{"source": "notion"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.Store(ctx, doc); err != nil {
			lastErr = fmt.Errorf("store page %s: %w", page.ID, err)
		}
	}
	return lastErr
}

// GetEnvToken retrieves Notion token from environment
func GetEnvToken() string {
	return os.Getenv("NOTION_API_TOKEN")
}

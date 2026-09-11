package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ti/router/tibrain/internal/security"
)

// secretsUpsertPayload is the expected JSON shape for secrets.upsert.
type secretsUpsertPayload struct {
	Provider string                 `json:"provider"`
	KeyType  string                 `json:"key_type"`
	Value    string                 `json:"value"`
	Source   string                 `json:"source"`
	Metadata map[string]interface{} `json:"metadata"`
	KeyID    string                 `json:"key_id"`
}

// secretsGetPayload is the expected JSON shape for secrets.get.
type secretsGetPayload struct {
	ID string `json:"id"`
}

// secretsGetByProviderPayload is the expected JSON shape for secrets.get_by_provider.
type secretsGetByProviderPayload struct {
	Provider string `json:"provider"`
}

// secretsDeletePayload is the expected JSON shape for secrets.delete.
type secretsDeletePayload struct {
	ID       string `json:"id"`
	Hard     bool   `json:"hard"`
	Provider string `json:"provider"`
	KeyID    string `json:"key_id"`
}

// secretsListPayload is the expected JSON shape for secrets.list.
type secretsListPayload struct {
	Provider string `json:"provider"`
}

func (m *Manager) secretsDB() (*sql.DB, error) {
	if m == nil || m.dbm == nil {
		return nil, fmt.Errorf("secrets vault not initialized")
	}
	return m.dbm.open("sqlite_main")
}

func (m *Manager) handleSecretsUpsert(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.mcpClientManager == nil {
		return errInternal("secrets vault not initialized"), nil
	}
	args := req.GetArguments()
	provider, _ := args["provider"].(string)
	keyType, _ := args["key_type"].(string)
	value, _ := args["value"].(string)
	source, _ := args["source"].(string)
	keyID, _ := args["key_id"].(string)
	meta := map[string]interface{}(nil)
	if raw, ok := args["metadata"].(map[string]interface{}); ok {
		meta = raw
	}
	provider = strings.TrimSpace(provider)
	keyType = strings.TrimSpace(keyType)
	keyID = strings.TrimSpace(keyID)
	if provider == "" || keyType == "" || value == "" {
		return errInvalidParams("provider, key_type, and value are required"), nil
	}
	if keyID == "" {
		keyID = provider + ":default"
	}
	enc, err := security.EncryptSecret(value)
	if err != nil {
		return errInternal(fmt.Sprintf("encrypt failed: %v", err)), nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	metaJSON := ""
	if meta != nil {
		b, _ := json.Marshal(meta)
		metaJSON = string(b)
	}
	db, err := m.secretsDB()
	if err != nil {
		return errInternal(err.Error()), nil
	}
	_, err = db.Exec(`
		INSERT INTO api_secrets_vault (id, provider, key_type, secret_blob, source, created_at, updated_at, last_used_at, metadata, revoked)
		VALUES (?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM api_secrets_vault WHERE id = ?), ?), ?, NULL, ?, 0)
	`, keyID, provider, keyType, enc, source, keyID, now, now, metaJSON)
	if err != nil {
		return errInternal(fmt.Sprintf("upsert failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","provider":"%s","key_type":"%s","source":"%s"}`, keyID, provider, keyType, source)), nil
}

func (m *Manager) handleSecretsGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.mcpClientManager == nil {
		return errInternal("secrets vault not initialized"), nil
	}
	args := req.GetArguments()
	id, _ := args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return errInvalidParams("id is required"), nil
	}
	var enc, provider, keyType, source, metaJSON string
	db, err := m.secretsDB()
	if err != nil {
		return errInternal(err.Error()), nil
	}
	err = db.QueryRow(`
		SELECT provider, key_type, secret_blob, source, metadata
		FROM api_secrets_vault
		WHERE id = ? AND revoked = 0
	`, id).Scan(&provider, &keyType, &enc, &source, &metaJSON)
	if err == sql.ErrNoRows {
		return errForbidden(fmt.Sprintf("secret %q not found", id)), nil
	}
	if err != nil {
		return errInternal(fmt.Sprintf("query failed: %v", err)), nil
	}
	value, err := security.DecryptSecret(enc)
	if err != nil {
		return errInternal(fmt.Sprintf("decrypt failed: %v", err)), nil
	}
	out := map[string]interface{}{
		"id":       id,
		"provider": provider,
		"key_type": keyType,
		"value":    value,
		"source":   source,
	}
	if metaJSON != "" {
		var meta map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		out["metadata"] = meta
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleSecretsGetByProvider(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.mcpClientManager == nil {
		return errInternal("secrets vault not initialized"), nil
	}
	args := req.GetArguments()
	provider, _ := args["provider"].(string)
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return errInvalidParams("provider is required"), nil
	}
	db, err := m.secretsDB()
	if err != nil {
		return errInternal(err.Error()), nil
	}
	rows, err := db.Query(`
		SELECT id, key_type, secret_blob, source, metadata, updated_at
		FROM api_secrets_vault
		WHERE provider = ? AND revoked = 0
	`, provider)
	if err != nil {
		return errInternal(fmt.Sprintf("query failed: %v", err)), nil
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var id, keyType, enc, source, metaJSON, updatedAt string
		if err := rows.Scan(&id, &keyType, &enc, &source, &metaJSON, &updatedAt); err != nil {
			return errInternal(fmt.Sprintf("scan failed: %v", err)), nil
		}
		value, err := security.DecryptSecret(enc)
		if err != nil {
			return errInternal(fmt.Sprintf("decrypt failed for %s: %v", id, err)), nil
		}
		item := map[string]interface{}{
			"id":         id,
			"provider":   provider,
			"key_type":   keyType,
			"value":      value,
			"source":     source,
			"updated_at": updatedAt,
		}
		if metaJSON != "" {
			var meta map[string]interface{}
			_ = json.Unmarshal([]byte(metaJSON), &meta)
			item["metadata"] = meta
		}
		results = append(results, item)
	}
	b, _ := json.Marshal(results)
	return mcp.NewToolResultText(string(b)), nil
}

func (m *Manager) handleSecretsDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.mcpClientManager == nil {
		return errInternal("secrets vault not initialized"), nil
	}
	args := req.GetArguments()
	id, _ := args["id"].(string)
	hard, _ := args["hard"].(bool)
	provider, _ := args["provider"].(string)
	keyID, _ := args["key_id"].(string)
	id = strings.TrimSpace(id)
	provider = strings.TrimSpace(provider)
	keyID = strings.TrimSpace(keyID)
	if id == "" && provider == "" {
		return errInvalidParams("id or provider is required"), nil
	}
	db, err := m.secretsDB()
	if err != nil {
		return errInternal(err.Error()), nil
	}
	if hard {
		if id != "" {
			_, err := db.Exec(`DELETE FROM api_secrets_vault WHERE id = ?`, id)
			if err != nil {
				return errInternal(fmt.Sprintf("delete failed: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf(`{"deleted":true,"id":"%s"}`, id)), nil
		}
		_, err := db.Exec(`DELETE FROM api_secrets_vault WHERE provider = ?`, provider)
		if err != nil {
			return errInternal(fmt.Sprintf("delete failed: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"deleted":true,"provider":"%s"}`, provider)), nil
	}
	// soft delete
	if id != "" {
		_, err := db.Exec(`UPDATE api_secrets_vault SET revoked = 1, updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
		if err != nil {
			return errInternal(fmt.Sprintf("revoke failed: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"revoked":true,"id":"%s"}`, id)), nil
	}
	if keyID != "" {
		_, err := db.Exec(`UPDATE api_secrets_vault SET revoked = 1, updated_at = ? WHERE provider = ? AND id = ?`, time.Now().UTC().Format(time.RFC3339), provider, keyID)
		if err != nil {
			return errInternal(fmt.Sprintf("revoke failed: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"revoked":true,"provider":"%s","key_id":"%s"}`, provider, keyID)), nil
	}
	_, err = db.Exec(`UPDATE api_secrets_vault SET revoked = 1, updated_at = ? WHERE provider = ?`, time.Now().UTC().Format(time.RFC3339), provider)
	if err != nil {
		return errInternal(fmt.Sprintf("revoke failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"revoked":true,"provider":"%s"}`, provider)), nil
}

func (m *Manager) handleSecretsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.mcpClientManager == nil {
		return errInternal("secrets vault not initialized"), nil
	}
	args := req.GetArguments()
	provider, _ := args["provider"].(string)
	provider = strings.TrimSpace(provider)
	query := `
		SELECT id, provider, key_type, source, created_at, updated_at, last_used_at, revoked
		FROM api_secrets_vault
	`
	var rows *sql.Rows
	var err error
	db, err := m.secretsDB()
	if err != nil {
		return errInternal(err.Error()), nil
	}
	if provider != "" {
		rows, err = db.Query(query+` WHERE provider = ? ORDER BY updated_at DESC`, provider)
	} else {
		rows, err = db.Query(query + ` ORDER BY updated_at DESC`)
	}
	if err != nil {
		return errInternal(fmt.Sprintf("query failed: %v", err)), nil
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var id, p, keyType, source, createdAt, updatedAt string
		var lastUsedAt sql.NullString
		var revoked int
		if err := rows.Scan(&id, &p, &keyType, &source, &createdAt, &updatedAt, &lastUsedAt, &revoked); err != nil {
			return errInternal(fmt.Sprintf("scan failed: %v", err)), nil
		}
		item := map[string]interface{}{
			"id":         id,
			"provider":   p,
			"key_type":   keyType,
			"source":     source,
			"created_at": createdAt,
			"updated_at": updatedAt,
			"revoked":    revoked != 0,
		}
		if lastUsedAt.Valid {
			item["last_used_at"] = lastUsedAt.String
		}
		results = append(results, item)
	}
	b, _ := json.Marshal(results)
	return mcp.NewToolResultText(string(b)), nil
}

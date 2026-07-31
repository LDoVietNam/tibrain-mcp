// Database schema updates for Ti Brain integration.
package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// updateIntegrationSchema ensures integration-related tables are seeded with default data.
// Table schemas are managed by migration files.
func updateIntegrationSchema(db *sql.DB) error {
	log.Printf("Starting updateIntegrationSchema")
	// First apply all migrations to ensure tables exist
	if err := ApplyMigrations(db); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	log.Printf("Applied migrations successfully")

	// Seed default RTK rules if empty
	if err := seedDefaultRTKRules(db); err != nil {
		log.Printf("Warning: Failed to seed default RTK rules: %v", err)
	}

	log.Printf("Integration schema verified")
	return nil
}

// seedDefaultRTKRules populates the rtk_rules table with dynamic defaults.
func seedDefaultRTKRules(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM rtk_rules").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Already seeded or customized
	}

	rules := []struct {
		ID           string
		RuleType     string
		ModelPattern string
		Config       string
		Priority     int
	}{
		{
			ID:           "rule_gemini_ultra",
			RuleType:     "compression",
			ModelPattern: "gemini*",
			Config:       `{"max_input_chars":200000,"preserve_last_n":20,"preserve_system":true,"compress_code_blocks":false}`,
			Priority:     100,
		},
		{
			ID:           "rule_claude_gpt_large",
			RuleType:     "compression",
			ModelPattern: "claude*",
			Config:       `{"max_input_chars":80000,"preserve_last_n":10,"preserve_system":true,"compress_code_blocks":false}`,
			Priority:     90,
		},
		{
			ID:           "rule_gpt4",
			RuleType:     "compression",
			ModelPattern: "gpt-4*",
			Config:       `{"max_input_chars":80000,"preserve_last_n":10,"preserve_system":true,"compress_code_blocks":false}`,
			Priority:     80,
		},
		{
			ID:           "rule_default_fallback",
			RuleType:     "compression",
			ModelPattern: "*",
			Config:       `{"max_input_chars":4000,"preserve_last_n":2,"preserve_system":true,"compress_code_blocks":true}`,
			Priority:     0,
		},
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.Prepare("INSERT INTO rtk_rules (id, rule_type, model_pattern, config, priority, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rule := range rules {
		_, err = stmt.Exec(rule.ID, rule.RuleType, rule.ModelPattern, rule.Config, rule.Priority, now, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

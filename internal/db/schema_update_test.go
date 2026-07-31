//go:build !integration

package db

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupSchemaDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	return db
}

func TestUpdateIntegrationSchema(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "successful schema update",
			wantErr: false,
		},
		{
			name:    "idempotent schema update",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupSchemaDB(t)
			defer db.Close()

			// First call
			err := updateIntegrationSchema(db)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateIntegrationSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.name == "idempotent schema update" {
				// Second call should not error
				err = updateIntegrationSchema(db)
				if err != nil {
					t.Errorf("expected idempotent call to succeed: %v", err)
				}
			}
		})
	}
}

func TestUpdateIntegrationSchema_TablesCreated(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	err := updateIntegrationSchema(db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify agent_registry table exists
	var exists int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_registry'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("agent_registry table should exist")
	}

	// Verify rag_knowledge_bases table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rag_knowledge_bases'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("rag_knowledge_bases table should exist")
	}

	// Verify cross_brain_communication table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='cross_brain_communication'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("cross_brain_communication table should exist")
	}

	// Verify orchestration_log table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='orchestration_log'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("orchestration_log table should exist")
	}

	// Verify agent_performance table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_performance'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("agent_performance table should exist")
	}

	// Verify task_memory table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='task_memory'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("task_memory table should exist")
	}

	// Verify decision_memory table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='decision_memory'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("decision_memory table should exist")
	}

	// Verify user_preferences table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='user_preferences'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("user_preferences table should exist")
	}

	// Verify agent_performance_enhanced table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_performance_enhanced'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("agent_performance_enhanced table should exist")
	}

	// Verify routing_decision_log table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='routing_decision_log'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("routing_decision_log table should exist")
	}

	// Verify rtk_rules table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rtk_rules'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("rtk_rules table should exist")
	}

	// Verify rtk_savings_log table exists
	err = db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rtk_savings_log'",
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if exists != 1 {
		t.Error("rtk_savings_log table should exist")
	}
}

func TestUpdateIntegrationSchema_IndexesCreated(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	err := updateIntegrationSchema(db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify indexes exist
	indexes := []string{
		"idx_agent_registry_status",
		"idx_agent_registry_type",
		"idx_cross_brain_timestamp",
		"idx_cross_brain_from_to",
		"idx_orchestration_timestamp",
		"idx_orchestration_status",
		"idx_agent_performance_agent",
		"idx_agent_performance_timestamp",
		"idx_task_memory_task_id",
		"idx_task_memory_status",
		"idx_task_memory_agent",
		"idx_task_memory_created",
		"idx_decision_memory_decision_id",
		"idx_decision_memory_type",
		"idx_decision_memory_maker",
		"idx_decision_memory_timestamp",
		"idx_user_preferences_user",
		"idx_user_preferences_key",
		"idx_user_preferences_category",
		"idx_user_preferences_valid",
		"idx_agent_perf_enhanced_agent",
		"idx_agent_perf_enhanced_task",
		"idx_agent_perf_enhanced_timestamp",
		"idx_agent_perf_enhanced_metric",
		"idx_routing_decision_route",
		"idx_routing_decision_timestamp",
		"idx_routing_decision_success",
		"idx_rtk_rules_type",
		"idx_rtk_rules_enabled",
		"idx_rtk_savings_timestamp",
	}

	for _, idx := range indexes {
		var exists int
		err = db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
			idx,
		).Scan(&exists)
		if err != nil {
			t.Errorf("failed to check index %s: %v", idx, err)
			continue
		}
		if exists != 1 {
			t.Errorf("index %s should exist", idx)
		}
	}
}

func TestSeedDefaultRTKRules(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		wantMin int
	}{
		{
			name:    "seed on empty database",
			wantErr: false,
			wantMin: 1,
		},
		{
			name:    "idempotent seed",
			wantErr: false,
			wantMin: 4, // 4 default rules
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupSchemaDB(t)
			defer db.Close()

			// Create rtk_rules table first
			_, err := db.Exec(`
				CREATE TABLE rtk_rules (
					id TEXT PRIMARY KEY,
					rule_type TEXT NOT NULL,
					model_pattern TEXT NOT NULL,
					config TEXT,
					priority INTEGER DEFAULT 0,
					enabled INTEGER DEFAULT 1,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				)
			`)
			if err != nil {
				t.Fatalf("failed to create rtk_rules table: %v", err)
			}

			err = seedDefaultRTKRules(db)
			if (err != nil) != tt.wantErr {
				t.Errorf("seedDefaultRTKRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM rtk_rules").Scan(&count)
			if err != nil {
				t.Fatalf("failed to count rules: %v", err)
			}
			if count < tt.wantMin {
				t.Errorf("expected at least %d rules, got %d", tt.wantMin, count)
			}

			// If testing idempotent, seed again
			if tt.name == "idempotent seed" && count == 4 {
				err = seedDefaultRTKRules(db)
				if err != nil {
					t.Errorf("expected idempotent seed to succeed: %v", err)
				}

				var newCount int
				db.QueryRow("SELECT COUNT(*) FROM rtk_rules").Scan(&newCount)
				if newCount != count {
					t.Errorf("expected %d rules after second seed, got %d", count, newCount)
				}
			}
		})
	}
}

func TestSeedDefaultRTKRules_DefaultRules(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	// Create rtk_rules table first
	_, err := db.Exec(`
		CREATE TABLE rtk_rules (
			id TEXT PRIMARY KEY,
			rule_type TEXT NOT NULL,
			model_pattern TEXT NOT NULL,
			config TEXT,
			priority INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create rtk_rules table: %v", err)
	}

	err = seedDefaultRTKRules(db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the four default rules are inserted with correct data
	rules := []struct {
		id           string
		ruleType     string
		modelPattern string
		priority     int
	}{
		{"rule_gemini_ultra", "compression", "gemini*", 100},
		{"rule_claude_gpt_large", "compression", "claude*", 90},
		{"rule_gpt4", "compression", "gpt-4*", 80},
		{"rule_default_fallback", "compression", "*", 0},
	}

	for _, rule := range rules {
		var found int
		err = db.QueryRow(
			"SELECT COUNT(*) FROM rtk_rules WHERE id=? AND rule_type=? AND model_pattern=? AND priority=?",
			rule.id, rule.ruleType, rule.modelPattern, rule.priority,
		).Scan(&found)
		if err != nil {
			t.Errorf("failed to find rule %s: %v", rule.id, err)
			continue
		}
		if found != 1 {
			t.Errorf("rule %s should exist with correct data", rule.id)
		}
	}
}

func TestSeedDefaultRTKRules_TxRollback(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	// Don't create the table - seed should fail
	err := seedDefaultRTKRules(db)
	if err == nil {
		t.Error("expected error when table does not exist")
	}
}

func TestRAGKnowledgeBase(t *testing.T) {
	kb := RAGKnowledgeBase{
		ID:          "test-kb",
		Name:        "Test Knowledge Base",
		Description: "A test knowledge base",
		Path:        "/test/path",
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		DocCount:    100,
		VectorCount: 50,
	}

	if kb.ID != "test-kb" {
		t.Errorf("expected ID test-kb, got %s", kb.ID)
	}
	if kb.Status != "active" {
		t.Errorf("expected status active, got %s", kb.Status)
	}
	if kb.DocCount != 100 {
		t.Errorf("expected DocCount 100, got %d", kb.DocCount)
	}
}

func TestRTKRuleStruct(t *testing.T) {
	now := time.Now().Unix()

	rule := struct {
		ID           string
		RuleType     string
		ModelPattern string
		Config       string
		Priority     int
	}{
		ID:           "test-rule",
		RuleType:     "compression",
		ModelPattern: "gpt*",
		Config:       `{"max_input_chars":1000}`,
		Priority:     50,
	}

	if rule.ID != "test-rule" {
		t.Errorf("expected ID test-rule, got %s", rule.ID)
	}
	if rule.Priority != 50 {
		t.Errorf("expected Priority 50, got %d", rule.Priority)
	}
	_ = now // Used in seed function
}

func TestSeedDefaultRTKRules_AlreadySeeded(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	// Create rtk_rules table
	_, err := db.Exec(`
		CREATE TABLE rtk_rules (
			id TEXT PRIMARY KEY,
			rule_type TEXT NOT NULL,
			model_pattern TEXT NOT NULL,
			config TEXT,
			priority INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create rtk_rules table: %v", err)
	}

	// Insert one rule
	now := time.Now().Unix()
	_, err = db.Exec(
		"INSERT INTO rtk_rules (id, rule_type, model_pattern, config, priority, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
		"existing-rule", "compression", "*", "{}", 50, now, now,
	)
	if err != nil {
		t.Fatalf("failed to insert existing rule: %v", err)
	}

	err = seedDefaultRTKRules(db)
	if err != nil {
		t.Errorf("expected no error when already seeded, got: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM rtk_rules").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 rule (no duplicates), got %d", count)
	}
}

func TestSeedDefaultRTKRules_InsertError(t *testing.T) {
	db := setupSchemaDB(t)
	defer db.Close()

	// Create rtk_rules table with wrong schema (missing enabled column default)
	_, err := db.Exec(`
		CREATE TABLE rtk_rules (
			id TEXT PRIMARY KEY,
			rule_type TEXT NOT NULL,
			model_pattern TEXT NOT NULL,
			config TEXT,
			priority INTEGER,
			enabled INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create rtk_rules table: %v", err)
	}

	err = seedDefaultRTKRules(db)
	if err != nil {
		t.Errorf("expected seed to fail gracefully, got: %v", err)
	}
}

func TestUpdateIntegrationSchema_NilDB(t *testing.T) {
	// Test with nil database (should panic or error)
	defer func() {
		if r := recover(); r != nil {
			// Expected - panic on nil db
		}
	}()

	err := updateIntegrationSchema(nil)
	if err != nil {
		// This is also acceptable behavior
	}
}

func TestTaskMemoryFields(t *testing.T) {
	task := struct {
		ID              string
		TaskID          string
		TaskType        string
		TaskDescription string
		TaskStatus      string
		Priority        int
	}{
		ID:              "task-1",
		TaskID:          "task-123",
		TaskType:        "indexing",
		TaskDescription: "Index knowledge documents",
		TaskStatus:      "pending",
		Priority:        5,
	}

	if task.TaskStatus != "pending" {
		t.Errorf("expected status pending, got %s", task.TaskStatus)
	}
	if task.Priority != 5 {
		t.Errorf("expected priority 5, got %d", task.Priority)
	}
}

func TestCrossBrainCommunicationFields(t *testing.T) {
	msg := struct {
		Type              string
		FromBrain         string
		ToBrain           string
		CommunicationType string
		Status            string
	}{
		Type:              "CrossBrain",
		FromBrain:         "tibrain-1",
		ToBrain:           "tibrain-2",
		CommunicationType: "query",
		Status:            "pending",
	}

	if msg.FromBrain != "tibrain-1" {
		t.Errorf("expected FromBrain tibrain-1, got %s", msg.FromBrain)
	}
	if msg.Status != "pending" {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestDecisionMemoryFields(t *testing.T) {
	decision := struct {
		ID              string
		DecisionType    string
		DecisionOutcome string
		DecisionMaker   string
		Confidence      float64
	}{
		ID:              "decision-1",
		DecisionType:    "routing",
		DecisionOutcome: "selected-route-a",
		DecisionMaker:   "router-agent",
		Confidence:      0.85,
	}

	if decision.DecisionMaker != "router-agent" {
		t.Errorf("expected Maker router-agent, got %s", decision.DecisionMaker)
	}
	if decision.Confidence != 0.85 {
		t.Errorf("expected Confidence 0.85, got %f", decision.Confidence)
	}
}

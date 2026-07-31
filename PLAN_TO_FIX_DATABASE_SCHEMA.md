# TiBrain Database Schema Fix Plan

## Issues Identified

1. **Duplicate Table Definitions**
   - `tool_registry`: Defined in both migration (8 columns) and schema_update.go (27 columns)
   - `agent_performance`: Defined in migration (5 columns) and schema_update.go (10 columns)

2. **Incomplete Schema**
   - `model_performance_stats` table missing columns used in code:
     * phase, total_tasks, total_quality, total_cost, total_latency, last_used, quality_history

3. **Migration Tracking Issues**
   - `ApplyMigrations` function doesn't properly record which migrations have been applied
   - No validation of migration order or versioning

4. **Schema Inconsistencies**
   - Various tables missing columns that are referenced in the code

## Solution Overview

1. Consolidate all table definitions into migration files (single source of truth)
2. Update migration files to include all required columns
3. Fix the migration application logic to properly track applied migrations
4. Update schema_update.go to only handle application-specific initialization
5. Ensure all tests pass after changes

## Detailed Implementation Steps

### Step 1: Fix Migration Files

#### Update 0001_init_schema.sql
- Remove duplicate table definitions that appear in schema_update.go
- Keep only the base tables that should be created initially
- Ensure all tables have the columns referenced in the code

#### Update model_performance_stats in 0004_prompt_intelligence.sql
Add missing columns:
```sql
ALTER TABLE model_performance_stats 
ADD COLUMN phase TEXT,
ADD COLUMN total_tasks INTEGER DEFAULT 0,
ADD COLUMN total_quality REAL DEFAULT 0.0,
ADD COLUMN total_cost REAL DEFAULT 0.0,
ADD COLUMN total_latency INTEGER DEFAULT 0,
ADD COLUMN last_used INTEGER,
ADD COLUMN quality_history TEXT;
```

#### Add missing columns to other tables as needed
Check all table definitions against what's referenced in:
- main.go 
- schema_update.go
- Any other .go files

### Step 2: Fix Migration Application Logic

#### Update internal/db/db.go ApplyMigrations function:
1. Read all .sql files from migrations directory
2. Sort them by filename to ensure proper order
3. For each migration file:
   - Extract version number from filename (e.g., 0001 from 0001_init_schema.sql)
   - Check if this version already exists in schema_migrations table
   - If not applied, execute the SQL and record it in schema_migrations
4. Return error if any migration fails

### Step 3: Simplify schema_update.go

#### Update internal/db/schema_update.go:
Remove all CREATE TABLE statements, keep only:
- Function signature and documentation
- Call to seedDefaultRTKRules (which should remain)
- Any application-specific initialization that's not schema-related

### Step 4: Update Tests if Needed

Review schema_update_test.go to ensure it still tests the correct behavior after changes.

### Step 5: Verification

1. Run all database-related tests
2. Start the application and verify it initializes correctly
3. Check that all tables have the expected columns
4. Verify that migration tracking works correctly

## Specific File Changes

### File: internal/db/migrations/0001_init_schema.sql
- Keep table definitions but ensure they match what's expected by code
- Remove any tables that are fully defined in schema_update.go (to avoid duplication)

### File: internal/db/migrations/0004_prompt_intelligence.sql
- Add the missing columns to model_performance_stats table as specified above

### File: internal/db/db.go
- Replace ApplyMigrations function with improved version that properly tracks migrations

### File: internal/db/schema_update.go
- Simplify to only:
  ```go
  func updateIntegrationSchema(db *sql.DB) error {
      // Seed default RTK rules if empty
      if err := seedDefaultRTKRules(db); err != nil {
          log.Printf("Warning: Failed to seed default RTK rules: %v", err)
      }
      
      log.Printf("Integration schema verified")
      return nil
  }
  ```

## Acceptance Criteria

1. Application starts without database errors
2. All tables have the columns referenced in the code
3. Migration tracking works (can see applied migrations in schema_migrations table)
4. Running the application multiple times doesn't attempt to re-apply migrations
5. All existing tests pass
6. No duplicate table definition errors during startup

## Estimated Effort

- Database schema fixes: 2-3 hours
- Migration logic fixes: 1-2 hours
- Testing and verification: 1-2 hours
- Total: 4-7 hours
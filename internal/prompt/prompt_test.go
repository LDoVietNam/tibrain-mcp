package prompt

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ti/router/tibrain/internal/ports"
	_ "modernc.org/sqlite"
)

func toRepo(r Repository) *sqliteRepository { return r.(*sqliteRepository) }

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?mode=memory")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_capsules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT,
		intent TEXT NOT NULL, domain TEXT, risk TEXT NOT NULL DEFAULT 'low',
		status TEXT NOT NULL DEFAULT 'draft', source_url TEXT, source_type TEXT,
		retrieved_at INTEGER, license_status TEXT,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create capsules: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_capsule_versions (
		id TEXT, version TEXT, content TEXT NOT NULL, content_hash TEXT NOT NULL,
		token_estimate INTEGER DEFAULT 0, placement TEXT NOT NULL,
		created_at INTEGER NOT NULL, PRIMARY KEY (id, version)
	)`)
	if err != nil {
		t.Fatalf("create versions: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_route_traces (
		id TEXT PRIMARY KEY, request_id TEXT NOT NULL, capsule_id TEXT NOT NULL,
		capsule_version TEXT, decision TEXT NOT NULL, confidence REAL,
		reason_code TEXT, latency_ms INTEGER, created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create traces: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_feedback (
		id TEXT PRIMARY KEY, request_id TEXT NOT NULL, capsule_id TEXT NOT NULL,
		capsule_version TEXT NOT NULL, outcome TEXT NOT NULL,
		user_override INTEGER DEFAULT 0, added_tokens INTEGER DEFAULT 0,
		provider_error_code TEXT, created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prompt_evaluations (
		id TEXT PRIMARY KEY, dataset_case TEXT NOT NULL, expected_capsule TEXT,
		actual_capsule TEXT, decision TEXT, confidence REAL, created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create evaluations: %v", err)
	}
	return db
}

func TestCreateCapsule(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	c := PromptCapsule{
		ID: "cap1", Name: "Test Capsule", Description: "desc",
		Intent: "qa", Domain: "support", Risk: CapsuleRiskLow,
		Status: CapsuleStatusActive, SourceType: "src",
	}
	if err := repo.CreateCapsule(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetCapsule(ctx, "cap1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Test Capsule" {
		t.Errorf("name: got %q", got.Name)
	}
}

func TestListCapsules(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	for _, c := range []PromptCapsule{
		{ID: "c1", Name: "A", Intent: "qa", Domain: "support", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "s"},
		{ID: "c2", Name: "B", Intent: "code", Domain: "dev", Risk: CapsuleRiskLow, Status: CapsuleStatusActive, SourceType: "s"},
		{ID: "c3", Name: "C", Intent: "qa", Domain: "support", Risk: CapsuleRiskHigh, Status: CapsuleStatusArchived, SourceType: "s"},
	} {
		if err := repo.CreateCapsule(ctx, c); err != nil {
			t.Fatalf("create %s: %v", c.ID, err)
		}
	}

	cases := []struct {
		intent, domain string
		status         CapsuleStatus
		want           int
	}{
		{"qa", "support", CapsuleStatusActive, 1},
		{"qa", "", CapsuleStatusActive, 1},
		{"", "", CapsuleStatusActive, 2},
		{"", "", CapsuleStatusArchived, 1},
		{"", "", CapsuleStatusActive, 2},
	}
	for i, tc := range cases {
		got, err := repo.ListCapsules(ctx, tc.intent, tc.domain, tc.status, 10)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if len(got) != tc.want {
			t.Errorf("case %d: got %d, want %d", i, len(got), tc.want)
		}
	}
}

func TestPreflight(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	got, err := toRepo(repo).Preflight(ctx, "qa", "support")
	if err != nil {
		t.Fatalf("preflight no match: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}

	if err := repo.CreateCapsule(ctx, PromptCapsule{
		ID: "p1", Name: "P1", Intent: "qa", Domain: "support",
		Risk: CapsuleRiskLow, Status: CapsuleStatusActive,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err = toRepo(repo).Preflight(ctx, "qa", "support")
	if err != nil {
		t.Fatalf("preflight match: %v", err)
	}
	if got == nil || got.ID != "p1" {
		t.Errorf("expected p1, got %+v", got)
	}
}

func TestRecordFeedback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	if err := toRepo(repo).RecordFeedback(ctx, "req1", 1, "good"); err != nil {
		t.Fatalf("record: %v", err)
	}

	list, err := repo.ListFeedback(ctx, "req1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestToCapsuleEnvelope(t *testing.T) {
	c := PromptCapsule{
		ID: "1", Name: "N", Description: "D",
		Intent: "qa", Domain: "sup", Risk: CapsuleRiskLow,
		Status: CapsuleStatusActive,
	}
	env := ToCapsuleEnvelope(&c)
	if env.ID != "1" || env.Name != "N" {
		t.Errorf("bad envelope: %+v", env)
	}
	if len(env.Intent) != 1 || env.Intent[0] != "qa" {
		t.Errorf("bad intent slice: %v", env.Intent)
	}
}

func TestSplitIntent(t *testing.T) {
	if len(splitIntent("")) != 0 {
		t.Error("expected empty")
	}
	if len(splitIntent("qa")) != 1 || splitIntent("qa")[0] != "qa" {
		t.Error("bad split")
	}
}

func TestNewRepository(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	if repo == nil {
		t.Fatal("nil repo")
	}
	var ok bool
	if _, ok = repo.(Repository); !ok {
		t.Fatal("does not implement Repository")
	}
	var _ ports.PromptPort = repo.(*sqliteRepository)
}

func TestDecisionConstants(t *testing.T) {
	if DecisionUse != "use" || DecisionSkip != "skip" || DecisionFallback != "fallback" {
		t.Error("bad decision constants")
	}
}

func TestStatusConstants(t *testing.T) {
	if CapsuleStatusDraft != "draft" || CapsuleStatusActive != "active" || CapsuleStatusArchived != "archived" {
		t.Error("bad status constants")
	}
}

func TestRiskConstants(t *testing.T) {
	if CapsuleRiskLow != "low" || CapsuleRiskMedium != "medium" || CapsuleRiskHigh != "high" {
		t.Error("bad risk constants")
	}
}

func TestVersionCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.CreateCapsule(ctx, PromptCapsule{
		ID: "v1", Name: "V", Intent: "test", Risk: CapsuleRiskLow, Status: CapsuleStatusActive,
	}); err != nil {
		t.Fatalf("create cap: %v", err)
	}

	if err := repo.CreateVersion(ctx, PromptVersion{
		ID: "v1", Version: "1.0", Content: "hello", ContentHash: "abc", Placement: "system",
	}); err != nil {
		t.Fatalf("create version: %v", err)
	}

	versions, err := repo.GetVersions(ctx, "v1")
	if err != nil {
		t.Fatalf("get versions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != "1.0" {
		t.Errorf("bad versions: %+v", versions)
	}

	v, err := repo.GetVersion(ctx, "v1", "1.0")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if v.Content != "hello" {
		t.Errorf("content: %s", v.Content)
	}
}

func TestTraceCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.CreateTrace(ctx, PromptTrace{
		ID: "t1", RequestID: "r1", CapsuleID: "c1", Decision: DecisionUse, Confidence: 0.9,
	}); err != nil {
		t.Fatalf("create trace: %v", err)
	}
}

func TestEvaluationCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.CreateEvaluation(ctx, PromptEvaluation{
		ID: "e1", DatasetCase: "case1", ExpectedCapsule: "ec", ActualCapsule: "ac",
		Decision: DecisionUse, Confidence: 0.8,
	}); err != nil {
		t.Fatalf("create eval: %v", err)
	}
}

func TestUpdateCapsuleStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	ctx := context.Background()

	if err := repo.CreateCapsule(ctx, PromptCapsule{
		ID: "st", Name: "S", Intent: "x", Risk: CapsuleRiskLow, Status: CapsuleStatusDraft, SourceType: "s",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateCapsuleStatus(ctx, "st", CapsuleStatusActive); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := repo.GetCapsule(ctx, "st")
	if got.Status != CapsuleStatusActive {
		t.Errorf("status: %s", got.Status)
	}
}

func TestGetCapsuleNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewRepository(db)
	_, err := repo.GetCapsule(context.Background(), "nope")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

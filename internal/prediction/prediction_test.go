package prediction

import (
	"context"
	"testing"
)

// --- helpers ---------------------------------------------------------------

func sampleModel(id, name string, quality float64) Model {
	return Model{
		ID:           id,
		Name:         name,
		Capabilities: []string{"text"},
		QualityScore: quality,
		Latency:      1.0,
		Cost:         0.01,
	}
}

func sampleTool(id, name, category string, quality float64) Tool {
	return Tool{
		ID:            id,
		Name:          name,
		Category:      category,
		Description:   "tool",
		QualityScore:  quality,
		SecurityScore: quality,
	}
}

func sampleSkill(id, name, domain string, patterns []string, quality float64) Skill {
	return Skill{
		ID:             id,
		Name:           name,
		Domain:         domain,
		IntentPatterns: patterns,
		QualityScore:   quality,
	}
}

// --- NewPredictionEngine ---------------------------------------------------

func TestNewPredictionEngine(t *testing.T) {
	e := NewPredictionEngine()
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
	if len(e.models) != 0 {
		t.Errorf("expected empty models, got %d", len(e.models))
	}
	if len(e.tools) != 0 {
		t.Errorf("expected empty tools, got %d", len(e.tools))
	}
	if len(e.skills) != 0 {
		t.Errorf("expected empty skills, got %d", len(e.skills))
	}
}

// --- AddModel / AddTool / AddSkill -----------------------------------------

func TestPredictionEngine_AddModel(t *testing.T) {
	e := NewPredictionEngine()
	m := sampleModel("m1", "Model One", 0.9)
	e.AddModel(m)
	if len(e.models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(e.models))
	}
	if e.models[0].ID != "m1" {
		t.Errorf("expected model m1, got %s", e.models[0].ID)
	}
}

func TestPredictionEngine_AddTool(t *testing.T) {
	e := NewPredictionEngine()
	t1 := sampleTool("t1", "Tool One", "coding", 0.8)
	e.AddTool(t1)
	if len(e.tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(e.tools))
	}
	if e.tools[0].ID != "t1" {
		t.Errorf("expected tool t1, got %s", e.tools[0].ID)
	}
}

func TestPredictionEngine_AddSkill(t *testing.T) {
	e := NewPredictionEngine()
	s1 := sampleSkill("s1", "Skill One", "coding", []string{"create"}, 0.7)
	e.AddSkill(s1)
	if len(e.skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(e.skills))
	}
	if e.skills[0].ID != "s1" {
		t.Errorf("expected skill s1, got %s", e.skills[0].ID)
	}
}

// --- Predict ---------------------------------------------------------------

func TestPredictionEngine_Predict_EmptyRegistry(t *testing.T) {
	e := NewPredictionEngine()
	res, err := e.Predict(context.Background(), "create a project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Task != "create a project" {
		t.Errorf("expected task preserved, got %q", res.Task)
	}
	if res.Intent != "create" {
		t.Errorf("expected intent create, got %q", res.Intent)
	}
	if res.Skill == nil {
		t.Fatal("expected default skill for empty registry")
	}
	if res.Skill.ID != "default" {
		t.Errorf("expected default skill, got %s", res.Skill.ID)
	}
	if res.Tool == nil {
		t.Fatal("expected default tool for empty registry")
	}
	if res.Tool.ID != "default" {
		t.Errorf("expected default tool, got %s", res.Tool.ID)
	}
	if res.Model == nil {
		t.Fatal("expected default model for empty registry")
	}
	if res.Model.ID != "default" {
		t.Errorf("expected default model, got %s", res.Model.ID)
	}
}

func TestPredictionEngine_Predict_UnknownIntent(t *testing.T) {
	e := NewPredictionEngine()
	e.AddSkill(sampleSkill("s1", "General", "general", []string{"execute"}, 0.5))
	e.AddTool(sampleTool("t1", "Tool", "general", 0.5))
	e.AddModel(sampleModel("m1", "Model", 0.5))

	intent := e.extractIntent("zzzzzzzzzzqwerty")
	if intent != "execute" {
		t.Errorf("expected execute fallback intent, got %q", intent)
	}

	res, err := e.Predict(context.Background(), "zzzzzzzzzzqwerty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Intent != "execute" {
		t.Errorf("expected intent execute, got %q", res.Intent)
	}
}

func TestPredictionEngine_Predict_WithPopulatedRegistry(t *testing.T) {
	tests := []struct {
		name        string
		task        string
		wantIntent  string
		wantSkillID string
		wantToolID  string
		wantModelID string
	}{
		{
			name:        "create intent picks matching skill",
			task:        "create a new component",
			wantIntent:  "create",
			wantSkillID: "write-skill",
			wantToolID:  "writer",
			wantModelID: "model-high-quality",
		},
		{
			name:        "list intent",
			task:        "list all files",
			wantIntent:  "list",
			wantSkillID: "search-skill",
			wantToolID:  "searcher",
			wantModelID: "model-high-quality",
		},
		{
			// No skill pattern matches "read"; predictSkill falls back to first skill.
			name:        "read intent falls back to first skill",
			task:        "read the config file",
			wantIntent:  "read",
			wantSkillID: "write-skill",
			wantToolID:  "writer",
			wantModelID: "model-high-quality",
		},
		{
			// No skill pattern matches "analyze"; predictSkill falls back to first skill.
			name:        "analyze intent falls back to first skill",
			task:        "analyze this code",
			wantIntent:  "analyze",
			wantSkillID: "write-skill",
			wantToolID:  "writer",
			wantModelID: "model-high-quality",
		},
		{
			// No skill pattern matches "delete"; predictSkill falls back to first skill.
			name:        "delete intent falls back to first skill",
			task:        "delete the temp cache",
			wantIntent:  "delete",
			wantSkillID: "write-skill",
			wantToolID:  "writer",
			wantModelID: "model-high-quality",
		},
		{
			// "execute" matches general-skill's pattern directly. general-skill's domain
			// ("general") has no matching tool category, so predictTool falls back to first tool.
			name:        "execute intent matches general skill",
			task:        "do something random",
			wantIntent:  "execute",
			wantSkillID: "general-skill",
			wantToolID:  "writer",
			wantModelID: "model-high-quality",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewPredictionEngine()
			e.AddModel(sampleModel("model-low", "Low Q", 0.4))
			e.AddModel(sampleModel("model-high-quality", "High Q", 0.9))
			e.AddSkill(sampleSkill("write-skill", "Writer", "writing", []string{"create"}, 0.8))
			e.AddSkill(sampleSkill("search-skill", "Searcher", "search", []string{"list"}, 0.7))
			e.AddSkill(sampleSkill("general-skill", "Generalist", "general", []string{"execute"}, 0.5))
			e.AddTool(sampleTool("writer", "WriterTool", "writing", 0.8))
			e.AddTool(sampleTool("searcher", "SearchTool", "search", 0.7))

			res, err := e.Predict(context.Background(), tt.task)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Intent != tt.wantIntent {
				t.Errorf("intent: got %q, want %q", res.Intent, tt.wantIntent)
			}
			if res.Skill == nil || res.Skill.ID != tt.wantSkillID {
				got := "nil"
				if res.Skill != nil {
					got = res.Skill.ID
				}
				t.Errorf("skill: got %s, want %s", got, tt.wantSkillID)
			}
			if res.Tool == nil || res.Tool.ID != tt.wantToolID {
				got := "nil"
				if res.Tool != nil {
					got = res.Tool.ID
				}
				t.Errorf("tool: got %s, want %s", got, tt.wantToolID)
			}
			if res.Model == nil || res.Model.ID != tt.wantModelID {
				got := "nil"
				if res.Model != nil {
					got = res.Model.ID
				}
				t.Errorf("model: got %s, want %s", got, tt.wantModelID)
			}
		})
	}
}

// --- extractIntent ---------------------------------------------------------

func TestPredictionEngine_extractIntent(t *testing.T) {
	tests := []struct {
		name string
		task string
		want string
	}{
		{name: "list keyword", task: "list all items", want: "list"},
		{name: "search keyword", task: "search for records", want: "list"},
		{name: "find keyword", task: "find me a file", want: "list"},
		{name: "create keyword", task: "create a project", want: "create"},
		{name: "write keyword", task: "write some docs", want: "create"},
		{name: "generate keyword", task: "generate a report", want: "create"},
		{name: "read keyword", task: "read this file", want: "read"},
		{name: "get keyword", task: "get the config", want: "read"},
		{name: "fetch keyword", task: "fetch data", want: "read"},
		{name: "analyze keyword", task: "analyze the logs", want: "analyze"},
		{name: "review keyword", task: "review the PR", want: "analyze"},
		{name: "audit keyword", task: "audit for issues", want: "analyze"},
		{name: "delete keyword", task: "delete the entry", want: "delete"},
		{name: "remove keyword", task: "remove old cache", want: "delete"},
		{name: "clean keyword", task: "clean up tmp", want: "delete"},
		{name: "unknown keyword defaults to execute", task: "do something", want: "execute"},
		{name: "case insensitive", task: "CREATE a new file", want: "create"},
		{name: "empty string defaults to execute", task: "", want: "execute"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewPredictionEngine()
			got := e.extractIntent(tt.task)
			if got != tt.want {
				t.Errorf("extractIntent(%q) = %q, want %q", tt.task, got, tt.want)
			}
		})
	}
}

// --- predictSkill ----------------------------------------------------------

func TestPredictionEngine_predictSkill(t *testing.T) {
	t.Run("first matching pattern wins", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddSkill(sampleSkill("s-a", "A", "a", []string{"create write"}, 0.8))
		e.AddSkill(sampleSkill("s-b", "B", "b", []string{"create generate"}, 0.7))

		got := e.predictSkill("create")
		if got.ID != "s-a" {
			t.Errorf("expected s-a, got %s", got.ID)
		}
	})

	t.Run("default skill when no match", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddSkill(sampleSkill("s1", "S1", "general", []string{"execute"}, 0.5))

		got := e.predictSkill("list")
		if got.ID != "s1" {
			t.Errorf("expected first skill fallback s1, got %s", got.ID)
		}
	})

	t.Run("synthetic skill when registry empty", func(t *testing.T) {
		e := NewPredictionEngine()
		got := e.predictSkill("list")
		if got.ID != "default" {
			t.Errorf("expected default skill, got %s", got.ID)
		}
		if got.Domain != "general" {
			t.Errorf("expected general domain, got %s", got.Domain)
		}
		if got.QualityScore != 0.5 {
			t.Errorf("expected quality 0.5, got %f", got.QualityScore)
		}
	})

	t.Run("pattern matching is case insensitive", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddSkill(sampleSkill("s1", "S1", "general", []string{"CREATE"}, 0.5))

		got := e.predictSkill("create")
		if got.ID != "s1" {
			t.Errorf("expected s1, got %s", got.ID)
		}
	})
}

// --- predictTool -----------------------------------------------------------

func TestPredictionEngine_predictTool(t *testing.T) {
	t.Run("tool in same domain as skill", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t-coding", "T1", "coding", 0.8))
		e.AddTool(sampleTool("t-other", "T2", "other", 0.6))

		skill := &Skill{ID: "s1", Domain: "coding", QualityScore: 0.8, IntentPatterns: []string{}}
		got := e.predictTool(skill)
		if got.ID != "t-coding" {
			t.Errorf("expected t-coding, got %s", got.ID)
		}
	})

	t.Run("first tool when domain not found", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t1", "T1", "coding", 0.8))
		e.AddTool(sampleTool("t2", "T2", "other", 0.6))

		skill := &Skill{ID: "s1", Domain: "nonexistent", QualityScore: 0.8}
		got := e.predictTool(skill)
		if got.ID != "t1" {
			t.Errorf("expected first tool t1, got %s", got.ID)
		}
	})

	t.Run("synthetic tool when registry empty", func(t *testing.T) {
		e := NewPredictionEngine()
		skill := &Skill{ID: "s1", Domain: "coding", QualityScore: 0.8}
		got := e.predictTool(skill)
		if got.ID != "default" {
			t.Errorf("expected default tool, got %s", got.ID)
		}
		if got.Category != "general" {
			t.Errorf("expected general category, got %s", got.Category)
		}
	})

	t.Run("domain matching is case insensitive", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t-coding", "T1", "CODING", 0.8))

		skill := &Skill{ID: "s1", Domain: "coding", QualityScore: 0.8}
		got := e.predictTool(skill)
		if got.ID != "t-coding" {
			t.Errorf("expected t-coding, got %s", got.ID)
		}
	})
}

// --- predictModel ----------------------------------------------------------

func TestPredictionEngine_predictModel(t *testing.T) {
	t.Run("best quality model selected", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddModel(sampleModel("m-low", "Low", 0.4))
		e.AddModel(sampleModel("m-high", "High", 0.9))
		e.AddModel(sampleModel("m-mid", "Mid", 0.6))

		skill := &Skill{QualityScore: 0.5}
		got := e.predictModel(skill)
		if got.ID != "m-high" {
			t.Errorf("expected m-high, got %s", got.ID)
		}
	})

	t.Run("highest score wins ties", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddModel(sampleModel("m1", "M1", 0.8))
		e.AddModel(sampleModel("m2", "M2", 0.8))

		skill := &Skill{QualityScore: 0.5}
		got := e.predictModel(skill)
		if got == nil {
			t.Fatal("expected non-nil model")
		}
		if got.QualityScore != 0.8 {
			t.Errorf("expected 0.8, got %f", got.QualityScore)
		}
	})

	t.Run("synthetic model when registry empty", func(t *testing.T) {
		e := NewPredictionEngine()
		skill := &Skill{QualityScore: 0.5}
		got := e.predictModel(skill)
		if got.ID != "default" {
			t.Errorf("expected default model, got %s", got.ID)
		}
		if got.QualityScore != 0.5 {
			t.Errorf("expected quality 0.5, got %f", got.QualityScore)
		}
	})

	t.Run("model with zero score not selected over default", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddModel(sampleModel("m-zero", "Zero", 0.0))

		skill := &Skill{QualityScore: 0.5}
		got := e.predictModel(skill)
		if got.ID != "default" {
			t.Errorf("expected default (zero-score excluded), got %s", got.ID)
		}
	})
}

// --- calculateScore --------------------------------------------------------

func TestPredictionEngine_calculateScore(t *testing.T) {
	tests := []struct {
		name  string
		skill *Skill
		tool  *Tool
		model *Model
		want  float64
	}{
		{
			name:  "all perfect 1.0",
			skill: &Skill{QualityScore: 1.0},
			tool:  &Tool{QualityScore: 1.0},
			model: &Model{QualityScore: 1.0},
			want:  1.0,
		},
		{
			name:  "all zero",
			skill: &Skill{QualityScore: 0},
			tool:  &Tool{QualityScore: 0},
			model: &Model{QualityScore: 0},
			want:  0,
		},
		{
			name:  "weighted mix",
			skill: &Skill{QualityScore: 0.6},
			tool:  &Tool{QualityScore: 0.8},
			model: &Model{QualityScore: 0.5},
			want:  0.6*0.3 + 0.8*0.3 + 0.5*0.4,
		},
		{
			name:  "default fallback values",
			skill: &Skill{QualityScore: 0.5},
			tool:  &Tool{QualityScore: 0.5},
			model: &Model{QualityScore: 0.5},
			want:  0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewPredictionEngine()
			got := e.calculateScore(tt.skill, tt.tool, tt.model)
			if got < 0 {
				t.Errorf("score should never be negative, got %f", got)
			}
			// Check weighting: model has 0.4 weight, skill & tool 0.3
			modelContrib := tt.model.QualityScore * 0.4
			skillContrib := tt.skill.QualityScore * 0.3
			toolContrib := tt.tool.QualityScore * 0.3
			expected := modelContrib + skillContrib + toolContrib
			if got != expected {
				t.Errorf("got %f, want %f", got, expected)
			}
			if got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

// --- GetBestModel ----------------------------------------------------------

func TestPredictionEngine_GetBestModel(t *testing.T) {
	t.Run("empty registry returns error", func(t *testing.T) {
		e := NewPredictionEngine()
		_, err := e.GetBestModel("coding")
		if err == nil {
			t.Fatal("expected error for empty registry")
		}
	})

	t.Run("returns highest quality model", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddModel(sampleModel("m1", "Low", 0.3))
		e.AddModel(sampleModel("m-best", "High", 0.95))
		e.AddModel(sampleModel("m2", "Mid", 0.6))

		got, err := e.GetBestModel("coding")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "m-best" {
			t.Errorf("expected m-best, got %s", got.ID)
		}
	})

	t.Run("first model with max score when tie", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddModel(sampleModel("m1", "First", 0.8))
		e.AddModel(sampleModel("m2", "Second", 0.8))

		got, err := e.GetBestModel("coding")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "m1" {
			t.Errorf("expected first model m1 on tie, got %s", got.ID)
		}
	})
}

// --- GetBestTool -----------------------------------------------------------

func TestPredictionEngine_GetBestTool(t *testing.T) {
	t.Run("empty registry returns error", func(t *testing.T) {
		e := NewPredictionEngine()
		_, err := e.GetBestTool("coding")
		if err == nil {
			t.Fatal("expected error for empty registry")
		}
		if err.Error() == "\"no models available\"" {
			// sanity: error message should reference tools, not models
		}
	})

	t.Run("no matching category returns error", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t1", "T1", "coding", 0.8))
		_, err := e.GetBestTool("nonexistent")
		if err == nil {
			t.Fatal("expected error for unmatched category")
		}
	})

	t.Run("returns best tool in matching category", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t1", "T1", "coding", 0.5))
		e.AddTool(sampleTool("t-best", "TBest", "coding", 0.9))
		e.AddTool(sampleTool("t3", "T3", "search", 0.9)) // higher quality, wrong category

		got, err := e.GetBestTool("coding")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "t-best" {
			t.Errorf("expected t-best, got %s", got.ID)
		}
	})

	t.Run("case insensitive category match", func(t *testing.T) {
		e := NewPredictionEngine()
		e.AddTool(sampleTool("t1", "T1", "CODING", 0.9))

		got, err := e.GetBestTool("coding")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "t1" {
			t.Errorf("expected t1, got %s", got.ID)
		}
	})
}

// --- Edge cases ------------------------------------------------------------

func TestPredictionEngine_Predict_EmptyQuery(t *testing.T) {
	e := NewPredictionEngine()
	e.AddSkill(sampleSkill("s1", "S1", "general", []string{"execute"}, 0.5))
	res, err := e.Predict(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error on empty query: %v", err)
	}
	if res.Task != "" {
		t.Errorf("expected empty task preserved, got %q", res.Task)
	}
	if res.Intent != "execute" {
		t.Errorf("empty query should default to execute, got %q", res.Intent)
	}
}

func TestPredictionEngine_Predict_LongQuery(t *testing.T) {
	e := NewPredictionEngine()
	e.AddModel(sampleModel("m1", "M1", 0.9))
	e.AddSkill(sampleSkill("s1", "S1", "general", []string{"list"}, 0.6))
	e.AddTool(sampleTool("t1", "T1", "general", 0.7))

	long := "list " + string(make([]byte, 10000))
	res, err := e.Predict(context.Background(), long)
	if err != nil {
		t.Fatalf("unexpected error on long query: %v", err)
	}
	if res.Intent != "list" {
		t.Errorf("expected intent list, got %q", res.Intent)
	}
	if res.Score < 0 {
		t.Errorf("score should not be negative, got %f", res.Score)
	}
}

func TestPredictionEngine_Predict_ResultTypes(t *testing.T) {
	e := NewPredictionEngine()
	e.AddModel(sampleModel("m1", "M1", 0.9))
	e.AddSkill(sampleSkill("s1", "S1", "general", []string{"execute"}, 0.6))
	e.AddTool(sampleTool("t1", "T1", "general", 0.7))

	res, err := e.Predict(context.Background(), "do a thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

// --- Score composition check ------------------------------------------------

func TestPredictionEngine_Predict_ScoreEqualsCalculateScore(t *testing.T) {
	e := NewPredictionEngine()
	e.AddModel(sampleModel("m1", "M1", 0.7))
	e.AddSkill(sampleSkill("s1", "S1", "general", []string{"execute"}, 0.6))
	e.AddTool(sampleTool("t1", "T1", "general", 0.5))

	res, err := e.Predict(context.Background(), "execute something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := e.calculateScore(res.Skill, res.Tool, res.Model)
	if res.Score != expected {
		t.Errorf("result score %f != calculateScore %f", res.Score, expected)
	}
}

// --- KnowledgeProvider interface (compile-time check) -----------------------

type mockKnowledge struct{}

func (m *mockKnowledge) Query(ctx context.Context, query string) ([]interface{}, error) {
	return nil, nil
}

func TestPredictionEngine_KnowledgeProviderInterface(t *testing.T) {
	// Compile-time check that the engine references a KnowledgeProvider.
	var _ KnowledgeProvider = (*mockKnowledge)(nil)
}

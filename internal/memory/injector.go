package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// Injector preloads relevant memory entries based on context signals:
//   - Working directory (project name, language)
//   - Domain hints from the parent context
//   - Task description keyword extraction
type Injector struct {
	engine         *PromotionEngine
	memoryBasePath string
}

// NewInjector creates a new Injector.
func NewInjector(engine *PromotionEngine) *Injector {
	return &Injector{
		engine:         engine,
		memoryBasePath: engine.memoryBasePath,
	}
}

// InjectionContext provides signals for context-aware memory preloading.
type InjectionContext struct {
	WorkingDir  string   // current working directory
	DomainHints []string // domain hints from parent actor context
	TaskDesc    string   // task description for keyword extraction
}

// InjectedMemory represents a pre-loaded memory entry with its relevance score.
type InjectedMemory struct {
	Entry   TieredMemoryEntry
	Score   float64 // relevance score 0..1
	Reasons []string
}

// Preload returns memory entries relevant to the given context.
func (inj *Injector) Preload(ctx InjectionContext) ([]InjectedMemory, error) {
	// Extract keywords from working dir, domain hints, and task description.
	keywords := inj.extractKeywords(ctx)

	var results []InjectedMemory

	// Load entries from all tiers, prioritizing human > core > archival > recall.
	tiers := []Tier{TierHuman, TierCore, TierArchival, TierRecall}
	for _, tier := range tiers {
		entries, err := inj.engine.loadTierEntries(tier)
		if err != nil {
			continue
		}
		for i := range entries {
			entry := &entries[i]
			score, reasons := matchEntryAgainstKeywords(*entry, keywords)
			if score > 0 {
				results = append(results, InjectedMemory{
					Entry:   *entry,
					Score:   score,
					Reasons: reasons,
				})
			}
		}
	}

	return results, nil
}

// extractKeywords derives search terms from context signals.
func (inj *Injector) extractKeywords(ctx InjectionContext) []string {
	keywords := make(map[string]bool)

	// Working directory: project name + language detection.
	if ctx.WorkingDir != "" {
		// Extract project name from the deepest non-empty directory component.
		dir := filepath.Base(ctx.WorkingDir)
		if dir != "." && dir != "/" && dir != "" {
			keywords[strings.ToLower(dir)] = true
		}
		// Detect language from file extension patterns in the directory.
		if ext := inj.detectLanguage(ctx.WorkingDir); ext != "" {
			keywords[ext] = true
		}
	}

	// Domain hints.
	for _, hint := range ctx.DomainHints {
		keywords[strings.ToLower(hint)] = true
	}

	// Task description: split into terms.
	if ctx.TaskDesc != "" {
		terms := strings.FieldsFunc(strings.ToLower(ctx.TaskDesc), func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
		})
		for _, term := range terms {
			if len(term) > 3 { // skip short terms
				keywords[term] = true
			}
		}
	}

	result := make([]string, 0, len(keywords))
	for k := range keywords {
		result = append(result, k)
	}
	return result
}

// detectLanguage infers a language tag from file extensions in the working dir.
func (inj *Injector) detectLanguage(dir string) string {
	extensions := map[string]string{
		".go":   "golang",
		".py":   "python",
		".ts":   "typescript",
		".js":   "javascript",
		".rs":   "rust",
		".kt":   "kotlin",
		".java": "java",
		".cpp":  "cpp",
		".cc":   "cpp",
		".hpp":  "cpp",
		".h":    "c",
		".c":    "c",
	}

	extCount := make(map[string]int)
	var recursiveScan func(d string, depth int)
	recursiveScan = func(d string, depth int) {
		if depth > 2 {
			return
		}
		items, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, item := range items {
			if item.IsDir() {
				recursiveScan(filepath.Join(d, item.Name()), depth+1)
				continue
			}
			ext := strings.ToLower(filepath.Ext(item.Name()))
			if lang, ok := extensions[ext]; ok {
				extCount[lang]++
			}
		}
	}
	recursiveScan(dir, 0)

	bestLang := ""
	bestCount := 0
	for lang, count := range extCount {
		if count > bestCount {
			bestLang = lang
			bestCount = count
		}
	}
	return bestLang
}

// matchEntryAgainstKeywords scores an entry based on keyword overlap in domain, tags, and content.
func matchEntryAgainstKeywords(entry TieredMemoryEntry, keywords []string) (float64, []string) {
	doc := strings.ToLower(entry.Domain+" "+strings.Join(entry.Tags, " ")) +
		"\n" + strings.ToLower(entry.Content)

	var matchedTerms []string
	score := 0.0

	for _, kw := range keywords {
		if len(kw) < 2 {
			continue
		}
		count := strings.Count(doc, kw)
		if count > 0 {
			matchedTerms = append(matchedTerms, kw)
			// Weight by frequency, capped.
			termScore := float64(count) * 0.1
			if termScore > 0.5 {
				termScore = 0.5
			}
			score += termScore
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score, matchedTerms
}

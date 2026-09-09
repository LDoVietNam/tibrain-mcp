// TiBrain main - Knowledge Service & Control Plane
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ti/router/tibrain/internal/api"
	"github.com/ti/router/tibrain/internal/cloudflare"
	"github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/db"
	"github.com/ti/router/tibrain/internal/knowledge"
	"github.com/ti/router/tibrain/internal/mcp"
	"github.com/ti/router/tibrain/internal/orchestration"
	"github.com/ti/router/tibrain/internal/predictive"
	"github.com/ti/router/tibrain/internal/prompt"
	"github.com/ti/router/tibrain/internal/rag"
	"github.com/ti/router/tibrain/internal/security"
)

func main() {
	// Subcommand "index": tibrain index --path <docs> [--config config.yaml]
	// Index thủ công tài liệu .md chuẩn RAG vào TiBrain rồi thoát.
	if len(os.Args) > 1 && os.Args[1] == "index" {
		runIndexCommand(os.Args[2:])
		return
	}

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database hub
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = getEnvOrDefault("TIBRAIN_DATA_DIR", "Z:\\03_DATA\\tibrain-database")
	}
	hub, err := db.NewHub(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer hub.Close()

	// Apply migrations
	if err := db.ApplyMigrations(hub.DB()); err != nil {
		log.Fatalf("Failed to apply migrations: %v", err)
	}

	guard := security.NewGuard(cfg)
	auditor := security.NewAuditor("", false, true)

	manager := mcp.NewManager(cfg, guard, auditor)

	// Initialize core services with shared DB
	retrievalRouter := rag.NewRetrievalRouter(hub)
	_ = orchestration.NewTiAgentOrchestrator(hub, retrievalRouter)
	integrationManager := orchestration.NewIntegrationManager(hub)
	_ = cloudflare.NewCloudflareService()
	_ = predictive.NewPredictiveMaintenance()

	// RAG startup indexing: index docs chuẩn RAG từ các nguồn khai báo
	// trong config.yaml (rag.sources). Idempotent — file không đổi được
	// skip qua content_hash, chạy lại mỗi lần khởi động không tốn kém.
	if sources := cfg.RAGIndexSources(); len(sources) > 0 {
		knowledgeIndexer := knowledge.NewKnowledgeIndexer(hub)
		opts := knowledge.KnowledgeIndexOptions{}
		for _, src := range sources {
			opts.Sources = append(opts.Sources, knowledge.KnowledgeIndexSource{
				Path:        src.Path,
				Category:    src.Category,
				Description: src.Description,
			})
		}
		if res, err := knowledgeIndexer.Index(opts); err != nil {
			log.Printf("[RAG] Lỗi index startup: %v", err)
		} else {
			log.Printf("[RAG] Startup indexing: %d chunks mới, %d skipped, %d errors, %d sources",
				res.Indexed, res.Skipped, len(res.Errors), res.Sources)
			for _, e := range res.Errors {
				log.Printf("[RAG] Source error: %s", e)
			}
		}
	}

	apiServer := NewAPIServer(hub, integrationManager)

	router := chi.NewRouter()

	// Prompt intelligence API (SPEC.md Phase 4: T-013/T-014/T-015)
	promptHandler := prompt.NewHTTPHandlerWithDB(hub.DB())
	promptHandler.RegisterRoutes(router)

	// Legacy prompt config for Tirouter sync
	promptAPI := api.NewPromptAPIHandler()
	router.Handle("/api/v2/runtime/prompts", promptAPI)

	// API Server routes (catch-all for /api/* paths not matched by prompt routes)
	router.Handle("/*", apiServer)

	// MCP endpoints
	router.Handle("/mcp", http.HandlerFunc(manager.HandleStreamableHTTP))
	router.Handle("/mcp/sse", http.HandlerFunc(manager.HandleSSE))

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	// Protocol info
	router.Handle("/mcp/protocol", http.HandlerFunc(handleProtocolInfo))

	port := getEnvOrDefault("PORT", "3005")
	addr := ":" + port
	log.Printf("TiBrain starting on %s", addr)
	log.Printf("Health: http://localhost:%s/health", addr)
	log.Printf("Prompts: http://localhost:%s/api/v1/prompt/preflight", addr)
	log.Printf("Prompt config: http://localhost:%s/api/v2/runtime/prompts", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// runIndexCommand xử lý subcommand "tibrain index": index thủ công tài liệu
// .md chuẩn RAG vào DB của TiBrain rồi thoát (không khởi server).
//
// Cách dùng:
//
//	tibrain index --path Z:\01_PROJECTS\apps\products\ti-agent\docs
//	tibrain index --path docs/a.md --path docs/b.md --category tools
//	tibrain index            # không có --path → dùng rag.sources từ config.yaml
func runIndexCommand(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	paths := multiFlag{}
	fs.Var(&paths, "path", "thư mục hoặc file .md cần index (lặp lại được nhiều lần)")
	category := fs.String("category", "", "category fallback cho các doc không khai báo trong frontmatter")
	configPath := fs.String("config", "config.yaml", "đường dẫn file config.yaml")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[RAG] Load config thất bại: %v\n", err)
		os.Exit(1)
	}

	// Ưu tiên --path; nếu không có thì dùng rag.sources từ config.
	var sources []config.RAGSourceConfig
	if len(paths) > 0 {
		for _, p := range paths {
			sources = append(sources, config.RAGSourceConfig{
				Path:     p,
				Category: *category,
			})
		}
	} else {
		sources = cfg.RAGIndexSources()
		if len(sources) == 0 {
			fmt.Fprintln(os.Stderr, "[RAG] Không có --path và config.yaml chưa khai rag.sources — không có gì để index.")
			fmt.Fprintln(os.Stderr, "      Ví dụ: tibrain index --path Z:\\01_PROJECTS\\apps\\products\\ti-agent\\docs")
			os.Exit(1)
		}
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = getEnvOrDefault("TIBRAIN_DATA_DIR", `Z:\03_DATA\tibrain-database`)
	}
	hub, err := db.NewHub(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[RAG] Khởi tạo DB thất bại (dataDir=%s): %v\n", dataDir, err)
		os.Exit(1)
	}
	defer hub.Close()
	if err := db.ApplyMigrations(hub.DB()); err != nil {
		fmt.Fprintf(os.Stderr, "[RAG] Apply migrations thất bại: %v\n", err)
		os.Exit(1)
	}

	indexer := knowledge.NewKnowledgeIndexer(hub)
	opts := knowledge.KnowledgeIndexOptions{}
	for _, src := range sources {
		opts.Sources = append(opts.Sources, knowledge.KnowledgeIndexSource{
			Path:        src.Path,
			Category:    src.Category,
			Description: src.Description,
		})
	}

	fmt.Printf("[RAG] Indexing %d source(s) vào %s...\n", len(sources), dataDir)
	res, err := indexer.Index(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[RAG] Index thất bại: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[RAG] Kết quả: %d chunks mới, %d skipped (không đổi/chưa chuẩn), %d errors, %d sources\n",
		res.Indexed, res.Skipped, len(res.Errors), res.Sources)
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "[RAG] Error: %s\n", e)
	}
	if len(res.Errors) > 0 {
		os.Exit(1) // có lỗi → exit code 1 để script/cron nhận biết
	}
}

// multiFlag cho phép lặp lại cùng 1 flag (--path a --path b).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }

func (m *multiFlag) Set(v string) error {
	abs, err := filepath.Abs(v)
	if err != nil {
		abs = v
	}
	*m = append(*m, abs)
	return nil
}

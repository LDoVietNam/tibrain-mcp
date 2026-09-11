// TiBrain main - Knowledge Service & Control Plane
package main

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/ti/router/tibrain/internal/api"
	"github.com/ti/router/tibrain/internal/cloudflare"
	"github.com/ti/router/tibrain/internal/config"
	"github.com/ti/router/tibrain/internal/db"
	"github.com/ti/router/tibrain/internal/knowledge"
	"github.com/ti/router/tibrain/internal/mcp"
	"github.com/ti/router/tibrain/internal/memory"
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
	log.Printf("[DEBUG] Config loaded: Server=%s:%d, MCP Servers=%d", cfg.Server.Host, cfg.Server.Port, len(cfg.MCP.Servers))
	for _, s := range cfg.MCP.Servers {
		log.Printf("[DEBUG] MCP Server: %s (transport=%s, enabled=%v, auto_start=%v)", s.Name, s.Transport, s.Enabled, s.AutoStart)
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

	// Initialize authenticator for MCP gateway
	authenticator := security.NewAuthenticator(
		cfg.BearerToken(),
		cfg.Auth.AllowedOrigins,
		cfg.Auth.RateLimitPerMinute,
	)

	// Initialize metrics collector
	memory.InitGlobalMetrics("tibrain", "memory")

	// Initialize cognitive memory manager
	mem := memory.NewCognitiveMemoryManager(hub.DB(), memory.GetGlobalMetrics())

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

	// Determine allowed roots for filesystem tools
	allowedRoots := cfg.AllowedRoots
	if len(allowedRoots) == 0 {
		// Default to home config and current directory
		home := getEnvOrDefault("HOME", "")
		if home == "" {
			home = getEnvOrDefault("USERPROFILE", "")
		}
		if home != "" {
			allowedRoots = append(allowedRoots, filepath.Join(home, ".config", "tibrain"))
		}
		allowedRoots = append(allowedRoots, ".")
	}

	manager := mcp.NewManager(cfg, guard, auditor, mem, retrievalRouter, allowedRoots, authenticator)

	apiServer := api.NewAPIServer(hub, integrationManager)

	router := chi.NewRouter()

	// MCP endpoints (must be before catch-all)
	router.Handle("/mcp/message", http.HandlerFunc(manager.HandleMessage))
	router.Handle("/mcp", http.HandlerFunc(manager.HandleStreamableHTTP))
	router.Handle("/mcp/sse", http.HandlerFunc(manager.HandleSSE))

	// Metrics endpoint
	router.Handle("/metrics", promhttp.Handler())

	// Prompt intelligence API (SPEC.md Phase 4: T-013/T-014/T-015)
	promptHandler := prompt.NewHTTPHandlerWithDB(hub.DB())
	promptHandler.RegisterRoutes(router)

	// MCP Management API
	mgmtHandler := mcp.NewManagementHandler(manager)
	mgmtHandler.RegisterRoutes(router)

	// Legacy prompt config for Tirouter sync
	promptAPI := api.NewPromptAPIHandler()
	router.Handle("/api/v2/runtime/prompts", promptAPI)

	// Protocol info
	router.Handle("/mcp/protocol", http.HandlerFunc(handleProtocolInfo))

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	// Secret vault upsert endpoint for Tirouter JS tooling.
	// Accepts JSON {provider, key_id, value, source?} and stores encrypted.
	router.HandleFunc("/v1/secrets/upsert", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeTiBrainRequest(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Provider string `json:"provider"`
			KeyID    string `json:"key_id"`
			Value    string `json:"value"`
			Source   string `json:"source"`
			KeyType  string `json:"key_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		provider := strings.TrimSpace(body.Provider)
		keyID := strings.TrimSpace(body.KeyID)
		keyType := strings.TrimSpace(body.KeyType)
		if keyType == "" {
			keyType = "api-key"
		}
		value := body.Value
		if provider == "" || keyID == "" || value == "" {
			http.Error(w, "provider, key_id, and value are required", http.StatusBadRequest)
			return
		}
		enc, err := security.EncryptSecret(value)
		if err != nil {
			http.Error(w, "encrypt failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := hub.DB().Exec("INSERT OR REPLACE INTO api_secrets (provider, key_id, encrypted_value, key_type, source, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			provider, keyID, enc, keyType, body.Source, time.Now().Unix()); err != nil {
			http.Error(w, "db insert failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"provider": provider,
			"key_id":   keyID,
			"value":    value,
			"source":   body.Source,
		})
	})

	// Secret vault resolve endpoint for Tirouter JS tooling.
	// Accepts JSON {provider, key_id} and returns decrypted value.
	router.HandleFunc("/v1/secrets/resolve", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeTiBrainRequest(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Provider string `json:"provider"`
			KeyID    string `json:"key_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		provider := strings.TrimSpace(body.Provider)
		keyID := strings.TrimSpace(body.KeyID)
		if provider == "" || keyID == "" {
			http.Error(w, "provider and key_id are required", http.StatusBadRequest)
			return
		}
		var enc string
		err := hub.DB().QueryRow("SELECT encrypted_value FROM api_secrets WHERE provider = ? AND key_id = ?", provider, keyID).Scan(&enc)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		value, err := security.DecryptSecret(enc)
		if err != nil {
			http.Error(w, "decrypt failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"provider": provider,
			"key_id":   keyID,
			"value":    value,
			"source":   "api_secrets",
		})
	})

	// API Server routes (catch-all for /api/* paths not matched by prompt routes)
	router.Handle("/*", apiServer)

	port := getEnvOrDefault("TIBRAIN_PORT", "3005")
	log.Printf("TiBrain starting on :%s", port)
	log.Printf("Health: http://localhost:%s/health", port)
	log.Printf("Prompts: http://localhost:%s/api/v1/prompt/preflight", port)
	log.Printf("Prompt config: http://localhost:%s/api/v2/runtime/prompts", port)

	if err := http.ListenAndServe(":"+port, router); err != nil {
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

	dataDir := getEnvOrDefault("TIBRAIN_DATA_DIR", "")
	if dataDir == "" {
		dataDir = cfg.DataDir
	}
	if dataDir == "" {
		dataDir = `Z:\03_DATA\tibrain-database`
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

func authorizeTiBrainRequest(w http.ResponseWriter, r *http.Request) bool {
	bearer := r.Header.Get("Authorization")
	if bearer == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return false
	}
	token := strings.TrimPrefix(bearer, "Bearer ")
	expected := getEnvOrDefault("TIBRAIN_MCP_BEARER_TOKEN", "")
	if expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
		return false
	}
	return true
}

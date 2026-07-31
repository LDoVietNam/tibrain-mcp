// TiBrain main - Knowledge Service & Control Plane
package main

import (
	"log"
	"net/http"
	"os"

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
	cfg := config.Default()

	// Initialize database hub
	dataDir := getEnvOrDefault("TIBRAIN_DATA_DIR", "Z:\\03_DATA\\tibrain-database")
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
	_ = knowledge.NewKnowledgeIndexer(hub)
	_ = cloudflare.NewCloudflareService()
	_ = predictive.NewPredictiveMaintenance()

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

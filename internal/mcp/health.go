// Package mcp provides health check endpoints for MCP clients
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the overall health of an MCP client
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ClientHealth represents health information for a single client
type ClientHealth struct {
	Name         string       `json:"name"`
	Status       HealthStatus `json:"status"`
	Connected    bool         `json:"connected"`
	ToolsCount   int          `json:"tools_count"`
	LastPing     time.Time    `json:"last_ping"`
	LastError    string       `json:"last_error,omitempty"`
	CircuitState CircuitState `json:"circuit_state"`
}

// HealthCheck handles MCP client health monitoring
type HealthCheck struct {
	mu         sync.RWMutex
	clientMgr  *ClientManager
	healthMap  map[string]ClientHealth
	checkFuncs map[string]func(ctx context.Context) error // Optional custom health checks per client
}

// NewHealthCheck creates a new health check monitor
func NewHealthCheck(clientMgr *ClientManager) *HealthCheck {
	return &HealthCheck{
		clientMgr:  clientMgr,
		healthMap:  make(map[string]ClientHealth),
		checkFuncs: make(map[string]func(ctx context.Context) error),
	}
}

// RegisterCustomCheck adds a custom health check for a specific client
func (hc *HealthCheck) RegisterCustomCheck(clientName string, check func(ctx context.Context) error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checkFuncs[clientName] = check
}

// CheckAll performs health checks on all managed clients
func (hc *HealthCheck) CheckAll(ctx context.Context) map[string]ClientHealth {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	results := make(map[string]ClientHealth)
	clients := hc.clientMgr.ListClients()

	for _, name := range clients {
		health := hc.checkClient(ctx, name)
		hc.healthMap[name] = health
		results[name] = health
	}

	return results
}

// checkClient performs health check on a single client
func (hc *HealthCheck) checkClient(ctx context.Context, name string) ClientHealth {
	client, exists := hc.clientMgr.GetClient(name)
	if !exists {
		return ClientHealth{
			Name:      name,
			Status:    HealthStatusUnknown,
			Connected: false,
		}
	}

	health := ClientHealth{
		Name:      name,
		Connected: client.IsConnected(),
		LastPing:  time.Now(),
	}

	if client.IsConnected() {
		// Get tool count
		tools, err := client.ListTools(ctx)
		if err != nil {
			health.Status = HealthStatusDegraded
			health.LastError = err.Error()
		} else {
			health.ToolsCount = len(tools)
			health.Status = HealthStatusHealthy
		}

		// Run custom check if registered
		if checkFunc, ok := hc.checkFuncs[name]; ok {
			if err := checkFunc(ctx); err != nil {
				health.Status = HealthStatusDegraded
				health.LastError = err.Error()
			}
		}

		// Check circuit breaker
		if cb := hc.clientMgr.CircuitBreaker(name); cb != nil {
			health.CircuitState = cb.State()
			if cb.State() == CircuitOpen {
				health.Status = HealthStatusUnhealthy
			}
		}
	} else {
		health.Status = HealthStatusUnhealthy
		health.LastError = "client not connected"
	}

	return health
}

// HTTPHandler returns an http.Handler for the health check endpoint
func (hc *HealthCheck) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		results := hc.CheckAll(ctx)

		// Determine overall status
		overallStatus := HealthStatusHealthy
		for _, h := range results {
			switch h.Status {
			case HealthStatusUnhealthy:
				overallStatus = HealthStatusUnhealthy
				goto writeResponse
			case HealthStatusDegraded:
				overallStatus = HealthStatusDegraded
			}
		}

	writeResponse:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    overallStatus,
			"clients":   results,
			"timestamp": time.Now().UTC(),
		})
	})
}

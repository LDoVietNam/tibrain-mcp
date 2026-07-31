package main

import "github.com/ti/router/tibrain/internal/db"

// CodeGraphService handles code graph operations
type CodeGraphService struct {
	hub *db.Hub
}

// NewCodeGraphService creates a new code graph service
func NewCodeGraphService(hub *db.Hub) *CodeGraphService {
	return &CodeGraphService{hub: hub}
}

// BuildOrUpdateGraph builds or updates the code graph
func (c *CodeGraphService) BuildOrUpdateGraph() error {
	return nil
}

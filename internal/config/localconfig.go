// Package config provides project-level configuration loading.
// Phase 3: supports .tibrainrc / .1mcprc in project root.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LocalConfig represents a project-level configuration file.
// Filename options: .tibrainrc (YAML) or .1mcprc (JSON).
type LocalConfig struct {
	ProjectName string            `json:"project_name" yaml:"project_name"`
	Version     string            `json:"version" yaml:"version"`
	MCPServers  []MCPServerConfig `json:"mcp_servers" yaml:"mcp_servers"`
	BasePath    string            `json:"base_path" yaml:"base_path"`
}

// DetectAndLoad searches for a project config file starting at cwd
// and walking up to root. Returns nil if no config file is found.
func DetectAndLoad() (*LocalConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		for _, filename := range []string{".tibrainrc", ".1mcprc"} {
			path := filepath.Join(dir, filename)
			if _, err := os.Stat(path); err == nil {
				cfg, err := loadLocalConfig(path)
				if err != nil {
					return nil, fmt.Errorf("load project config %s: %w", path, err)
				}
				return cfg, nil
			}
		}
	}
	return nil, nil
}

// loadLocalConfig loads a YAML or JSON project config file.
func loadLocalConfig(path string) (*LocalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &LocalConfig{}

	// Try YAML first, then JSON
	if yamlCfg, err := loadYamlConfig(data); err == nil {
		cfg = yamlCfg
	} else if jsonCfg, err := loadJsonConfig(data); err == nil {
		cfg = jsonCfg
	} else {
		return nil, fmt.Errorf("unsupported config format in %s", path)
	}

	return cfg, nil
}

func loadYamlConfig(data []byte) (*LocalConfig, error) {
	cfg := &LocalConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse yaml config: %w", err)
	}
	return cfg, nil
}

func loadJsonConfig(data []byte) (*LocalConfig, error) {
	cfg := &LocalConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse json config: %w", err)
	}
	return cfg, nil
}

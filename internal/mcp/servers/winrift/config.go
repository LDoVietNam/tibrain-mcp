package winrift

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads Winrift MCP configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Resolve relative paths
	if cfg.RootDir == "" {
		cfg.RootDir = getEnvOrDefault("WINRIFT_ROOT", "Z:\\01_PROJECTS\\apps\\Winrift-main")
	}

	if cfg.WrapperDir == "" {
		cfg.WrapperDir = filepath.Join(
			filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))),
			"integrations", "winrift", "ps_wrapper",
		)
	}

	if cfg.PSPowerShell == "" {
		cfg.PSPowerShell = "powershell.exe"
	}

	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 300
	}

	if cfg.CacheTTLSeconds == 0 {
		cfg.CacheTTLSeconds = 30
	}

	return &cfg, nil
}

// SaveConfig writes configuration to YAML file
func (c *Config) SaveConfig(path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ValidateConfig checks that all required paths exist
func (c *Config) ValidateConfig() error {
	if c.RootDir == "" {
		return fmt.Errorf("root_dir is required")
	}

	if _, err := os.Stat(c.RootDir); os.IsNotExist(err) {
		return fmt.Errorf("winrift root directory does not exist: %s", c.RootDir)
	}

	if c.WrapperDir != "" {
		if _, err := os.Stat(c.WrapperDir); os.IsNotExist(err) {
			return fmt.Errorf("ps wrapper directory does not exist: %s", c.WrapperDir)
		}
	}

	return nil
}

// DefaultConfigFile returns default config path
func DefaultConfigFile() string {
	// Check common locations
	locations := []string{
		"Z:\\01_PROJECTS\\apps\\products\\tibrain\\config\\winrift.yaml",
		"./config/winrift.yaml",
		"~/.tibrain/winrift.yaml",
	}

	for _, loc := range locations {
		expanded := os.ExpandEnv(loc)
		if _, err := os.Stat(expanded); err == nil {
			return expanded
		}
	}

	return ""
}

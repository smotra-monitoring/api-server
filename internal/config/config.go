package config

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadAndValidate loads configuration from a config file
func LoadAndValidate(filepath string) (*Config, error) {
	cfg, err := loadFromFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from file: %w", err)
	}

	cfg.applyEnvOverrides()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the configuration.
func (c *Config) applyEnvOverrides() {
	if pw := os.Getenv("DATABASE_PASSWORD"); pw != "" && c.PostgresConfig != nil {
		c.PostgresConfig.Password = pw
	}
}

// loadFromFile loads configuration from a YAML or JSON file
func loadFromFile(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{}

	// Try YAML first, then JSON
	err = yaml.Unmarshal(data, cfg)
	if err == nil {
		return cfg, nil
	}

	// Try JSON
	jsonErr := json.Unmarshal(data, cfg)
	if jsonErr == nil {
		return cfg, nil
	}

	return nil, fmt.Errorf("failed to parse config file as YAML or JSON: YAML error: %w, JSON error: %v", err, jsonErr)
}

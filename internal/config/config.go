// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds global configuration
type Config struct {
	Embedding EmbeddingConfig `yaml:"embedding"`
	Storage   StorageConfig   `yaml:"storage"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type EmbeddingConfig struct {
	Provider string `yaml:"provider"` // "voyage"
	Model    string `yaml:"model"`    // "voyage-4-large"
}

type StorageConfig struct {
	QdrantURL string `yaml:"qdrant_url"`
	Neo4jURL  string `yaml:"neo4j_url"`
	RedisURL  string `yaml:"redis_url"`
}

type LoggingConfig struct {
	Level     string `yaml:"level"` // error|warn|info|debug
	MaxSizeMB int    `yaml:"max_size_mb"`
	MaxFiles  int    `yaml:"max_files"`
}

// RepoConfig holds per-repository configuration
type RepoConfig struct {
	Name          string            `yaml:"name"`
	DefaultBranch string            `yaml:"default_branch"`
	Modules       map[string]Module `yaml:"modules"`
	Include       []string          `yaml:"include"`
	Exclude       []string          `yaml:"exclude"`
}

type Module struct {
	Description string            `yaml:"description"`
	Submodules  map[string]string `yaml:"submodules"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Embedding: EmbeddingConfig{
			Provider: "voyage",
			Model:    "voyage-4-large",
		},
		Storage: StorageConfig{
			QdrantURL: "http://localhost:6333",
			Neo4jURL:  "bolt://localhost:7687",
			RedisURL:  "redis://localhost:6379",
		},
		Logging: LoggingConfig{
			Level:     "info",
			MaxSizeMB: 50,
			MaxFiles:  3,
		},
	}
}

// LoadConfig loads config from file or returns defaults
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadRepoConfig loads .ai-devtools.yaml from repo root
func LoadRepoConfig(repoPath string) (*RepoConfig, error) {
	path := filepath.Join(repoPath, ".ai-devtools.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		CodeIndex RepoConfig `yaml:"code-index"`
	}

	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.CodeIndex, nil
}

# config package

Configuration loading for global and per-repo settings.

## Purpose

Load and provide default configuration for the indexer. Supports global config file and per-repository `.ai-devtools.yaml`.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Config` | Global config | `config.go:16-20` |
| `EmbeddingConfig` | Embedding settings | `config.go:22-25` |
| `StorageConfig` | Storage URLs | `config.go:27-31` |
| `LoggingConfig` | Logging settings | `config.go:33-37` |
| `RepoConfig` | Per-repo config | `config.go:39-45` |
| `Module` | Module definition | `config.go:47-50` |

## Usage

```go
// Global config
cfg, err := config.LoadConfig("~/.config/code-index/config.yaml")
// Returns defaults if file missing

// Repo config
repoCfg, err := config.LoadRepoConfig("/path/to/repo")
// Reads .ai-devtools.yaml from repo root
```

## Default Values

| Setting | Default |
|---------|---------|
| `embedding.provider` | `voyage` |
| `embedding.model` | `voyage-4-large` |
| `storage.qdrant_url` | `http://localhost:6333` |
| `storage.neo4j_url` | `bolt://localhost:7687` |
| `storage.redis_url` | `redis://localhost:6379` |
| `logging.level` | `info` |
| `logging.max_size_mb` | `50` |
| `logging.max_files` | `3` |

## File Locations

| Config | Path |
|--------|------|
| Global | `~/.config/code-index/config.yaml` |
| Repo | `<repo>/.ai-devtools.yaml` |

## Repo Config Format

```yaml
code-index:
  name: my-repo
  default_branch: main
  modules:
    fisio:
      description: "Core module"
      submodules:
        imports: "Data importers"
  include:
    - "**/*.py"
  exclude:
    - "**/vendor/**"
```

## Gotchas

1. **Missing global config** - Returns defaults, not an error
2. **Missing repo config** - Returns error (required for indexing)
3. **YAML wrapper** - Repo config nested under `code-index:` key

// Package search provides the semantic code search handler for MCP.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/randalmurphy/ai-devtools-admin/internal/cache"
	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/embedding"
	"github.com/randalmurphy/ai-devtools-admin/internal/mcp"
	"github.com/randalmurphy/ai-devtools-admin/internal/metrics"
	"github.com/randalmurphy/ai-devtools-admin/internal/store"
)

// Handler implements mcp.Handler for code search.
type Handler struct {
	config   *config.Config
	embedder *embedding.VoyageClient
	store    *store.QdrantStore
	cache    *cache.RedisCache
	metrics  *metrics.Logger
	logger   *slog.Logger
}

// NewHandler creates a new search handler.
func NewHandler(cfg *config.Config, voyageKey string, logger *slog.Logger) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)

	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	var queryCache *cache.RedisCache
	if cfg.Storage.RedisURL != "" {
		queryCache, err = cache.NewRedisCache(cfg.Storage.RedisURL)
		if err != nil {
			logger.Warn("Redis cache unavailable, continuing without cache", "error", err)
		}
	}

	// Initialize metrics logger
	var metricsLogger *metrics.Logger
	homeDir, _ := os.UserHomeDir()
	metricsPath := filepath.Join(homeDir, ".local", "share", "code-index", "metrics.jsonl")
	if err := os.MkdirAll(filepath.Dir(metricsPath), 0755); err == nil {
		metricsLogger, _ = metrics.NewLogger(metricsPath)
	}

	return &Handler{
		config:   cfg,
		embedder: embedder,
		store:    qdrantStore,
		cache:    queryCache,
		metrics:  metricsLogger,
		logger:   logger,
	}, nil
}

// Close releases resources held by the handler.
func (h *Handler) Close() error {
	if h.cache != nil {
		h.cache.Close()
	}
	if h.store != nil {
		h.store.Close()
	}
	if h.metrics != nil {
		h.metrics.Close()
	}
	return nil
}

// ListTools returns available tools (implements mcp.Handler).
func (h *Handler) ListTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "search_code",
			Description: "Find code by concept using semantic search. Use when you don't know exact symbol names but know what you're looking for.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]mcp.Property{
					"query": {
						Type:        "string",
						Description: "Describe what you're looking for in natural language",
					},
					"repo": {
						Type:        "string",
						Description: "Repository to search: r3, m32rimm, or all (default: inferred from cwd)",
					},
					"module": {
						Type:        "string",
						Description: "Filter to specific module (e.g., 'fisio.imports')",
					},
					"include_tests": {
						Type:        "string",
						Description: "Test file handling: include (default), exclude, or only",
						Enum:        []string{"include", "exclude", "only"},
					},
					"limit": {
						Type:        "number",
						Description: "Maximum results to return (default: 10)",
					},
				},
				Required: []string{"query"},
			},
		},
	}
}

// CallTool processes a tool invocation (implements mcp.Handler).
func (h *Handler) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	switch name {
	case "search_code":
		return h.searchCode(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ListResources returns available resources (implements mcp.Handler).
func (h *Handler) ListResources() []mcp.Resource {
	return []mcp.Resource{
		{
			URI:         "codeindex://relevant",
			Name:        "Contextually relevant code",
			Description: "Auto-retrieved code based on conversation context",
			MimeType:    "text/markdown",
		},
	}
}

// ReadResource processes a resource read (implements mcp.Handler).
func (h *Handler) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	switch uri {
	case "codeindex://relevant":
		return h.getRelevantContext(ctx)
	default:
		return nil, fmt.Errorf("unknown resource: %s", uri)
	}
}

func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	startTime := time.Now()

	// Parse arguments
	query, _ := args["query"].(string)
	if query == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{{Type: "text", Text: "query parameter is required"}},
			IsError: true,
		}, nil
	}

	repo, _ := args["repo"].(string)
	if repo == "" {
		repo = h.inferRepo()
	}

	module, _ := args["module"].(string)
	includeTests, _ := args["include_tests"].(string)
	if includeTests == "" {
		includeTests = "include"
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	if h.logger != nil {
		h.logger.Info("search_code called",
			"query", query,
			"repo", repo,
			"module", module,
			"limit", limit,
		)
	}

	// Check cache if available
	var cacheKey string
	if h.cache != nil {
		version, _ := h.cache.GetIndexVersion(ctx, repo)
		cacheKey = cache.QueryCacheKey(repo, query, version)

		if cached, err := h.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			if h.logger != nil {
				h.logger.Debug("cache hit", "query", query, "repo", repo)
			}
			// Log metrics for cache hit
			if h.metrics != nil {
				h.metrics.LogSearch(query, "concept", -1, time.Since(startTime).Milliseconds(), true)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{{Type: "text", Text: cached}},
			}, nil
		}
	}

	// Generate query embedding
	vectors, err := h.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// Build filter
	filter := make(map[string]interface{})
	if repo != "" && repo != "all" {
		filter["repo"] = repo
	}
	if module != "" {
		filter["module_path"] = module
	}
	switch includeTests {
	case "exclude":
		filter["is_test"] = false
	case "only":
		filter["is_test"] = true
	}

	// Search - get extra for weighting adjustment
	results, err := h.store.Search(ctx, "chunks", vectors[0], limit*2, filter)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Apply retrieval weight and truncate
	results = h.applyWeights(results, limit)

	// Format response
	response := h.formatSearchResponse(query, results, repo)

	// Cache result
	if h.cache != nil && cacheKey != "" {
		ttl := time.Duration(h.config.Cache.QueryTTLMinutes) * time.Minute
		if err := h.cache.Set(ctx, cacheKey, response, ttl); err != nil {
			h.logger.Warn("failed to cache result", "error", err)
		}
	}

	// Log metrics for cache miss
	if h.metrics != nil {
		h.metrics.LogSearch(query, "concept", len(results), time.Since(startTime).Milliseconds(), false)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: response}},
	}, nil
}

// applyWeights re-ranks results by score * retrieval_weight, then truncates.
func (h *Handler) applyWeights(chunks []chunk.Chunk, limit int) []chunk.Chunk {
	// Sort by effective score (score * retrieval_weight) descending
	sort.Slice(chunks, func(i, j int) bool {
		scoreI := chunks[i].Score * chunks[i].RetrievalWeight
		scoreJ := chunks[j].Score * chunks[j].RetrievalWeight
		return scoreI > scoreJ
	})

	if len(chunks) > limit {
		chunks = chunks[:limit]
	}
	return chunks
}

func (h *Handler) formatSearchResponse(query string, results []chunk.Chunk, repo string) string {
	if len(results) == 0 {
		return h.formatEmptyResponse(query, repo)
	}

	response := SearchResponse{
		QueryType:  "concept_search",
		Results:    make([]SearchResult, len(results)),
		TotalCount: len(results),
		HasMore:    false,
	}

	for i, c := range results {
		response.Results[i] = SearchResult{
			FilePath:   c.FilePath,
			Module:     c.ModulePath,
			SymbolName: c.SymbolName,
			Kind:       c.Kind,
			StartLine:  c.StartLine,
			EndLine:    c.EndLine,
			Content:    c.Content,
			Docstring:  c.Docstring,
			IsTest:     c.IsTest,
		}
	}

	data, _ := json.MarshalIndent(response, "", "  ")
	return string(data)
}

func (h *Handler) formatEmptyResponse(query, repo string) string {
	response := map[string]interface{}{
		"results":    []interface{}{},
		"query_type": "concept_search",
		"message":    fmt.Sprintf("No direct matches for '%s'", query),
		"suggestions": []string{
			"Try broader search terms",
			"Check if the repository is indexed: code-indexer status",
		},
	}

	if repo != "" && repo != "all" {
		response["hint"] = fmt.Sprintf("Searched only in %s. Try repo: 'all' for cross-repo search.", repo)
	}

	data, _ := json.MarshalIndent(response, "", "  ")
	return string(data)
}

func (h *Handler) getRelevantContext(ctx context.Context) (*mcp.ReadResourceResult, error) {
	// Placeholder for conversation-aware context
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{
				URI:      "codeindex://relevant",
				MimeType: "text/markdown",
				Text:     "No contextual suggestions available. Use search_code tool for explicit searches.",
			},
		},
	}, nil
}

func (h *Handler) inferRepo() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	homeDir, _ := os.UserHomeDir()
	reposDir := filepath.Join(homeDir, "repos")

	if rel, err := filepath.Rel(reposDir, cwd); err == nil {
		// Check we're actually under reposDir (rel doesn't start with ..)
		if !strings.HasPrefix(rel, "..") {
			// First path component is the repo name
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			if len(parts) > 0 && parts[0] != "." {
				return parts[0]
			}
		}
	}

	return ""
}

// SearchResponse is the structured search result.
type SearchResponse struct {
	QueryType  string         `json:"query_type"`
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count"`
	HasMore    bool           `json:"has_more"`
	Cursor     string         `json:"cursor,omitempty"`
}

// SearchResult is a single search result.
type SearchResult struct {
	FilePath   string `json:"file_path"`
	Module     string `json:"module"`
	SymbolName string `json:"symbol_name,omitempty"`
	Kind       string `json:"kind,omitempty"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Content    string `json:"content"`
	Docstring  string `json:"docstring,omitempty"`
	IsTest     bool   `json:"is_test"`
}

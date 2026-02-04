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

	"github.com/randalmurphal/code-indexer/internal/cache"
	"github.com/randalmurphal/code-indexer/internal/chunk"
	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/embedding"
	"github.com/randalmurphal/code-indexer/internal/graph"
	"github.com/randalmurphal/code-indexer/internal/mcp"
	"github.com/randalmurphal/code-indexer/internal/metrics"
	"github.com/randalmurphal/code-indexer/internal/store"
)

// Handler implements mcp.Handler for code search.
type Handler struct {
	config        *config.Config
	embedder      *embedding.VoyageClient
	store         *store.QdrantStore
	graphStore    *graph.Neo4jStore
	cache         *cache.RedisCache
	metrics       *metrics.Logger
	classifier    *Classifier
	suggestionGen *SuggestionGenerator
	logger        *slog.Logger
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

	// Initialize Neo4j graph store if configured
	var graphStore *graph.Neo4jStore
	if cfg.Storage.Neo4jURL != "" {
		// Try to connect to Neo4j (optional - graph expansion won't work without it)
		neo4jUser := os.Getenv("NEO4J_USER")
		if neo4jUser == "" {
			neo4jUser = "neo4j"
		}
		neo4jPass := os.Getenv("NEO4J_PASSWORD")

		if neo4jPass != "" {
			graphStore, err = graph.NewNeo4jStore(cfg.Storage.Neo4jURL, neo4jUser, neo4jPass)
			if err != nil {
				logger.Warn("Neo4j unavailable, graph expansion disabled", "error", err)
			}
		} else {
			logger.Warn("NEO4J_PASSWORD not set, graph expansion disabled")
		}
	}

	return &Handler{
		config:        cfg,
		embedder:      embedder,
		store:         qdrantStore,
		graphStore:    graphStore,
		cache:         queryCache,
		metrics:       metricsLogger,
		classifier:    NewClassifier(),
		suggestionGen: NewSuggestionGenerator(),
		logger:        logger,
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
	if h.graphStore != nil {
		h.graphStore.Close(context.Background())
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
					"cursor": {
						Type:        "string",
						Description: "Pagination cursor from previous response (for fetching next page)",
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

	// Handle cursor for pagination
	var offset int
	if cursorStr, ok := args["cursor"].(string); ok && cursorStr != "" {
		cursor, err := DecodeCursor(cursorStr)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{{Type: "text", Text: fmt.Sprintf("invalid cursor: %s", err.Error())}},
				IsError: true,
			}, nil
		}
		offset = cursor.Offset
	}

	// Classify query to determine search strategy
	queryType := h.classifier.Classify(query)
	strategy := h.classifier.Route(queryType)

	// Override limit if strategy specifies
	if strategy.MaxResults > 0 && strategy.MaxResults < limit {
		limit = strategy.MaxResults
	}

	if h.logger != nil {
		h.logger.Info("search_code called",
			"query", query,
			"query_type", string(queryType),
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
			if h.metrics != nil {
				h.metrics.LogSearch(query, string(queryType), -1, time.Since(startTime).Milliseconds(), true)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{{Type: "text", Text: cached}},
			}, nil
		}
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

	// Route to appropriate search based on strategy
	// Fetch more results than needed for pagination
	fetchLimit := offset + limit + 1
	var results []chunk.Chunk
	var err error

	switch {
	case strategy.UseSymbolIndex:
		results, err = h.searchBySymbol(ctx, query, filter, fetchLimit)
	case strategy.UsePatternIndex:
		results, err = h.searchByPattern(ctx, query, filter, fetchLimit)
	default:
		results, err = h.searchSemantic(ctx, query, filter, fetchLimit)
	}

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Apply graph expansion if enabled and graph store is available
	if strategy.UseGraphExpansion && h.graphStore != nil && len(results) > 0 {
		results = h.expandWithGraph(ctx, results, repo, strategy.GraphDepth, fetchLimit)
	}

	// Convert chunks to search results for pagination
	searchResults := make([]SearchResult, len(results))
	for i, c := range results {
		searchResults[i] = SearchResult{
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

	// Apply pagination
	queryHash := HashQuery(query, repo, module)
	paginated := Paginate(searchResults, offset, limit, queryHash, string(queryType))

	// Format response
	var response string
	if len(paginated.Results) == 0 && offset == 0 {
		response = h.formatEmptyResponse(query, repo)
	} else {
		data, _ := json.MarshalIndent(paginated, "", "  ")
		response = string(data)
	}

	// Cache result
	if h.cache != nil && cacheKey != "" {
		ttl := time.Duration(h.config.Cache.QueryTTLMinutes) * time.Minute
		if err := h.cache.Set(ctx, cacheKey, response, ttl); err != nil {
			h.logger.Warn("failed to cache result", "error", err)
		}
	}

	// Log metrics
	if h.metrics != nil {
		h.metrics.LogSearch(query, string(queryType), len(paginated.Results), time.Since(startTime).Milliseconds(), false)
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

// searchSemantic performs vector similarity search.
func (h *Handler) searchSemantic(ctx context.Context, query string, filter map[string]interface{}, limit int) ([]chunk.Chunk, error) {
	vectors, err := h.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// Get extra results for weighting adjustment
	results, err := h.store.Search(ctx, "chunks", vectors[0], limit*2, filter)
	if err != nil {
		return nil, err
	}

	return h.applyWeights(results, limit), nil
}

// searchBySymbol searches for exact or fuzzy symbol name matches.
func (h *Handler) searchBySymbol(ctx context.Context, query string, filter map[string]interface{}, limit int) ([]chunk.Chunk, error) {
	symbolName := extractSymbolName(query)
	if symbolName == "" {
		// Fall back to semantic search if no symbol found
		return h.searchSemantic(ctx, query, filter, limit)
	}

	// Add symbol filter
	symbolFilter := make(map[string]interface{})
	for k, v := range filter {
		symbolFilter[k] = v
	}
	symbolFilter["symbol_name"] = symbolName

	// Try exact match first
	results, err := h.store.SearchByFilter(ctx, "chunks", symbolFilter, limit)
	if err != nil {
		return nil, err
	}

	// If no exact match, fall back to semantic search
	if len(results) == 0 {
		return h.searchSemantic(ctx, query, filter, limit)
	}

	return results, nil
}

// searchByPattern searches for code matching known patterns.
func (h *Handler) searchByPattern(ctx context.Context, query string, filter map[string]interface{}, limit int) ([]chunk.Chunk, error) {
	// First, search for pattern description chunks
	patternFilter := make(map[string]interface{})
	for k, v := range filter {
		patternFilter[k] = v
	}
	patternFilter["kind"] = "pattern"

	results, err := h.store.SearchByFilter(ctx, "chunks", patternFilter, limit)
	if err != nil {
		return nil, err
	}

	// If we found pattern chunks, return them
	if len(results) > 0 {
		return results, nil
	}

	// Fall back to semantic search for pattern-related queries
	return h.searchSemantic(ctx, query, filter, limit)
}

func (h *Handler) formatEmptyResponse(query, repo string) string {
	// Generate suggestions based on query
	suggestions := h.suggestionGen.Generate(query)
	response := h.suggestionGen.FormatEmptyResponse(query, repo, suggestions)

	data, _ := json.MarshalIndent(response, "", "  ")
	return string(data)
}

func (h *Handler) getRelevantContext(ctx context.Context) (*mcp.ReadResourceResult, error) {
	// Get context from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return h.emptyRelevantContext(), nil
	}

	// Infer repo from cwd
	repo := h.inferRepo()
	if repo == "" {
		return h.emptyRelevantContext(), nil
	}

	// Find relevant files based on current directory
	var suggestions []string

	// Try to use graph to find related files based on cwd
	if h.graphStore != nil {
		// Get relative path within repo
		homeDir, _ := os.UserHomeDir()
		repoPath := filepath.Join(homeDir, "repos", repo)
		relCwd, _ := filepath.Rel(repoPath, cwd)

		// Find files in or near current directory
		relatedFiles, err := h.graphStore.FindRelatedFiles(ctx, repo, relCwd, 10)
		if err == nil && len(relatedFiles) > 0 {
			for _, f := range relatedFiles {
				suggestions = append(suggestions, fmt.Sprintf("- `%s` (related via imports/calls)", f.Path))
			}
		}
	}

	// If no graph results, use semantic search based on directory name
	if len(suggestions) == 0 {
		dirName := filepath.Base(cwd)
		if dirName != "." && dirName != repo {
			results, err := h.searchSemantic(ctx, dirName, map[string]interface{}{"repo": repo}, 5)
			if err == nil {
				for _, c := range results {
					suggestions = append(suggestions, fmt.Sprintf("- `%s:%d-%d` %s (%s)",
						c.FilePath, c.StartLine, c.EndLine, c.SymbolName, c.Kind))
				}
			}
		}
	}

	if len(suggestions) == 0 {
		return h.emptyRelevantContext(), nil
	}

	// Format response
	text := fmt.Sprintf("# Relevant Context for %s\n\n", repo)
	text += fmt.Sprintf("Based on current directory: `%s`\n\n", cwd)
	text += "## Related Code\n\n"
	for _, s := range suggestions {
		text += s + "\n"
	}
	text += "\n*Use `search_code` for more specific queries.*"

	// Log the context injection if metrics available
	if h.metrics != nil {
		h.metrics.LogContextInject(cwd, len(suggestions), 0.7)
	}

	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{
				URI:      "codeindex://relevant",
				MimeType: "text/markdown",
				Text:     text,
			},
		},
	}, nil
}

func (h *Handler) emptyRelevantContext() *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{
				URI:      "codeindex://relevant",
				MimeType: "text/markdown",
				Text:     "No contextual suggestions available. Use `search_code` tool for explicit searches.",
			},
		},
	}
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

// expandWithGraph expands search results using graph relationships.
// For each result, it finds related symbols via CALLS, EXTENDS, and IMPORTS
// relationships and adds them to the result set.
func (h *Handler) expandWithGraph(ctx context.Context, results []chunk.Chunk, repo string, depth int, limit int) []chunk.Chunk {
	if h.graphStore == nil || len(results) == 0 {
		return results
	}

	// Collect symbol names from results
	var symbolNames []string
	seenSymbols := make(map[string]bool)
	for _, c := range results {
		if c.SymbolName != "" && !seenSymbols[c.SymbolName] {
			symbolNames = append(symbolNames, c.SymbolName)
			seenSymbols[c.SymbolName] = true
		}
	}

	if len(symbolNames) == 0 {
		return results
	}

	// Expand from the found symbols
	expandedSymbols, err := h.graphStore.ExpandFromSymbols(ctx, repo, symbolNames, depth, limit)
	if err != nil {
		h.logger.Warn("graph expansion failed", "error", err)
		return results
	}

	if len(expandedSymbols) == 0 {
		return results
	}

	// Look up chunks for expanded symbols
	seenChunks := make(map[string]bool)
	for _, c := range results {
		seenChunks[c.ID] = true
	}

	for _, sym := range expandedSymbols {
		// Skip symbols we already have
		if seenSymbols[sym.Name] {
			continue
		}

		// Search for the symbol's chunk
		filter := map[string]interface{}{
			"repo":        repo,
			"symbol_name": sym.Name,
		}

		chunks, err := h.store.SearchByFilter(ctx, "chunks", filter, 1)
		if err != nil || len(chunks) == 0 {
			continue
		}

		// Add if not already in results
		c := chunks[0]
		if !seenChunks[c.ID] {
			// Mark as expanded result with lower score
			c.Score = 0.5 // Lower than direct results
			results = append(results, c)
			seenChunks[c.ID] = true
		}
	}

	// Re-apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results
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

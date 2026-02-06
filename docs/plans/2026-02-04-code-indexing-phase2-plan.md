# Code Indexing Phase 2: MCP Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create an MCP server that exposes code search to Claude Code, with hooks for automatic context injection and index updates.

**Architecture:** Go MCP server (`code-index-mcp`) using stdio transport, Redis for query caching, JSONL metrics logging. Integrates with Claude Code via MCP registration and hooks.

**Tech Stack:** Go 1.21+, MCP SDK (stdio), Redis, existing indexer components

**Prerequisites:** Phase 1 complete (indexer pipeline working)

**Design Doc:** `docs/plans/2026-02-04-code-indexing-design.md`

---

## Task 1: MCP Server Scaffold

**Files:**
- Create: `cmd/code-index-mcp/main.go`
- Create: `internal/mcp/server.go`
- Create: `internal/mcp/types.go`

**Step 1: Create MCP types**

```go
// internal/mcp/types.go
package mcp

import "encoding/json"

// JSON-RPC types for MCP protocol

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP-specific types

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool definitions

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Resource definitions

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

type ReadResourceParams struct {
	URI string `json:"uri"`
}

type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}
```

**Step 2: Create MCP server**

```go
// internal/mcp/server.go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Handler processes MCP requests
type Handler interface {
	HandleToolCall(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error)
	HandleResourceRead(ctx context.Context, uri string) (*ReadResourceResult, error)
	GetTools() []Tool
	GetResources() []Resource
}

// Server implements the MCP protocol over stdio
type Server struct {
	handler Handler
	logger  *slog.Logger

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	mu       sync.Mutex
	shutdown bool
}

// NewServer creates a new MCP server
func NewServer(handler Handler, logger *slog.Logger) *Server {
	return &Server{
		handler: handler,
		logger:  logger,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

// Run starts the server and processes requests
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.stdin)
	// Increase buffer size for large messages
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Error("failed to parse request", "error", err)
			continue
		}

		resp := s.handleRequest(ctx, &req)
		if resp != nil {
			s.sendResponse(resp)
		}
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *Request) *Response {
	s.logger.Debug("handling request", "method", req.Method, "id", req.ID)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return nil // Notification, no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(ctx, req)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &Error{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    "code-index-mcp",
			Version: "0.1.0",
		},
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleToolsList(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListToolsResult{Tools: s.handler.GetTools()},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32602, Message: "invalid params"},
		}
	}

	result, err := s.handler.HandleToolCall(ctx, params.Name, params.Arguments)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: &CallToolResult{
				Content: []Content{{Type: "text", Text: err.Error()}},
				IsError: true,
			},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleResourcesList(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ListResourcesResult{Resources: s.handler.GetResources()},
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, req *Request) *Response {
	var params ReadResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32602, Message: "invalid params"},
		}
	}

	result, err := s.handler.HandleResourceRead(ctx, params.URI)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: -32000, Message: err.Error()},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) sendResponse(resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		return
	}

	fmt.Fprintln(s.stdout, string(data))
}

// LogToStderr writes a message to stderr (for Claude to see)
func (s *Server) LogToStderr(format string, args ...interface{}) {
	fmt.Fprintf(s.stderr, "[code-index] "+format+"\n", args...)
}
```

**Step 3: Create main entry point**

```go
// cmd/code-index-mcp/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/mcp"
	"github.com/randalmurphal/code-indexer/internal/search"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "code-index-mcp",
	Short: "MCP server for code indexing",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	// Setup logging to file (not stdout - that's for MCP protocol)
	logFile, err := setupLogging()
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Load config
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		logger.Warn("failed to load config, using defaults", "error", err)
		cfg = config.DefaultConfig()
	}

	// Create search handler
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY not set")
	}

	handler, err := search.NewHandler(cfg, voyageKey, logger)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// Create and run MCP server
	server := mcp.NewServer(handler, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down")
		cancel()
	}()

	logger.Info("starting MCP server")
	return server.Run(ctx)
}

func setupLogging() (*os.File, error) {
	homeDir, _ := os.UserHomeDir()
	logDir := homeDir + "/.local/share/code-index/logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(logDir+"/mcp-server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

func getGlobalConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return homeDir + "/.config/code-index/config.yaml"
}
```

**Step 4: Build and verify**

```bash
go build -o bin/code-index-mcp ./cmd/code-index-mcp
./bin/code-index-mcp --help
```

**Step 5: Commit**

```bash
git add cmd/code-index-mcp/ internal/mcp/
git commit -m "feat: scaffold MCP server with stdio transport"
```

---

## Task 2: Search Handler with search_code Tool

**Files:**
- Create: `internal/search/handler.go`
- Create: `internal/search/handler_test.go`

**Step 1: Write test for search handler**

```go
// internal/search/handler_test.go
package search

import (
	"context"
	"os"
	"testing"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerGetTools(t *testing.T) {
	// Can test without external services
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	tools := handler.GetTools()

	require.Len(t, tools, 1)
	assert.Equal(t, "search_code", tools[0].Name)
	assert.Contains(t, tools[0].Description, "Find code by concept")

	// Verify required params
	assert.Contains(t, tools[0].InputSchema.Required, "query")
}

func TestHandlerSearchIntegration(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" || os.Getenv("QDRANT_URL") == "" {
		t.Skip("Integration test requires VOYAGE_API_KEY and QDRANT_URL")
	}

	cfg := config.DefaultConfig()
	handler, err := NewHandler(cfg, os.Getenv("VOYAGE_API_KEY"), nil)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := handler.HandleToolCall(ctx, "search_code", map[string]interface{}{
		"query": "hello world",
		"limit": float64(5),
	})
	require.NoError(t, err)

	assert.False(t, result.IsError)
	assert.NotEmpty(t, result.Content)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create search handler**

```go
// internal/search/handler.go
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/randalmurphal/code-indexer/internal/chunk"
	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/embedding"
	"github.com/randalmurphal/code-indexer/internal/mcp"
	"github.com/randalmurphal/code-indexer/internal/store"
)

// Handler implements mcp.Handler for code search
type Handler struct {
	config   *config.Config
	embedder *embedding.VoyageClient
	store    *store.QdrantStore
	logger   *slog.Logger
}

// NewHandler creates a new search handler
func NewHandler(cfg *config.Config, voyageKey string, logger *slog.Logger) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)

	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	return &Handler{
		config:   cfg,
		embedder: embedder,
		store:    qdrantStore,
		logger:   logger,
	}, nil
}

// GetTools returns available tools
func (h *Handler) GetTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "search_code",
			Description: "Find code by concept when you don't know exact symbol names. Use for semantic discovery of functions, classes, and patterns.",
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

// GetResources returns available resources
func (h *Handler) GetResources() []mcp.Resource {
	return []mcp.Resource{
		{
			URI:         "codeindex://relevant",
			Name:        "Contextually relevant code",
			Description: "Auto-retrieved code based on conversation context",
			MimeType:    "text/markdown",
		},
	}
}

// HandleToolCall processes a tool invocation
func (h *Handler) HandleToolCall(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	switch name {
	case "search_code":
		return h.searchCode(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// HandleResourceRead processes a resource read
func (h *Handler) HandleResourceRead(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	switch uri {
	case "codeindex://relevant":
		return h.getRelevantContext(ctx)
	default:
		return nil, fmt.Errorf("unknown resource: %s", uri)
	}
}

func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
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

	h.logger.Info("search_code called",
		"query", query,
		"repo", repo,
		"module", module,
		"limit", limit,
	)

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

	// Search
	results, err := h.store.Search(ctx, "chunks", vectors[0], limit*2, filter) // Get extra for weighting
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Apply retrieval weight and sort
	results = h.applyWeights(results, limit)

	// Format response
	response := h.formatSearchResponse(query, results, repo)

	return &mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: response}},
	}, nil
}

func (h *Handler) applyWeights(chunks []chunk.Chunk, limit int) []chunk.Chunk {
	// Sort by effective score (score * retrieval_weight)
	// For now, just truncate to limit since Qdrant doesn't return scores easily
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
		QueryType:   "concept_search",
		Results:     make([]SearchResult, len(results)),
		TotalCount:  len(results),
		HasMore:     false,
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
	// TODO: Implement conversation-aware context
	// For now, return empty
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
	// Try to infer repo from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Check if we're in a known repo
	homeDir, _ := os.UserHomeDir()
	reposDir := filepath.Join(homeDir, "repos")

	if rel, err := filepath.Rel(reposDir, cwd); err == nil && !filepath.IsAbs(rel) {
		parts := filepath.SplitList(rel)
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return ""
}

// SearchResponse is the structured search result
type SearchResponse struct {
	QueryType  string         `json:"query_type"`
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count"`
	HasMore    bool           `json:"has_more"`
	Cursor     string         `json:"cursor,omitempty"`
}

// SearchResult is a single search result
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
```

**Step 4: Run tests**

Run: `go test ./internal/search/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/search/
git commit -m "feat: add search handler with search_code tool"
```

---

## Task 3: Redis Query Cache

**Files:**
- Create: `internal/cache/redis.go`
- Create: `internal/cache/redis_test.go`
- Modify: `internal/search/handler.go` (add caching)

**Step 1: Write test for cache**

```go
// internal/cache/redis_test.go
package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisCache(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	cache, err := NewRedisCache(redisURL)
	if err != nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()

	// Test set and get
	key := "test:query:abc123"
	value := `{"results": []}`

	err = cache.Set(ctx, key, value, 1*time.Minute)
	require.NoError(t, err)

	got, err := cache.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	// Test invalidation
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	got, err = cache.Get(ctx, key)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestQueryCacheKey(t *testing.T) {
	key := QueryCacheKey("test-repo", "hello world", 42)
	assert.Contains(t, key, "query:")
	assert.Contains(t, key, "test-repo")
	assert.Contains(t, key, ":42") // version
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create Redis cache**

```go
// internal/cache/redis.go
package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides caching via Redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(url string) (*RedisCache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// Get retrieves a value from cache
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Set stores a value in cache with TTL
func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// DeletePattern removes all keys matching pattern
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// GetIndexVersion retrieves the current index version for a repo
func (c *RedisCache) GetIndexVersion(ctx context.Context, repo string) (int64, error) {
	val, err := c.client.Get(ctx, "index:version:"+repo).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// IncrIndexVersion increments the index version
func (c *RedisCache) IncrIndexVersion(ctx context.Context, repo string) (int64, error) {
	return c.client.Incr(ctx, "index:version:"+repo).Result()
}

// QueryCacheKey generates a cache key for a search query
func QueryCacheKey(repo, query string, version int64) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("query:%s:%x:%d", repo, h[:8], version)
}

// EmbeddingCacheKey generates a cache key for an embedding
func EmbeddingCacheKey(model, contentHash string) string {
	return fmt.Sprintf("embed:%s:%s", model, contentHash)
}
```

**Step 4: Add Redis dependency**

```bash
go get github.com/redis/go-redis/v9
go mod tidy
```

**Step 5: Update search handler to use cache**

Add to `internal/search/handler.go`:

```go
// Add to Handler struct
type Handler struct {
	config   *config.Config
	embedder *embedding.VoyageClient
	store    *store.QdrantStore
	cache    *cache.RedisCache // Add this
	logger   *slog.Logger
}

// Update NewHandler
func NewHandler(cfg *config.Config, voyageKey string, logger *slog.Logger) (*Handler, error) {
	// ... existing code ...

	var queryCache *cache.RedisCache
	if cfg.Storage.RedisURL != "" {
		queryCache, err = cache.NewRedisCache(cfg.Storage.RedisURL)
		if err != nil {
			logger.Warn("Redis cache unavailable, continuing without cache", "error", err)
		}
	}

	return &Handler{
		config:   cfg,
		embedder: embedder,
		store:    qdrantStore,
		cache:    queryCache,
		logger:   logger,
	}, nil
}

// Update searchCode to check cache first
func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	// ... parse args ...

	// Check cache if available
	if h.cache != nil {
		version, _ := h.cache.GetIndexVersion(ctx, repo)
		cacheKey := cache.QueryCacheKey(repo, query, version)

		if cached, err := h.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			h.logger.Debug("cache hit", "query", query)
			return &mcp.CallToolResult{
				Content: []mcp.Content{{Type: "text", Text: cached}},
			}, nil
		}
	}

	// ... existing search logic ...

	// Cache result
	if h.cache != nil {
		version, _ := h.cache.GetIndexVersion(ctx, repo)
		cacheKey := cache.QueryCacheKey(repo, query, version)
		h.cache.Set(ctx, cacheKey, response, 10*time.Minute)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: response}},
	}, nil
}
```

**Step 6: Run tests**

Run: `go test ./internal/cache/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/cache/ internal/search/
git commit -m "feat: add Redis query cache with TTL"
```

---

## Task 4: Metrics Logging

**Files:**
- Create: `internal/metrics/logger.go`
- Create: `internal/metrics/logger_test.go`
- Modify: `internal/search/handler.go` (add metrics)

**Step 1: Write test for metrics logger**

```go
// internal/metrics/logger_test.go
package metrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "metrics.jsonl")

	logger, err := NewLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	// Log a search event
	logger.LogSearch("auth timeout", "concept", 5, 120)

	// Log a context inject event
	logger.LogContextInject("auth.js", 3, 0.82)

	// Log a file read event
	logger.LogFileRead("sessionStore.js", true)

	// Verify file has content
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	assert.Contains(t, string(data), `"event":"search"`)
	assert.Contains(t, string(data), `"query":"auth timeout"`)
	assert.Contains(t, string(data), `"event":"context_inject"`)
	assert.Contains(t, string(data), `"event":"file_read"`)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create metrics logger**

```go
// internal/metrics/logger.go
package metrics

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Logger writes metrics events to JSONL file
type Logger struct {
	file *os.File
	mu   sync.Mutex
}

// Event is a single metrics event
type Event struct {
	Timestamp string                 `json:"ts"`
	Event     string                 `json:"event"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NewLogger creates a new metrics logger
func NewLogger(path string) (*Logger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &Logger{file: file}, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	return l.file.Close()
}

func (l *Logger) log(event string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339),
		"event": event,
	}
	for k, v := range data {
		e[k] = v
	}

	line, _ := json.Marshal(e)
	l.file.Write(line)
	l.file.Write([]byte("\n"))
}

// LogSearch logs a search query event
func (l *Logger) LogSearch(query, queryType string, results int, latencyMs int64) {
	l.log("search", map[string]interface{}{
		"query":      query,
		"query_type": queryType,
		"results":    results,
		"latency_ms": latencyMs,
	})
}

// LogContextInject logs a context injection event
func (l *Logger) LogContextInject(file string, suggestions int, confidence float64) {
	l.log("context_inject", map[string]interface{}{
		"file":        file,
		"suggestions": suggestions,
		"confidence":  confidence,
	})
}

// LogFileRead logs when Claude reads a file
func (l *Logger) LogFileRead(file string, wasSuggested bool) {
	l.log("file_read", map[string]interface{}{
		"file":          file,
		"was_suggested": wasSuggested,
	})
}

// LogIndexUpdate logs an index update event
func (l *Logger) LogIndexUpdate(repo string, filesChanged, chunksUpdated int) {
	l.log("index_update", map[string]interface{}{
		"repo":           repo,
		"files_changed":  filesChanged,
		"chunks_updated": chunksUpdated,
	})
}

// LogError logs an error event
func (l *Logger) LogError(operation, message string) {
	l.log("error", map[string]interface{}{
		"operation": operation,
		"message":   message,
	})
}
```

**Step 4: Run tests**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

**Step 5: Integrate metrics into search handler**

Update `internal/search/handler.go`:

```go
// Add to Handler struct
type Handler struct {
	// ... existing fields ...
	metrics *metrics.Logger
}

// Update NewHandler to create metrics logger
func NewHandler(cfg *config.Config, voyageKey string, logger *slog.Logger) (*Handler, error) {
	// ... existing code ...

	var metricsLogger *metrics.Logger
	homeDir, _ := os.UserHomeDir()
	metricsPath := filepath.Join(homeDir, ".local", "share", "code-index", "metrics.jsonl")
	if err := os.MkdirAll(filepath.Dir(metricsPath), 0755); err == nil {
		metricsLogger, _ = metrics.NewLogger(metricsPath)
	}

	return &Handler{
		// ... existing fields ...
		metrics: metricsLogger,
	}, nil
}

// Update searchCode to log metrics
func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	startTime := time.Now()

	// ... existing search logic ...

	// Log metrics
	if h.metrics != nil {
		h.metrics.LogSearch(query, "concept", len(results), time.Since(startTime).Milliseconds())
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: response}},
	}, nil
}
```

**Step 6: Commit**

```bash
git add internal/metrics/ internal/search/
git commit -m "feat: add JSONL metrics logging for search events"
```

---

## Task 5: Claude Code Hooks

**Files:**
- Create: `cmd/code-indexer/suggest.go`
- Create: `cmd/code-indexer/invalidate.go`
- Create: `scripts/install-hooks.sh`

**Step 1: Create suggest-context command**

```go
// cmd/code-indexer/suggest.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/store"
	"github.com/spf13/cobra"
)

var suggestCmd = &cobra.Command{
	Use:   "suggest-context [file-path]",
	Short: "Suggest related files for context (used by Claude Code hooks)",
	Args:  cobra.ExactArgs(1),
	RunE:  runSuggestContext,
}

func init() {
	rootCmd.AddCommand(suggestCmd)
}

func runSuggestContext(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Load config
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Connect to stores
	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		// Silently fail - don't break Claude's read
		return nil
	}

	ctx := context.Background()

	// Find related files
	related, err := findRelatedFiles(ctx, qdrantStore, filePath)
	if err != nil || len(related) == 0 {
		return nil // No suggestions, silent
	}

	// Output to stderr (visible to Claude)
	fmt.Fprintf(os.Stderr, "[code-index] Related files for %s:\n", filepath.Base(filePath))
	for _, r := range related {
		fmt.Fprintf(os.Stderr, "  - %s (%s)\n", r.Path, r.Reason)
	}

	return nil
}

type RelatedFile struct {
	Path   string
	Reason string
}

func findRelatedFiles(ctx context.Context, store *store.QdrantStore, filePath string) ([]RelatedFile, error) {
	// Query for chunks from this file
	chunks, err := store.Search(ctx, "chunks", nil, 1, map[string]interface{}{
		"file_path": filePath,
	})
	if err != nil || len(chunks) == 0 {
		return nil, err
	}

	// Use the chunk's vector to find similar code in other files
	// TODO: Implement proper graph-based suggestions
	// For now, return empty (graph integration needed)

	return nil, nil
}
```

**Step 2: Create invalidate-file command**

```go
// cmd/code-indexer/invalidate.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/randalmurphal/code-indexer/internal/cache"
	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/spf13/cobra"
)

var invalidateCmd = &cobra.Command{
	Use:   "invalidate-file [file-path]",
	Short: "Mark a file as needing re-indexing (used by Claude Code hooks)",
	Args:  cobra.ExactArgs(1),
	RunE:  runInvalidateFile,
}

func init() {
	rootCmd.AddCommand(invalidateCmd)
}

func runInvalidateFile(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Load config
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Connect to Redis
	redisCache, err := cache.NewRedisCache(cfg.Storage.RedisURL)
	if err != nil {
		// Silently fail - don't break Claude's write
		return nil
	}

	ctx := context.Background()

	// Infer repo from path
	repo := inferRepoFromPath(filePath)
	if repo == "" {
		return nil
	}

	// Increment index version to invalidate query cache
	newVersion, err := redisCache.IncrIndexVersion(ctx, repo)
	if err != nil {
		return nil
	}

	// Mark file as needing re-index
	err = redisCache.Set(ctx, "stale:"+filePath, "1", 0) // No expiry
	if err != nil {
		return nil
	}

	// Output to stderr
	fmt.Fprintf(os.Stderr, "[code-index] Marked %s for re-indexing (version: %d)\n", filePath, newVersion)

	return nil
}

func inferRepoFromPath(path string) string {
	homeDir, _ := os.UserHomeDir()

	// Check known repo paths
	repos := map[string]string{
		homeDir + "/repos/r3":      "r3",
		homeDir + "/repos/m32rimm": "m32rimm",
	}

	for prefix, name := range repos {
		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			return name
		}
	}

	return ""
}
```

**Step 3: Create hook installation script**

```bash
#!/bin/bash
# scripts/install-hooks.sh
# Install Claude Code hooks for a repository

set -e

REPO_PATH="${1:-.}"
REPO_PATH=$(cd "$REPO_PATH" && pwd)

# Create .claude directory if needed
CLAUDE_DIR="$REPO_PATH/.claude"
mkdir -p "$CLAUDE_DIR"

# Create settings.json with hooks
cat > "$CLAUDE_DIR/settings.json" << 'EOF'
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read",
        "hooks": [{
          "type": "command",
          "command": "code-indexer suggest-context \"$CLAUDE_FILE_PATH\""
        }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [{
          "type": "command",
          "command": "code-indexer invalidate-file \"$CLAUDE_FILE_PATH\""
        }]
      }
    ]
  }
}
EOF

echo "Installed Claude Code hooks to $CLAUDE_DIR/settings.json"
echo ""
echo "Hooks configured:"
echo "  - PreToolUse/Read: Suggests related files"
echo "  - PostToolUse/Write|Edit: Invalidates index for changed files"
```

**Step 4: Make script executable**

```bash
chmod +x scripts/install-hooks.sh
```

**Step 5: Commit**

```bash
git add cmd/code-indexer/suggest.go cmd/code-indexer/invalidate.go scripts/
git commit -m "feat: add Claude Code hooks for context suggestions and invalidation"
```

---

## Task 6: MCP Registration and E2E Test

**Files:**
- Create: `scripts/register-mcp.sh`
- Create: `test/e2e/mcp_test.go`

**Step 1: Create MCP registration script**

```bash
#!/bin/bash
# scripts/register-mcp.sh
# Register code-index-mcp with Claude Code

set -e

# Get the binary path
BINARY_PATH="${1:-$(which code-index-mcp 2>/dev/null || echo "$HOME/repos/code-indexer/bin/code-index-mcp")}"

if [ ! -f "$BINARY_PATH" ]; then
    echo "Error: code-index-mcp binary not found at $BINARY_PATH"
    echo "Build it first: go build -o bin/code-index-mcp ./cmd/code-index-mcp"
    exit 1
fi

# Create Claude settings directory
CLAUDE_DIR="$HOME/.claude"
mkdir -p "$CLAUDE_DIR"

# Read existing settings or create new
SETTINGS_FILE="$CLAUDE_DIR/settings.json"
if [ -f "$SETTINGS_FILE" ]; then
    # Backup existing
    cp "$SETTINGS_FILE" "$SETTINGS_FILE.bak"
fi

# Add MCP server configuration
# Using jq if available, otherwise create fresh
if command -v jq &> /dev/null && [ -f "$SETTINGS_FILE" ]; then
    jq --arg path "$BINARY_PATH" '.mcpServers["code-index"] = {
        "command": $path,
        "args": ["serve"],
        "env": {
            "VOYAGE_API_KEY": "${VOYAGE_API_KEY}"
        }
    }' "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" && mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
else
    cat > "$SETTINGS_FILE" << EOF
{
  "mcpServers": {
    "code-index": {
      "command": "$BINARY_PATH",
      "args": ["serve"],
      "env": {
        "VOYAGE_API_KEY": "\${VOYAGE_API_KEY}"
      }
    }
  }
}
EOF
fi

echo "Registered code-index-mcp with Claude Code"
echo ""
echo "Configuration added to $SETTINGS_FILE"
echo ""
echo "Make sure VOYAGE_API_KEY is set in your environment."
echo "Restart Claude Code to load the new MCP server."
```

**Step 2: Create E2E test for MCP**

```go
// test/e2e/mcp_test.go
package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPServerProtocol(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" {
		t.Skip("VOYAGE_API_KEY not set")
	}

	// Build MCP server
	cmd := exec.Command("go", "build", "-o", "bin/code-index-mcp", "./cmd/code-index-mcp")
	cmd.Dir = getProjectRoot()
	require.NoError(t, cmd.Run())

	// Start MCP server
	mcpCmd := exec.Command(getProjectRoot() + "/bin/code-index-mcp", "serve")
	mcpCmd.Env = os.Environ()

	stdin, err := mcpCmd.StdinPipe()
	require.NoError(t, err)

	stdout, err := mcpCmd.StdoutPipe()
	require.NoError(t, err)

	require.NoError(t, mcpCmd.Start())
	defer mcpCmd.Process.Kill()

	reader := bufio.NewReader(stdout)

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test",
				"version": "1.0.0",
			},
		},
	}

	sendRequest(t, stdin, initReq)
	resp := readResponse(t, reader)

	assert.Equal(t, "2.0", resp["jsonrpc"])
	assert.NotNil(t, resp["result"])

	// Send initialized notification
	sendRequest(t, stdin, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
	})

	// List tools
	sendRequest(t, stdin, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})

	resp = readResponse(t, reader)
	result := resp["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})

	assert.Len(t, tools, 1)
	tool := tools[0].(map[string]interface{})
	assert.Equal(t, "search_code", tool["name"])
}

func sendRequest(t *testing.T, w io.Writer, req map[string]interface{}) {
	data, err := json.Marshal(req)
	require.NoError(t, err)
	_, err = w.Write(append(data, '\n'))
	require.NoError(t, err)
}

func readResponse(t *testing.T, r *bufio.Reader) map[string]interface{} {
	line, err := r.ReadBytes('\n')
	require.NoError(t, err)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(line, &resp))
	return resp
}
```

**Step 3: Make scripts executable**

```bash
chmod +x scripts/register-mcp.sh
```

**Step 4: Run E2E tests**

Run: `VOYAGE_API_KEY=your-key go test ./test/e2e/... -v -run TestMCP`
Expected: PASS

**Step 5: Commit**

```bash
git add scripts/register-mcp.sh test/e2e/mcp_test.go
git commit -m "feat: add MCP registration script and protocol E2E test"
```

---

## Checkpoint: Phase 2 Complete

At this point you have:

1. **MCP server scaffold** with stdio transport
2. **Search handler** with search_code tool
3. **Redis query cache** with TTL and invalidation
4. **Metrics logging** to JSONL
5. **Claude Code hooks** for suggestions and invalidation
6. **MCP registration** script

**To verify everything works:**

```bash
# Build binaries
go build -o bin/code-indexer ./cmd/code-indexer
go build -o bin/code-index-mcp ./cmd/code-index-mcp

# Register MCP server
./scripts/register-mcp.sh

# Install hooks in a repo
./scripts/install-hooks.sh ~/repos/m32rimm

# Restart Claude Code and test search_code tool
```

**Next:** Phase 3 adds intelligence (query classification, patterns, AGENTS.md).

# Code Indexing Phase 4: Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add final polish: hierarchical chunking for large files, secret detection, pagination, metrics CLI, and background sync daemon.

**Architecture:** Extends existing components with refinements for production use.

**Tech Stack:** Go, existing components

**Prerequisites:** Phase 3 complete (intelligence features working)

**Design Doc:** `docs/plans/2026-02-04-code-indexing-design.md`

---

## Task 1: Hierarchical Chunking for Large Files

**Files:**
- Modify: `internal/chunk/extractor.go`
- Create: `internal/chunk/hierarchy.go`
- Create: `internal/chunk/hierarchy_test.go`

**Step 1: Write test for hierarchical chunking**

```go
// internal/chunk/hierarchy_test.go
package chunk

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHierarchicalChunking(t *testing.T) {
	// Simulate a large class with many methods
	var methods []string
	for i := 0; i < 60; i++ {
		methods = append(methods, `
    def method_`+string(rune('a'+i%26))+string(rune('0'+i/26))+`(self):
        """Method `+string(rune(i))+` does something."""
        return "result"`)
	}

	code := `
class LargeClass:
    """A class with many methods."""

    def __init__(self):
        self.value = 0
` + strings.Join(methods, "\n")

	extractor := NewExtractor()
	extractor.SetHierarchicalChunking(true)

	chunks, err := extractor.Extract([]byte(code), "large.py", "test", "test.module")
	require.NoError(t, err)

	// Should have:
	// - 1 class summary chunk
	// - Multiple method chunks with context headers
	assert.True(t, len(chunks) > 50, "should have many chunks")

	// Find class summary chunk
	var summaryChunk *Chunk
	for i := range chunks {
		if chunks[i].Kind == "class_summary" {
			summaryChunk = &chunks[i]
			break
		}
	}
	require.NotNil(t, summaryChunk, "should have class summary")
	assert.Contains(t, summaryChunk.Content, "LargeClass")
	assert.Contains(t, summaryChunk.Content, "Methods:") // Should list methods

	// Check method chunks have context headers
	for _, chunk := range chunks {
		if chunk.Kind == "method" {
			assert.NotEmpty(t, chunk.ContextHeader, "methods should have context header")
			assert.Contains(t, chunk.ContextHeader, "LargeClass")
		}
	}
}

func TestChunkSizeEstimation(t *testing.T) {
	tests := []struct {
		content   string
		maxTokens int
		shouldSplit bool
	}{
		{"short content", 500, false},
		{strings.Repeat("word ", 600), 500, true}, // ~600 tokens
		{strings.Repeat("x", 2000), 500, true},    // ~500 tokens
	}

	for _, tt := range tests {
		chunk := Chunk{Content: tt.content}
		tokens := chunk.TokenEstimate()

		if tt.shouldSplit {
			assert.True(t, tokens > tt.maxTokens, "should exceed max tokens")
		} else {
			assert.True(t, tokens <= tt.maxTokens, "should fit in max tokens")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/chunk/... -v -run TestHierarchical`
Expected: FAIL

**Step 3: Create hierarchical chunking**

```go
// internal/chunk/hierarchy.go
package chunk

import (
	"fmt"
	"strings"

	"github.com/randalmurphal/ai-devtools-admin/internal/parser"
)

const (
	MaxChunkTokens     = 500
	LargeClassMethods  = 50
)

// HierarchicalChunker creates hierarchical chunks for large files
type HierarchicalChunker struct {
	maxTokens        int
	largeClassThreshold int
}

// NewHierarchicalChunker creates a new chunker
func NewHierarchicalChunker() *HierarchicalChunker {
	return &HierarchicalChunker{
		maxTokens:           MaxChunkTokens,
		largeClassThreshold: LargeClassMethods,
	}
}

// ChunkSymbols converts symbols to chunks with hierarchy awareness
func (h *HierarchicalChunker) ChunkSymbols(symbols []parser.Symbol, filePath, repo, modulePath string, isTest bool) []Chunk {
	// Group symbols by class
	classSymbols := make(map[string][]parser.Symbol)
	var topLevel []parser.Symbol

	for _, sym := range symbols {
		if sym.Parent != "" {
			classSymbols[sym.Parent] = append(classSymbols[sym.Parent], sym)
		} else {
			topLevel = append(topLevel, sym)
		}
	}

	var chunks []Chunk
	moduleRoot, submodule := parseModulePath(modulePath)
	weight := float32(1.0)
	if isTest {
		weight = 0.5
	}

	// Process top-level symbols
	for _, sym := range topLevel {
		if sym.Kind == parser.SymbolClass {
			methods := classSymbols[sym.Name]

			if len(methods) > h.largeClassThreshold {
				// Large class: create summary + individual method chunks
				chunks = append(chunks, h.createClassSummary(sym, methods, filePath, repo, modulePath, moduleRoot, submodule, weight))

				for _, method := range methods {
					chunk := h.createMethodChunk(method, sym.Name, filePath, repo, modulePath, moduleRoot, submodule, weight)
					chunks = append(chunks, chunk)
				}
			} else {
				// Normal class: single chunk with all methods
				chunks = append(chunks, h.createClassChunk(sym, methods, filePath, repo, modulePath, moduleRoot, submodule, weight))
			}
		} else {
			// Function or other top-level symbol
			chunks = append(chunks, h.createSymbolChunk(sym, filePath, repo, modulePath, moduleRoot, submodule, weight))
		}
	}

	return chunks
}

func (h *HierarchicalChunker) createClassSummary(class parser.Symbol, methods []parser.Symbol, filePath, repo, modulePath, moduleRoot, submodule string, weight float32) Chunk {
	// Build method list
	var methodNames []string
	for _, m := range methods {
		methodNames = append(methodNames, m.Name)
	}

	summary := fmt.Sprintf("class %s:\n    \"\"\"%s\"\"\"\n\n    Methods: %s",
		class.Name,
		class.Docstring,
		strings.Join(methodNames, ", "))

	return Chunk{
		ID:              generateChunkID(repo, filePath, class.Name+"_summary", class.StartLine),
		Repo:            repo,
		FilePath:        filePath,
		StartLine:       class.StartLine,
		EndLine:         class.EndLine,
		Type:            ChunkTypeCode,
		Kind:            "class_summary",
		ModulePath:      modulePath,
		ModuleRoot:      moduleRoot,
		Submodule:       submodule,
		SymbolName:      class.Name,
		Content:         summary,
		Docstring:       class.Docstring,
		IsTest:          weight < 1.0,
		RetrievalWeight: weight,
	}
}

func (h *HierarchicalChunker) createMethodChunk(method parser.Symbol, className, filePath, repo, modulePath, moduleRoot, submodule string, weight float32) Chunk {
	contextHeader := fmt.Sprintf("# File: %s\n# Class: %s\n# Related methods in same class", filePath, className)

	return Chunk{
		ID:              generateChunkID(repo, filePath, method.Name, method.StartLine),
		Repo:            repo,
		FilePath:        filePath,
		StartLine:       method.StartLine,
		EndLine:         method.EndLine,
		Type:            ChunkTypeCode,
		Kind:            "method",
		ModulePath:      modulePath,
		ModuleRoot:      moduleRoot,
		Submodule:       submodule,
		SymbolName:      method.Name,
		Content:         method.Content,
		ContextHeader:   contextHeader,
		Signature:       method.Signature,
		Docstring:       method.Docstring,
		IsTest:          weight < 1.0,
		RetrievalWeight: weight,
	}
}

func (h *HierarchicalChunker) createClassChunk(class parser.Symbol, methods []parser.Symbol, filePath, repo, modulePath, moduleRoot, submodule string, weight float32) Chunk {
	return Chunk{
		ID:              generateChunkID(repo, filePath, class.Name, class.StartLine),
		Repo:            repo,
		FilePath:        filePath,
		StartLine:       class.StartLine,
		EndLine:         class.EndLine,
		Type:            ChunkTypeCode,
		Kind:            "class",
		ModulePath:      modulePath,
		ModuleRoot:      moduleRoot,
		Submodule:       submodule,
		SymbolName:      class.Name,
		Content:         class.Content,
		Docstring:       class.Docstring,
		IsTest:          weight < 1.0,
		RetrievalWeight: weight,
	}
}

func (h *HierarchicalChunker) createSymbolChunk(sym parser.Symbol, filePath, repo, modulePath, moduleRoot, submodule string, weight float32) Chunk {
	return Chunk{
		ID:              generateChunkID(repo, filePath, sym.Name, sym.StartLine),
		Repo:            repo,
		FilePath:        filePath,
		StartLine:       sym.StartLine,
		EndLine:         sym.EndLine,
		Type:            ChunkTypeCode,
		Kind:            string(sym.Kind),
		ModulePath:      modulePath,
		ModuleRoot:      moduleRoot,
		Submodule:       submodule,
		SymbolName:      sym.Name,
		Content:         sym.Content,
		Signature:       sym.Signature,
		Docstring:       sym.Docstring,
		IsTest:          weight < 1.0,
		RetrievalWeight: weight,
	}
}
```

**Step 4: Update extractor to use hierarchical chunking**

```go
// Add to internal/chunk/extractor.go

func (e *Extractor) SetHierarchicalChunking(enabled bool) {
	e.hierarchical = enabled
}

// Update Extract method to use HierarchicalChunker when enabled
```

**Step 5: Run tests**

Run: `go test ./internal/chunk/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/chunk/
git commit -m "feat: add hierarchical chunking for large files"
```

---

## Task 2: Secret Detection and Redaction

**Files:**
- Create: `internal/security/secrets.go`
- Create: `internal/security/secrets_test.go`
- Modify: `internal/chunk/extractor.go`

**Step 1: Write test for secret detection**

```go
// internal/security/secrets_test.go
package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSecrets(t *testing.T) {
	detector := NewSecretDetector()

	tests := []struct {
		name     string
		content  string
		expected int // number of secrets
	}{
		{
			name:     "API key",
			content:  `API_KEY = "sk-1234567890abcdefghijklmnop"`,
			expected: 1,
		},
		{
			name:     "AWS access key",
			content:  `aws_access_key_id = "AKIAIOSFODNN7EXAMPLE"`,
			expected: 1,
		},
		{
			name:     "Connection string",
			content:  `DATABASE_URL = "mongodb://admin:password123@localhost:27017/db"`,
			expected: 1,
		},
		{
			name:     "Private key",
			content:  "-----BEGIN RSA PRIVATE KEY-----\nMIIEpA...\n-----END RSA PRIVATE KEY-----",
			expected: 1,
		},
		{
			name:     "Password in code",
			content:  `password = "supersecret123"`,
			expected: 1,
		},
		{
			name:     "No secrets",
			content:  `def hello():\n    return "world"`,
			expected: 0,
		},
		{
			name:     "Placeholder (not secret)",
			content:  `API_KEY = "your-api-key-here"`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secrets := detector.Detect(tt.content)
			assert.Len(t, secrets, tt.expected)
		})
	}
}

func TestRedactSecrets(t *testing.T) {
	detector := NewSecretDetector()

	content := `
DATABASE_URL = "mongodb://admin:supersecret@prod.db.com:27017/mydb"
API_KEY = "sk-abcdef1234567890"
`

	secrets := detector.Detect(content)
	require.Len(t, secrets, 2)

	redacted := detector.Redact(content, secrets)

	assert.Contains(t, redacted, "[REDACTED]")
	assert.NotContains(t, redacted, "supersecret")
	assert.NotContains(t, redacted, "sk-abcdef")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/security/... -v`
Expected: FAIL

**Step 3: Create secret detector**

```go
// internal/security/secrets.go
package security

import (
	"regexp"
	"strings"
)

// Secret represents a detected secret
type Secret struct {
	Type      string `json:"type"`
	Line      int    `json:"line"`
	StartPos  int    `json:"start_pos"`
	EndPos    int    `json:"end_pos"`
	Redacted  string `json:"redacted"` // What to replace with
}

// SecretDetector detects secrets in code
type SecretDetector struct {
	patterns []secretPattern
	placeholders []string
}

type secretPattern struct {
	name    string
	regex   *regexp.Regexp
	redact  func(match string) string
}

// NewSecretDetector creates a new detector with default patterns
func NewSecretDetector() *SecretDetector {
	return &SecretDetector{
		patterns: []secretPattern{
			{
				name:  "api_key",
				regex: regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret)\s*[=:]\s*["']([a-zA-Z0-9_\-]{20,})["']`),
				redact: func(match string) string {
					return regexp.MustCompile(`["'][^"']+["']`).ReplaceAllString(match, `"[REDACTED]"`)
				},
			},
			{
				name:  "aws_access_key",
				regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
				redact: func(match string) string {
					return "[REDACTED_AWS_KEY]"
				},
			},
			{
				name:  "password",
				regex: regexp.MustCompile(`(?i)(password|passwd|pwd|secret)\s*[=:]\s*["']([^\s"']{8,})["']`),
				redact: func(match string) string {
					return regexp.MustCompile(`["'][^"']+["']`).ReplaceAllString(match, `"[REDACTED]"`)
				},
			},
			{
				name:  "connection_string",
				regex: regexp.MustCompile(`(?i)(mongodb|postgres|mysql|redis|amqp):\/\/[^\s"']+`),
				redact: func(match string) string {
					// Keep protocol and host, redact credentials
					re := regexp.MustCompile(`(://[^:]+:)[^@]+(@)`)
					return re.ReplaceAllString(match, "${1}[REDACTED]${2}")
				},
			},
			{
				name:  "private_key",
				regex: regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
				redact: func(match string) string {
					return "[REDACTED_PRIVATE_KEY]"
				},
			},
			{
				name:  "jwt_token",
				regex: regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`),
				redact: func(match string) string {
					return "[REDACTED_JWT]"
				},
			},
		},
		placeholders: []string{
			"your-", "example", "placeholder", "xxx", "changeme",
			"TODO", "FIXME", "<", ">", "${", "{{",
		},
	}
}

// Detect finds secrets in content
func (d *SecretDetector) Detect(content string) []Secret {
	var secrets []Secret

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip if line looks like a placeholder
		if d.isPlaceholder(line) {
			continue
		}

		for _, pattern := range d.patterns {
			matches := pattern.regex.FindAllStringIndex(line, -1)
			for _, match := range matches {
				secrets = append(secrets, Secret{
					Type:     pattern.name,
					Line:     lineNum + 1,
					StartPos: match[0],
					EndPos:   match[1],
				})
			}
		}
	}

	return secrets
}

// Redact replaces secrets with redacted versions
func (d *SecretDetector) Redact(content string, secrets []Secret) string {
	if len(secrets) == 0 {
		return content
	}

	result := content

	for _, pattern := range d.patterns {
		result = pattern.regex.ReplaceAllStringFunc(result, pattern.redact)
	}

	return result
}

func (d *SecretDetector) isPlaceholder(line string) bool {
	lower := strings.ToLower(line)
	for _, p := range d.placeholders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// HasSecrets checks if content contains secrets
func (d *SecretDetector) HasSecrets(content string) bool {
	return len(d.Detect(content)) > 0
}
```

**Step 4: Run tests**

Run: `go test ./internal/security/... -v`
Expected: PASS

**Step 5: Integrate into chunk extractor**

```go
// Update internal/chunk/extractor.go

import "github.com/randalmurphal/ai-devtools-admin/internal/security"

type Extractor struct {
	// ... existing fields ...
	secretDetector *security.SecretDetector
}

func NewExtractor() *Extractor {
	return &Extractor{
		// ... existing ...
		secretDetector: security.NewSecretDetector(),
	}
}

// In Extract method, after creating chunk:
if e.secretDetector.HasSecrets(chunk.Content) {
	secrets := e.secretDetector.Detect(chunk.Content)
	chunk.Content = e.secretDetector.Redact(chunk.Content, secrets)
	chunk.HasSecrets = true
}
```

**Step 6: Commit**

```bash
git add internal/security/
git commit -m "feat: add secret detection and redaction"
```

---

## Task 3: Pagination Support

**Files:**
- Create: `internal/search/pagination.go`
- Modify: `internal/search/handler.go`

**Step 1: Create pagination helper**

```go
// internal/search/pagination.go
package search

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Cursor represents pagination state
type Cursor struct {
	QueryHash  string    `json:"q"`
	Offset     int       `json:"o"`
	CreatedAt  time.Time `json:"t"`
}

// EncodeCursor creates an opaque cursor string
func EncodeCursor(queryHash string, offset int) string {
	cursor := Cursor{
		QueryHash: queryHash,
		Offset:    offset,
		CreatedAt: time.Now(),
	}

	data, _ := json.Marshal(cursor)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor parses a cursor string
func DecodeCursor(s string) (*Cursor, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding")
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor format")
	}

	// Check expiry (10 minutes)
	if time.Since(cursor.CreatedAt) > 10*time.Minute {
		return nil, fmt.Errorf("cursor expired")
	}

	return &cursor, nil
}

// PaginatedResponse wraps search results with pagination info
type PaginatedResponse struct {
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count"`
	HasMore    bool           `json:"has_more"`
	Cursor     string         `json:"cursor,omitempty"`
}

// Paginate applies pagination to results
func Paginate(results []SearchResult, offset, limit int, queryHash string) PaginatedResponse {
	total := len(results)

	// Apply offset
	if offset >= len(results) {
		return PaginatedResponse{
			Results:    []SearchResult{},
			TotalCount: total,
			HasMore:    false,
		}
	}
	results = results[offset:]

	// Apply limit
	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	// Generate cursor for next page
	var cursor string
	if hasMore {
		cursor = EncodeCursor(queryHash, offset+limit)
	}

	return PaginatedResponse{
		Results:    results,
		TotalCount: total,
		HasMore:    hasMore,
		Cursor:     cursor,
	}
}
```

**Step 2: Update handler to use pagination**

```go
// Update internal/search/handler.go searchCode method

func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	// ... parse args ...

	// Handle cursor for pagination
	var offset int
	if cursorStr, ok := args["cursor"].(string); ok && cursorStr != "" {
		cursor, err := DecodeCursor(cursorStr)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{{Type: "text", Text: err.Error()}},
				IsError: true,
			}, nil
		}
		offset = cursor.Offset
	}

	// ... search logic ...

	// Apply pagination
	queryHash := hashQuery(query, repo, module)
	paginated := Paginate(results, offset, limit, queryHash)

	// Format response
	response, _ := json.MarshalIndent(paginated, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: string(response)}},
	}, nil
}

func hashQuery(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return fmt.Sprintf("%x", h[:8])
}
```

**Step 3: Commit**

```bash
git add internal/search/pagination.go internal/search/handler.go
git commit -m "feat: add cursor-based pagination for search results"
```

---

## Task 4: Metrics CLI

**Files:**
- Create: `cmd/code-indexer/metrics.go`
- Create: `internal/metrics/analyzer.go`

**Step 1: Create metrics analyzer**

```go
// internal/metrics/analyzer.go
package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Analyzer processes metrics logs
type Analyzer struct {
	logPath string
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(logPath string) *Analyzer {
	return &Analyzer{logPath: logPath}
}

// Summary contains aggregated metrics
type Summary struct {
	Period          string                  `json:"period"`
	TotalSearches   int                     `json:"total_searches"`
	SearchesByType  map[string]int          `json:"searches_by_type"`
	AvgLatencyMs    int64                   `json:"avg_latency_ms"`
	ZeroResultCount int                     `json:"zero_result_count"`
	TopQueries      []QueryCount            `json:"top_queries"`
}

type QueryCount struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// Analyze processes logs for a time period
func (a *Analyzer) Analyze(since time.Duration) (*Summary, error) {
	file, err := os.Open(a.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-since)
	summary := &Summary{
		Period:         since.String(),
		SearchesByType: make(map[string]int),
	}

	queryCounts := make(map[string]int)
	var totalLatency int64
	var latencyCount int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Parse timestamp
		tsStr, ok := event["ts"].(string)
		if !ok {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil || ts.Before(cutoff) {
			continue
		}

		// Process by event type
		eventType, _ := event["event"].(string)
		switch eventType {
		case "search":
			summary.TotalSearches++

			if qt, ok := event["query_type"].(string); ok {
				summary.SearchesByType[qt]++
			}

			if results, ok := event["results"].(float64); ok && results == 0 {
				summary.ZeroResultCount++
			}

			if latency, ok := event["latency_ms"].(float64); ok {
				totalLatency += int64(latency)
				latencyCount++
			}

			if query, ok := event["query"].(string); ok {
				queryCounts[query]++
			}
		}
	}

	// Calculate average latency
	if latencyCount > 0 {
		summary.AvgLatencyMs = totalLatency / int64(latencyCount)
	}

	// Get top queries
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range queryCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	for i := 0; i < len(sorted) && i < 10; i++ {
		summary.TopQueries = append(summary.TopQueries, QueryCount{
			Query: sorted[i].Key,
			Count: sorted[i].Value,
		})
	}

	return summary, nil
}

// GetZeroResultQueries returns queries that returned no results
func (a *Analyzer) GetZeroResultQueries(since time.Duration) ([]QueryCount, error) {
	file, err := os.Open(a.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-since)
	queryCounts := make(map[string]int)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		tsStr, ok := event["ts"].(string)
		if !ok {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil || ts.Before(cutoff) {
			continue
		}

		eventType, _ := event["event"].(string)
		if eventType != "search" {
			continue
		}

		results, _ := event["results"].(float64)
		if results == 0 {
			query, _ := event["query"].(string)
			queryCounts[query]++
		}
	}

	var result []QueryCount
	for q, c := range queryCounts {
		result = append(result, QueryCount{Query: q, Count: c})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result, nil
}
```

**Step 2: Create metrics command**

```go
// cmd/code-indexer/metrics.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/randalmurphal/ai-devtools-admin/internal/metrics"
	"github.com/spf13/cobra"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Analyze usage metrics",
	RunE:  runMetrics,
}

var (
	metricsSince       string
	metricsZeroResults bool
	metricsJSON        bool
)

func init() {
	metricsCmd.Flags().StringVar(&metricsSince, "last", "7d", "Time period (e.g., 1h, 24h, 7d, 30d)")
	metricsCmd.Flags().BoolVar(&metricsZeroResults, "zero-results", false, "Show only zero-result queries")
	metricsCmd.Flags().BoolVar(&metricsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(metricsCmd)
}

func runMetrics(cmd *cobra.Command, args []string) error {
	// Parse duration
	duration, err := parseDuration(metricsSince)
	if err != nil {
		return err
	}

	// Get metrics path
	homeDir, _ := os.UserHomeDir()
	metricsPath := filepath.Join(homeDir, ".local", "share", "code-index", "metrics.jsonl")

	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		fmt.Println("No metrics data found. Use the search_code tool to generate metrics.")
		return nil
	}

	analyzer := metrics.NewAnalyzer(metricsPath)

	if metricsZeroResults {
		queries, err := analyzer.GetZeroResultQueries(duration)
		if err != nil {
			return err
		}

		if metricsJSON {
			data, _ := json.MarshalIndent(queries, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("Zero-result queries (last %s):\n\n", metricsSince)
			if len(queries) == 0 {
				fmt.Println("  No zero-result queries found.")
			}
			for _, q := range queries {
				fmt.Printf("  - \"%s\" (%d times)\n", q.Query, q.Count)
			}
		}
		return nil
	}

	summary, err := analyzer.Analyze(duration)
	if err != nil {
		return err
	}

	if metricsJSON {
		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Metrics Summary (last %s):\n\n", metricsSince)
		fmt.Printf("  Total searches:      %d\n", summary.TotalSearches)
		fmt.Printf("  Avg latency:         %dms\n", summary.AvgLatencyMs)
		fmt.Printf("  Zero-result queries: %d\n", summary.ZeroResultCount)
		fmt.Println()
		fmt.Println("  Searches by type:")
		for t, c := range summary.SearchesByType {
			fmt.Printf("    - %s: %d\n", t, c)
		}
		fmt.Println()
		fmt.Println("  Top queries:")
		for _, q := range summary.TopQueries {
			fmt.Printf("    - \"%s\" (%d times)\n", q.Query, q.Count)
		}
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if len(s) > 0 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err == nil {
			return time.Duration(d) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
```

**Step 3: Run and verify**

```bash
go build -o bin/code-indexer ./cmd/code-indexer
./bin/code-indexer metrics --last 7d
./bin/code-indexer metrics --zero-results
./bin/code-indexer metrics --json
```

**Step 4: Commit**

```bash
git add cmd/code-indexer/metrics.go internal/metrics/analyzer.go
git commit -m "feat: add metrics CLI with summary and zero-result analysis"
```

---

## Task 5: Background Sync Daemon

**Files:**
- Create: `cmd/code-indexer/watch.go`
- Create: `internal/sync/daemon.go`

**Step 1: Create sync daemon**

```go
// internal/sync/daemon.go
package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/randalmurphal/ai-devtools-admin/internal/cache"
	"github.com/randalmurphal/ai-devtools-admin/internal/config"
	"github.com/randalmurphal/ai-devtools-admin/internal/indexer"
)

// Daemon watches repositories and syncs on changes
type Daemon struct {
	repos    []RepoWatch
	interval time.Duration
	indexer  *indexer.Indexer
	cache    *cache.RedisCache
	logger   *slog.Logger
}

// RepoWatch defines a repository to watch
type RepoWatch struct {
	Name   string
	Path   string
	Config *config.RepoConfig
}

// NewDaemon creates a new sync daemon
func NewDaemon(repos []RepoWatch, interval time.Duration, idx *indexer.Indexer, c *cache.RedisCache, logger *slog.Logger) *Daemon {
	return &Daemon{
		repos:    repos,
		interval: interval,
		indexer:  idx,
		cache:    c,
		logger:   logger,
	}
}

// Run starts the daemon
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("starting sync daemon", "interval", d.interval, "repos", len(d.repos))

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Initial sync
	d.syncAll(ctx)

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon shutting down")
			return ctx.Err()
		case <-ticker.C:
			d.syncAll(ctx)
		}
	}
}

func (d *Daemon) syncAll(ctx context.Context) {
	for _, repo := range d.repos {
		if err := d.syncRepo(ctx, repo); err != nil {
			d.logger.Error("sync failed", "repo", repo.Name, "error", err)
		}
	}
}

func (d *Daemon) syncRepo(ctx context.Context, repo RepoWatch) error {
	d.logger.Debug("checking repo", "name", repo.Name)

	// Get current HEAD
	headPath := filepath.Join(repo.Path, ".git", "HEAD")
	headData, err := os.ReadFile(headPath)
	if err != nil {
		return err
	}
	currentHead := string(headData)

	// Get cached HEAD
	cacheKey := "head:" + repo.Name
	cachedHead, _ := d.cache.Get(ctx, cacheKey)

	if currentHead == cachedHead {
		d.logger.Debug("repo unchanged", "name", repo.Name)
		return nil
	}

	d.logger.Info("repo changed, syncing", "name", repo.Name)

	// Run incremental index
	result, err := d.indexer.Index(ctx, repo.Path, repo.Config)
	if err != nil {
		return err
	}

	d.logger.Info("sync complete",
		"repo", repo.Name,
		"files", result.FilesProcessed,
		"chunks", result.ChunksCreated,
	)

	// Update cached HEAD
	d.cache.Set(ctx, cacheKey, currentHead, 0)

	return nil
}
```

**Step 2: Create watch command**

```go
// cmd/code-indexer/watch.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/randalmurphal/ai-devtools-admin/internal/cache"
	"github.com/randalmurphal/ai-devtools-admin/internal/config"
	"github.com/randalmurphal/ai-devtools-admin/internal/indexer"
	"github.com/randalmurphal/ai-devtools-admin/internal/sync"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch repositories and sync on changes",
	RunE:  runWatch,
}

var (
	watchRepos    string
	watchInterval string
)

func init() {
	watchCmd.Flags().StringVar(&watchRepos, "repos", "", "Comma-separated repo names to watch (e.g., r3,m32rimm)")
	watchCmd.Flags().StringVar(&watchInterval, "interval", "60s", "Check interval (e.g., 30s, 5m)")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	if watchRepos == "" {
		return fmt.Errorf("--repos is required")
	}

	interval, err := time.ParseDuration(watchInterval)
	if err != nil {
		return fmt.Errorf("invalid interval: %w", err)
	}

	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load global config
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Get API key
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY not set")
	}

	// Create indexer
	idx, err := indexer.NewIndexer(cfg, voyageKey)
	if err != nil {
		return fmt.Errorf("failed to create indexer: %w", err)
	}

	// Create cache
	redisCache, err := cache.NewRedisCache(cfg.Storage.RedisURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Build repo list
	homeDir, _ := os.UserHomeDir()
	repoNames := strings.Split(watchRepos, ",")
	var repos []sync.RepoWatch

	for _, name := range repoNames {
		name = strings.TrimSpace(name)
		repoPath := filepath.Join(homeDir, "repos", name)

		repoCfg, err := config.LoadRepoConfig(repoPath)
		if err != nil {
			logger.Warn("failed to load repo config", "repo", name, "error", err)
			continue
		}

		repos = append(repos, sync.RepoWatch{
			Name:   name,
			Path:   repoPath,
			Config: repoCfg,
		})
	}

	if len(repos) == 0 {
		return fmt.Errorf("no valid repos found")
	}

	// Create and run daemon
	daemon := sync.NewDaemon(repos, interval, idx, redisCache, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	return daemon.Run(ctx)
}
```

**Step 3: Test the daemon**

```bash
go build -o bin/code-indexer ./cmd/code-indexer
./bin/code-indexer watch --repos m32rimm,r3 --interval 60s
```

**Step 4: Commit**

```bash
git add cmd/code-indexer/watch.go internal/sync/
git commit -m "feat: add background sync daemon for watching repo changes"
```

---

## Checkpoint: Phase 4 Complete

At this point you have:

1. **Hierarchical chunking** for large files with class summaries
2. **Secret detection** and automatic redaction
3. **Cursor-based pagination** for large result sets
4. **Metrics CLI** with summary and zero-result analysis
5. **Background sync daemon** for watching repo changes

**Full system is now complete!**

**To deploy:**

```bash
# Build all binaries
go build -o bin/code-indexer ./cmd/code-indexer
go build -o bin/code-index-mcp ./cmd/code-index-mcp

# Install to PATH (optional)
sudo cp bin/code-indexer /usr/local/bin/
sudo cp bin/code-index-mcp /usr/local/bin/

# Initialize repos
code-indexer init ~/repos/m32rimm
code-indexer init ~/repos/r3

# Run initial index
code-indexer index m32rimm
code-indexer index r3

# Register MCP server
./scripts/register-mcp.sh

# Install hooks
./scripts/install-hooks.sh ~/repos/m32rimm
./scripts/install-hooks.sh ~/repos/r3

# Start daemon (in tmux/systemd)
code-indexer watch --repos m32rimm,r3 --interval 60s
```

**Verification:**

```bash
# Check status
code-indexer status

# Check metrics
code-indexer metrics --last 7d

# In Claude Code, test search
# search_code("authentication flow")
```

# Code Indexing Phase 3: Intelligence Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add intelligent features: query classification, pattern detection, AGENTS.md integration, graceful empty results, and module hierarchy support.

**Architecture:** Extends existing search handler with query routing, adds pattern detection during indexing, parses AGENTS.md as navigation docs.

**Tech Stack:** Go, existing components, tree-sitter for pattern analysis

**Prerequisites:** Phase 2 complete (MCP server working)

**Design Doc:** `docs/plans/2026-02-04-code-indexing-design.md`

---

## Task 1: Query Classification

**Files:**
- Create: `internal/search/classifier.go`
- Create: `internal/search/classifier_test.go`
- Modify: `internal/search/handler.go`

**Step 1: Write test for classifier**

```go
// internal/search/classifier_test.go
package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected QueryType
	}{
		// Symbol lookup - quoted terms or identifiers
		{`"validateToken"`, QueryTypeSymbol},
		{`what is getUserById`, QueryTypeSymbol},
		{`find handleAuthError`, QueryTypeSymbol},

		// Relationship queries
		{`what calls validateToken`, QueryTypeRelationship},
		{`functions that use redis`, QueryTypeRelationship},
		{`who imports auth module`, QueryTypeRelationship},

		// Flow queries
		{`data flow from API to database`, QueryTypeFlow},
		{`how does request get to handler`, QueryTypeFlow},
		{`path from login to session`, QueryTypeFlow},

		// Pattern queries
		{`how do importers work`, QueryTypePattern},
		{`pattern for error handling`, QueryTypePattern},
		{`typical structure of a test`, QueryTypePattern},

		// Default: concept search
		{`authentication timeout handling`, QueryTypeConcept},
		{`where is user validation`, QueryTypeConcept},
		{`error handling for database`, QueryTypeConcept},
	}

	classifier := NewClassifier()

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := classifier.Classify(tt.query)
			assert.Equal(t, tt.expected, got, "query: %s", tt.query)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run TestClassify`
Expected: FAIL

**Step 3: Create classifier**

```go
// internal/search/classifier.go
package search

import (
	"regexp"
	"strings"
)

// QueryType represents the type of search query
type QueryType string

const (
	QueryTypeSymbol       QueryType = "symbol"
	QueryTypeConcept      QueryType = "concept"
	QueryTypeRelationship QueryType = "relationship"
	QueryTypeFlow         QueryType = "flow"
	QueryTypePattern      QueryType = "pattern"
)

// Classifier determines the type of a search query
type Classifier struct {
	quotedTermRe     *regexp.Regexp
	identifierRe     *regexp.Regexp
	relationshipWords []string
	flowWords        []string
	patternWords     []string
}

// NewClassifier creates a new query classifier
func NewClassifier() *Classifier {
	return &Classifier{
		quotedTermRe: regexp.MustCompile(`"[^"]+"|\` + "`[^`]+`"),
		identifierRe: regexp.MustCompile(`\b(get|set|is|has|find|handle|create|delete|update|validate|check|process)[A-Z][a-zA-Z]*\b|` +
			`\b[a-z]+(_[a-z]+)+\b|` + // snake_case
			`\b[A-Z][a-z]+([A-Z][a-z]+)+\b`), // PascalCase
		relationshipWords: []string{
			"calls", "call", "calling",
			"uses", "use", "using",
			"imports", "import", "importing",
			"depends", "dependency", "dependencies",
			"references", "reference", "referencing",
			"invokes", "invoke", "invoking",
		},
		flowWords: []string{
			"flow", "flows",
			"path from", "path to",
			"how does", "how do",
			"get to", "gets to",
			"route", "routing",
			"pipeline",
			"chain",
		},
		patternWords: []string{
			"pattern", "patterns",
			"how do .* work",
			"typical", "typically",
			"standard", "convention",
			"structure of",
			"example of",
		},
	}
}

// Classify determines the query type
func (c *Classifier) Classify(query string) QueryType {
	lower := strings.ToLower(query)

	// Check for quoted terms (explicit symbol lookup)
	if c.quotedTermRe.MatchString(query) {
		return QueryTypeSymbol
	}

	// Check for identifier patterns (camelCase, snake_case, PascalCase)
	if c.identifierRe.MatchString(query) {
		return QueryTypeSymbol
	}

	// Check for relationship words
	for _, word := range c.relationshipWords {
		if strings.Contains(lower, word) {
			return QueryTypeRelationship
		}
	}

	// Check for flow words
	for _, word := range c.flowWords {
		if strings.Contains(lower, word) {
			return QueryTypeFlow
		}
	}

	// Check for pattern words
	for _, word := range c.patternWords {
		if strings.Contains(word, ".*") {
			// Regex pattern
			re := regexp.MustCompile(word)
			if re.MatchString(lower) {
				return QueryTypePattern
			}
		} else if strings.Contains(lower, word) {
			return QueryTypePattern
		}
	}

	// Default: concept search
	return QueryTypeConcept
}

// Route returns the retrieval strategy for a query type
func (c *Classifier) Route(qt QueryType) RetrievalStrategy {
	switch qt {
	case QueryTypeSymbol:
		return RetrievalStrategy{
			UseSemanticSearch:  false,
			UseSymbolIndex:     true,
			UseGraphExpansion:  false,
			MaxResults:         10,
		}
	case QueryTypeRelationship:
		return RetrievalStrategy{
			UseSemanticSearch:  false,
			UseSymbolIndex:     true,
			UseGraphExpansion:  true,
			GraphDepth:         1,
			MaxResults:         20,
		}
	case QueryTypeFlow:
		return RetrievalStrategy{
			UseSemanticSearch:  true,
			UseSymbolIndex:     false,
			UseGraphExpansion:  true,
			GraphDepth:         3,
			MaxResults:         15,
		}
	case QueryTypePattern:
		return RetrievalStrategy{
			UseSemanticSearch:  false,
			UsePatternIndex:    true,
			UseGraphExpansion:  false,
			MaxResults:         5,
		}
	default: // Concept
		return RetrievalStrategy{
			UseSemanticSearch:  true,
			UseSymbolIndex:     false,
			UseGraphExpansion:  true,
			GraphDepth:         1,
			MaxResults:         10,
		}
	}
}

// RetrievalStrategy defines how to execute a search
type RetrievalStrategy struct {
	UseSemanticSearch bool
	UseSymbolIndex    bool
	UsePatternIndex   bool
	UseGraphExpansion bool
	GraphDepth        int
	MaxResults        int
}
```

**Step 4: Integrate classifier into handler**

Update `internal/search/handler.go`:

```go
// Add to Handler struct
type Handler struct {
	// ... existing fields ...
	classifier *Classifier
}

// Update NewHandler
func NewHandler(...) (*Handler, error) {
	// ... existing code ...
	return &Handler{
		// ... existing fields ...
		classifier: NewClassifier(),
	}, nil
}

// Update searchCode
func (h *Handler) searchCode(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	// ... parse args ...

	// Classify query
	queryType := h.classifier.Classify(query)
	strategy := h.classifier.Route(queryType)

	h.logger.Info("query classified",
		"query", query,
		"type", queryType,
		"strategy", strategy,
	)

	// Execute based on strategy
	var results []chunk.Chunk
	var err error

	if strategy.UseSymbolIndex {
		results, err = h.searchBySymbol(ctx, query, repo, strategy.MaxResults)
	} else if strategy.UsePatternIndex {
		results, err = h.searchByPattern(ctx, query, repo)
	} else {
		// Default semantic search
		results, err = h.searchSemantic(ctx, query, repo, module, includeTests, strategy.MaxResults)
	}

	if err != nil {
		return nil, err
	}

	// Graph expansion if enabled
	if strategy.UseGraphExpansion && len(results) > 0 {
		results = h.expandWithGraph(ctx, results, strategy.GraphDepth)
	}

	// Format response with query type
	response := h.formatSearchResponseWithType(query, queryType, results, repo)

	// ... cache and return ...
}

func (h *Handler) searchBySymbol(ctx context.Context, query, repo string, limit int) ([]chunk.Chunk, error) {
	// Extract symbol name from query
	symbolName := extractSymbolName(query)
	if symbolName == "" {
		// Fall back to semantic search
		return h.searchSemantic(ctx, query, repo, "", "include", limit)
	}

	// Search by symbol_name field
	filter := map[string]interface{}{
		"symbol_name": symbolName,
	}
	if repo != "" && repo != "all" {
		filter["repo"] = repo
	}

	// Use exact match in Qdrant (requires payload index)
	return h.store.SearchByFilter(ctx, "chunks", filter, limit)
}

func (h *Handler) searchByPattern(ctx context.Context, query, repo string) ([]chunk.Chunk, error) {
	// Search for pattern chunks
	filter := map[string]interface{}{
		"kind": "pattern",
	}
	if repo != "" && repo != "all" {
		filter["repo"] = repo
	}

	return h.store.SearchByFilter(ctx, "chunks", filter, 10)
}

func (h *Handler) searchSemantic(ctx context.Context, query, repo, module, includeTests string, limit int) ([]chunk.Chunk, error) {
	// ... existing semantic search implementation ...
}

func (h *Handler) expandWithGraph(ctx context.Context, results []chunk.Chunk, depth int) []chunk.Chunk {
	// TODO: Implement graph expansion using Neo4j
	// For now, return as-is
	return results
}

func extractSymbolName(query string) string {
	// Extract quoted term
	re := regexp.MustCompile(`"([^"]+)"`)
	if matches := re.FindStringSubmatch(query); len(matches) > 1 {
		return matches[1]
	}

	// Extract identifier pattern
	re = regexp.MustCompile(`\b(get|set|is|has|find|handle|create|delete|update|validate|check|process)[A-Z][a-zA-Z]*\b`)
	if match := re.FindString(query); match != "" {
		return match
	}

	// Extract snake_case
	re = regexp.MustCompile(`\b([a-z]+_[a-z_]+)\b`)
	if match := re.FindString(query); match != "" {
		return match
	}

	return ""
}
```

**Step 5: Add SearchByFilter to store**

```go
// Add to internal/store/qdrant.go

// SearchByFilter searches using payload filters without vector
func (s *QdrantStore) SearchByFilter(ctx context.Context, collection string, filter map[string]interface{}, limit int) ([]chunk.Chunk, error) {
	qdrantFilter := buildFilter(filter)

	results, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Filter:         qdrantFilter,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	chunks := make([]chunk.Chunk, len(results))
	for i, r := range results {
		chunks[i] = payloadToChunk(r.Id.GetUuid(), r.Payload)
	}

	return chunks, nil
}
```

**Step 6: Run tests**

Run: `go test ./internal/search/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/search/ internal/store/
git commit -m "feat: add query classification with routing strategies"
```

---

## Task 2: Pattern Detection During Indexing

**Files:**
- Create: `internal/pattern/detector.go`
- Create: `internal/pattern/detector_test.go`
- Modify: `internal/indexer/indexer.go`

**Step 1: Write test for pattern detection**

```go
// internal/pattern/detector_test.go
package pattern

import (
	"testing"

	"github.com/randalmurphal/code-indexer/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPatterns(t *testing.T) {
	// Simulate symbols from multiple similar files
	symbols := []parser.Symbol{
		// aws_import.py
		{Name: "AWSImporter", Kind: parser.SymbolClass, FilePath: "imports/aws_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},

		// azure_import.py - same structure
		{Name: "AzureImporter", Kind: parser.SymbolClass, FilePath: "imports/azure_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},

		// gcp_import.py - same structure
		{Name: "GCPImporter", Kind: parser.SymbolClass, FilePath: "imports/gcp_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
	}

	detector := NewDetector(DetectorConfig{
		MinClusterSize:      3,
		SimilarityThreshold: 0.8,
	})

	patterns := detector.Detect(symbols)

	require.Len(t, patterns, 1)
	assert.Equal(t, "Importer", patterns[0].Name)
	assert.Len(t, patterns[0].Members, 3)
	assert.Contains(t, patterns[0].Methods, "fetch_data")
	assert.Contains(t, patterns[0].Methods, "transform")
	assert.Contains(t, patterns[0].Methods, "upsert")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/pattern/... -v`
Expected: FAIL

**Step 3: Create pattern detector**

```go
// internal/pattern/detector.go
package pattern

import (
	"sort"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/parser"
)

// Pattern represents a detected code pattern
type Pattern struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Methods       []string `json:"methods"`
	Members       []string `json:"members"`       // File paths following this pattern
	CanonicalFile string   `json:"canonical_file"` // Best example
}

// DetectorConfig configures pattern detection
type DetectorConfig struct {
	MinClusterSize      int
	SimilarityThreshold float64
}

// Detector identifies patterns in code
type Detector struct {
	config DetectorConfig
}

// NewDetector creates a new pattern detector
func NewDetector(config DetectorConfig) *Detector {
	if config.MinClusterSize == 0 {
		config.MinClusterSize = 5
	}
	if config.SimilarityThreshold == 0 {
		config.SimilarityThreshold = 0.8
	}
	return &Detector{config: config}
}

// Detect finds patterns in a set of symbols
func (d *Detector) Detect(symbols []parser.Symbol) []Pattern {
	// Group symbols by file
	fileSymbols := make(map[string][]parser.Symbol)
	for _, sym := range symbols {
		fileSymbols[sym.FilePath] = append(fileSymbols[sym.FilePath], sym)
	}

	// Extract structural signatures for each file
	signatures := make(map[string]FileSignature)
	for file, syms := range fileSymbols {
		signatures[file] = extractSignature(syms)
	}

	// Cluster files by signature similarity
	clusters := d.clusterBySignature(signatures)

	// Convert clusters to patterns
	var patterns []Pattern
	for _, cluster := range clusters {
		if len(cluster) >= d.config.MinClusterSize {
			pattern := d.clusterToPattern(cluster, signatures)
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// FileSignature represents the structural shape of a file
type FileSignature struct {
	HasClass    bool
	ClassName   string
	Methods     []string
	HasInit     bool
	Decorators  []string
}

func extractSignature(symbols []parser.Symbol) FileSignature {
	sig := FileSignature{}

	for _, sym := range symbols {
		switch sym.Kind {
		case parser.SymbolClass:
			sig.HasClass = true
			sig.ClassName = sym.Name
		case parser.SymbolMethod:
			sig.Methods = append(sig.Methods, sym.Name)
			if sym.Name == "__init__" || sym.Name == "constructor" {
				sig.HasInit = true
			}
		}
	}

	sort.Strings(sig.Methods)
	return sig
}

func (d *Detector) clusterBySignature(signatures map[string]FileSignature) [][]string {
	files := make([]string, 0, len(signatures))
	for file := range signatures {
		files = append(files, file)
	}

	// Simple clustering: group files with matching method sets
	visited := make(map[string]bool)
	var clusters [][]string

	for _, file := range files {
		if visited[file] {
			continue
		}

		cluster := []string{file}
		visited[file] = true

		for _, other := range files {
			if visited[other] {
				continue
			}

			similarity := d.computeSimilarity(signatures[file], signatures[other])
			if similarity >= d.config.SimilarityThreshold {
				cluster = append(cluster, other)
				visited[other] = true
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

func (d *Detector) computeSimilarity(a, b FileSignature) float64 {
	if !a.HasClass || !b.HasClass {
		return 0
	}

	// Jaccard similarity of method sets
	setA := make(map[string]bool)
	for _, m := range a.Methods {
		setA[m] = true
	}

	setB := make(map[string]bool)
	for _, m := range b.Methods {
		setB[m] = true
	}

	intersection := 0
	for m := range setA {
		if setB[m] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func (d *Detector) clusterToPattern(files []string, signatures map[string]FileSignature) Pattern {
	// Find common methods across all files
	methodCounts := make(map[string]int)
	for _, file := range files {
		for _, method := range signatures[file].Methods {
			methodCounts[method]++
		}
	}

	var commonMethods []string
	threshold := len(files) * 8 / 10 // 80%
	for method, count := range methodCounts {
		if count >= threshold {
			commonMethods = append(commonMethods, method)
		}
	}

	// Infer pattern name from class names
	patternName := inferPatternName(files, signatures)

	// Pick canonical file (first alphabetically)
	sort.Strings(files)
	canonical := files[0]

	return Pattern{
		Name:          patternName,
		Description:   generatePatternDescription(patternName, commonMethods),
		Methods:       commonMethods,
		Members:       files,
		CanonicalFile: canonical,
	}
}

func inferPatternName(files []string, signatures map[string]FileSignature) string {
	// Find common suffix in class names
	var classNames []string
	for _, file := range files {
		if sig, ok := signatures[file]; ok && sig.ClassName != "" {
			classNames = append(classNames, sig.ClassName)
		}
	}

	if len(classNames) == 0 {
		return "Unknown"
	}

	// Find common suffix (e.g., "Importer" from AWSImporter, AzureImporter)
	first := classNames[0]
	for i := 1; i <= len(first); i++ {
		suffix := first[len(first)-i:]
		allMatch := true
		for _, name := range classNames[1:] {
			if !strings.HasSuffix(name, suffix) {
				allMatch = false
				break
			}
		}
		if allMatch && i > 3 { // Minimum 4 char suffix
			return suffix
		}
	}

	return "Pattern"
}

func generatePatternDescription(name string, methods []string) string {
	return "Classes following the " + name + " pattern implement: " + strings.Join(methods, ", ")
}
```

**Step 4: Run tests**

Run: `go test ./internal/pattern/... -v`
Expected: PASS

**Step 5: Integrate into indexer**

Update `internal/indexer/indexer.go` to detect patterns after parsing a directory and create pattern chunks.

**Step 6: Commit**

```bash
git add internal/pattern/
git commit -m "feat: add pattern detection for similar code structures"
```

---

## Task 3: AGENTS.md Integration

**Files:**
- Create: `internal/docs/agents.go`
- Create: `internal/docs/agents_test.go`
- Modify: `internal/indexer/indexer.go`

**Step 1: Write test for AGENTS.md parser**

```go
// internal/docs/agents_test.go
package docs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentsMD(t *testing.T) {
	content := `# Fisio

Main ETL engine for data imports.

## Entry Points

- ` + "`fisio/main.py`" + ` - CLI entry point
- ` + "`fisio/imports/command.py`" + ` - Import commands

## Key Patterns

### Import Pattern

All importers inherit from BaseImporter and implement:
- fetch_data()
- transform()
- upsert()

## Important Classes

- ` + "`BOUpserter`" + ` - Business object upserter
- ` + "`RetryManager`" + ` - Handles retries with backoff
`

	doc, err := ParseAgentsMD([]byte(content), "fisio/AGENTS.md", "m32rimm")
	require.NoError(t, err)

	assert.Equal(t, "fisio/AGENTS.md", doc.Path)
	assert.Equal(t, "Fisio", doc.Title)
	assert.Contains(t, doc.Description, "ETL engine")

	// Check entry points
	require.Len(t, doc.EntryPoints, 2)
	assert.Equal(t, "fisio/main.py", doc.EntryPoints[0])

	// Check mentioned symbols
	assert.Contains(t, doc.MentionedSymbols, "BOUpserter")
	assert.Contains(t, doc.MentionedSymbols, "RetryManager")

	// Check sections
	require.Len(t, doc.Sections, 3) // Entry Points, Key Patterns, Important Classes
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/docs/... -v`
Expected: FAIL

**Step 3: Create AGENTS.md parser**

```go
// internal/docs/agents.go
package docs

import (
	"regexp"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/chunk"
)

// AgentsDoc represents a parsed AGENTS.md file
type AgentsDoc struct {
	Path             string
	Repo             string
	Module           string
	Title            string
	Description      string
	EntryPoints      []string
	MentionedSymbols []string
	MentionedFiles   []string
	Sections         []Section
}

// Section represents a section of the document
type Section struct {
	Heading     string
	HeadingPath string // Full path: "Key Patterns > Import Pattern"
	Level       int
	Content     string
	StartLine   int
	EndLine     int
}

// ParseAgentsMD parses an AGENTS.md file
func ParseAgentsMD(content []byte, filePath, repo string) (*AgentsDoc, error) {
	text := string(content)
	lines := strings.Split(text, "\n")

	doc := &AgentsDoc{
		Path: filePath,
		Repo: repo,
	}

	// Extract module from path
	parts := strings.Split(filePath, "/")
	if len(parts) > 1 {
		doc.Module = parts[0]
	}

	// Parse headings and sections
	var currentSection *Section
	var headingStack []string

	headingRe := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	codeBlockRe := regexp.MustCompile("^`([^`]+)`$")
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")

	for i, line := range lines {
		if matches := headingRe.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			heading := matches[2]

			// Update heading stack
			for len(headingStack) >= level {
				headingStack = headingStack[:len(headingStack)-1]
			}
			headingStack = append(headingStack, heading)

			// Save previous section
			if currentSection != nil {
				currentSection.EndLine = i - 1
				doc.Sections = append(doc.Sections, *currentSection)
			}

			// Start new section
			currentSection = &Section{
				Heading:     heading,
				HeadingPath: strings.Join(headingStack, " > "),
				Level:       level,
				StartLine:   i + 1,
			}

			// Extract title from first h1
			if level == 1 && doc.Title == "" {
				doc.Title = heading
			}

			continue
		}

		// Accumulate content in current section
		if currentSection != nil {
			currentSection.Content += line + "\n"
		} else if doc.Description == "" && strings.TrimSpace(line) != "" {
			// First non-empty line before any heading is description
			doc.Description = strings.TrimSpace(line)
		}

		// Extract entry points
		if strings.Contains(strings.ToLower(line), "entry point") ||
			(currentSection != nil && strings.Contains(strings.ToLower(currentSection.Heading), "entry")) {
			// Look for code blocks that look like file paths
			for _, match := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
				if isFilePath(match[1]) {
					doc.EntryPoints = append(doc.EntryPoints, match[1])
				}
			}
		}

		// Extract mentioned symbols and files
		for _, match := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
			code := match[1]
			if isFilePath(code) {
				doc.MentionedFiles = append(doc.MentionedFiles, code)
			} else if isSymbol(code) {
				doc.MentionedSymbols = append(doc.MentionedSymbols, code)
			}
		}
	}

	// Save last section
	if currentSection != nil {
		currentSection.EndLine = len(lines)
		doc.Sections = append(doc.Sections, *currentSection)
	}

	return doc, nil
}

// ToChunks converts the document to indexable chunks
func (d *AgentsDoc) ToChunks() []chunk.Chunk {
	var chunks []chunk.Chunk

	for _, section := range d.Sections {
		c := chunk.Chunk{
			Repo:            d.Repo,
			FilePath:        d.Path,
			StartLine:       section.StartLine,
			EndLine:         section.EndLine,
			Type:            chunk.ChunkTypeDoc,
			Kind:            "navigation",
			ModulePath:      d.Module,
			ModuleRoot:      d.Module,
			HeadingPath:     section.HeadingPath,
			Content:         section.Content,
			RetrievalWeight: 1.5, // Boost for navigation docs
		}
		c.ID = chunk.GenerateID(d.Repo, d.Path, section.Heading, section.StartLine)
		chunks = append(chunks, c)
	}

	return chunks
}

func isFilePath(s string) bool {
	return strings.Contains(s, "/") ||
		strings.HasSuffix(s, ".py") ||
		strings.HasSuffix(s, ".js") ||
		strings.HasSuffix(s, ".ts") ||
		strings.HasSuffix(s, ".go")
}

func isSymbol(s string) bool {
	// PascalCase or contains underscore but no slash
	if strings.Contains(s, "/") {
		return false
	}
	return regexp.MustCompile(`^[A-Z][a-zA-Z0-9]+$`).MatchString(s) ||
		regexp.MustCompile(`^[a-z_][a-z0-9_]+$`).MatchString(s)
}
```

**Step 4: Run tests**

Run: `go test ./internal/docs/... -v`
Expected: PASS

**Step 5: Integrate into indexer to parse AGENTS.md files**

**Step 6: Commit**

```bash
git add internal/docs/
git commit -m "feat: add AGENTS.md parser for navigation docs"
```

---

## Task 4: Graceful Empty Results with Suggestions

**Files:**
- Create: `internal/search/suggestions.go`
- Create: `internal/search/suggestions_test.go`
- Modify: `internal/search/handler.go`

**Step 1: Write test for suggestions**

```go
// internal/search/suggestions_test.go
package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSuggestions(t *testing.T) {
	gen := NewSuggestionGenerator()

	// Add some known terms
	gen.AddKnownTerms([]string{
		"auth", "authentication", "login", "session", "token",
		"database", "mongo", "db", "storage",
		"queue", "celery", "async", "message",
	})

	suggestions := gen.Generate("kafka consumer throttling")

	// Should suggest related terms since kafka isn't known
	assert.NotEmpty(t, suggestions)

	// Should find queue/async/message as related to "kafka"
	found := false
	for _, s := range suggestions {
		if s.Term == "queue" || s.Term == "async" || s.Term == "message" {
			found = true
			break
		}
	}
	assert.True(t, found, "should suggest message queue related terms")
}

func TestSynonymLookup(t *testing.T) {
	gen := NewSuggestionGenerator()

	synonyms := gen.GetSynonyms("auth")
	assert.Contains(t, synonyms, "authentication")
	assert.Contains(t, synonyms, "login")

	synonyms = gen.GetSynonyms("db")
	assert.Contains(t, synonyms, "database")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run TestSuggestion`
Expected: FAIL

**Step 3: Create suggestion generator**

```go
// internal/search/suggestions.go
package search

import (
	"sort"
	"strings"
)

// SuggestionGenerator creates search suggestions
type SuggestionGenerator struct {
	synonyms   map[string][]string
	knownTerms map[string]int // term -> count
}

// Suggestion is a search suggestion
type Suggestion struct {
	Term   string `json:"term"`
	Count  int    `json:"count,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NewSuggestionGenerator creates a new generator with default synonyms
func NewSuggestionGenerator() *SuggestionGenerator {
	return &SuggestionGenerator{
		synonyms: map[string][]string{
			"auth":           {"authentication", "login", "session", "token", "credential"},
			"authentication": {"auth", "login", "session", "token"},
			"db":             {"database", "mongo", "sql", "storage", "persistence"},
			"database":       {"db", "mongo", "sql", "storage"},
			"queue":          {"message", "celery", "async", "kafka", "rabbit"},
			"kafka":          {"queue", "message", "celery", "async"},
			"error":          {"exception", "failure", "fault", "issue"},
			"test":           {"spec", "unit", "integration", "mock"},
			"config":         {"configuration", "settings", "options", "env"},
			"http":           {"request", "response", "api", "rest", "endpoint"},
			"api":            {"endpoint", "rest", "http", "route"},
			"user":           {"account", "profile", "member", "person"},
			"file":           {"document", "blob", "storage", "upload"},
			"cache":          {"redis", "memory", "store", "ttl"},
			"log":            {"logging", "logger", "audit", "trace"},
			"timeout":        {"expiry", "ttl", "deadline", "retry"},
		},
		knownTerms: make(map[string]int),
	}
}

// AddKnownTerms adds terms that exist in the index
func (g *SuggestionGenerator) AddKnownTerms(terms []string) {
	for _, term := range terms {
		g.knownTerms[strings.ToLower(term)]++
	}
}

// GetSynonyms returns synonyms for a term
func (g *SuggestionGenerator) GetSynonyms(term string) []string {
	return g.synonyms[strings.ToLower(term)]
}

// Generate creates suggestions for a failed query
func (g *SuggestionGenerator) Generate(query string) []Suggestion {
	words := strings.Fields(strings.ToLower(query))
	suggestions := make(map[string]*Suggestion)

	for _, word := range words {
		// Check synonyms
		for _, syn := range g.synonyms[word] {
			if count, exists := g.knownTerms[syn]; exists {
				if existing, ok := suggestions[syn]; ok {
					existing.Count = count
				} else {
					suggestions[syn] = &Suggestion{
						Term:   syn,
						Count:  count,
						Reason: "synonym for '" + word + "'",
					}
				}
			}
		}

		// Check partial matches in known terms
		for term, count := range g.knownTerms {
			if strings.Contains(term, word) || strings.Contains(word, term) {
				if _, ok := suggestions[term]; !ok {
					suggestions[term] = &Suggestion{
						Term:   term,
						Count:  count,
						Reason: "partial match",
					}
				}
			}
		}
	}

	// Convert to slice and sort by count
	result := make([]Suggestion, 0, len(suggestions))
	for _, s := range suggestions {
		result = append(result, *s)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	// Limit to top 5
	if len(result) > 5 {
		result = result[:5]
	}

	return result
}

// FormatEmptyResponse creates a helpful response when search returns nothing
func (g *SuggestionGenerator) FormatEmptyResponse(query, repo string, suggestions []Suggestion) map[string]interface{} {
	response := map[string]interface{}{
		"results":    []interface{}{},
		"query_type": "concept_search",
		"message":    "No direct matches for '" + query + "'",
	}

	if len(suggestions) > 0 {
		suggestionStrs := make([]string, len(suggestions))
		for i, s := range suggestions {
			if s.Count > 0 {
				suggestionStrs[i] = "Try: '" + s.Term + "' (" + string(rune(s.Count)) + " results)"
			} else {
				suggestionStrs[i] = "Try: '" + s.Term + "'"
			}
		}
		response["suggestions"] = suggestionStrs
	} else {
		response["suggestions"] = []string{
			"Try broader search terms",
			"Check if the repository is indexed: code-indexer status",
		}
	}

	if repo != "" && repo != "all" {
		response["hint"] = "Searched only in " + repo + ". Try repo: 'all' for cross-repo search."
	}

	return response
}
```

**Step 4: Run tests**

Run: `go test ./internal/search/... -v -run TestSuggestion`
Expected: PASS

**Step 5: Integrate into handler**

**Step 6: Commit**

```bash
git add internal/search/suggestions.go internal/search/suggestions_test.go
git commit -m "feat: add suggestion generator for empty search results"
```

---

## Task 5: Module Hierarchy Support

**Files:**
- Modify: `internal/config/config.go` (already has Module struct)
- Create: `internal/indexer/module.go`
- Modify: `internal/search/handler.go`

**Step 1: Create module resolver**

```go
// internal/indexer/module.go
package indexer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/config"
)

// ModuleResolver resolves file paths to module paths
type ModuleResolver struct {
	repoPath string
	config   *config.RepoConfig
	cache    map[string]string
}

// NewModuleResolver creates a new resolver
func NewModuleResolver(repoPath string, cfg *config.RepoConfig) *ModuleResolver {
	return &ModuleResolver{
		repoPath: repoPath,
		config:   cfg,
		cache:    make(map[string]string),
	}
}

// Resolve converts a file path to a module path
func (r *ModuleResolver) Resolve(filePath string) (modulePath, moduleRoot, submodule string) {
	if cached, ok := r.cache[filePath]; ok {
		parts := strings.SplitN(cached, ":", 3)
		return parts[0], parts[1], parts[2]
	}

	// Get relative path
	relPath := filePath
	if filepath.IsAbs(filePath) {
		relPath, _ = filepath.Rel(r.repoPath, filePath)
	}

	// Remove file extension
	relPath = strings.TrimSuffix(relPath, filepath.Ext(relPath))

	// Convert path separators to dots
	modulePath = strings.ReplaceAll(relPath, string(filepath.Separator), ".")

	// Handle duplicate prefixes (e.g., fisio/fisio -> fisio)
	parts := strings.Split(modulePath, ".")
	if len(parts) >= 2 && parts[0] == parts[1] {
		parts = parts[1:]
		modulePath = strings.Join(parts, ".")
	}

	// Extract root and submodule
	if len(parts) > 0 {
		moduleRoot = parts[0]
	}
	if len(parts) > 1 {
		submodule = parts[1]
	}

	// Cache result
	r.cache[filePath] = modulePath + ":" + moduleRoot + ":" + submodule

	return modulePath, moduleRoot, submodule
}

// DetectModules auto-detects module structure from filesystem
func DetectModules(repoPath string) map[string]config.Module {
	modules := make(map[string]config.Module)

	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return modules
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "venv" {
			continue
		}

		dirPath := filepath.Join(repoPath, name)

		// Check if it's a Python package
		if _, err := os.Stat(filepath.Join(dirPath, "__init__.py")); err == nil {
			modules[name] = config.Module{
				Description: "Python package: " + name,
				Submodules:  detectSubmodules(dirPath),
			}
			continue
		}

		// Check if it's a JS/TS package
		if _, err := os.Stat(filepath.Join(dirPath, "package.json")); err == nil {
			modules[name] = config.Module{
				Description: "Node package: " + name,
			}
			continue
		}

		// Check for nested package (e.g., fisio/fisio)
		nestedPath := filepath.Join(dirPath, name)
		if _, err := os.Stat(filepath.Join(nestedPath, "__init__.py")); err == nil {
			modules[name] = config.Module{
				Description: "Python package: " + name,
				Submodules:  detectSubmodules(nestedPath),
			}
		}
	}

	return modules
}

func detectSubmodules(packagePath string) map[string]string {
	submodules := make(map[string]string)

	entries, err := os.ReadDir(packagePath)
	if err != nil {
		return submodules
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}

		subPath := filepath.Join(packagePath, name)
		if _, err := os.Stat(filepath.Join(subPath, "__init__.py")); err == nil {
			submodules[name] = "Submodule: " + name
		}
	}

	return submodules
}
```

**Step 2: Update handler to support module filtering**

The handler already supports `module` parameter. Ensure it properly filters using `module_path` prefix matching.

**Step 3: Commit**

```bash
git add internal/indexer/module.go
git commit -m "feat: add module hierarchy detection and resolution"
```

---

## Checkpoint: Phase 3 Complete

At this point you have:

1. **Query classification** with routing strategies
2. **Pattern detection** for similar code structures
3. **AGENTS.md parser** for navigation docs
4. **Suggestion generator** for empty results
5. **Module hierarchy** support

**To verify:**

```bash
# Rebuild
go build -o bin/code-indexer ./cmd/code-indexer
go build -o bin/code-index-mcp ./cmd/code-index-mcp

# Re-index with pattern detection
./bin/code-indexer index m32rimm

# Test different query types
# In Claude Code, use search_code with various queries
```

**Next:** Phase 4 adds polish (large file chunking, secrets, pagination, metrics CLI).

# Code Indexing Phase 1: Core Indexing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the foundational indexing pipeline that parses Python/JS codebases, extracts semantic chunks, and stores them in Qdrant/Neo4j for retrieval.

**Architecture:** Go CLI (`code-indexer`) using tree-sitter for AST parsing, voyage-4-large for embeddings, Qdrant for vectors, Neo4j for relationships, Redis for caching. Builds on existing `~/repos/graphrag` library.

**Tech Stack:** Go 1.21+, tree-sitter (smacker/go-tree-sitter), graphrag library, Qdrant, Neo4j, Redis, Voyage AI API

**Design Doc:** `docs/plans/2026-02-04-code-indexing-design.md`

---

## Prerequisites

Before starting, ensure infrastructure is running:

```bash
cd ~/repos/graphrag && docker compose up -d
# Verify: Qdrant at :6333, Neo4j at :7687, Redis at :6379
```

---

## Task 1: Project Scaffolding

**Files:**
- Create: `cmd/code-indexer/main.go`
- Create: `internal/config/config.go`
- Create: `go.mod`
- Create: `go.sum`

**Step 1: Initialize Go module**

```bash
cd ~/repos/code-indexer
go mod init github.com/randalmurphal/code-indexer
```

**Step 2: Create main.go with CLI skeleton**

```go
// cmd/code-indexer/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "code-indexer",
	Short: "Semantic code indexing for Claude Code",
	Long:  `Index codebases for semantic search and context-aware retrieval.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("code-indexer v0.1.0")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

**Step 3: Create config structure**

```go
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
	Level      string `yaml:"level"` // error|warn|info|debug
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxFiles   int    `yaml:"max_files"`
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
```

**Step 4: Add dependencies**

```bash
go get github.com/spf13/cobra
go get gopkg.in/yaml.v3
go mod tidy
```

**Step 5: Verify build**

Run: `go build -o bin/code-indexer ./cmd/code-indexer`
Expected: Binary created at `bin/code-indexer`

Run: `./bin/code-indexer version`
Expected: `code-indexer v0.1.0`

**Step 6: Commit**

```bash
git add cmd/ internal/ go.mod go.sum
git commit -m "feat: scaffold code-indexer CLI with config loading"
```

---

## Task 2: Tree-sitter Parser Integration

**Files:**
- Create: `internal/parser/parser.go`
- Create: `internal/parser/python.go`
- Create: `internal/parser/javascript.go`
- Create: `internal/parser/parser_test.go`

**Step 1: Write test for Python parsing**

```go
// internal/parser/parser_test.go
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePythonFunction(t *testing.T) {
	code := `
def hello(name: str) -> str:
    """Greet someone by name."""
    return f"Hello, {name}!"
`
	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.py")
	require.NoError(t, err)

	require.Len(t, symbols, 1)
	assert.Equal(t, "hello", symbols[0].Name)
	assert.Equal(t, SymbolFunction, symbols[0].Kind)
	assert.Equal(t, 2, symbols[0].StartLine)
	assert.Equal(t, 4, symbols[0].EndLine)
	assert.Contains(t, symbols[0].Content, "def hello")
	assert.Contains(t, symbols[0].Docstring, "Greet someone")
}

func TestParsePythonClass(t *testing.T) {
	code := `
class User:
    """Represents a user in the system."""

    def __init__(self, name: str):
        self.name = name

    def greet(self) -> str:
        return f"Hello, {self.name}"
`
	p, err := NewParser(LanguagePython)
	require.NoError(t, err)

	symbols, err := p.Parse([]byte(code), "test.py")
	require.NoError(t, err)

	// Should have class + 2 methods
	require.Len(t, symbols, 3)

	// Class
	assert.Equal(t, "User", symbols[0].Name)
	assert.Equal(t, SymbolClass, symbols[0].Kind)

	// Methods
	assert.Equal(t, "__init__", symbols[1].Name)
	assert.Equal(t, SymbolMethod, symbols[1].Kind)
	assert.Equal(t, "User", symbols[1].Parent)

	assert.Equal(t, "greet", symbols[2].Name)
	assert.Equal(t, SymbolMethod, symbols[2].Kind)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/parser/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create parser types**

```go
// internal/parser/parser.go
package parser

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

// Language represents a supported programming language
type Language string

const (
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
)

// SymbolKind represents the type of code symbol
type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolClass    SymbolKind = "class"
	SymbolMethod   SymbolKind = "method"
	SymbolVariable SymbolKind = "variable"
)

// Symbol represents a parsed code symbol
type Symbol struct {
	Name      string     `json:"name"`
	Kind      SymbolKind `json:"kind"`
	FilePath  string     `json:"file_path"`
	StartLine int        `json:"start_line"`
	EndLine   int        `json:"end_line"`
	Content   string     `json:"content"`
	Docstring string     `json:"docstring,omitempty"`
	Parent    string     `json:"parent,omitempty"`    // For methods: class name
	Signature string     `json:"signature,omitempty"` // Function signature
}

// Parser wraps tree-sitter for a specific language
type Parser struct {
	language Language
	parser   *sitter.Parser
	lang     *sitter.Language
}

// NewParser creates a parser for the given language
func NewParser(lang Language) (*Parser, error) {
	p := sitter.NewParser()

	var l *sitter.Language
	switch lang {
	case LanguagePython:
		l = getPythonLanguage()
	case LanguageJavaScript, LanguageTypeScript:
		l = getJavaScriptLanguage()
	default:
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	p.SetLanguage(l)

	return &Parser{
		language: lang,
		parser:   p,
		lang:     l,
	}, nil
}

// Parse parses source code and extracts symbols
func (p *Parser) Parse(source []byte, filePath string) ([]Symbol, error) {
	tree, err := p.parser.ParseCtx(nil, nil, source)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	defer tree.Close()

	switch p.language {
	case LanguagePython:
		return extractPythonSymbols(tree.RootNode(), source, filePath)
	case LanguageJavaScript, LanguageTypeScript:
		return extractJavaScriptSymbols(tree.RootNode(), source, filePath)
	default:
		return nil, fmt.Errorf("extraction not implemented for: %s", p.language)
	}
}

// DetectLanguage determines language from file extension
func DetectLanguage(filePath string) (Language, bool) {
	switch {
	case hasExtension(filePath, ".py"):
		return LanguagePython, true
	case hasExtension(filePath, ".js", ".jsx"):
		return LanguageJavaScript, true
	case hasExtension(filePath, ".ts", ".tsx"):
		return LanguageTypeScript, true
	default:
		return "", false
	}
}

func hasExtension(path string, exts ...string) bool {
	for _, ext := range exts {
		if len(path) > len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	return false
}
```

**Step 4: Create Python extractor**

```go
// internal/parser/python.go
package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

func getPythonLanguage() *sitter.Language {
	return python.GetLanguage()
}

func extractPythonSymbols(root *sitter.Node, source []byte, filePath string) ([]Symbol, error) {
	var symbols []Symbol

	// Walk the tree
	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractPythonNode(cursor, source, filePath, "", &symbols)

	return symbols, nil
}

func extractPythonNode(cursor *sitter.TreeCursor, source []byte, filePath, parent string, symbols *[]Symbol) {
	node := cursor.CurrentNode()

	switch node.Type() {
	case "function_definition":
		sym := extractPythonFunction(node, source, filePath, parent)
		*symbols = append(*symbols, sym)

	case "class_definition":
		sym := extractPythonClass(node, source, filePath)
		*symbols = append(*symbols, sym)

		// Extract methods within class
		if body := findChild(node, "block"); body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child.Type() == "function_definition" {
					methodSym := extractPythonFunction(child, source, filePath, sym.Name)
					methodSym.Kind = SymbolMethod
					*symbols = append(*symbols, methodSym)
				}
			}
		}
		return // Don't recurse into class, we handled methods above
	}

	// Recurse into children
	if cursor.GoToFirstChild() {
		extractPythonNode(cursor, source, filePath, parent, symbols)
		for cursor.GoToNextSibling() {
			extractPythonNode(cursor, source, filePath, parent, symbols)
		}
		cursor.GoToParent()
	}
}

func extractPythonFunction(node *sitter.Node, source []byte, filePath, parent string) Symbol {
	name := ""
	if nameNode := findChild(node, "identifier"); nameNode != nil {
		name = nodeContent(nameNode, source)
	}

	docstring := ""
	if body := findChild(node, "block"); body != nil {
		if body.ChildCount() > 0 {
			firstStmt := body.Child(0)
			if firstStmt.Type() == "expression_statement" {
				if str := findChild(firstStmt, "string"); str != nil {
					docstring = cleanDocstring(nodeContent(str, source))
				}
			}
		}
	}

	// Build signature from parameters
	signature := "def " + name
	if params := findChild(node, "parameters"); params != nil {
		signature += nodeContent(params, source)
	}
	if retType := findChild(node, "type"); retType != nil {
		signature += " -> " + nodeContent(retType, source)
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolFunction,
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1, // 1-indexed
		EndLine:   int(node.EndPoint().Row) + 1,
		Content:   nodeContent(node, source),
		Docstring: docstring,
		Parent:    parent,
		Signature: signature,
	}
}

func extractPythonClass(node *sitter.Node, source []byte, filePath string) Symbol {
	name := ""
	if nameNode := findChild(node, "identifier"); nameNode != nil {
		name = nodeContent(nameNode, source)
	}

	docstring := ""
	if body := findChild(node, "block"); body != nil {
		if body.ChildCount() > 0 {
			firstStmt := body.Child(0)
			if firstStmt.Type() == "expression_statement" {
				if str := findChild(firstStmt, "string"); str != nil {
					docstring = cleanDocstring(nodeContent(str, source))
				}
			}
		}
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolClass,
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		Content:   nodeContent(node, source),
		Docstring: docstring,
	}
}

// Helper functions

func findChild(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == nodeType {
			return child
		}
	}
	return nil
}

func nodeContent(node *sitter.Node, source []byte) string {
	return string(source[node.StartByte():node.EndByte()])
}

func cleanDocstring(s string) string {
	// Remove quotes
	if len(s) >= 6 && (s[:3] == `"""` || s[:3] == `'''`) {
		s = s[3 : len(s)-3]
	} else if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		s = s[1 : len(s)-1]
	}
	return s
}
```

**Step 5: Create JavaScript extractor (stub)**

```go
// internal/parser/javascript.go
package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
)

func getJavaScriptLanguage() *sitter.Language {
	return javascript.GetLanguage()
}

func extractJavaScriptSymbols(root *sitter.Node, source []byte, filePath string) ([]Symbol, error) {
	var symbols []Symbol

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractJavaScriptNode(cursor, source, filePath, "", &symbols)

	return symbols, nil
}

func extractJavaScriptNode(cursor *sitter.TreeCursor, source []byte, filePath, parent string, symbols *[]Symbol) {
	node := cursor.CurrentNode()

	switch node.Type() {
	case "function_declaration":
		sym := extractJSFunction(node, source, filePath)
		*symbols = append(*symbols, sym)

	case "class_declaration":
		sym := extractJSClass(node, source, filePath)
		*symbols = append(*symbols, sym)

		// Extract methods
		if body := findChild(node, "class_body"); body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child.Type() == "method_definition" {
					methodSym := extractJSMethod(child, source, filePath, sym.Name)
					*symbols = append(*symbols, methodSym)
				}
			}
		}
		return

	case "arrow_function", "function":
		// Handle anonymous functions assigned to variables
		// Check if parent is variable_declarator
	}

	if cursor.GoToFirstChild() {
		extractJavaScriptNode(cursor, source, filePath, parent, symbols)
		for cursor.GoToNextSibling() {
			extractJavaScriptNode(cursor, source, filePath, parent, symbols)
		}
		cursor.GoToParent()
	}
}

func extractJSFunction(node *sitter.Node, source []byte, filePath string) Symbol {
	name := ""
	if nameNode := findChild(node, "identifier"); nameNode != nil {
		name = nodeContent(nameNode, source)
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolFunction,
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		Content:   nodeContent(node, source),
	}
}

func extractJSClass(node *sitter.Node, source []byte, filePath string) Symbol {
	name := ""
	if nameNode := findChild(node, "identifier"); nameNode != nil {
		name = nodeContent(nameNode, source)
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolClass,
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		Content:   nodeContent(node, source),
	}
}

func extractJSMethod(node *sitter.Node, source []byte, filePath, parent string) Symbol {
	name := ""
	if nameNode := findChild(node, "property_identifier"); nameNode != nil {
		name = nodeContent(nameNode, source)
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolMethod,
		FilePath:  filePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		Content:   nodeContent(node, source),
		Parent:    parent,
	}
}
```

**Step 6: Add test dependency and tree-sitter**

```bash
go get github.com/stretchr/testify
go get github.com/smacker/go-tree-sitter
go get github.com/smacker/go-tree-sitter/python
go get github.com/smacker/go-tree-sitter/javascript
go mod tidy
```

**Step 7: Run tests**

Run: `go test ./internal/parser/... -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/parser/
git commit -m "feat: add tree-sitter parser for Python and JavaScript"
```

---

## Task 3: Chunk Model and Extractor

**Files:**
- Create: `internal/chunk/chunk.go`
- Create: `internal/chunk/extractor.go`
- Create: `internal/chunk/extractor_test.go`

**Step 1: Write test for chunk extraction**

```go
// internal/chunk/extractor_test.go
package chunk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractChunksFromPython(t *testing.T) {
	code := `
"""Module for user management."""

def get_user(user_id: int) -> dict:
    """Fetch user by ID."""
    return {"id": user_id}

class UserService:
    """Service for user operations."""

    def __init__(self, db):
        self.db = db

    def create(self, name: str) -> dict:
        """Create a new user."""
        return self.db.insert({"name": name})
`

	extractor := NewExtractor()
	chunks, err := extractor.Extract([]byte(code), "users.py", "m32rimm", "fisio.common")
	require.NoError(t, err)

	// Should have: get_user, UserService, __init__, create
	require.Len(t, chunks, 4)

	// Check function chunk
	funcChunk := findChunkByName(chunks, "get_user")
	require.NotNil(t, funcChunk)
	assert.Equal(t, ChunkTypeCode, funcChunk.Type)
	assert.Equal(t, "function", funcChunk.Kind)
	assert.Equal(t, "m32rimm", funcChunk.Repo)
	assert.Equal(t, "fisio.common", funcChunk.ModulePath)
	assert.False(t, funcChunk.IsTest)
	assert.Equal(t, float32(1.0), funcChunk.RetrievalWeight)

	// Check method chunk has parent context
	createChunk := findChunkByName(chunks, "create")
	require.NotNil(t, createChunk)
	assert.Equal(t, "method", createChunk.Kind)
	assert.Contains(t, createChunk.ContextHeader, "UserService")
}

func TestExtractChunksFromTest(t *testing.T) {
	code := `
def test_get_user():
    result = get_user(1)
    assert result["id"] == 1
`

	extractor := NewExtractor()
	chunks, err := extractor.Extract([]byte(code), "test_users.py", "m32rimm", "fisio.tests")
	require.NoError(t, err)

	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsTest)
	assert.Equal(t, float32(0.5), chunks[0].RetrievalWeight)
}

func findChunkByName(chunks []Chunk, name string) *Chunk {
	for i := range chunks {
		if chunks[i].SymbolName == name {
			return &chunks[i]
		}
	}
	return nil
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/chunk/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create chunk types**

```go
// internal/chunk/chunk.go
package chunk

// ChunkType distinguishes code from documentation
type ChunkType string

const (
	ChunkTypeCode ChunkType = "code"
	ChunkTypeDoc  ChunkType = "doc"
)

// Chunk represents an indexable unit of code or documentation
type Chunk struct {
	// Identity
	ID       string `json:"id"` // Generated: hash of repo+path+symbol+lines
	Repo     string `json:"repo"`
	FilePath string `json:"file_path"`

	// Location
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`

	// Classification
	Type            ChunkType `json:"type"`              // code | doc
	Kind            string    `json:"kind,omitempty"`    // function | class | method | pattern
	ModulePath      string    `json:"module_path"`       // fisio.imports.aws
	ModuleRoot      string    `json:"module_root"`       // fisio
	Submodule       string    `json:"submodule"`         // imports
	SymbolName      string    `json:"symbol_name,omitempty"`
	HeadingPath     string    `json:"heading_path,omitempty"` // For docs

	// Content
	Content       string `json:"content"`
	ContextHeader string `json:"context_header,omitempty"` // Injected context for methods
	Signature     string `json:"signature,omitempty"`
	Docstring     string `json:"docstring,omitempty"`

	// Metadata
	IsTest          bool    `json:"is_test"`
	RetrievalWeight float32 `json:"retrieval_weight"` // 1.0 normal, 0.5 for tests
	HasSecrets      bool    `json:"has_secrets"`
	FollowsPattern  string  `json:"follows_pattern,omitempty"`

	// Vector (populated after embedding)
	Vector []float32 `json:"vector,omitempty"`
}

// TokenEstimate returns rough token count for the chunk
func (c *Chunk) TokenEstimate() int {
	// Rough estimate: ~4 chars per token
	return len(c.Content) / 4
}
```

**Step 4: Create extractor**

```go
// internal/chunk/extractor.go
package chunk

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/parser"
)

// Extractor converts parsed symbols into chunks
type Extractor struct {
	testPatterns []string
}

// NewExtractor creates a chunk extractor with default test patterns
func NewExtractor() *Extractor {
	return &Extractor{
		testPatterns: []string{
			"test_",
			"_test.py",
			"_test.go",
			".test.js",
			".test.ts",
			".spec.js",
			".spec.ts",
			"/tests/",
			"/__tests__/",
		},
	}
}

// Extract parses code and returns chunks
func (e *Extractor) Extract(source []byte, filePath, repo, modulePath string) ([]Chunk, error) {
	lang, ok := parser.DetectLanguage(filePath)
	if !ok {
		return nil, fmt.Errorf("unsupported file type: %s", filePath)
	}

	p, err := parser.NewParser(lang)
	if err != nil {
		return nil, err
	}

	symbols, err := p.Parse(source, filePath)
	if err != nil {
		return nil, err
	}

	isTest := e.isTestFile(filePath)
	moduleRoot, submodule := parseModulePath(modulePath)

	var chunks []Chunk

	// Build map of classes for context injection
	classContent := make(map[string]string)
	for _, sym := range symbols {
		if sym.Kind == parser.SymbolClass {
			classContent[sym.Name] = sym.Content
		}
	}

	for _, sym := range symbols {
		chunk := Chunk{
			Repo:       repo,
			FilePath:   filePath,
			StartLine:  sym.StartLine,
			EndLine:    sym.EndLine,
			Type:       ChunkTypeCode,
			Kind:       string(sym.Kind),
			ModulePath: modulePath,
			ModuleRoot: moduleRoot,
			Submodule:  submodule,
			SymbolName: sym.Name,
			Content:    sym.Content,
			Signature:  sym.Signature,
			Docstring:  sym.Docstring,
			IsTest:     isTest,
		}

		// Set retrieval weight
		if isTest {
			chunk.RetrievalWeight = 0.5
		} else {
			chunk.RetrievalWeight = 1.0
		}

		// Inject context header for methods
		if sym.Kind == parser.SymbolMethod && sym.Parent != "" {
			chunk.ContextHeader = fmt.Sprintf("# File: %s\n# Class: %s\n", filePath, sym.Parent)
		}

		// Generate ID
		chunk.ID = generateChunkID(repo, filePath, sym.Name, sym.StartLine)

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func (e *Extractor) isTestFile(filePath string) bool {
	lower := strings.ToLower(filePath)
	for _, pattern := range e.testPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func parseModulePath(modulePath string) (root, sub string) {
	parts := strings.SplitN(modulePath, ".", 2)
	root = parts[0]
	if len(parts) > 1 {
		sub = parts[1]
	}
	return
}

func generateChunkID(repo, filePath, symbolName string, startLine int) string {
	data := fmt.Sprintf("%s:%s:%s:%d", repo, filePath, symbolName, startLine)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}
```

**Step 5: Run tests**

Run: `go test ./internal/chunk/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/chunk/
git commit -m "feat: add chunk model and extractor with test file detection"
```

---

## Task 4: Qdrant Storage Integration

**Files:**
- Create: `internal/store/qdrant.go`
- Create: `internal/store/qdrant_test.go`

**Step 1: Write test for Qdrant storage**

```go
// internal/store/qdrant_test.go
package store

import (
	"context"
	"os"
	"testing"

	"github.com/randalmurphal/code-indexer/internal/chunk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQdrantStore(t *testing.T) {
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set, skipping integration test")
	}

	ctx := context.Background()
	store, err := NewQdrantStore(os.Getenv("QDRANT_URL"))
	require.NoError(t, err)

	// Clean up test collection
	collectionName := "test_chunks"
	_ = store.DeleteCollection(ctx, collectionName)

	// Create collection
	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	// Insert chunk
	testChunk := chunk.Chunk{
		ID:              "test-001",
		Repo:            "test-repo",
		FilePath:        "test.py",
		StartLine:       1,
		EndLine:         10,
		Type:            chunk.ChunkTypeCode,
		Kind:            "function",
		ModulePath:      "test.module",
		ModuleRoot:      "test",
		Submodule:       "module",
		SymbolName:      "test_func",
		Content:         "def test_func(): pass",
		IsTest:          false,
		RetrievalWeight: 1.0,
		Vector:          make([]float32, 1024), // Zero vector for test
	}

	err = store.UpsertChunks(ctx, collectionName, []chunk.Chunk{testChunk})
	require.NoError(t, err)

	// Search (will return our chunk since it's the only one)
	results, err := store.Search(ctx, collectionName, make([]float32, 1024), 10, nil)
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "test-001", results[0].ID)
	assert.Equal(t, "test_func", results[0].SymbolName)

	// Clean up
	err = store.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `QDRANT_URL=http://localhost:6333 go test ./internal/store/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create Qdrant store**

```go
// internal/store/qdrant.go
package store

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
	"github.com/randalmurphal/code-indexer/internal/chunk"
)

// QdrantStore handles vector storage in Qdrant
type QdrantStore struct {
	client *qdrant.Client
}

// NewQdrantStore creates a new Qdrant store
func NewQdrantStore(url string) (*QdrantStore, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	return &QdrantStore{client: client}, nil
}

// EnsureCollection creates collection if it doesn't exist
func (s *QdrantStore) EnsureCollection(ctx context.Context, name string, vectorSize int) error {
	exists, err := s.client.CollectionExists(ctx, name)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

// DeleteCollection removes a collection
func (s *QdrantStore) DeleteCollection(ctx context.Context, name string) error {
	return s.client.DeleteCollection(ctx, name)
}

// UpsertChunks inserts or updates chunks
func (s *QdrantStore) UpsertChunks(ctx context.Context, collection string, chunks []chunk.Chunk) error {
	points := make([]*qdrant.PointStruct, len(chunks))

	for i, c := range chunks {
		payload := map[string]interface{}{
			"repo":             c.Repo,
			"file_path":        c.FilePath,
			"start_line":       c.StartLine,
			"end_line":         c.EndLine,
			"type":             string(c.Type),
			"kind":             c.Kind,
			"module_path":      c.ModulePath,
			"module_root":      c.ModuleRoot,
			"submodule":        c.Submodule,
			"symbol_name":      c.SymbolName,
			"content":          c.Content,
			"context_header":   c.ContextHeader,
			"signature":        c.Signature,
			"docstring":        c.Docstring,
			"is_test":          c.IsTest,
			"retrieval_weight": c.RetrievalWeight,
			"has_secrets":      c.HasSecrets,
			"follows_pattern":  c.FollowsPattern,
		}

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDStr(c.ID),
			Vectors: qdrant.NewVectors(c.Vector...),
			Payload: qdrant.NewValueMap(payload),
		}
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         points,
	})

	return err
}

// Search performs vector similarity search
func (s *QdrantStore) Search(ctx context.Context, collection string, vector []float32, limit int, filter map[string]interface{}) ([]chunk.Chunk, error) {
	var qdrantFilter *qdrant.Filter
	if filter != nil {
		qdrantFilter = buildFilter(filter)
	}

	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(vector...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		Filter:         qdrantFilter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	chunks := make([]chunk.Chunk, len(results))
	for i, r := range results {
		chunks[i] = payloadToChunk(r.Id.GetUuid(), r.Payload)
		chunks[i].Vector = nil // Don't return vectors in results
	}

	return chunks, nil
}

func buildFilter(filter map[string]interface{}) *qdrant.Filter {
	var must []*qdrant.Condition

	for key, value := range filter {
		switch v := value.(type) {
		case string:
			must = append(must, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: key,
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Keyword{Keyword: v},
						},
					},
				},
			})
		case bool:
			must = append(must, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: key,
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Boolean{Boolean: v},
						},
					},
				},
			})
		}
	}

	return &qdrant.Filter{Must: must}
}

func payloadToChunk(id string, payload map[string]*qdrant.Value) chunk.Chunk {
	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			return v.GetStringValue()
		}
		return ""
	}
	getInt := func(key string) int {
		if v, ok := payload[key]; ok {
			return int(v.GetIntegerValue())
		}
		return 0
	}
	getBool := func(key string) bool {
		if v, ok := payload[key]; ok {
			return v.GetBoolValue()
		}
		return false
	}
	getFloat := func(key string) float32 {
		if v, ok := payload[key]; ok {
			return float32(v.GetDoubleValue())
		}
		return 0
	}

	return chunk.Chunk{
		ID:              id,
		Repo:            getString("repo"),
		FilePath:        getString("file_path"),
		StartLine:       getInt("start_line"),
		EndLine:         getInt("end_line"),
		Type:            chunk.ChunkType(getString("type")),
		Kind:            getString("kind"),
		ModulePath:      getString("module_path"),
		ModuleRoot:      getString("module_root"),
		Submodule:       getString("submodule"),
		SymbolName:      getString("symbol_name"),
		Content:         getString("content"),
		ContextHeader:   getString("context_header"),
		Signature:       getString("signature"),
		Docstring:       getString("docstring"),
		IsTest:          getBool("is_test"),
		RetrievalWeight: getFloat("retrieval_weight"),
		HasSecrets:      getBool("has_secrets"),
		FollowsPattern:  getString("follows_pattern"),
	}
}
```

**Step 4: Add Qdrant client dependency**

```bash
go get github.com/qdrant/go-client
go mod tidy
```

**Step 5: Run tests**

Run: `QDRANT_URL=http://localhost:6333 go test ./internal/store/... -v`
Expected: PASS (requires Qdrant running)

**Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat: add Qdrant vector store with search and upsert"
```

---

## Task 5: Voyage AI Embedding Client

**Files:**
- Create: `internal/embedding/voyage.go`
- Create: `internal/embedding/voyage_test.go`

**Step 1: Write test for embedding**

```go
// internal/embedding/voyage_test.go
package embedding

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoyageEmbed(t *testing.T) {
	apiKey := os.Getenv("VOYAGE_API_KEY")
	if apiKey == "" {
		t.Skip("VOYAGE_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	client := NewVoyageClient(apiKey, "voyage-4-large")

	texts := []string{
		"def hello(): return 'world'",
		"function greet() { return 'hi'; }",
	}

	vectors, err := client.Embed(ctx, texts)
	require.NoError(t, err)

	require.Len(t, vectors, 2)
	assert.Len(t, vectors[0], 1024) // voyage-4-large dimension
	assert.Len(t, vectors[1], 1024)

	// Vectors should be normalized (magnitude ~1)
	magnitude := float32(0)
	for _, v := range vectors[0] {
		magnitude += v * v
	}
	assert.InDelta(t, 1.0, magnitude, 0.01)
}
```

**Step 2: Run test to verify it fails**

Run: `VOYAGE_API_KEY=your-key go test ./internal/embedding/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create Voyage client**

```go
// internal/embedding/voyage.go
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const voyageAPIURL = "https://api.voyageai.com/v1/embeddings"

// VoyageClient handles embeddings via Voyage AI API
type VoyageClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewVoyageClient creates a new Voyage embedding client
func NewVoyageClient(apiKey, model string) *VoyageClient {
	return &VoyageClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

type voyageResponse struct {
	Data  []voyageEmbedding `json:"data"`
	Usage voyageUsage       `json:"usage"`
}

type voyageEmbedding struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type voyageUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// Embed generates embeddings for the given texts
func (c *VoyageClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := voyageRequest{
		Input:     texts,
		Model:     c.model,
		InputType: "document",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", voyageAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var voyageResp voyageResponse
	if err := json.Unmarshal(body, &voyageResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Sort by index to ensure order matches input
	vectors := make([][]float32, len(texts))
	for _, emb := range voyageResp.Data {
		vectors[emb.Index] = emb.Embedding
	}

	return vectors, nil
}

// EmbedBatched handles large inputs by batching
func (c *VoyageClient) EmbedBatched(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 128 // Voyage default max
	}

	var allVectors [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		vectors, err := c.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d failed: %w", i, end, err)
		}

		allVectors = append(allVectors, vectors...)
	}

	return allVectors, nil
}

// Dimension returns the vector dimension for the model
func (c *VoyageClient) Dimension() int {
	switch c.model {
	case "voyage-4-large", "voyage-3-large", "voyage-code-3":
		return 1024
	case "voyage-4", "voyage-3":
		return 1024
	case "voyage-4-lite", "voyage-3-lite":
		return 512
	default:
		return 1024
	}
}
```

**Step 4: Run tests**

Run: `VOYAGE_API_KEY=your-key go test ./internal/embedding/... -v`
Expected: PASS (requires valid API key)

**Step 5: Commit**

```bash
git add internal/embedding/
git commit -m "feat: add Voyage AI embedding client with batching"
```

---

## Task 6: Indexer Pipeline

**Files:**
- Create: `internal/indexer/indexer.go`
- Create: `internal/indexer/indexer_test.go`
- Create: `internal/indexer/walker.go`

**Step 1: Write test for indexer**

```go
// internal/indexer/indexer_test.go
package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexerWalk(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create a Python file
	pyContent := `
def hello():
    """Say hello."""
    return "Hello"
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.py"), []byte(pyContent), 0644)
	require.NoError(t, err)

	// Create a test file
	testContent := `
def test_hello():
    assert hello() == "Hello"
`
	err = os.WriteFile(filepath.Join(tmpDir, "test_hello.py"), []byte(testContent), 0644)
	require.NoError(t, err)

	// Create __pycache__ (should be excluded)
	err = os.MkdirAll(filepath.Join(tmpDir, "__pycache__"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "__pycache__", "test.pyc"), []byte("binary"), 0644)
	require.NoError(t, err)

	// Walk and count files
	walker := NewWalker([]string{"**/*.py"}, []string{"**/__pycache__/**"})

	var files []string
	err = walker.Walk(tmpDir, func(path string) error {
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)

	// Should find 2 Python files, not the .pyc
	require.Len(t, files, 2)
}

func TestIndexerIntegration(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" || os.Getenv("QDRANT_URL") == "" {
		t.Skip("VOYAGE_API_KEY or QDRANT_URL not set")
	}

	// This would be a full integration test
	// Skipping detailed implementation for now
	t.Skip("Integration test requires full setup")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/indexer/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Create file walker**

```go
// internal/indexer/walker.go
package indexer

import (
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

// Walker traverses directories respecting include/exclude patterns
type Walker struct {
	includes []string
	excludes []string
}

// NewWalker creates a new file walker
func NewWalker(includes, excludes []string) *Walker {
	if len(includes) == 0 {
		includes = []string{"**/*.py", "**/*.js", "**/*.ts", "**/*.tsx", "**/*.jsx"}
	}

	// Default excludes
	defaultExcludes := []string{
		"**/.git/**",
		"**/__pycache__/**",
		"**/*.pyc",
		"**/node_modules/**",
		"**/venv/**",
		"**/.venv/**",
		"**/dist/**",
		"**/build/**",
	}
	excludes = append(defaultExcludes, excludes...)

	return &Walker{
		includes: includes,
		excludes: excludes,
	}
}

// Walk traverses the directory calling fn for each matching file
func (w *Walker) Walk(root string, fn func(path string) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// Check if directory should be excluded
			relPath, _ := filepath.Rel(root, path)
			for _, pattern := range w.excludes {
				matched, _ := doublestar.Match(pattern, relPath+"/")
				if matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		relPath, _ := filepath.Rel(root, path)

		// Check excludes first
		for _, pattern := range w.excludes {
			matched, _ := doublestar.Match(pattern, relPath)
			if matched {
				return nil
			}
		}

		// Check includes
		for _, pattern := range w.includes {
			matched, _ := doublestar.Match(pattern, relPath)
			if matched {
				return fn(path)
			}
		}

		return nil
	})
}
```

**Step 4: Create indexer**

```go
// internal/indexer/indexer.go
package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/chunk"
	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/embedding"
	"github.com/randalmurphal/code-indexer/internal/store"
)

// Indexer coordinates the indexing pipeline
type Indexer struct {
	config    *config.Config
	extractor *chunk.Extractor
	embedder  *embedding.VoyageClient
	store     *store.QdrantStore
	logger    *slog.Logger
}

// NewIndexer creates a new indexer
func NewIndexer(cfg *config.Config, voyageKey string) (*Indexer, error) {
	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)

	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	return &Indexer{
		config:    cfg,
		extractor: chunk.NewExtractor(),
		embedder:  embedder,
		store:     qdrantStore,
		logger:    slog.Default(),
	}, nil
}

// IndexResult contains indexing statistics
type IndexResult struct {
	FilesProcessed int
	ChunksCreated  int
	Errors         []error
}

// Index indexes a repository
func (idx *Indexer) Index(ctx context.Context, repoPath string, repoCfg *config.RepoConfig) (*IndexResult, error) {
	result := &IndexResult{}

	// Ensure collection exists
	collectionName := "chunks"
	if err := idx.store.EnsureCollection(ctx, collectionName, idx.embedder.Dimension()); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	// Walk files
	walker := NewWalker(repoCfg.Include, repoCfg.Exclude)

	var allChunks []chunk.Chunk

	err := walker.Walk(repoPath, func(path string) error {
		idx.logger.Info("processing file", "path", path)

		source, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read %s: %w", path, err))
			return nil // Continue with other files
		}

		relPath, _ := filepath.Rel(repoPath, path)
		modulePath := inferModulePath(relPath, repoCfg)

		chunks, err := idx.extractor.Extract(source, relPath, repoCfg.Name, modulePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("extract %s: %w", path, err))
			return nil
		}

		allChunks = append(allChunks, chunks...)
		result.FilesProcessed++

		return nil
	})

	if err != nil {
		return result, fmt.Errorf("walk failed: %w", err)
	}

	if len(allChunks) == 0 {
		return result, nil
	}

	// Generate embeddings
	idx.logger.Info("generating embeddings", "chunks", len(allChunks))

	texts := make([]string, len(allChunks))
	for i, c := range allChunks {
		// Combine content with context for better embeddings
		text := c.Content
		if c.Docstring != "" {
			text = c.Docstring + "\n\n" + text
		}
		if c.ContextHeader != "" {
			text = c.ContextHeader + "\n" + text
		}
		texts[i] = text
	}

	vectors, err := idx.embedder.EmbedBatched(ctx, texts, 64)
	if err != nil {
		return result, fmt.Errorf("embedding failed: %w", err)
	}

	for i := range allChunks {
		allChunks[i].Vector = vectors[i]
	}

	// Store in Qdrant
	idx.logger.Info("storing chunks", "count", len(allChunks))

	// Batch upsert
	batchSize := 100
	for i := 0; i < len(allChunks); i += batchSize {
		end := i + batchSize
		if end > len(allChunks) {
			end = len(allChunks)
		}

		if err := idx.store.UpsertChunks(ctx, collectionName, allChunks[i:end]); err != nil {
			return result, fmt.Errorf("upsert failed: %w", err)
		}
	}

	result.ChunksCreated = len(allChunks)

	return result, nil
}

func inferModulePath(relPath string, cfg *config.RepoConfig) string {
	// Convert file path to module path
	// e.g., "fisio/fisio/imports/aws.py" -> "fisio.imports.aws"

	dir := filepath.Dir(relPath)
	parts := strings.Split(dir, string(filepath.Separator))

	// Remove common prefixes that duplicate
	// e.g., fisio/fisio -> fisio
	if len(parts) >= 2 && parts[0] == parts[1] {
		parts = parts[1:]
	}

	return strings.Join(parts, ".")
}
```

**Step 5: Add doublestar dependency**

```bash
go get github.com/bmatcuk/doublestar/v4
go mod tidy
```

**Step 6: Run tests**

Run: `go test ./internal/indexer/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/indexer/
git commit -m "feat: add indexer pipeline with file walker and batched processing"
```

---

## Task 7: CLI Commands (init, index, status)

**Files:**
- Modify: `cmd/code-indexer/main.go`
- Create: `cmd/code-indexer/init.go`
- Create: `cmd/code-indexer/index.go`
- Create: `cmd/code-indexer/status.go`

**Step 1: Create init command**

```go
// cmd/code-indexer/init.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init [repo-path]",
	Short: "Initialize indexing configuration for a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	repoPath := args[0]

	// Resolve to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if repo exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	}

	// Check for existing config
	configPath := filepath.Join(absPath, ".ai-devtools.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		return nil
	}

	// Detect project type and create config
	repoName := filepath.Base(absPath)
	defaultBranch := detectDefaultBranch(absPath)

	config := map[string]interface{}{
		"code-index": map[string]interface{}{
			"name":           repoName,
			"default_branch": defaultBranch,
			"include":        detectIncludes(absPath),
			"exclude":        []string{},
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Review and customize the config file\n")
	fmt.Printf("  2. Run: code-indexer index %s\n", repoName)

	return nil
}

func detectDefaultBranch(repoPath string) string {
	// Try to read from git
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	if data, err := os.ReadFile(headPath); err == nil {
		// Parse "ref: refs/heads/main" or similar
		content := string(data)
		if len(content) > 16 && content[:16] == "ref: refs/heads/" {
			return content[16 : len(content)-1] // Remove newline
		}
	}
	return "main"
}

func detectIncludes(repoPath string) []string {
	includes := []string{}

	// Check for Python
	if hasFiles(repoPath, "*.py") {
		includes = append(includes, "**/*.py")
	}

	// Check for JavaScript/TypeScript
	if hasFiles(repoPath, "*.js") || hasFiles(repoPath, "*.ts") {
		includes = append(includes, "**/*.js", "**/*.ts", "**/*.jsx", "**/*.tsx")
	}

	if len(includes) == 0 {
		// Default
		includes = []string{"**/*.py", "**/*.js", "**/*.ts"}
	}

	return includes
}

func hasFiles(dir string, pattern string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	if len(matches) > 0 {
		return true
	}
	// Check one level down
	matches, _ = filepath.Glob(filepath.Join(dir, "*", pattern))
	return len(matches) > 0
}
```

**Step 2: Create index command**

```go
// cmd/code-indexer/index.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/indexer"
	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index [repo-name-or-path]",
	Short: "Index a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runIndex,
}

var (
	indexIncremental bool
)

func init() {
	indexCmd.Flags().BoolVar(&indexIncremental, "incremental", false, "Only index changed files")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, args []string) error {
	repoArg := args[0]

	// Resolve repo path
	repoPath := repoArg
	if !filepath.IsAbs(repoPath) {
		// Check if it's a registered repo name or relative path
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			// Try ~/repos/{name}
			homeDir, _ := os.UserHomeDir()
			repoPath = filepath.Join(homeDir, "repos", repoArg)
		}
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", absPath)
	}

	// Load configs
	globalCfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	repoCfg, err := config.LoadRepoConfig(absPath)
	if err != nil {
		return fmt.Errorf("failed to load repo config: %w\nRun 'code-indexer init %s' first", err, absPath)
	}

	// Get API key
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY environment variable not set")
	}

	// Create indexer
	idx, err := indexer.NewIndexer(globalCfg, voyageKey)
	if err != nil {
		return fmt.Errorf("failed to create indexer: %w", err)
	}

	// Run indexing
	fmt.Printf("Indexing %s (%s)...\n", repoCfg.Name, absPath)

	ctx := context.Background()
	result, err := idx.Index(ctx, absPath, repoCfg)
	if err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	// Report results
	fmt.Printf("\nIndexing complete:\n")
	fmt.Printf("  Files processed: %d\n", result.FilesProcessed)
	fmt.Printf("  Chunks created:  %d\n", result.ChunksCreated)

	if len(result.Errors) > 0 {
		fmt.Printf("  Errors: %d\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("    - %v\n", e)
		}
	}

	return nil
}

func getGlobalConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "code-index", "config.yaml")
}
```

**Step 3: Create status command**

```go
// cmd/code-indexer/status.go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		fmt.Println("No global config found, using defaults")
		cfg = config.DefaultConfig()
	}

	// Connect to Qdrant
	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Qdrant at %s: %w", cfg.Storage.QdrantURL, err)
	}

	ctx := context.Background()

	// Get collection info
	info, err := qdrantStore.CollectionInfo(ctx, "chunks")
	if err != nil {
		fmt.Println("No index found. Run 'code-indexer index <repo>' to create one.")
		return nil
	}

	fmt.Println("Index Status:")
	fmt.Printf("  Collection: chunks\n")
	fmt.Printf("  Points:     %d\n", info.PointsCount)
	fmt.Printf("  Vectors:    %d dimensions\n", info.VectorSize)
	fmt.Printf("  Status:     %s\n", info.Status)

	return nil
}
```

**Step 4: Add CollectionInfo to store**

```go
// Add to internal/store/qdrant.go

// CollectionInfo contains collection metadata
type CollectionInfo struct {
	PointsCount int64
	VectorSize  int
	Status      string
}

// CollectionInfo gets collection metadata
func (s *QdrantStore) CollectionInfo(ctx context.Context, name string) (*CollectionInfo, error) {
	info, err := s.client.GetCollectionInfo(ctx, name)
	if err != nil {
		return nil, err
	}

	vectorSize := 0
	if params := info.Config.GetParams(); params != nil {
		vectorSize = int(params.GetSize())
	}

	return &CollectionInfo{
		PointsCount: int64(info.PointsCount),
		VectorSize:  vectorSize,
		Status:      info.Status.String(),
	}, nil
}
```

**Step 5: Update main.go to include new commands**

The init() functions in each file will automatically register commands.

**Step 6: Build and test CLI**

```bash
go build -o bin/code-indexer ./cmd/code-indexer

# Test version
./bin/code-indexer version

# Test status (should show no index)
./bin/code-indexer status

# Test init
./bin/code-indexer init /tmp/test-repo
```

**Step 7: Commit**

```bash
git add cmd/code-indexer/ internal/store/qdrant.go
git commit -m "feat: add CLI commands init, index, and status"
```

---

## Task 8: End-to-End Test

**Files:**
- Create: `test/e2e/index_test.go`

**Step 1: Write E2E test**

```go
// test/e2e/index_test.go
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexEndToEnd(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" {
		t.Skip("VOYAGE_API_KEY not set")
	}
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set")
	}

	// Build CLI
	cmd := exec.Command("go", "build", "-o", "bin/code-indexer", "./cmd/code-indexer")
	cmd.Dir = getProjectRoot()
	require.NoError(t, cmd.Run())

	// Create test repo
	tmpDir := t.TempDir()
	testRepo := filepath.Join(tmpDir, "test-repo")
	require.NoError(t, os.MkdirAll(testRepo, 0755))

	// Add test file
	pyCode := `
def greet(name: str) -> str:
    """Greet someone."""
    return f"Hello, {name}!"

class Greeter:
    """A greeter class."""

    def __init__(self, prefix: str):
        self.prefix = prefix

    def greet(self, name: str) -> str:
        return f"{self.prefix} {name}!"
`
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "greeter.py"), []byte(pyCode), 0644))

	// Initialize repo
	cliPath := filepath.Join(getProjectRoot(), "bin", "code-indexer")

	initCmd := exec.Command(cliPath, "init", testRepo)
	initCmd.Env = os.Environ()
	output, err := initCmd.CombinedOutput()
	require.NoError(t, err, "init failed: %s", output)

	// Index repo
	indexCmd := exec.Command(cliPath, "index", testRepo)
	indexCmd.Env = os.Environ()
	output, err = indexCmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", output)

	// Verify output mentions chunks created
	require.Contains(t, string(output), "Chunks created:")

	// Check status
	statusCmd := exec.Command(cliPath, "status")
	statusCmd.Env = os.Environ()
	output, err = statusCmd.CombinedOutput()
	require.NoError(t, err, "status failed: %s", output)
	require.Contains(t, string(output), "Points:")
}

func getProjectRoot() string {
	// Walk up until we find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
```

**Step 2: Run E2E test**

Run: `VOYAGE_API_KEY=your-key QDRANT_URL=http://localhost:6333 go test ./test/e2e/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add test/
git commit -m "test: add end-to-end test for indexing pipeline"
```

---

## Checkpoint: Phase 1 Complete

At this point you have:

1. **CLI scaffold** with config loading
2. **Tree-sitter parser** for Python and JavaScript
3. **Chunk extractor** with test file detection
4. **Qdrant storage** with search and upsert
5. **Voyage embeddings** with batching
6. **Indexer pipeline** coordinating all components
7. **CLI commands**: init, index, status
8. **E2E test** validating the pipeline

**To verify everything works:**

```bash
# Start infrastructure
cd ~/repos/graphrag && docker compose up -d

# Build
cd ~/repos/code-indexer
go build -o bin/code-indexer ./cmd/code-indexer

# Test on a small directory first
./bin/code-indexer init ~/repos/m32rimm
./bin/code-indexer index m32rimm

# Check status
./bin/code-indexer status
```

---

## Next Phase Preview

Phase 2 will add:
- MCP server with search_code tool
- Claude Code hooks integration
- Redis query caching
- Basic metrics logging

Create a new plan document for Phase 2 when ready.

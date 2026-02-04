package chunk

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/randalmurphy/ai-devtools-admin/internal/parser"
	"github.com/randalmurphy/ai-devtools-admin/internal/security"
)

// Extractor converts parsed symbols into chunks.
type Extractor struct {
	testPatterns        []string
	hierarchical        bool
	hierarchicalChunker *HierarchicalChunker
	secretDetector      *security.SecretDetector
}

// NewExtractor creates a chunk extractor with default test patterns.
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
		hierarchicalChunker: NewHierarchicalChunker(),
		secretDetector:      security.NewSecretDetector(),
	}
}

// SetHierarchicalChunking enables or disables hierarchical chunking for large files.
func (e *Extractor) SetHierarchicalChunking(enabled bool) {
	e.hierarchical = enabled
}

// ExtractResult contains chunks and relationships from extraction.
type ExtractResult struct {
	Chunks        []Chunk
	Relationships []parser.Relationship
}

// Extract parses code and returns chunks.
func (e *Extractor) Extract(source []byte, filePath, repo, modulePath string) ([]Chunk, error) {
	result, err := e.ExtractWithRelationships(source, filePath, repo, modulePath)
	if err != nil {
		return nil, err
	}
	return result.Chunks, nil
}

// ExtractWithRelationships parses code and returns both chunks and relationships.
func (e *Extractor) ExtractWithRelationships(source []byte, filePath, repo, modulePath string) (*ExtractResult, error) {
	lang, ok := parser.DetectLanguage(filePath)
	if !ok {
		return nil, fmt.Errorf("unsupported file type: %s", filePath)
	}

	p, err := parser.NewParser(lang)
	if err != nil {
		return nil, err
	}

	// Use ParseWithRelationships to get both symbols and relationships
	parseResult, err := p.ParseWithRelationships(source, filePath)
	if err != nil {
		return nil, err
	}

	symbols := parseResult.Symbols
	relationships := parseResult.Relationships

	isTest := e.isTestFile(filePath)

	// Use hierarchical chunking if enabled
	if e.hierarchical {
		chunks := e.hierarchicalChunker.ChunkSymbols(symbols, filePath, repo, modulePath, isTest)
		return &ExtractResult{Chunks: chunks, Relationships: relationships}, nil
	}

	// Standard chunking
	moduleRoot, submodule := parseModulePath(modulePath)

	var chunks []Chunk

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

		// Detect and redact secrets
		if e.secretDetector.HasSecrets(chunk.Content) {
			secrets := e.secretDetector.Detect(chunk.Content)
			chunk.Content = e.secretDetector.Redact(chunk.Content, secrets)
			chunk.HasSecrets = true
		}

		chunks = append(chunks, chunk)
	}

	return &ExtractResult{Chunks: chunks, Relationships: relationships}, nil
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
	// Format as UUID (8-4-4-4-12 hex format) using first 16 bytes of hash
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4],
		hash[4:6],
		hash[6:8],
		hash[8:10],
		hash[10:16])
}

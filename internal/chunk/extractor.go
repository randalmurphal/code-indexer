package chunk

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/randalmurphy/ai-devtools-admin/internal/parser"
)

// Extractor converts parsed symbols into chunks.
type Extractor struct {
	testPatterns []string
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
	}
}

// Extract parses code and returns chunks.
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

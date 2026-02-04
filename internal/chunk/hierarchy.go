package chunk

import (
	"fmt"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/parser"
)

const (
	// MaxChunkTokens is the target maximum tokens per chunk.
	MaxChunkTokens = 500
	// LargeClassMethods is the threshold for splitting classes into summaries.
	LargeClassMethods = 50
)

// HierarchicalChunker creates hierarchical chunks for large files.
type HierarchicalChunker struct {
	maxTokens           int
	largeClassThreshold int
}

// NewHierarchicalChunker creates a new chunker.
func NewHierarchicalChunker() *HierarchicalChunker {
	return &HierarchicalChunker{
		maxTokens:           MaxChunkTokens,
		largeClassThreshold: LargeClassMethods,
	}
}

// ChunkSymbols converts symbols to chunks with hierarchy awareness.
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
				chunks = append(chunks, h.createClassChunk(sym, filePath, repo, modulePath, moduleRoot, submodule, weight))
				// Also add individual method chunks
				for _, method := range methods {
					chunk := h.createMethodChunk(method, sym.Name, filePath, repo, modulePath, moduleRoot, submodule, weight)
					chunks = append(chunks, chunk)
				}
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

func (h *HierarchicalChunker) createClassChunk(class parser.Symbol, filePath, repo, modulePath, moduleRoot, submodule string, weight float32) Chunk {
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

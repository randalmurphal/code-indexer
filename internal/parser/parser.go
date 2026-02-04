// Package parser provides tree-sitter based parsing for extracting symbols from source code.
package parser

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Language represents a supported programming language.
type Language string

const (
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
)

// SymbolKind represents the type of code symbol.
type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolClass    SymbolKind = "class"
	SymbolMethod   SymbolKind = "method"
	SymbolVariable SymbolKind = "variable"
)

// Symbol represents a parsed code symbol.
type Symbol struct {
	Name      string     `json:"name"`
	Kind      SymbolKind `json:"kind"`
	FilePath  string     `json:"file_path"`
	StartLine int        `json:"start_line"`
	EndLine   int        `json:"end_line"`
	Content   string     `json:"content"`
	Docstring string     `json:"docstring,omitempty"`
	Parent    string     `json:"parent,omitempty"`
	Signature string     `json:"signature,omitempty"`
}

// Parser wraps tree-sitter for a specific language.
type Parser struct {
	language Language
	parser   *sitter.Parser
	lang     *sitter.Language
}

// NewParser creates a parser for the given language.
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

// Parse parses source code and extracts symbols.
func (p *Parser) Parse(source []byte, filePath string) ([]Symbol, error) {
	tree, err := p.parser.ParseCtx(context.Background(), nil, source)
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

// DetectLanguage determines language from file extension.
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
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

# parser package

Tree-sitter based AST parsing for code symbol extraction.

## Purpose

Extract semantic symbols (functions, classes, methods) from source code using tree-sitter grammars. Provides language-agnostic interface with language-specific extractors.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Language` | Supported language enum | `parser.go:12-17` |
| `SymbolKind` | Symbol type enum | `parser.go:19-26` |
| `Symbol` | Extracted code symbol | `parser.go:28-38` |
| `Parser` | Tree-sitter wrapper | `parser.go:40-45` |
| `RelationshipKind` | Relationship type enum | `relationships.go:11-16` |
| `Relationship` | Code relationship | `relationships.go:19-27` |
| `ParseResult` | Symbols + relationships | `relationships.go:30-33` |

## Usage

```go
// Basic symbol extraction
p, err := parser.NewParser(parser.LanguagePython)
symbols, err := p.Parse(sourceCode, "file.py")

// With relationship extraction
result, err := p.ParseWithRelationships(sourceCode, "file.py")
// result.Symbols - functions, classes, methods
// result.Relationships - imports, calls, extends
```

## Supported Languages

| Language | Extensions | Extractor |
|----------|------------|-----------|
| Python | `.py` | `python.go` |
| JavaScript | `.js`, `.jsx` | `javascript.go` |
| TypeScript | `.ts`, `.tsx` | `javascript.go` (shared) |

## Symbol Fields

| Field | Description |
|-------|-------------|
| `Name` | Symbol identifier |
| `Kind` | function, class, method, variable |
| `FilePath` | Source file |
| `StartLine`, `EndLine` | 1-indexed line numbers |
| `Content` | Full source text |
| `Docstring` | Extracted docstring (Python) |
| `Parent` | Parent class for methods |
| `Signature` | Function signature |

## Python Extraction

- Functions: `function_definition` nodes
- Classes: `class_definition` nodes
- Methods: Functions inside class `block`
- Docstrings: First `string` in function/class body

## JavaScript Extraction

- Functions: `function_declaration` nodes
- Classes: `class_declaration` nodes
- Methods: `method_definition` inside `class_body`
- Arrow functions: Not yet extracted (TODO)

## Relationship Extraction

| Kind | Source | Target | Description |
|------|--------|--------|-------------|
| `imports` | File | Module path | Import/require statements |
| `calls` | Symbol | Symbol name | Function/method calls |
| `extends` | Class | Base class | Class inheritance |

## Gotchas

1. **Line numbers are 1-indexed** (tree-sitter uses 0-indexed rows)
2. **TypeScript uses JS parser** - TS-specific syntax (interfaces, type annotations) not extracted
3. **Nested functions** - Parent field tracks nesting for Python
4. **Cursor management** - Always `defer cursor.Close()` to prevent memory leaks
5. **Relationship targets** - CALLS/EXTENDS targets are names only, resolution happens at graph level

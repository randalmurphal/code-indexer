package parser

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// RelationshipKind represents the type of code relationship.
type RelationshipKind string

const (
	RelationshipImports RelationshipKind = "imports"
	RelationshipCalls   RelationshipKind = "calls"
	RelationshipExtends RelationshipKind = "extends"
)

// Relationship represents a relationship between code elements.
type Relationship struct {
	Kind       RelationshipKind `json:"kind"`
	SourceFile string           `json:"source_file"`
	SourceName string           `json:"source_name,omitempty"` // Symbol name for calls/extends
	SourceLine int              `json:"source_line,omitempty"` // Line where relationship occurs
	TargetPath string           `json:"target_path,omitempty"` // For imports: module path
	TargetName string           `json:"target_name,omitempty"` // For calls/extends: target symbol
}

// ParseResult contains symbols and relationships from parsing.
type ParseResult struct {
	Symbols       []Symbol
	Relationships []Relationship
}

// ParseWithRelationships parses source and extracts both symbols and relationships.
func (p *Parser) ParseWithRelationships(source []byte, filePath string) (*ParseResult, error) {
	tree, err := p.parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var symbols []Symbol
	var relationships []Relationship

	switch p.language {
	case LanguagePython:
		symbols, _ = extractPythonSymbols(tree.RootNode(), source, filePath)
		relationships = extractPythonRelationships(tree.RootNode(), source, filePath)
	case LanguageJavaScript, LanguageTypeScript:
		symbols, _ = extractJavaScriptSymbols(tree.RootNode(), source, filePath)
		relationships = extractJavaScriptRelationships(tree.RootNode(), source, filePath)
	}

	return &ParseResult{
		Symbols:       symbols,
		Relationships: relationships,
	}, nil
}

// extractPythonRelationships extracts imports, calls, and inheritance from Python AST.
func extractPythonRelationships(root *sitter.Node, source []byte, filePath string) []Relationship {
	var rels []Relationship

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractPythonRels(cursor, source, filePath, "", &rels)
	return rels
}

func extractPythonRels(cursor *sitter.TreeCursor, source []byte, filePath, currentFunc string, rels *[]Relationship) {
	node := cursor.CurrentNode()

	switch node.Type() {
	case "import_statement":
		// import foo, bar
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "dotted_name" {
				modulePath := nodeContent(child, source)
				*rels = append(*rels, Relationship{
					Kind:       RelationshipImports,
					SourceFile: filePath,
					SourceLine: int(node.StartPoint().Row) + 1,
					TargetPath: modulePath,
				})
			}
		}

	case "import_from_statement":
		// from foo import bar
		if moduleNode := findChild(node, "dotted_name"); moduleNode != nil {
			modulePath := nodeContent(moduleNode, source)
			*rels = append(*rels, Relationship{
				Kind:       RelationshipImports,
				SourceFile: filePath,
				SourceLine: int(node.StartPoint().Row) + 1,
				TargetPath: modulePath,
			})
		} else if moduleNode := findChild(node, "relative_import"); moduleNode != nil {
			// Handle relative imports like: from . import foo
			modulePath := nodeContent(moduleNode, source)
			*rels = append(*rels, Relationship{
				Kind:       RelationshipImports,
				SourceFile: filePath,
				SourceLine: int(node.StartPoint().Row) + 1,
				TargetPath: modulePath,
			})
		}

	case "class_definition":
		className := ""
		if nameNode := findChild(node, "identifier"); nameNode != nil {
			className = nodeContent(nameNode, source)
		}

		// Check for base classes (extends)
		if argList := findChild(node, "argument_list"); argList != nil {
			for i := 0; i < int(argList.ChildCount()); i++ {
				child := argList.Child(i)
				if child.Type() == "identifier" {
					baseName := nodeContent(child, source)
					*rels = append(*rels, Relationship{
						Kind:       RelationshipExtends,
						SourceFile: filePath,
						SourceName: className,
						SourceLine: int(node.StartPoint().Row) + 1,
						TargetName: baseName,
					})
				} else if child.Type() == "attribute" {
					// Handle qualified base class: module.ClassName
					baseName := nodeContent(child, source)
					*rels = append(*rels, Relationship{
						Kind:       RelationshipExtends,
						SourceFile: filePath,
						SourceName: className,
						SourceLine: int(node.StartPoint().Row) + 1,
						TargetName: baseName,
					})
				}
			}
		}

		// Continue extracting within class body
		if body := findChild(node, "block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractPythonRels(bodyCursor, source, filePath, className, rels)
		}
		return

	case "function_definition":
		funcName := ""
		if nameNode := findChild(node, "identifier"); nameNode != nil {
			funcName = nodeContent(nameNode, source)
		}
		if currentFunc != "" {
			funcName = currentFunc + "." + funcName
		}

		// Extract calls within function body
		if body := findChild(node, "block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractPythonRels(bodyCursor, source, filePath, funcName, rels)
		}
		return

	case "call":
		// Function call
		callTarget := extractCallTarget(node, source)
		if callTarget != "" && currentFunc != "" {
			*rels = append(*rels, Relationship{
				Kind:       RelationshipCalls,
				SourceFile: filePath,
				SourceName: currentFunc,
				SourceLine: int(node.StartPoint().Row) + 1,
				TargetName: callTarget,
			})
		}
	}

	// Recurse into children
	if cursor.GoToFirstChild() {
		extractPythonRels(cursor, source, filePath, currentFunc, rels)
		for cursor.GoToNextSibling() {
			extractPythonRels(cursor, source, filePath, currentFunc, rels)
		}
		cursor.GoToParent()
	}
}

func extractCallTarget(node *sitter.Node, source []byte) string {
	// call node has function as first child
	if node.ChildCount() == 0 {
		return ""
	}

	funcNode := node.Child(0)
	switch funcNode.Type() {
	case "identifier":
		return nodeContent(funcNode, source)
	case "attribute":
		// obj.method() - return the full attribute chain
		return nodeContent(funcNode, source)
	}
	return ""
}

// extractJavaScriptRelationships extracts imports, calls, and inheritance from JS/TS AST.
func extractJavaScriptRelationships(root *sitter.Node, source []byte, filePath string) []Relationship {
	var rels []Relationship

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractJSRels(cursor, source, filePath, "", &rels)
	return rels
}

func extractJSRels(cursor *sitter.TreeCursor, source []byte, filePath, currentFunc string, rels *[]Relationship) {
	node := cursor.CurrentNode()

	switch node.Type() {
	case "import_statement":
		// import X from 'module' or import { X } from 'module'
		if sourceNode := findChildByType(node, "string"); sourceNode != nil {
			modulePath := strings.Trim(nodeContent(sourceNode, source), `"'`)
			*rels = append(*rels, Relationship{
				Kind:       RelationshipImports,
				SourceFile: filePath,
				SourceLine: int(node.StartPoint().Row) + 1,
				TargetPath: modulePath,
			})
		}

	case "call_expression":
		// require('module')
		if funcNode := node.Child(0); funcNode != nil {
			if funcNode.Type() == "identifier" && nodeContent(funcNode, source) == "require" {
				if args := findChildByType(node, "arguments"); args != nil {
					if strArg := findChildByType(args, "string"); strArg != nil {
						modulePath := strings.Trim(nodeContent(strArg, source), `"'`)
						*rels = append(*rels, Relationship{
							Kind:       RelationshipImports,
							SourceFile: filePath,
							SourceLine: int(node.StartPoint().Row) + 1,
							TargetPath: modulePath,
						})
					}
				}
			} else if currentFunc != "" {
				// Regular function call
				callTarget := extractJSCallTarget(funcNode, source)
				if callTarget != "" {
					*rels = append(*rels, Relationship{
						Kind:       RelationshipCalls,
						SourceFile: filePath,
						SourceName: currentFunc,
						SourceLine: int(node.StartPoint().Row) + 1,
						TargetName: callTarget,
					})
				}
			}
		}

	case "class_declaration":
		className := ""
		if nameNode := findChildByType(node, "identifier"); nameNode != nil {
			className = nodeContent(nameNode, source)
		}

		// Check for extends - class_heritage contains "extends" keyword and identifier directly
		if heritage := findChildByType(node, "class_heritage"); heritage != nil {
			for i := 0; i < int(heritage.ChildCount()); i++ {
				child := heritage.Child(i)
				if child.Type() == "identifier" {
					baseName := nodeContent(child, source)
					*rels = append(*rels, Relationship{
						Kind:       RelationshipExtends,
						SourceFile: filePath,
						SourceName: className,
						SourceLine: int(node.StartPoint().Row) + 1,
						TargetName: baseName,
					})
				} else if child.Type() == "member_expression" {
					// Handle qualified names like React.Component
					baseName := nodeContent(child, source)
					*rels = append(*rels, Relationship{
						Kind:       RelationshipExtends,
						SourceFile: filePath,
						SourceName: className,
						SourceLine: int(node.StartPoint().Row) + 1,
						TargetName: baseName,
					})
				}
			}
		}

		// Extract within class body
		if body := findChildByType(node, "class_body"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractJSRels(bodyCursor, source, filePath, className, rels)
		}
		return

	case "function_declaration":
		funcName := ""
		if nameNode := findChildByType(node, "identifier"); nameNode != nil {
			funcName = nodeContent(nameNode, source)
		}
		if currentFunc != "" && funcName != "" {
			funcName = currentFunc + "." + funcName
		}

		// Extract within function body
		if body := findChildByType(node, "statement_block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractJSRels(bodyCursor, source, filePath, funcName, rels)
		}
		return

	case "method_definition":
		methodName := ""
		if nameNode := findChildByType(node, "property_identifier"); nameNode != nil {
			methodName = nodeContent(nameNode, source)
		}
		fullName := methodName
		if currentFunc != "" && methodName != "" {
			fullName = currentFunc + "." + methodName
		}

		// Extract within method body
		if body := findChildByType(node, "statement_block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractJSRels(bodyCursor, source, filePath, fullName, rels)
		}
		return

	case "arrow_function", "function":
		// Anonymous functions - use parent context
		if body := findChildByType(node, "statement_block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractJSRels(bodyCursor, source, filePath, currentFunc, rels)
		}
		return
	}

	// Recurse into children
	if cursor.GoToFirstChild() {
		extractJSRels(cursor, source, filePath, currentFunc, rels)
		for cursor.GoToNextSibling() {
			extractJSRels(cursor, source, filePath, currentFunc, rels)
		}
		cursor.GoToParent()
	}
}

func extractJSCallTarget(node *sitter.Node, source []byte) string {
	switch node.Type() {
	case "identifier":
		return nodeContent(node, source)
	case "member_expression":
		// obj.method - return full expression
		return nodeContent(node, source)
	}
	return ""
}

func findChildByType(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == nodeType {
			return child
		}
	}
	return nil
}

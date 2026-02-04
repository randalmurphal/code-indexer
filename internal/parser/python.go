package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

func getPythonLanguage() *sitter.Language {
	return python.GetLanguage()
}

func extractPythonSymbols(
	root *sitter.Node,
	source []byte,
	filePath string,
) ([]Symbol, error) {
	var symbols []Symbol

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractPythonNode(cursor, source, filePath, "", &symbols)

	return symbols, nil
}

func extractPythonNode(
	cursor *sitter.TreeCursor,
	source []byte,
	filePath, parent string,
	symbols *[]Symbol,
) {
	node := cursor.CurrentNode()

	switch node.Type() {
	case "function_definition":
		sym := extractPythonFunction(node, source, filePath, parent)
		*symbols = append(*symbols, sym)

		// Recurse into function body for nested functions
		if body := findChild(node, "block"); body != nil {
			bodyCursor := sitter.NewTreeCursor(body)
			defer bodyCursor.Close()
			extractPythonNode(bodyCursor, source, filePath, sym.Name, symbols)
		}
		return

	case "class_definition":
		sym := extractPythonClass(node, source, filePath)
		*symbols = append(*symbols, sym)

		// Extract methods within class
		if body := findChild(node, "block"); body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child.Type() == "function_definition" {
					methodSym := extractPythonFunction(
						child, source, filePath, sym.Name,
					)
					methodSym.Kind = SymbolMethod
					*symbols = append(*symbols, methodSym)
				}
			}
		}
		return
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

func extractPythonFunction(
	node *sitter.Node,
	source []byte,
	filePath, parent string,
) Symbol {
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
		StartLine: int(node.StartPoint().Row) + 1,
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
	// Remove triple quotes
	if len(s) >= 6 && (s[:3] == `"""` || s[:3] == `'''`) {
		s = s[3 : len(s)-3]
	} else if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		s = s[1 : len(s)-1]
	}
	return s
}

package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
)

func getJavaScriptLanguage() *sitter.Language {
	return javascript.GetLanguage()
}

func extractJavaScriptSymbols(
	root *sitter.Node,
	source []byte,
	filePath string,
) ([]Symbol, error) {
	var symbols []Symbol

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	extractJavaScriptNode(cursor, source, filePath, "", &symbols)

	return symbols, nil
}

func extractJavaScriptNode(
	cursor *sitter.TreeCursor,
	source []byte,
	filePath, parent string,
	symbols *[]Symbol,
) {
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

func extractJSMethod(
	node *sitter.Node,
	source []byte,
	filePath, parent string,
) Symbol {
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

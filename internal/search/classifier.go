// Package search provides semantic code search functionality.
package search

import (
	"regexp"
	"strings"
)

// QueryType represents the type of search query.
type QueryType string

const (
	QueryTypeSymbol       QueryType = "symbol"
	QueryTypeConcept      QueryType = "concept"
	QueryTypeRelationship QueryType = "relationship"
	QueryTypeFlow         QueryType = "flow"
	QueryTypePattern      QueryType = "pattern"
)

// Classifier determines the type of a search query.
type Classifier struct {
	quotedTermRe      *regexp.Regexp
	identifierRe      *regexp.Regexp
	relationshipWords []string
	flowWords         []string
	patternWords      []string
	patternRegexes    []*regexp.Regexp
}

// NewClassifier creates a new query classifier.
func NewClassifier() *Classifier {
	c := &Classifier{
		quotedTermRe: regexp.MustCompile(`"[^"]+"` + "|`[^`]+`"),
		identifierRe: regexp.MustCompile(
			`\b(get|set|is|has|find|handle|create|delete|update|validate|check|process)[A-Z][a-zA-Z]*\b|` + // camelCase methods
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
			"get to", "gets to",
			"route", "routing",
			"pipeline",
			"chain",
		},
		patternWords: []string{
			"pattern", "patterns",
			"typical", "typically",
			"standard", "convention",
			"structure of",
			"example of",
		},
	}

	// Compile pattern regexes
	c.patternRegexes = []*regexp.Regexp{
		regexp.MustCompile(`how do .* work`),
		regexp.MustCompile(`how does .* work`),
	}

	return c
}

// Classify determines the query type.
func (c *Classifier) Classify(query string) QueryType {
	lower := strings.ToLower(query)

	// Check for quoted terms (explicit symbol lookup) - highest priority
	if c.quotedTermRe.MatchString(query) {
		return QueryTypeSymbol
	}

	// Check pattern regexes first (before relationship words)
	for _, re := range c.patternRegexes {
		if re.MatchString(lower) {
			return QueryTypePattern
		}
	}

	// Check for pattern words
	for _, word := range c.patternWords {
		if strings.Contains(lower, word) {
			return QueryTypePattern
		}
	}

	// Check for relationship words (before identifiers)
	for _, word := range c.relationshipWords {
		if containsWord(lower, word) {
			return QueryTypeRelationship
		}
	}

	// Check for flow words
	for _, word := range c.flowWords {
		if strings.Contains(lower, word) {
			return QueryTypeFlow
		}
	}

	// Check for identifier patterns (camelCase, snake_case, PascalCase)
	// Only if no other type matched
	if c.identifierRe.MatchString(query) {
		return QueryTypeSymbol
	}

	// Default: concept search
	return QueryTypeConcept
}

// containsWord checks if the text contains the word as a separate word.
func containsWord(text, word string) bool {
	// Check for word boundaries
	idx := strings.Index(text, word)
	if idx == -1 {
		return false
	}

	// Check left boundary
	if idx > 0 {
		prev := text[idx-1]
		if prev != ' ' && prev != '\t' && prev != '\n' && prev != ',' && prev != '.' {
			return false
		}
	}

	// Check right boundary
	end := idx + len(word)
	if end < len(text) {
		next := text[end]
		if next != ' ' && next != '\t' && next != '\n' && next != ',' && next != '.' && next != 's' {
			return false
		}
	}

	return true
}

// Route returns the retrieval strategy for a query type.
func (c *Classifier) Route(qt QueryType) RetrievalStrategy {
	switch qt {
	case QueryTypeSymbol:
		return RetrievalStrategy{
			UseSemanticSearch: false,
			UseSymbolIndex:    true,
			UseGraphExpansion: false,
			MaxResults:        10,
		}
	case QueryTypeRelationship:
		return RetrievalStrategy{
			UseSemanticSearch: false,
			UseSymbolIndex:    true,
			UseGraphExpansion: true,
			GraphDepth:        1,
			MaxResults:        20,
		}
	case QueryTypeFlow:
		return RetrievalStrategy{
			UseSemanticSearch: true,
			UseSymbolIndex:    false,
			UseGraphExpansion: true,
			GraphDepth:        3,
			MaxResults:        15,
		}
	case QueryTypePattern:
		return RetrievalStrategy{
			UseSemanticSearch: false,
			UsePatternIndex:   true,
			UseGraphExpansion: false,
			MaxResults:        5,
		}
	default: // Concept
		return RetrievalStrategy{
			UseSemanticSearch: true,
			UseSymbolIndex:    false,
			UseGraphExpansion: true,
			GraphDepth:        1,
			MaxResults:        10,
		}
	}
}

// RetrievalStrategy defines how to execute a search.
type RetrievalStrategy struct {
	UseSemanticSearch bool
	UseSymbolIndex    bool
	UsePatternIndex   bool
	UseGraphExpansion bool
	GraphDepth        int
	MaxResults        int
}

// extractSymbolName extracts a symbol name from a query.
func extractSymbolName(query string) string {
	// Extract quoted term
	re := regexp.MustCompile(`"([^"]+)"`)
	if matches := re.FindStringSubmatch(query); len(matches) > 1 {
		return matches[1]
	}

	// Extract backtick term
	re = regexp.MustCompile("`([^`]+)`")
	if matches := re.FindStringSubmatch(query); len(matches) > 1 {
		return matches[1]
	}

	// Extract identifier pattern - camelCase methods
	re = regexp.MustCompile(`\b(get|set|is|has|find|handle|create|delete|update|validate|check|process)[A-Z][a-zA-Z]*\b`)
	if match := re.FindString(query); match != "" {
		return match
	}

	// Extract PascalCase
	re = regexp.MustCompile(`\b[A-Z][a-z]+([A-Z][a-z]+)+\b`)
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

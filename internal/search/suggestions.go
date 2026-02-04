package search

import (
	"fmt"
	"sort"
	"strings"
)

// SuggestionGenerator creates search suggestions for empty results.
type SuggestionGenerator struct {
	synonyms   map[string][]string
	knownTerms map[string]int // term -> count
}

// Suggestion is a search suggestion.
type Suggestion struct {
	Term   string `json:"term"`
	Count  int    `json:"count,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NewSuggestionGenerator creates a new generator with default synonyms.
func NewSuggestionGenerator() *SuggestionGenerator {
	return &SuggestionGenerator{
		synonyms: map[string][]string{
			"auth":           {"authentication", "login", "session", "token", "credential"},
			"authentication": {"auth", "login", "session", "token"},
			"db":             {"database", "mongo", "sql", "storage", "persistence"},
			"database":       {"db", "mongo", "sql", "storage"},
			"queue":          {"message", "celery", "async", "kafka", "rabbit"},
			"kafka":          {"queue", "message", "celery", "async"},
			"error":          {"exception", "failure", "fault", "issue"},
			"test":           {"spec", "unit", "integration", "mock"},
			"config":         {"configuration", "settings", "options", "env"},
			"http":           {"request", "response", "api", "rest", "endpoint"},
			"api":            {"endpoint", "rest", "http", "route"},
			"user":           {"account", "profile", "member", "person"},
			"file":           {"document", "blob", "storage", "upload"},
			"cache":          {"redis", "memory", "store", "ttl"},
			"log":            {"logging", "logger", "audit", "trace"},
			"timeout":        {"expiry", "ttl", "deadline", "retry"},
		},
		knownTerms: make(map[string]int),
	}
}

// AddKnownTerms adds terms that exist in the index.
func (g *SuggestionGenerator) AddKnownTerms(terms []string) {
	for _, term := range terms {
		g.knownTerms[strings.ToLower(term)]++
	}
}

// GetSynonyms returns synonyms for a term.
func (g *SuggestionGenerator) GetSynonyms(term string) []string {
	return g.synonyms[strings.ToLower(term)]
}

// Generate creates suggestions for a failed query.
func (g *SuggestionGenerator) Generate(query string) []Suggestion {
	words := strings.Fields(strings.ToLower(query))
	suggestions := make(map[string]*Suggestion)

	for _, word := range words {
		// Check synonyms
		for _, syn := range g.synonyms[word] {
			if count, exists := g.knownTerms[syn]; exists {
				if existing, ok := suggestions[syn]; ok {
					existing.Count = count
				} else {
					suggestions[syn] = &Suggestion{
						Term:   syn,
						Count:  count,
						Reason: fmt.Sprintf("synonym for '%s'", word),
					}
				}
			}
		}

		// Check partial matches in known terms
		for term, count := range g.knownTerms {
			if strings.Contains(term, word) || strings.Contains(word, term) {
				if _, ok := suggestions[term]; !ok {
					suggestions[term] = &Suggestion{
						Term:   term,
						Count:  count,
						Reason: "partial match",
					}
				}
			}
		}
	}

	// Convert to slice and sort by count
	result := make([]Suggestion, 0, len(suggestions))
	for _, s := range suggestions {
		result = append(result, *s)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	// Limit to top 5
	if len(result) > 5 {
		result = result[:5]
	}

	return result
}

// FormatEmptyResponse creates a helpful response when search returns nothing.
func (g *SuggestionGenerator) FormatEmptyResponse(query, repo string, suggestions []Suggestion) map[string]interface{} {
	response := map[string]interface{}{
		"results":    []interface{}{},
		"query_type": "concept_search",
		"message":    fmt.Sprintf("No direct matches for '%s'", query),
	}

	if len(suggestions) > 0 {
		suggestionStrs := make([]string, len(suggestions))
		for i, s := range suggestions {
			if s.Count > 0 {
				suggestionStrs[i] = fmt.Sprintf("Try: '%s' (%d results)", s.Term, s.Count)
			} else {
				suggestionStrs[i] = fmt.Sprintf("Try: '%s'", s.Term)
			}
		}
		response["suggestions"] = suggestionStrs
	} else {
		response["suggestions"] = []string{
			"Try broader search terms",
			"Check if the repository is indexed: code-indexer status",
		}
	}

	if repo != "" && repo != "all" {
		response["hint"] = fmt.Sprintf("Searched only in %s. Try repo: 'all' for cross-repo search.", repo)
	}

	return response
}

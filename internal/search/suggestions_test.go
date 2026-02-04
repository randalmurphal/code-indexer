package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSuggestions(t *testing.T) {
	gen := NewSuggestionGenerator()

	// Add some known terms
	gen.AddKnownTerms([]string{
		"auth", "authentication", "login", "session", "token",
		"database", "mongo", "db", "storage",
		"queue", "celery", "async", "message",
	})

	suggestions := gen.Generate("kafka consumer throttling")

	// Should suggest related terms since kafka isn't known
	assert.NotEmpty(t, suggestions)

	// Should find queue/async/message as related to "kafka"
	found := false
	for _, s := range suggestions {
		if s.Term == "queue" || s.Term == "async" || s.Term == "message" {
			found = true
			break
		}
	}
	assert.True(t, found, "should suggest message queue related terms")
}

func TestSynonymLookup(t *testing.T) {
	gen := NewSuggestionGenerator()

	synonyms := gen.GetSynonyms("auth")
	assert.Contains(t, synonyms, "authentication")
	assert.Contains(t, synonyms, "login")

	synonyms = gen.GetSynonyms("db")
	assert.Contains(t, synonyms, "database")
}

func TestGenerateSuggestionsWithPartialMatch(t *testing.T) {
	gen := NewSuggestionGenerator()

	gen.AddKnownTerms([]string{
		"user_service", "user_handler", "user_model",
		"auth_service", "auth_handler",
	})

	suggestions := gen.Generate("find user")

	// Should find partial matches
	found := false
	for _, s := range suggestions {
		if s.Term == "user_service" || s.Term == "user_handler" || s.Term == "user_model" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find partial matches for 'user'")
}

func TestSuggestionGeneratorNoSuggestions(t *testing.T) {
	gen := NewSuggestionGenerator()

	// Empty known terms
	suggestions := gen.Generate("completely unknown term")

	// Should return empty - no matches possible
	assert.Empty(t, suggestions)
}

func TestFormatEmptyResponseWithSuggestions(t *testing.T) {
	gen := NewSuggestionGenerator()
	gen.AddKnownTerms([]string{"auth", "login"})

	suggestions := gen.Generate("authentication")
	response := gen.FormatEmptyResponse("authentication", "my-repo", suggestions)

	assert.Contains(t, response, "results")
	assert.Contains(t, response, "message")
	assert.Contains(t, response, "suggestions")
}

func TestSuggestionLimitedToFive(t *testing.T) {
	gen := NewSuggestionGenerator()

	// Add many terms that match
	gen.AddKnownTerms([]string{
		"user_a", "user_b", "user_c", "user_d", "user_e",
		"user_f", "user_g", "user_h", "user_i", "user_j",
	})

	suggestions := gen.Generate("user")

	// Should be limited to 5
	assert.LessOrEqual(t, len(suggestions), 5)
}

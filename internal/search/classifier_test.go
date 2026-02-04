package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected QueryType
	}{
		// Symbol lookup - quoted terms
		{`"validateToken"`, QueryTypeSymbol},
		{`find "UserService"`, QueryTypeSymbol},
		{"what is `getUserById`", QueryTypeSymbol},

		// Symbol lookup - identifiers
		{`what is getUserById`, QueryTypeSymbol},
		{`find handleAuthError`, QueryTypeSymbol},
		{`where is process_payment`, QueryTypeSymbol},
		{`show me UserService`, QueryTypeSymbol},

		// Relationship queries
		{`what calls validateToken`, QueryTypeRelationship},
		{`functions that use redis`, QueryTypeRelationship},
		{`who imports auth module`, QueryTypeRelationship},
		{`what depends on database`, QueryTypeRelationship},
		{`show references to config`, QueryTypeRelationship},

		// Flow queries
		{`data flow from API to database`, QueryTypeFlow},
		{`how does request get to handler`, QueryTypeFlow},
		{`path from login to session`, QueryTypeFlow},
		{`routing of authentication`, QueryTypeFlow},
		{`pipeline for data processing`, QueryTypeFlow},

		// Pattern queries
		{`how do importers work`, QueryTypePattern},
		{`how does the importer pattern work`, QueryTypePattern},
		{`pattern for error handling`, QueryTypePattern},
		{`typical structure of a test`, QueryTypePattern},
		{`standard convention for models`, QueryTypePattern},

		// Default: concept search
		{`authentication timeout handling`, QueryTypeConcept},
		{`where is user validation`, QueryTypeConcept},
		{`error handling for database`, QueryTypeConcept},
		{`retry logic`, QueryTypeConcept},
		{`caching strategy`, QueryTypeConcept},
	}

	classifier := NewClassifier()

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := classifier.Classify(tt.query)
			assert.Equal(t, tt.expected, got, "query: %s", tt.query)
		})
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{`"validateToken"`, "validateToken"},
		{`find "UserService"`, "UserService"},
		{"what is `getUserById`", "getUserById"},
		{`what is getUserById`, "getUserById"},
		{`find handleAuthError`, "handleAuthError"},
		{`where is process_payment`, "process_payment"},
		{`show me UserService`, "UserService"},
		{`authentication timeout`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := extractSymbolName(tt.query)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRouteStrategy(t *testing.T) {
	classifier := NewClassifier()

	// Symbol queries use symbol index
	strategy := classifier.Route(QueryTypeSymbol)
	assert.True(t, strategy.UseSymbolIndex)
	assert.False(t, strategy.UseSemanticSearch)

	// Concept queries use semantic search
	strategy = classifier.Route(QueryTypeConcept)
	assert.True(t, strategy.UseSemanticSearch)
	assert.False(t, strategy.UseSymbolIndex)

	// Relationship queries use graph expansion
	strategy = classifier.Route(QueryTypeRelationship)
	assert.True(t, strategy.UseGraphExpansion)

	// Pattern queries use pattern index
	strategy = classifier.Route(QueryTypePattern)
	assert.True(t, strategy.UsePatternIndex)
}

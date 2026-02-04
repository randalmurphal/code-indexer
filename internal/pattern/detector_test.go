package pattern

import (
	"testing"

	"github.com/randalmurphy/ai-devtools-admin/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPatterns(t *testing.T) {
	// Simulate symbols from multiple similar files
	symbols := []parser.Symbol{
		// aws_import.py
		{Name: "AWSImporter", Kind: parser.SymbolClass, FilePath: "imports/aws_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/aws_import.py", Parent: "AWSImporter"},

		// azure_import.py - same structure
		{Name: "AzureImporter", Kind: parser.SymbolClass, FilePath: "imports/azure_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/azure_import.py", Parent: "AzureImporter"},

		// gcp_import.py - same structure
		{Name: "GCPImporter", Kind: parser.SymbolClass, FilePath: "imports/gcp_import.py"},
		{Name: "__init__", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "fetch_data", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "transform", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
		{Name: "upsert", Kind: parser.SymbolMethod, FilePath: "imports/gcp_import.py", Parent: "GCPImporter"},
	}

	detector := NewDetector(DetectorConfig{
		MinClusterSize:      3,
		SimilarityThreshold: 0.8,
	})

	patterns := detector.Detect(symbols)

	require.Len(t, patterns, 1)
	assert.Equal(t, "Importer", patterns[0].Name)
	assert.Len(t, patterns[0].Members, 3)
	assert.Contains(t, patterns[0].Methods, "fetch_data")
	assert.Contains(t, patterns[0].Methods, "transform")
	assert.Contains(t, patterns[0].Methods, "upsert")
}

func TestNoPatternDetectedForDifferentStructures(t *testing.T) {
	symbols := []parser.Symbol{
		// Different structures
		{Name: "UserService", Kind: parser.SymbolClass, FilePath: "services/user.py"},
		{Name: "get_user", Kind: parser.SymbolMethod, FilePath: "services/user.py", Parent: "UserService"},

		{Name: "AuthHelper", Kind: parser.SymbolClass, FilePath: "helpers/auth.py"},
		{Name: "validate_token", Kind: parser.SymbolMethod, FilePath: "helpers/auth.py", Parent: "AuthHelper"},
		{Name: "refresh_token", Kind: parser.SymbolMethod, FilePath: "helpers/auth.py", Parent: "AuthHelper"},

		{Name: "ConfigLoader", Kind: parser.SymbolClass, FilePath: "utils/config.py"},
		{Name: "load", Kind: parser.SymbolMethod, FilePath: "utils/config.py", Parent: "ConfigLoader"},
		{Name: "save", Kind: parser.SymbolMethod, FilePath: "utils/config.py", Parent: "ConfigLoader"},
		{Name: "validate", Kind: parser.SymbolMethod, FilePath: "utils/config.py", Parent: "ConfigLoader"},
	}

	detector := NewDetector(DetectorConfig{
		MinClusterSize:      3,
		SimilarityThreshold: 0.8,
	})

	patterns := detector.Detect(symbols)
	assert.Len(t, patterns, 0, "should not detect patterns for dissimilar structures")
}

func TestPatternDetectionWithDefaultConfig(t *testing.T) {
	// With default config (MinClusterSize=5), need 5 similar files
	symbols := []parser.Symbol{}

	// Create 5 similar handlers
	for i, name := range []string{"User", "Product", "Order", "Payment", "Shipping"} {
		filePath := "handlers/" + name + "_handler.py"
		symbols = append(symbols,
			parser.Symbol{Name: name + "Handler", Kind: parser.SymbolClass, FilePath: filePath},
			parser.Symbol{Name: "get", Kind: parser.SymbolMethod, FilePath: filePath, Parent: name + "Handler"},
			parser.Symbol{Name: "list", Kind: parser.SymbolMethod, FilePath: filePath, Parent: name + "Handler"},
			parser.Symbol{Name: "create", Kind: parser.SymbolMethod, FilePath: filePath, Parent: name + "Handler"},
			parser.Symbol{Name: "update", Kind: parser.SymbolMethod, FilePath: filePath, Parent: name + "Handler"},
			parser.Symbol{Name: "delete", Kind: parser.SymbolMethod, FilePath: filePath, Parent: name + "Handler"},
		)
		_ = i // silence unused variable warning
	}

	detector := NewDetector(DetectorConfig{}) // Use defaults

	patterns := detector.Detect(symbols)
	require.Len(t, patterns, 1)
	assert.Equal(t, "Handler", patterns[0].Name)
	assert.Len(t, patterns[0].Members, 5)
}

func TestSimilarityComputation(t *testing.T) {
	detector := NewDetector(DetectorConfig{})

	// Identical signatures should have similarity 1.0
	sigA := FileSignature{
		HasClass:  true,
		ClassName: "FooImporter",
		Methods:   []string{"fetch", "transform", "upsert"},
		HasInit:   true,
	}
	sigB := FileSignature{
		HasClass:  true,
		ClassName: "BarImporter",
		Methods:   []string{"fetch", "transform", "upsert"},
		HasInit:   true,
	}

	similarity := detector.computeSimilarity(sigA, sigB)
	assert.Equal(t, 1.0, similarity)

	// No overlap should have similarity 0
	sigC := FileSignature{
		HasClass:  true,
		ClassName: "Other",
		Methods:   []string{"foo", "bar", "baz"},
		HasInit:   true,
	}

	similarity = detector.computeSimilarity(sigA, sigC)
	assert.Equal(t, 0.0, similarity)
}

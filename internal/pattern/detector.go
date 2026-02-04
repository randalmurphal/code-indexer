// Package pattern provides pattern detection for similar code structures.
package pattern

import (
	"sort"
	"strings"

	"github.com/randalmurphal/code-indexer/internal/parser"
)

// Pattern represents a detected code pattern.
type Pattern struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Methods       []string `json:"methods"`
	Members       []string `json:"members"`        // File paths following this pattern
	CanonicalFile string   `json:"canonical_file"` // Best example
}

// DetectorConfig configures pattern detection.
type DetectorConfig struct {
	MinClusterSize      int
	SimilarityThreshold float64
}

// Detector identifies patterns in code.
type Detector struct {
	config DetectorConfig
}

// NewDetector creates a new pattern detector.
func NewDetector(config DetectorConfig) *Detector {
	if config.MinClusterSize == 0 {
		config.MinClusterSize = 5
	}
	if config.SimilarityThreshold == 0 {
		config.SimilarityThreshold = 0.8
	}
	return &Detector{config: config}
}

// Detect finds patterns in a set of symbols.
func (d *Detector) Detect(symbols []parser.Symbol) []Pattern {
	// Group symbols by file
	fileSymbols := make(map[string][]parser.Symbol)
	for _, sym := range symbols {
		fileSymbols[sym.FilePath] = append(fileSymbols[sym.FilePath], sym)
	}

	// Extract structural signatures for each file
	signatures := make(map[string]FileSignature)
	for file, syms := range fileSymbols {
		signatures[file] = extractSignature(syms)
	}

	// Cluster files by signature similarity
	clusters := d.clusterBySignature(signatures)

	// Convert clusters to patterns
	var patterns []Pattern
	for _, cluster := range clusters {
		if len(cluster) >= d.config.MinClusterSize {
			pattern := d.clusterToPattern(cluster, signatures)
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// FileSignature represents the structural shape of a file.
type FileSignature struct {
	HasClass   bool
	ClassName  string
	Methods    []string
	HasInit    bool
	Decorators []string
}

func extractSignature(symbols []parser.Symbol) FileSignature {
	sig := FileSignature{}

	for _, sym := range symbols {
		switch sym.Kind {
		case parser.SymbolClass:
			sig.HasClass = true
			sig.ClassName = sym.Name
		case parser.SymbolMethod:
			sig.Methods = append(sig.Methods, sym.Name)
			if sym.Name == "__init__" || sym.Name == "constructor" {
				sig.HasInit = true
			}
		}
	}

	sort.Strings(sig.Methods)
	return sig
}

func (d *Detector) clusterBySignature(signatures map[string]FileSignature) [][]string {
	files := make([]string, 0, len(signatures))
	for file := range signatures {
		files = append(files, file)
	}

	// Sort for deterministic ordering
	sort.Strings(files)

	// Simple clustering: group files with matching method sets
	visited := make(map[string]bool)
	var clusters [][]string

	for _, file := range files {
		if visited[file] {
			continue
		}

		cluster := []string{file}
		visited[file] = true

		for _, other := range files {
			if visited[other] {
				continue
			}

			similarity := d.computeSimilarity(signatures[file], signatures[other])
			if similarity >= d.config.SimilarityThreshold {
				cluster = append(cluster, other)
				visited[other] = true
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

func (d *Detector) computeSimilarity(a, b FileSignature) float64 {
	if !a.HasClass || !b.HasClass {
		return 0
	}

	// Jaccard similarity of method sets
	setA := make(map[string]bool)
	for _, m := range a.Methods {
		setA[m] = true
	}

	setB := make(map[string]bool)
	for _, m := range b.Methods {
		setB[m] = true
	}

	intersection := 0
	for m := range setA {
		if setB[m] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func (d *Detector) clusterToPattern(files []string, signatures map[string]FileSignature) Pattern {
	// Find common methods across all files
	methodCounts := make(map[string]int)
	for _, file := range files {
		for _, method := range signatures[file].Methods {
			methodCounts[method]++
		}
	}

	var commonMethods []string
	threshold := len(files) * 8 / 10 // 80%
	for method, count := range methodCounts {
		if count >= threshold {
			commonMethods = append(commonMethods, method)
		}
	}
	sort.Strings(commonMethods)

	// Infer pattern name from class names
	patternName := inferPatternName(files, signatures)

	// Pick canonical file (first alphabetically)
	sort.Strings(files)
	canonical := files[0]

	return Pattern{
		Name:          patternName,
		Description:   generatePatternDescription(patternName, commonMethods),
		Methods:       commonMethods,
		Members:       files,
		CanonicalFile: canonical,
	}
}

func inferPatternName(files []string, signatures map[string]FileSignature) string {
	// Find common suffix in class names
	var classNames []string
	for _, file := range files {
		if sig, ok := signatures[file]; ok && sig.ClassName != "" {
			classNames = append(classNames, sig.ClassName)
		}
	}

	if len(classNames) == 0 {
		return "Unknown"
	}

	// Find longest common suffix (e.g., "Importer" from AWSImporter, AzureImporter)
	first := classNames[0]
	longestSuffix := ""

	for i := 1; i <= len(first); i++ {
		suffix := first[len(first)-i:]
		allMatch := true
		for _, name := range classNames[1:] {
			if !strings.HasSuffix(name, suffix) {
				allMatch = false
				break
			}
		}
		if allMatch {
			longestSuffix = suffix
		}
	}

	// Return if at least 4 chars
	if len(longestSuffix) >= 4 {
		return longestSuffix
	}

	return "Pattern"
}

func generatePatternDescription(name string, methods []string) string {
	return "Classes following the " + name + " pattern implement: " + strings.Join(methods, ", ")
}

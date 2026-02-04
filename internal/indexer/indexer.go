package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/docs"
	"github.com/randalmurphy/ai-devtools-admin/internal/embedding"
	"github.com/randalmurphy/ai-devtools-admin/internal/graph"
	"github.com/randalmurphy/ai-devtools-admin/internal/parser"
	"github.com/randalmurphy/ai-devtools-admin/internal/pattern"
	"github.com/randalmurphy/ai-devtools-admin/internal/store"
)

// Indexer coordinates the indexing pipeline: file discovery, parsing,
// embedding generation, and storage.
type Indexer struct {
	config          *config.Config
	extractor       *chunk.Extractor
	embedder        *embedding.VoyageClient
	store           *store.QdrantStore
	patternDetector *pattern.Detector
	moduleResolver  *ModuleResolver // Initialized per-repo during Index
	logger          *slog.Logger
}

// NewIndexer creates a new indexer with the given configuration.
func NewIndexer(cfg *config.Config, voyageKey string) (*Indexer, error) {
	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)

	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	patternDetector := pattern.NewDetector(pattern.DetectorConfig{
		MinClusterSize:      5,
		SimilarityThreshold: 0.8,
	})

	// Create extractor with hierarchical chunking enabled
	extractor := chunk.NewExtractor()
	extractor.SetHierarchicalChunking(true)

	return &Indexer{
		config:          cfg,
		extractor:       extractor,
		embedder:        embedder,
		store:           qdrantStore,
		patternDetector: patternDetector,
		logger:          slog.Default(),
	}, nil
}

// IndexResult contains statistics from an indexing run.
type IndexResult struct {
	FilesProcessed int
	FilesSkipped   int // For incremental: files unchanged
	ChunksCreated  int
	Errors         []error
}

// IndexOptions configures the indexing behavior.
type IndexOptions struct {
	Incremental bool              // Only index changed files
	GraphStore  *graph.Neo4jStore // For incremental: store/retrieve file hashes
}

// Index processes a repository, extracting code chunks, generating embeddings,
// and storing them in the vector database.
func (idx *Indexer) Index(ctx context.Context, repoPath string, repoCfg *config.RepoConfig) (*IndexResult, error) {
	return idx.IndexWithOptions(ctx, repoPath, repoCfg, IndexOptions{})
}

// IndexWithOptions processes a repository with configurable options.
func (idx *Indexer) IndexWithOptions(ctx context.Context, repoPath string, repoCfg *config.RepoConfig, opts IndexOptions) (*IndexResult, error) {
	result := &IndexResult{}

	// Initialize module resolver for this repo
	idx.moduleResolver = NewModuleResolver(repoPath, repoCfg)

	// Ensure collection exists
	collectionName := "chunks"
	if err := idx.store.EnsureCollection(ctx, collectionName, idx.embedder.Dimension()); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	// Get existing file hashes for incremental indexing
	var existingHashes map[string]string
	if opts.Incremental && opts.GraphStore != nil {
		var err error
		existingHashes, err = opts.GraphStore.GetAllFileHashes(ctx, repoCfg.Name)
		if err != nil {
			idx.logger.Warn("failed to get existing hashes, falling back to full index", "error", err)
			existingHashes = nil
		}
	}

	// Walk files and extract chunks, collecting symbols for pattern detection
	walker := NewWalker(repoCfg.Include, repoCfg.Exclude)
	var allChunks []chunk.Chunk
	var allSymbols []parser.Symbol
	var allRelationships []parser.Relationship

	// Track files to update in graph store
	var filesToUpdate []graph.File

	err := walker.Walk(repoPath, func(path string) error {
		source, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read %s: %w", path, err))
			return nil // Continue with other files
		}

		relPath, _ := filepath.Rel(repoPath, path)

		// Check if file has changed (incremental mode)
		currentHash := computeFileHash(source)
		if opts.Incremental && existingHashes != nil {
			if oldHash, exists := existingHashes[relPath]; exists && oldHash == currentHash {
				// File unchanged, skip indexing
				idx.logger.Debug("skipping unchanged file", "path", relPath)
				result.FilesSkipped++
				return nil
			}
		}

		idx.logger.Info("processing file", "path", relPath)

		modulePath, moduleRoot, _ := idx.moduleResolver.Resolve(relPath)

		extractResult, err := idx.extractor.ExtractWithRelationships(source, relPath, repoCfg.Name, modulePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("extract %s: %w", path, err))
			return nil
		}

		// Collect symbols for pattern detection
		symbols := idx.extractSymbols(source, relPath)
		allSymbols = append(allSymbols, symbols...)

		allChunks = append(allChunks, extractResult.Chunks...)
		allRelationships = append(allRelationships, extractResult.Relationships...)
		result.FilesProcessed++

		// Track file for graph update
		if opts.GraphStore != nil {
			filesToUpdate = append(filesToUpdate, graph.File{
				Path:        relPath,
				Repo:        repoCfg.Name,
				ModuleRoot:  moduleRoot,
				Hash:        currentHash,
				LastIndexed: time.Now(),
			})
		}

		return nil
	})

	if err != nil {
		return result, fmt.Errorf("walk failed: %w", err)
	}

	if len(allChunks) == 0 {
		return result, nil
	}

	// Detect patterns and mark chunks
	idx.logger.Info("detecting patterns", "symbols", len(allSymbols))
	patterns := idx.patternDetector.Detect(allSymbols)
	idx.logger.Info("patterns detected", "count", len(patterns))

	// Build file->pattern mapping
	filePatterns := make(map[string]string)
	for _, p := range patterns {
		for _, member := range p.Members {
			filePatterns[member] = p.Name
		}
	}

	// Mark chunks with their pattern
	for i := range allChunks {
		if patternName, ok := filePatterns[allChunks[i].FilePath]; ok {
			allChunks[i].FollowsPattern = patternName
		}
	}

	// Create pattern chunks
	patternChunks := idx.createPatternChunks(patterns, repoCfg.Name)
	allChunks = append(allChunks, patternChunks...)

	// Index AGENTS.md and CLAUDE.md files for navigation
	docChunks := idx.indexNavigationDocs(repoPath, repoCfg.Name)
	idx.logger.Info("navigation docs indexed", "chunks", len(docChunks))
	allChunks = append(allChunks, docChunks...)

	// Generate embeddings
	idx.logger.Info("generating embeddings", "chunks", len(allChunks))

	texts := make([]string, len(allChunks))
	for i, c := range allChunks {
		texts[i] = buildEmbeddingText(c)
	}

	vectors, err := idx.embedder.EmbedBatched(ctx, texts, 64)
	if err != nil {
		return result, fmt.Errorf("embedding failed: %w", err)
	}

	for i := range allChunks {
		allChunks[i].Vector = vectors[i]
	}

	// Store in Qdrant with batched upserts
	idx.logger.Info("storing chunks", "count", len(allChunks))

	batchSize := 100
	for i := 0; i < len(allChunks); i += batchSize {
		end := i + batchSize
		if end > len(allChunks) {
			end = len(allChunks)
		}

		if err := idx.store.UpsertChunks(ctx, collectionName, allChunks[i:end]); err != nil {
			return result, fmt.Errorf("upsert failed: %w", err)
		}
	}

	result.ChunksCreated = len(allChunks)

	// Update graph store with file hashes (for incremental indexing)
	if opts.GraphStore != nil && len(filesToUpdate) > 0 {
		idx.logger.Info("updating file hashes in graph", "files", len(filesToUpdate))
		for _, file := range filesToUpdate {
			if err := opts.GraphStore.UpsertFile(ctx, file); err != nil {
				idx.logger.Warn("failed to update file hash", "path", file.Path, "error", err)
			}
		}
	}

	// Store relationships in graph database
	if opts.GraphStore != nil && len(allRelationships) > 0 {
		idx.logger.Info("storing relationships in graph", "count", len(allRelationships))
		idx.storeRelationships(ctx, opts.GraphStore, repoCfg.Name, allRelationships, allSymbols)
	}

	return result, nil
}

// buildEmbeddingText combines chunk content with context for better embeddings.
func buildEmbeddingText(c chunk.Chunk) string {
	var parts []string

	if c.ContextHeader != "" {
		parts = append(parts, c.ContextHeader)
	}
	if c.Docstring != "" {
		parts = append(parts, c.Docstring)
	}
	parts = append(parts, c.Content)

	return strings.Join(parts, "\n\n")
}

// inferModulePath converts a file path to a module path.
// e.g., "fisio/fisio/imports/aws.py" -> "fisio.imports"
func inferModulePath(relPath string, cfg *config.RepoConfig) string {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return "."
	}

	// Normalize path separators
	dir = filepath.ToSlash(dir)
	parts := strings.Split(dir, "/")

	// Remove duplicated prefix (e.g., fisio/fisio -> fisio)
	if len(parts) >= 2 && parts[0] == parts[1] {
		parts = parts[1:]
	}

	return strings.Join(parts, ".")
}

// extractSymbols parses a file and returns symbols for pattern detection.
func (idx *Indexer) extractSymbols(source []byte, filePath string) []parser.Symbol {
	lang, ok := parser.DetectLanguage(filePath)
	if !ok {
		return nil
	}

	p, err := parser.NewParser(lang)
	if err != nil {
		return nil
	}

	symbols, err := p.Parse(source, filePath)
	if err != nil {
		return nil
	}

	return symbols
}

// createPatternChunks creates indexable chunks for detected patterns.
func (idx *Indexer) createPatternChunks(patterns []pattern.Pattern, repo string) []chunk.Chunk {
	var chunks []chunk.Chunk

	for _, p := range patterns {
		// Create a pattern description chunk
		content := fmt.Sprintf("# %s Pattern\n\n%s\n\n## Example Files\n", p.Name, p.Description)
		for _, member := range p.Members {
			content += fmt.Sprintf("- %s\n", member)
		}
		content += fmt.Sprintf("\n## Canonical Example\n%s\n", p.CanonicalFile)

		c := chunk.Chunk{
			ID:              chunk.GenerateID(repo, "patterns", p.Name, 0),
			Repo:            repo,
			FilePath:        p.CanonicalFile,
			Type:            chunk.ChunkTypeDoc,
			Kind:            "pattern",
			SymbolName:      p.Name,
			Content:         content,
			RetrievalWeight: 1.5, // Boost pattern chunks
		}

		chunks = append(chunks, c)
	}

	return chunks
}

// indexNavigationDocs finds and indexes AGENTS.md and CLAUDE.md files.
func (idx *Indexer) indexNavigationDocs(repoPath, repo string) []chunk.Chunk {
	var allChunks []chunk.Chunk

	// Files to look for
	navFiles := []string{"AGENTS.md", "CLAUDE.md"}

	// Walk directory tree looking for navigation docs
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden and common excluded directories
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "venv" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this is a navigation doc
		fileName := d.Name()
		isNavDoc := false
		for _, navFile := range navFiles {
			if fileName == navFile {
				isNavDoc = true
				break
			}
		}

		if !isNavDoc {
			return nil
		}

		// Read and parse the file
		content, err := os.ReadFile(path)
		if err != nil {
			idx.logger.Warn("failed to read nav doc", "path", path, "error", err)
			return nil
		}

		relPath, _ := filepath.Rel(repoPath, path)
		idx.logger.Info("indexing navigation doc", "path", relPath)

		doc, err := docs.ParseAgentsMD(content, relPath, repo)
		if err != nil {
			idx.logger.Warn("failed to parse nav doc", "path", path, "error", err)
			return nil
		}

		chunks := doc.ToChunks()
		allChunks = append(allChunks, chunks...)

		return nil
	})

	if err != nil {
		idx.logger.Warn("error walking for nav docs", "error", err)
	}

	return allChunks
}

// computeFileHash returns a SHA-256 hash of the file content.
func computeFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// storeRelationships stores extracted relationships in Neo4j.
func (idx *Indexer) storeRelationships(ctx context.Context, graphStore *graph.Neo4jStore, repo string, relationships []parser.Relationship, symbols []parser.Symbol) {
	// Build symbol lookup map for resolving relationships
	symbolMap := make(map[string]parser.Symbol)
	for _, sym := range symbols {
		// Key by name for lookup
		symbolMap[sym.Name] = sym
	}

	for _, rel := range relationships {
		var err error

		switch rel.Kind {
		case parser.RelationshipImports:
			// For imports, we need to resolve the module path to a file path
			// This is a best-effort operation since the target may be external
			err = graphStore.CreateImportRelationship(ctx, repo, rel.SourceFile, rel.TargetPath)

		case parser.RelationshipCalls:
			// Create CALLS relationship between symbols
			if targetSym, exists := symbolMap[rel.TargetName]; exists {
				caller := graph.Symbol{
					Name:      rel.SourceName,
					FilePath:  rel.SourceFile,
					Repo:      repo,
					StartLine: rel.SourceLine,
				}
				callee := graph.Symbol{
					Name:      targetSym.Name,
					FilePath:  targetSym.FilePath,
					Repo:      repo,
					StartLine: targetSym.StartLine,
				}
				err = graphStore.CreateCallRelationship(ctx, repo, caller, callee)
			}

		case parser.RelationshipExtends:
			// Create EXTENDS relationship between symbols
			if targetSym, exists := symbolMap[rel.TargetName]; exists {
				child := graph.Symbol{
					Name:      rel.SourceName,
					FilePath:  rel.SourceFile,
					Repo:      repo,
					StartLine: rel.SourceLine,
				}
				parent := graph.Symbol{
					Name:      targetSym.Name,
					FilePath:  targetSym.FilePath,
					Repo:      repo,
					StartLine: targetSym.StartLine,
				}
				err = graphStore.CreateExtendsRelationship(ctx, repo, child, parent)
			}
		}

		if err != nil {
			idx.logger.Debug("failed to store relationship", "kind", rel.Kind, "source", rel.SourceFile, "error", err)
		}
	}
}

package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/embedding"
	"github.com/randalmurphy/ai-devtools-admin/internal/store"
)

// Indexer coordinates the indexing pipeline: file discovery, parsing,
// embedding generation, and storage.
type Indexer struct {
	config    *config.Config
	extractor *chunk.Extractor
	embedder  *embedding.VoyageClient
	store     *store.QdrantStore
	logger    *slog.Logger
}

// NewIndexer creates a new indexer with the given configuration.
func NewIndexer(cfg *config.Config, voyageKey string) (*Indexer, error) {
	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)

	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	return &Indexer{
		config:    cfg,
		extractor: chunk.NewExtractor(),
		embedder:  embedder,
		store:     qdrantStore,
		logger:    slog.Default(),
	}, nil
}

// IndexResult contains statistics from an indexing run.
type IndexResult struct {
	FilesProcessed int
	ChunksCreated  int
	Errors         []error
}

// Index processes a repository, extracting code chunks, generating embeddings,
// and storing them in the vector database.
func (idx *Indexer) Index(ctx context.Context, repoPath string, repoCfg *config.RepoConfig) (*IndexResult, error) {
	result := &IndexResult{}

	// Ensure collection exists
	collectionName := "chunks"
	if err := idx.store.EnsureCollection(ctx, collectionName, idx.embedder.Dimension()); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	// Walk files and extract chunks
	walker := NewWalker(repoCfg.Include, repoCfg.Exclude)
	var allChunks []chunk.Chunk

	err := walker.Walk(repoPath, func(path string) error {
		idx.logger.Info("processing file", "path", path)

		source, err := os.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read %s: %w", path, err))
			return nil // Continue with other files
		}

		relPath, _ := filepath.Rel(repoPath, path)
		modulePath := inferModulePath(relPath, repoCfg)

		chunks, err := idx.extractor.Extract(source, relPath, repoCfg.Name, modulePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("extract %s: %w", path, err))
			return nil
		}

		allChunks = append(allChunks, chunks...)
		result.FilesProcessed++

		return nil
	})

	if err != nil {
		return result, fmt.Errorf("walk failed: %w", err)
	}

	if len(allChunks) == 0 {
		return result, nil
	}

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

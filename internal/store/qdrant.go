// Package store provides vector storage backends for code chunks.
package store

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
)

// QdrantStore handles vector storage in Qdrant.
type QdrantStore struct {
	client *qdrant.Client
}

// NewQdrantStore creates a new Qdrant store.
func NewQdrantStore(url string) (*QdrantStore, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}

	return &QdrantStore{client: client}, nil
}

// Close closes the Qdrant connection.
func (s *QdrantStore) Close() error {
	return s.client.Close()
}

// EnsureCollection creates collection if it doesn't exist.
func (s *QdrantStore) EnsureCollection(ctx context.Context, name string, vectorSize int) error {
	exists, err := s.client.CollectionExists(ctx, name)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

// DeleteCollection removes a collection.
func (s *QdrantStore) DeleteCollection(ctx context.Context, name string) error {
	return s.client.DeleteCollection(ctx, name)
}

// UpsertChunks inserts or updates chunks.
func (s *QdrantStore) UpsertChunks(ctx context.Context, collection string, chunks []chunk.Chunk) error {
	points := make([]*qdrant.PointStruct, len(chunks))

	for i, c := range chunks {
		payload := map[string]interface{}{
			"repo":             c.Repo,
			"file_path":        c.FilePath,
			"start_line":       c.StartLine,
			"end_line":         c.EndLine,
			"type":             string(c.Type),
			"kind":             c.Kind,
			"module_path":      c.ModulePath,
			"module_root":      c.ModuleRoot,
			"submodule":        c.Submodule,
			"symbol_name":      c.SymbolName,
			"heading_path":     c.HeadingPath,
			"content":          c.Content,
			"context_header":   c.ContextHeader,
			"signature":        c.Signature,
			"docstring":        c.Docstring,
			"is_test":          c.IsTest,
			"retrieval_weight": c.RetrievalWeight,
			"has_secrets":      c.HasSecrets,
			"follows_pattern":  c.FollowsPattern,
		}

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewID(c.ID),
			Vectors: qdrant.NewVectors(c.Vector...),
			Payload: qdrant.NewValueMap(payload),
		}
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         points,
	})

	return err
}

// Search performs vector similarity search.
func (s *QdrantStore) Search(ctx context.Context, collection string, vector []float32, limit int, filter map[string]interface{}) ([]chunk.Chunk, error) {
	var qdrantFilter *qdrant.Filter
	if filter != nil {
		qdrantFilter = buildFilter(filter)
	}

	results, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(vector...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		Filter:         qdrantFilter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	chunks := make([]chunk.Chunk, len(results))
	for i, r := range results {
		chunks[i] = payloadToChunk(r.Id.GetUuid(), r.Payload)
		chunks[i].Vector = nil // Don't return vectors in results
		chunks[i].Score = r.Score
	}

	return chunks, nil
}

// SearchByFilter searches using payload filters without vector similarity.
func (s *QdrantStore) SearchByFilter(ctx context.Context, collection string, filter map[string]interface{}, limit int) ([]chunk.Chunk, error) {
	qdrantFilter := buildFilter(filter)

	results, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Filter:         qdrantFilter,
		Limit:          qdrant.PtrOf(uint32(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	chunks := make([]chunk.Chunk, len(results))
	for i, r := range results {
		chunks[i] = payloadToChunk(r.Id.GetUuid(), r.Payload)
	}

	return chunks, nil
}

// CollectionInfo contains collection metadata.
type CollectionInfo struct {
	PointsCount int64
	VectorSize  int
	Status      string
}

// CollectionInfo gets collection metadata.
func (s *QdrantStore) CollectionInfo(ctx context.Context, name string) (*CollectionInfo, error) {
	info, err := s.client.GetCollectionInfo(ctx, name)
	if err != nil {
		return nil, err
	}

	vectorSize := 0
	if params := info.Config.GetParams(); params != nil {
		if vecConfig := params.GetVectorsConfig(); vecConfig != nil {
			if vecParams := vecConfig.GetParams(); vecParams != nil {
				vectorSize = int(vecParams.GetSize())
			}
		}
	}

	pointsCount := int64(0)
	if info.PointsCount != nil {
		pointsCount = int64(*info.PointsCount)
	}

	return &CollectionInfo{
		PointsCount: pointsCount,
		VectorSize:  vectorSize,
		Status:      info.Status.String(),
	}, nil
}

func buildFilter(filter map[string]interface{}) *qdrant.Filter {
	var must []*qdrant.Condition

	for key, value := range filter {
		switch v := value.(type) {
		case string:
			must = append(must, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: key,
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Keyword{Keyword: v},
						},
					},
				},
			})
		case bool:
			must = append(must, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: key,
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Boolean{Boolean: v},
						},
					},
				},
			})
		}
	}

	return &qdrant.Filter{Must: must}
}

func payloadToChunk(id string, payload map[string]*qdrant.Value) chunk.Chunk {
	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			return v.GetStringValue()
		}
		return ""
	}
	getInt := func(key string) int {
		if v, ok := payload[key]; ok {
			return int(v.GetIntegerValue())
		}
		return 0
	}
	getBool := func(key string) bool {
		if v, ok := payload[key]; ok {
			return v.GetBoolValue()
		}
		return false
	}
	getFloat := func(key string) float32 {
		if v, ok := payload[key]; ok {
			return float32(v.GetDoubleValue())
		}
		return 0
	}

	return chunk.Chunk{
		ID:              id,
		Repo:            getString("repo"),
		FilePath:        getString("file_path"),
		StartLine:       getInt("start_line"),
		EndLine:         getInt("end_line"),
		Type:            chunk.ChunkType(getString("type")),
		Kind:            getString("kind"),
		ModulePath:      getString("module_path"),
		ModuleRoot:      getString("module_root"),
		Submodule:       getString("submodule"),
		SymbolName:      getString("symbol_name"),
		HeadingPath:     getString("heading_path"),
		Content:         getString("content"),
		ContextHeader:   getString("context_header"),
		Signature:       getString("signature"),
		Docstring:       getString("docstring"),
		IsTest:          getBool("is_test"),
		RetrievalWeight: getFloat("retrieval_weight"),
		HasSecrets:      getBool("has_secrets"),
		FollowsPattern:  getString("follows_pattern"),
	}
}

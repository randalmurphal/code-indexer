// Package embedding provides embedding clients for generating vector representations.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const voyageAPIURL = "https://api.voyageai.com/v1/embeddings"

// VoyageClient handles embeddings via Voyage AI API.
type VoyageClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewVoyageClient creates a new Voyage embedding client.
func NewVoyageClient(apiKey, model string) *VoyageClient {
	return &VoyageClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

type voyageResponse struct {
	Data  []voyageEmbedding `json:"data"`
	Usage voyageUsage       `json:"usage"`
}

type voyageEmbedding struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type voyageUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// Embed generates embeddings for the given texts.
func (c *VoyageClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := voyageRequest{
		Input:     texts,
		Model:     c.model,
		InputType: "document",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", voyageAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var voyageResp voyageResponse
	if err := json.Unmarshal(body, &voyageResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Sort by index to ensure order matches input
	vectors := make([][]float32, len(texts))
	for _, emb := range voyageResp.Data {
		vectors[emb.Index] = emb.Embedding
	}

	return vectors, nil
}

// EmbedBatched handles large inputs by batching.
func (c *VoyageClient) EmbedBatched(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 128 // Voyage default max
	}

	var allVectors [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		vectors, err := c.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d failed: %w", i, end, err)
		}

		allVectors = append(allVectors, vectors...)
	}

	return allVectors, nil
}

// Dimension returns the vector dimension for the model.
func (c *VoyageClient) Dimension() int {
	switch c.model {
	case "voyage-4-large", "voyage-3-large", "voyage-code-3":
		return 1024
	case "voyage-4", "voyage-3":
		return 1024
	case "voyage-4-lite", "voyage-3-lite":
		return 512
	default:
		return 1024
	}
}

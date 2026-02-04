package search

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Cursor represents pagination state.
type Cursor struct {
	QueryHash string    `json:"q"`
	Offset    int       `json:"o"`
	CreatedAt time.Time `json:"t"`
}

// EncodeCursor creates an opaque cursor string.
func EncodeCursor(queryHash string, offset int) string {
	cursor := Cursor{
		QueryHash: queryHash,
		Offset:    offset,
		CreatedAt: time.Now(),
	}

	data, _ := json.Marshal(cursor)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor parses a cursor string.
func DecodeCursor(s string) (*Cursor, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding")
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor format")
	}

	// Check expiry (10 minutes)
	if time.Since(cursor.CreatedAt) > 10*time.Minute {
		return nil, fmt.Errorf("cursor expired")
	}

	return &cursor, nil
}

// PaginatedResponse wraps search results with pagination info.
type PaginatedResponse struct {
	QueryType  string         `json:"query_type"`
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count"`
	HasMore    bool           `json:"has_more"`
	Cursor     string         `json:"cursor,omitempty"`
}

// Paginate applies pagination to results.
func Paginate(results []SearchResult, offset, limit int, queryHash string, queryType string) PaginatedResponse {
	total := len(results)

	// Apply offset
	if offset >= len(results) {
		return PaginatedResponse{
			QueryType:  queryType,
			Results:    []SearchResult{},
			TotalCount: total,
			HasMore:    false,
		}
	}
	results = results[offset:]

	// Apply limit
	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	// Generate cursor for next page
	var cursor string
	if hasMore {
		cursor = EncodeCursor(queryHash, offset+limit)
	}

	return PaginatedResponse{
		QueryType:  queryType,
		Results:    results,
		TotalCount: total,
		HasMore:    hasMore,
		Cursor:     cursor,
	}
}

// HashQuery creates a deterministic hash for query parameters.
func HashQuery(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return fmt.Sprintf("%x", h[:8])
}

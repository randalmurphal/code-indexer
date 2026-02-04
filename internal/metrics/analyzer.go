package metrics

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"time"
)

// Analyzer processes metrics logs.
type Analyzer struct {
	logPath string
}

// NewAnalyzer creates a new analyzer.
func NewAnalyzer(logPath string) *Analyzer {
	return &Analyzer{logPath: logPath}
}

// Summary contains aggregated metrics.
type Summary struct {
	Period          string         `json:"period"`
	TotalSearches   int            `json:"total_searches"`
	SearchesByType  map[string]int `json:"searches_by_type"`
	AvgLatencyMs    int64          `json:"avg_latency_ms"`
	ZeroResultCount int            `json:"zero_result_count"`
	CacheHits       int            `json:"cache_hits"`
	TopQueries      []QueryCount   `json:"top_queries"`
}

// QueryCount represents a query with its count.
type QueryCount struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// Analyze processes logs for a time period.
func (a *Analyzer) Analyze(since time.Duration) (*Summary, error) {
	file, err := os.Open(a.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-since)
	summary := &Summary{
		Period:         since.String(),
		SearchesByType: make(map[string]int),
	}

	queryCounts := make(map[string]int)
	var totalLatency int64
	var latencyCount int

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Parse timestamp
		tsStr, ok := event["ts"].(string)
		if !ok {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil || ts.Before(cutoff) {
			continue
		}

		// Process by event type
		eventType, _ := event["event"].(string)
		switch eventType {
		case "search":
			summary.TotalSearches++

			if qt, ok := event["query_type"].(string); ok {
				summary.SearchesByType[qt]++
			}

			if results, ok := event["results"].(float64); ok && results == 0 {
				summary.ZeroResultCount++
			}

			if latency, ok := event["latency_ms"].(float64); ok {
				totalLatency += int64(latency)
				latencyCount++
			}

			if cacheHit, ok := event["cache_hit"].(bool); ok && cacheHit {
				summary.CacheHits++
			}

			if query, ok := event["query"].(string); ok {
				queryCounts[query]++
			}
		}
	}

	// Calculate average latency
	if latencyCount > 0 {
		summary.AvgLatencyMs = totalLatency / int64(latencyCount)
	}

	// Get top queries
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range queryCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	for i := 0; i < len(sorted) && i < 10; i++ {
		summary.TopQueries = append(summary.TopQueries, QueryCount{
			Query: sorted[i].Key,
			Count: sorted[i].Value,
		})
	}

	return summary, nil
}

// GetZeroResultQueries returns queries that returned no results.
func (a *Analyzer) GetZeroResultQueries(since time.Duration) ([]QueryCount, error) {
	file, err := os.Open(a.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-since)
	queryCounts := make(map[string]int)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		tsStr, ok := event["ts"].(string)
		if !ok {
			continue
		}
		ts, err := time.Parse(time.RFC3339, tsStr)
		if err != nil || ts.Before(cutoff) {
			continue
		}

		eventType, _ := event["event"].(string)
		if eventType != "search" {
			continue
		}

		results, _ := event["results"].(float64)
		if results == 0 {
			query, _ := event["query"].(string)
			queryCounts[query]++
		}
	}

	var result []QueryCount
	for q, c := range queryCounts {
		result = append(result, QueryCount{Query: q, Count: c})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result, nil
}

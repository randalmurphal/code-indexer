package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzerAnalyze(t *testing.T) {
	// Create temp log file with test data
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "metrics.jsonl")

	now := time.Now().UTC()
	recentTS := now.Add(-1 * time.Hour).Format(time.RFC3339)
	oldTS := now.Add(-25 * time.Hour).Format(time.RFC3339)

	logData := `{"ts":"` + recentTS + `","event":"search","query":"auth flow","query_type":"concept","results":5,"latency_ms":100,"cache_hit":false}
{"ts":"` + recentTS + `","event":"search","query":"auth flow","query_type":"concept","results":3,"latency_ms":150,"cache_hit":true}
{"ts":"` + recentTS + `","event":"search","query":"user service","query_type":"symbol","results":0,"latency_ms":50,"cache_hit":false}
{"ts":"` + oldTS + `","event":"search","query":"old query","query_type":"concept","results":10,"latency_ms":200,"cache_hit":false}
`
	err := os.WriteFile(logPath, []byte(logData), 0644)
	require.NoError(t, err)

	analyzer := NewAnalyzer(logPath)
	summary, err := analyzer.Analyze(24 * time.Hour)
	require.NoError(t, err)

	assert.Equal(t, 3, summary.TotalSearches)           // Only recent events
	assert.Equal(t, 2, summary.SearchesByType["concept"])
	assert.Equal(t, 1, summary.SearchesByType["symbol"])
	assert.Equal(t, 1, summary.ZeroResultCount)
	assert.Equal(t, 1, summary.CacheHits)
	assert.Equal(t, int64(100), summary.AvgLatencyMs) // (100+150+50)/3 = 100

	// Check top queries
	require.NotEmpty(t, summary.TopQueries)
	assert.Equal(t, "auth flow", summary.TopQueries[0].Query)
	assert.Equal(t, 2, summary.TopQueries[0].Count)
}

func TestAnalyzerGetZeroResultQueries(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "metrics.jsonl")

	now := time.Now().UTC()
	recentTS := now.Add(-1 * time.Hour).Format(time.RFC3339)

	logData := `{"ts":"` + recentTS + `","event":"search","query":"found query","query_type":"concept","results":5}
{"ts":"` + recentTS + `","event":"search","query":"missing thing","query_type":"concept","results":0}
{"ts":"` + recentTS + `","event":"search","query":"missing thing","query_type":"concept","results":0}
{"ts":"` + recentTS + `","event":"search","query":"another missing","query_type":"symbol","results":0}
`
	err := os.WriteFile(logPath, []byte(logData), 0644)
	require.NoError(t, err)

	analyzer := NewAnalyzer(logPath)
	zeroResults, err := analyzer.GetZeroResultQueries(24 * time.Hour)
	require.NoError(t, err)

	require.Len(t, zeroResults, 2)
	assert.Equal(t, "missing thing", zeroResults[0].Query)
	assert.Equal(t, 2, zeroResults[0].Count)
	assert.Equal(t, "another missing", zeroResults[1].Query)
	assert.Equal(t, 1, zeroResults[1].Count)
}

func TestAnalyzerEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "empty.jsonl")
	err := os.WriteFile(logPath, []byte(""), 0644)
	require.NoError(t, err)

	analyzer := NewAnalyzer(logPath)
	summary, err := analyzer.Analyze(24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.TotalSearches)
}

func TestAnalyzerFileNotFound(t *testing.T) {
	analyzer := NewAnalyzer("/nonexistent/path/metrics.jsonl")
	_, err := analyzer.Analyze(24 * time.Hour)
	assert.Error(t, err)
}

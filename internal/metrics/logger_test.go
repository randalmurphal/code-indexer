package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "metrics.jsonl")

	logger, err := NewLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	// Log a search event
	logger.LogSearch("auth timeout", "concept", 5, 120, false)

	// Log a context inject event
	logger.LogContextInject("auth.js", 3, 0.82)

	// Log a file read event
	logger.LogFileRead("sessionStore.js", true)

	// Log an index update event
	logger.LogIndexUpdate("r3", 10, 45)

	// Log an error event
	logger.LogError("search", "connection timeout")

	// Verify file has content
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	content := string(data)

	// Verify each event type is present
	assert.Contains(t, content, `"event":"search"`)
	assert.Contains(t, content, `"query":"auth timeout"`)
	assert.Contains(t, content, `"cache_hit":false`)

	assert.Contains(t, content, `"event":"context_inject"`)
	assert.Contains(t, content, `"file":"auth.js"`)

	assert.Contains(t, content, `"event":"file_read"`)
	assert.Contains(t, content, `"was_suggested":true`)

	assert.Contains(t, content, `"event":"index_update"`)
	assert.Contains(t, content, `"chunks_updated":45`)

	assert.Contains(t, content, `"event":"error"`)
	assert.Contains(t, content, `"operation":"search"`)

	// Verify JSONL format (one JSON object per line)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	assert.Len(t, lines, 5)
}

func TestMetricsLoggerConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "metrics.jsonl")

	logger, err := NewLogger(logPath)
	require.NoError(t, err)
	defer logger.Close()

	// Write concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.LogSearch("query", "concept", n, int64(n*10), false)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify file has all 10 lines
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 10)
}

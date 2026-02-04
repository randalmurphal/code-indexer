// Package metrics provides JSONL event logging for analytics.
package metrics

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Logger writes metrics events to JSONL file.
type Logger struct {
	file *os.File
	mu   sync.Mutex
}

// NewLogger creates a new metrics logger.
func NewLogger(path string) (*Logger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &Logger{file: file}, nil
}

// Close closes the log file.
func (l *Logger) Close() error {
	return l.file.Close()
}

func (l *Logger) log(event string, data map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339),
		"event": event,
	}
	for k, v := range data {
		e[k] = v
	}

	line, _ := json.Marshal(e)
	l.file.Write(line)
	l.file.Write([]byte("\n"))
}

// LogSearch logs a search query event.
func (l *Logger) LogSearch(query, queryType string, results int, latencyMs int64, cacheHit bool) {
	l.log("search", map[string]interface{}{
		"query":      query,
		"query_type": queryType,
		"results":    results,
		"latency_ms": latencyMs,
		"cache_hit":  cacheHit,
	})
}

// LogContextInject logs a context injection event.
func (l *Logger) LogContextInject(file string, suggestions int, confidence float64) {
	l.log("context_inject", map[string]interface{}{
		"file":        file,
		"suggestions": suggestions,
		"confidence":  confidence,
	})
}

// LogFileRead logs when Claude reads a file.
func (l *Logger) LogFileRead(file string, wasSuggested bool) {
	l.log("file_read", map[string]interface{}{
		"file":          file,
		"was_suggested": wasSuggested,
	})
}

// LogIndexUpdate logs an index update event.
func (l *Logger) LogIndexUpdate(repo string, filesChanged, chunksUpdated int) {
	l.log("index_update", map[string]interface{}{
		"repo":           repo,
		"files_changed":  filesChanged,
		"chunks_updated": chunksUpdated,
	})
}

// LogError logs an error event.
func (l *Logger) LogError(operation, message string) {
	l.log("error", map[string]interface{}{
		"operation": operation,
		"message":   message,
	})
}

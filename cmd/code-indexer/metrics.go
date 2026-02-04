package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/randalmurphy/ai-devtools-admin/internal/metrics"
	"github.com/spf13/cobra"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Analyze usage metrics",
	Long:  `Analyze search usage metrics from the log file.`,
	RunE:  runMetrics,
}

var (
	metricsSince       string
	metricsZeroResults bool
	metricsJSON        bool
)

func init() {
	metricsCmd.Flags().StringVar(&metricsSince, "last", "7d", "Time period (e.g., 1h, 24h, 7d, 30d)")
	metricsCmd.Flags().BoolVar(&metricsZeroResults, "zero-results", false, "Show only zero-result queries")
	metricsCmd.Flags().BoolVar(&metricsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(metricsCmd)
}

func runMetrics(cmd *cobra.Command, args []string) error {
	// Parse duration
	duration, err := parseDuration(metricsSince)
	if err != nil {
		return fmt.Errorf("invalid time period: %w", err)
	}

	// Get metrics path
	homeDir, _ := os.UserHomeDir()
	metricsPath := filepath.Join(homeDir, ".local", "share", "code-index", "metrics.jsonl")

	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		fmt.Println("No metrics data found. Use the search_code tool to generate metrics.")
		return nil
	}

	analyzer := metrics.NewAnalyzer(metricsPath)

	if metricsZeroResults {
		queries, err := analyzer.GetZeroResultQueries(duration)
		if err != nil {
			return err
		}

		if metricsJSON {
			data, _ := json.MarshalIndent(queries, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("Zero-result queries (last %s):\n\n", metricsSince)
			if len(queries) == 0 {
				fmt.Println("  No zero-result queries found.")
			}
			for _, q := range queries {
				fmt.Printf("  - \"%s\" (%d times)\n", q.Query, q.Count)
			}
		}
		return nil
	}

	summary, err := analyzer.Analyze(duration)
	if err != nil {
		return err
	}

	if metricsJSON {
		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Metrics Summary (last %s):\n\n", metricsSince)
		fmt.Printf("  Total searches:      %d\n", summary.TotalSearches)
		fmt.Printf("  Avg latency:         %dms\n", summary.AvgLatencyMs)
		fmt.Printf("  Cache hits:          %d\n", summary.CacheHits)
		fmt.Printf("  Zero-result queries: %d\n", summary.ZeroResultCount)
		fmt.Println()
		if len(summary.SearchesByType) > 0 {
			fmt.Println("  Searches by type:")
			for t, c := range summary.SearchesByType {
				fmt.Printf("    - %s: %d\n", t, c)
			}
			fmt.Println()
		}
		if len(summary.TopQueries) > 0 {
			fmt.Println("  Top queries:")
			for _, q := range summary.TopQueries {
				fmt.Printf("    - \"%s\" (%d times)\n", q.Query, q.Count)
			}
		}
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if len(s) > 0 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err == nil {
			return time.Duration(d) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

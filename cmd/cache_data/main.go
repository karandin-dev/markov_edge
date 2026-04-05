package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"markov_screener/internal/market"
)

type Period struct {
	Name  string
	Start time.Time
	End   time.Time
}

func main() {
	client := market.NewBybitClient()

	symbols := []string{
		"ETHUSDT",
		"SOLUSDT",
		"LINKUSDT",
		"HBARUSDT",
		"BTCUSDT",
	}

	timeframe := "15m"

	periods := []Period{
		{
			Name:  "2023",
			Start: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:  "2024",
			Start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:  "2025",
			Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:  "2026_YTD",
			Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Now().UTC(),
		},
	}

	cacheDir := filepath.Join("data", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		panic(err)
	}

	fmt.Println("CACHE EXPORT")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Symbols: %d\n", len(symbols))
	fmt.Printf("Periods: %d\n", len(periods))
	fmt.Println()

	totalJobs := len(symbols) * len(periods)
	jobNum := 0

	for _, symbol := range symbols {
		for _, p := range periods {
			jobNum++

			fmt.Printf("[%d/%d] %s | %s\n", jobNum, totalJobs, symbol, p.Name)
			fmt.Printf("  range: %s -> %s\n",
				p.Start.Format("2006-01-02"),
				p.End.Format("2006-01-02"),
			)

			filename := cacheFilename(symbol, timeframe, p.Start, p.End)
			fullpath := filepath.Join(cacheDir, filename)

			if _, err := os.Stat(fullpath); err == nil {
				fmt.Printf("  already exists, skip: %s\n\n", fullpath)
				continue
			}

			candles, err := fetchWithRetry(client, symbol, timeframe, p.Start, p.End)
			if err != nil {
				fmt.Printf("  skip: %v\n\n", err)
				continue
			}

			if err := saveCandlesJSON(fullpath, candles); err != nil {
				fmt.Printf("  save error: %v\n\n", err)
				continue
			}

			fmt.Printf("  candles: %d\n", len(candles))
			fmt.Printf("  saved:   %s\n\n", fullpath)

			time.Sleep(1500 * time.Millisecond)
		}
	}

	fmt.Println("DONE")
}

func fetchWithRetry(
	client *market.BybitClient,
	symbol string,
	timeframe string,
	start, end time.Time,
) ([]market.Candle, error) {
	var lastErr error

	for attempt := 1; attempt <= 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		candles, err := client.FetchOHLCVRange(ctx, symbol, timeframe, start, end)
		cancel()

		if err == nil {
			return candles, nil
		}

		lastErr = err
		wait := time.Duration(2*attempt) * time.Second

		if isRateLimitError(err) {
			fmt.Printf("  rate limit, retry %d/5 after %v\n", attempt, wait)
		} else {
			fmt.Printf("  retry %d/5 after error: %v\n", attempt, err)
		}

		time.Sleep(wait)
	}

	return nil, lastErr
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too many visits") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "retcode=10006")
}

func cacheFilename(symbol, timeframe string, start, end time.Time) string {
	return fmt.Sprintf(
		"%s_%s_%s_%s.json",
		symbol,
		timeframe,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
}

func saveCandlesJSON(path string, candles []market.Candle) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(candles)
}

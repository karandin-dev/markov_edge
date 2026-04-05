package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"sync"
	"time"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"
	"markov_screener/internal/market"
	"markov_screener/internal/markov"
)

type jobResult struct {
	Score market.SymbolScore
	Err   error
}

func main() {
	configPath := flag.String("config", "configs/screener.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	if len(cfg.Symbols) == 0 {
		panic("config symbols is empty")
	}

	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	client := market.NewBybitClientWithBaseURL(cfg.Exchange.BaseURL)

	fmt.Println("MARKOV SCREENER")
	fmt.Println("--------------------------------------------------")
	fmt.Println("TIMEFRAME:", cfg.Timeframe)
	fmt.Println("WORKERS:", workers)
	fmt.Println("SYMBOLS:", cfg.Symbols)

	runCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	results := runScreener(runCtx, client, cfg, workers)

	printResults(results)
}

func runScreener(
	parentCtx context.Context,
	client exchange.MarketDataClient,
	cfg config.Config,
	workers int,
) []market.SymbolScore {
	jobs := make(chan string)
	results := make(chan jobResult)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(parentCtx, client, cfg, jobs, results, &wg)
	}

	go func() {
		defer close(jobs)
		for _, symbol := range cfg.Symbols {
			select {
			case <-parentCtx.Done():
				return
			case jobs <- symbol:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	scores := make([]market.SymbolScore, 0, len(cfg.Symbols))

	for res := range results {
		if res.Err != nil {
			fmt.Println("ERROR:", res.Err)
			continue
		}
		scores = append(scores, res.Score)
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].LongScore > scores[j].LongScore
		}
		return scores[i].Score > scores[j].Score
	})

	return scores
}

func worker(
	parentCtx context.Context,
	client exchange.MarketDataClient,
	cfg config.Config,
	jobs <-chan string,
	results chan<- jobResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	tfDur, err := timeframeDuration(cfg.Timeframe)
	if err != nil {
		results <- jobResult{
			Err: fmt.Errorf("invalid timeframe %q: %w", cfg.Timeframe, err),
		}
		return
	}

	lookbackBars := maxInt(cfg.Candles, cfg.MarkovWindow+cfg.VolWindow+20)

	for symbol := range jobs {
		fmt.Println("processing:", symbol)

		ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)

		end := time.Now().UTC()
		start := end.Add(-time.Duration(lookbackBars) * tfDur)

		candles, err := client.FetchOHLCVRange(ctx, symbol, cfg.Timeframe, start, end)
		if err != nil {
			cancel()
			results <- jobResult{
				Err: fmt.Errorf("%s fetch error: %w", symbol, err),
			}
			continue
		}

		score, err := markov.AnalyzeSymbol(symbol, candles, cfg)
		cancel()

		if err != nil {
			results <- jobResult{
				Err: fmt.Errorf("%s analyze error: %w", symbol, err),
			}
			continue
		}

		results <- jobResult{
			Score: score,
		}
	}
}

func printResults(results []market.SymbolScore) {
	fmt.Println()
	fmt.Println("MARKOV SCREENER RESULTS")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------------------------------")
	fmt.Printf("%-12s %-12s %-12s %-14s %-14s %-8s %-8s %-8s %-8s %-8s %-8s %-10s %-8s %-8s %-8s %-8s %-8s\n",
		"SYMBOL", "PRICE", "STATE", "REGIME", "SIGNAL", "CONT1", "REV1", "PERS1", "CONT2", "REV2", "PERS2", "ENTROPY", "SCORE", "LONG", "SHORT", "UP1", "DOWN1")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------------------------------")

	for _, r := range results {
		fmt.Printf("%-12s %-12.4f %-12s %-14s %-14s %-8.4f %-8.4f %-8.4f %-8.4f %-8.4f %-8.4f %-10.4f %-8.4f %-8.4f %-8.4f %-8.4f %-8.4f\n",
			r.Symbol,
			r.LastPrice,
			r.LastState.String(),
			r.RegimeClass,
			r.HumanSignal,
			r.ContinuationProb,
			r.ReversalProb,
			r.PersistenceProb,
			r.ContinuationProb2,
			r.ReversalProb2,
			r.PersistenceProb2,
			r.Entropy,
			r.Score,
			r.LongScore,
			r.ShortScore,
			r.ProbUp1,
			r.ProbDown1,
		)
	}
}

func timeframeDuration(tf string) (time.Duration, error) {
	switch tf {
	case "1m":
		return time.Minute, nil
	case "3m":
		return 3 * time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "2h":
		return 2 * time.Hour, nil
	case "4h":
		return 4 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "12h":
		return 12 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "1w":
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe: %s", tf)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

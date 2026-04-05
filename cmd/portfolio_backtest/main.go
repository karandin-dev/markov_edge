package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"markov_screener/internal/backtest"
	"markov_screener/internal/config"
	"markov_screener/internal/market"
	"markov_screener/internal/storage"
)

const StrategyVersion = "v0.7-markov-edge"

type LoadResult struct {
	Symbol  string
	Candles []market.Candle
	Obs     []backtest.Observation
	Source  string
	Err     error
}

func main() {
	configPath := flag.String("config", "configs/backtest.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	db, err := storage.InitDB(filepath.Join("data", "experiments.db"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := createTradesTable(db.Conn()); err != nil {
		panic(err)
	}
	if err := ensureTradesColumns(db.Conn()); err != nil {
		panic(err)
	}

	symbols := []string{
		"LINKUSDT",
	}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cacheDir := filepath.Join("data", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		panic(err)
	}

	profileCfg := backtest.ConservativeProfileConfig()
	backtest.SetProfileConfig(profileCfg)

	opts := backtest.NewStrategyOptionsFromConfig(cfg)
	opts.Commission = cfg.Commission

	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	fmt.Println("PORTFOLIO DATA LOAD")
	fmt.Println("--------------------------------------------------")
	fmt.Println("PROFILE MODE: conservative")
	fmt.Println("REGIME FILTER: sl1/sl2/sl3 enabled")
	fmt.Println("STRATEGY VERSION:", StrategyVersion)
	fmt.Println("TIMEFRAME:", cfg.Timeframe)
	fmt.Println("WORKERS:", workers)
	fmt.Println("COMMISSION:", opts.Commission)

	data, obsMap := loadCachedDataParallel(
		cacheDir,
		symbols,
		cfg.Timeframe,
		start,
		end,
		workers,
	)

	popts := backtest.PortfolioOptions{
		InitialBalance:   1000.0,
		RiskPerTrade:     0.01,
		MaxPortfolioRisk: 0.05,
		AllowLong:        true,
		AllowShort:       cfg.IncludeShorts,
		StrategyOptions:  opts,
	}

	fmt.Println()
	fmt.Println("RUNNING BACKTEST...")
	fmt.Println("--------------------------------------------------")

	trades, _, stats := backtest.RunPortfolioBacktest(data, obsMap, popts)

	backtest.PrintPortfolioStats(stats)
	backtest.PrintPortfolioTradeBreakdown(trades)

	runID := fmt.Sprintf("%s_%d", StrategyVersion, time.Now().Unix())

	if err := saveTrades(db.Conn(), runID, trades); err != nil {
		fmt.Println("save error:", err)
	} else {
		fmt.Println("saved trades, run_id:", runID)
	}

	cfgJSON, _ := json.MarshalIndent(map[string]any{
		"strategy":       StrategyVersion,
		"profile":        "conservative",
		"symbols":        symbols,
		"timeframe":      cfg.Timeframe,
		"workers":        workers,
		"include_shorts": cfg.IncludeShorts,
		"commission":     opts.Commission,
		"start":          start,
		"end":            end,
	}, "", "  ")

	fmt.Println("\nCONFIG:")
	fmt.Println(string(cfgJSON))
}

func createTradesTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS trades (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT,
		strategy_version TEXT,
		symbol TEXT,
		direction INTEGER,
		entry_time INTEGER,
		exit_time INTEGER,
		entry_price REAL,
		exit_price REAL,
		size_usd REAL,
		pnl_usd REAL,
		pnl_pct REAL,
		fee_usd REAL,
		hold_bars INTEGER,
		reason TEXT,
		entry_zscore REAL,
		entry_entropy REAL,
		entry_state TEXT,
		entry_long_score REAL,
		entry_short_score REAL
	);`
	_, err := db.Exec(query)
	return err
}

func saveTrades(db *sql.DB, runID string, trades []backtest.PortfolioTrade) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
	INSERT INTO trades (
		run_id, strategy_version,
		symbol, direction,
		entry_time, exit_time,
		entry_price, exit_price,
		size_usd,
		pnl_usd, pnl_pct,
		fee_usd,
		hold_bars, reason,
		entry_zscore, entry_entropy, entry_state,
		entry_long_score, entry_short_score
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, t := range trades {
		_, err := stmt.Exec(
			runID, StrategyVersion,
			t.Symbol, t.Direction,
			t.EntryTime, t.ExitTime,
			t.EntryPrice, t.ExitPrice,
			t.SizeUSD,
			t.PnlUSD, t.PnlPctOnPos,
			t.FeeUSD,
			t.HoldBars, t.Reason,
			t.EntryZScore, t.EntryEntropy, t.EntryState,
			t.EntryLongScore, t.EntryShortScore,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func ensureTradesColumns(db *sql.DB) error {
	type columnDef struct {
		name string
		typ  string
	}

	required := []columnDef{
		{name: "entry_zscore", typ: "REAL"},
		{name: "entry_entropy", typ: "REAL"},
		{name: "entry_state", typ: "TEXT"},
		{name: "entry_long_score", typ: "REAL"},
		{name: "entry_short_score", typ: "REAL"},
	}

	existing := make(map[string]bool)
	rows, err := db.Query(`PRAGMA table_info(trades)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, col := range required {
		if existing[col.name] {
			continue
		}
		query := fmt.Sprintf("ALTER TABLE trades ADD COLUMN %s %s", col.name, col.typ)
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func loadCachedDataParallel(
	cacheDir string,
	symbols []string,
	timeframe string,
	start, end time.Time,
	workers int,
) (map[string][]market.Candle, map[string][]backtest.Observation) {
	jobs := make(chan string)
	results := make(chan LoadResult, len(symbols))

	var wg sync.WaitGroup

	workerFn := func(id int) {
		defer wg.Done()

		for symbol := range jobs {
			fmt.Printf("[worker %d] preparing %s...\n", id, symbol)

			candles, err := loadCandlesFromYearlyCache(
				cacheDir,
				symbol,
				timeframe,
				start,
				end,
			)
			if err != nil {
				results <- LoadResult{
					Symbol: symbol,
					Err:    err,
				}
				continue
			}

			if len(candles) < 500 {
				results <- LoadResult{
					Symbol: symbol,
					Err:    fmt.Errorf("not enough candles: %d", len(candles)),
				}
				continue
			}

			obs := backtest.BuildObservations(symbol, candles)

			results <- LoadResult{
				Symbol:  symbol,
				Candles: candles,
				Obs:     obs,
				Source:  "cache",
				Err:     nil,
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go workerFn(i + 1)
	}

	go func() {
		for _, symbol := range symbols {
			jobs <- symbol
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	data := make(map[string][]market.Candle)
	obsMap := make(map[string][]backtest.Observation)

	for r := range results {
		if r.Err != nil {
			fmt.Printf("skip %s: %v\n", r.Symbol, r.Err)
			continue
		}

		data[r.Symbol] = r.Candles
		obsMap[r.Symbol] = r.Obs

		fmt.Printf("done %s: candles=%d obs=%d source=%s\n",
			r.Symbol, len(r.Candles), len(r.Obs), r.Source)
	}

	return data, obsMap
}

func cacheFilename(symbol, timeframe string, start, end time.Time) string {
	safeStart := start.UTC().Format("2006-01-02")
	safeEnd := end.UTC().Format("2006-01-02")
	return fmt.Sprintf("%s_%s_%s_%s.json",
		strings.ToUpper(symbol),
		timeframe,
		safeStart,
		safeEnd,
	)
}

func loadCandlesJSON(path string) ([]market.Candle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var candles []market.Candle
	if err := json.Unmarshal(raw, &candles); err != nil {
		return nil, err
	}

	return candles, nil
}

func loadCandlesFromYearlyCache(
	cacheDir, symbol, timeframe string,
	start, end time.Time,
) ([]market.Candle, error) {
	all := make([]market.Candle, 0)

	for year := start.Year(); year < end.Year(); year++ {
		fileStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		fileEnd := fileStart.AddDate(1, 0, 0)

		filename := fmt.Sprintf(
			"%s_%s_%s_%s.json",
			symbol,
			timeframe,
			fileStart.Format("2006-01-02"),
			fileEnd.Format("2006-01-02"),
		)

		path := filepath.Join(cacheDir, filename)

		candles, err := loadCandlesJSON(path)
		if err != nil {
			fmt.Println("skip", symbol, ":", err)
			continue
		}

		all = append(all, candles...)
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no cached data found")
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.Before(all[j].Time)
	})

	result := make([]market.Candle, 0, len(all))
	seen := make(map[int64]bool)

	for _, c := range all {
		ts := c.Time.Unix()

		if seen[ts] {
			continue
		}
		seen[ts] = true

		if !c.Time.Before(start) && c.Time.Before(end) {
			result = append(result, c)
		}
	}

	return result, nil
}

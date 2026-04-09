package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"
	"markov_screener/internal/markov"

	"github.com/bytedance/gopkg/util/logger"
)

// ============================================================================
// 🔄 Основные циклы работы бота
// ============================================================================

// runLiveLoop — главный бесконечный цикл, ожидающий закрытия свечей
func runLiveLoop(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
	states map[string]*symbolRuntimeState,
	activePositions map[string]*markov.Position, // ← Используем markov.Position
) {
	pollEvery := time.Duration(cfg.Runtime.PollIntervalSeconds) * time.Second
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-parentCtx.Done():
			fmt.Println()
			fmt.Println("LIVE LOOP STOPPED")
			return

		case now := <-ticker.C:
			if !isAfterCandleClose(now.UTC(), cfg.Timeframe) {
				continue
			}

			changed := false
			var cycleCloseTime time.Time

			for _, symbol := range cfg.Symbols {
				ctx, cancel := context.WithTimeout(parentCtx, 20*time.Second)
				lastClosed, err := fetchLastClosedCandleTime(ctx, marketClient, cfg, symbol)
				cancel()

				if err != nil {
					fmt.Printf("[%s] CHECK ERROR: %v\n", symbol, err)
					continue
				}

				st := states[symbol]
				if lastClosed.After(st.LastCandleTime) {
					changed = true
					cycleCloseTime = lastClosed
					break
				}
			}

			if !changed {
				continue
			}

			fmt.Println()
			fmt.Println("NEW CLOSED CANDLE DETECTED")
			fmt.Println("TIME:", time.Now().UTC().Format(time.RFC3339))
			fmt.Println("CYCLE ANCHOR:", cycleCloseTime.Format(time.RFC3339))
			fmt.Println("--------------------------------------------------")

			runAndPrintCycle(parentCtx, marketClient, tradingClient, cfg, states, false, cycleCloseTime)

			// 🔥 Обновление MAE/MFE для активных позиций
			for _, pos := range activePositions {
				price, err := getCurrentPrice(marketClient, pos.Symbol)
				if err != nil {
					logger.Warnf("[%s] Skip MAE update: %v", pos.Symbol, err)
					continue
				}
				pos.UpdateMetrics(price)
			}
		}
	}
}

// getCurrentPrice получает текущую цену для символа
// getCurrentPrice получает текущую цену для символа
func getCurrentPrice(client exchange.MarketDataClient, symbol string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Используем FetchOHLCVRange с небольшим временным окном
	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute) // Последние 5 минут

	candles, err := client.FetchOHLCVRange(ctx, symbol, "1m", start, now)
	if err != nil {
		return 0, err
	}

	if len(candles) == 0 {
		return 0, fmt.Errorf("no candles received for %s", symbol)
	}

	// Возвращаем цену последней свечи
	return candles[len(candles)-1].Close, nil
}

// runAndPrintCycle — оркестратор одного шага цикла (анализ + торговля)
func runAndPrintCycle(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
	states map[string]*symbolRuntimeState,
	isInitial bool,
	cycleCloseTime time.Time,
) {
	results := runDemoTraderCycle(parentCtx, marketClient, tradingClient, cfg, states, cycleCloseTime)

	fmt.Println("DEMO TRADER RESULTS")
	fmt.Println("--------------------------------------------------")
	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("[%s] ERROR: %v\n", r.Symbol, r.Err)
			continue
		}
		fmt.Println(r.Text)
	}

	now := time.Now().UTC()
	for _, r := range results {
		st := states[r.Symbol]
		if st == nil {
			continue
		}
		st.LastRunAt = now
		st.LastSignalText = r.Text
	}

	// Обновляем время последней свечи для всех символов
	for _, symbol := range cfg.Symbols {
		ctx, cancel := context.WithTimeout(parentCtx, 20*time.Second)
		lastClosed, err := fetchLastClosedCandleTime(ctx, marketClient, cfg, symbol)
		cancel()
		if err == nil {
			states[symbol].LastCandleTime = lastClosed
		}
	}

	if isInitial {
		fmt.Println("INITIAL CYCLE COMPLETE")
	} else {
		fmt.Println("LIVE CYCLE COMPLETE")
	}
}

// runDemoTraderCycle — распределитель задач по воркерам (concurrency)
func runDemoTraderCycle(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
	states map[string]*symbolRuntimeState,
	cycleCloseTime time.Time,
) []tradeResult {
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan string)
	results := make(chan tradeResult)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				res := processSymbol(parentCtx, marketClient, tradingClient, cfg, states[symbol], symbol, cycleCloseTime)
				results <- res
			}
		}()
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

	out := make([]tradeResult, 0, len(cfg.Symbols))
	for r := range results {
		out = append(out, r)
	}

	return out
}

// ============================================================================
// 🚀 Bootstrap и Инициализация
// ============================================================================

// bootstrapStates — начальная загрузка истории и синхронизация позиций
func bootstrapStates(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
) map[string]*symbolRuntimeState {
	states := make(map[string]*symbolRuntimeState, len(cfg.Symbols))
	for _, symbol := range cfg.Symbols {
		states[symbol] = &symbolRuntimeState{
			Symbol: symbol,
		}
	}

	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	type bootstrapResult struct {
		Symbol         string
		LastCandleTime time.Time
		Err            error
	}

	jobs := make(chan string)
	results := make(chan bootstrapResult)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				ctx, cancel := context.WithTimeout(parentCtx, 45*time.Second)
				lastCandleTime, err := fetchLastClosedCandleTime(ctx, marketClient, cfg, symbol)
				cancel()

				if err == nil {
					syncCtx, syncCancel := context.WithTimeout(parentCtx, 20*time.Second)
					pos, posErr := tradingClient.GetPosition(syncCtx, symbol)
					syncCancel()

					// Если на бирже есть открытая позиция — подхватываем её
					if posErr == nil && pos.Side != exchange.PositionSideNone && pos.Size > 0 {
						st := states[symbol]
						st.InPosition = true
						st.Direction = directionFromPositionSide(pos.Side)
						st.EntryPrice = pos.AvgPrice
						st.EntryAnchor = lastCandleTime
					}
				}

				results <- bootstrapResult{
					Symbol:         symbol,
					LastCandleTime: lastCandleTime,
					Err:            err,
				}
			}
		}()
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

	for res := range results {
		if res.Err != nil {
			fmt.Printf("[%s] BOOTSTRAP ERROR: %v\n", res.Symbol, res.Err)
			continue
		}
		states[res.Symbol].LastCandleTime = res.LastCandleTime
	}

	return states
}

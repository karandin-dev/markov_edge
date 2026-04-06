package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"
	"markov_screener/internal/market"
	"markov_screener/internal/markov"
)

type symbolRuntimeState struct {
	Symbol string

	LastCandleTime time.Time
	LastRunAt      time.Time
	LastSignalText string

	InPosition       bool
	Direction        int // 1 long, -1 short
	EntryAnchor      time.Time
	EntryPrice       float64
	EntryScore       float64 // <-- ADAPTIVE EXIT
	PeakPrice        float64 // <-- ADAPTIVE EXIT
	TrailingActive   bool    // <-- ADAPTIVE EXIT
	BaselineEntropy  float64 // <-- ADAPTIVE EXIT (fallback ~1.0-1.2)
	HoldBars         int
	LongBarsAgainst  int
	ShortBarsAgainst int
	CooldownBarsLeft int

	PreviouslyLoggedThruster bool // чтобы не спамить про трейлинг
	EntryPnlWasNegative      bool // чтобы отследить первый плюс
}

type tradeCandidate struct {
	Symbol     string
	Side       exchange.Side
	LastPrice  float64
	Score      float64
	LongScore  float64
	ShortScore float64
	Reason     string
}

type tradeResult struct {
	Symbol string
	Text   string
	Err    error
}

func main() {
	_ = godotenv.Load()

	// 🔥 SETUP FILE LOGGING
	os.MkdirAll("logs", 0755)
	logPath := fmt.Sprintf("logs/massacre_%s.log", time.Now().Format("20060102_150405"))

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Errorf("failed to create log file: %w", err))
	}

	// Пишем и в консоль, и в файл
	multiWriter := io.MultiWriter(os.Stdout, logFile)

	// Простая обёртка для логирования
	logPrintf := func(format string, args ...interface{}) {
		fmt.Fprintf(multiWriter, format, args...)
	}

	// 🔥 Теперь используй logPrintf вместо fmt.Printf во всём коде
	// Пример:
	logPrintf("DEMO TRADER LIVE EXECUTION\n")
	logPrintf("--------------------------------------------------\n")

	configPath := flag.String("config", "configs/screener.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	if len(cfg.Symbols) == 0 {
		panic("config symbols is empty")
	}

	secrets, err := config.LoadExchangeSecrets(cfg)
	if err != nil {
		panic(fmt.Errorf("load exchange secrets: %w", err))
	}
	if secrets.APIKey == "" || secrets.APISecret == "" {
		panic("api key/secret are empty")
	}

	marketClient := market.NewBybitClientWithBaseURL(cfg.Exchange.BaseURL)
	tradingClient := exchange.NewBybitPrivateClient(
		cfg.Exchange.BaseURL,
		secrets.APIKey,
		secrets.APISecret,
		cfg.Exchange.Category,
	)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authCtx, authCancel := context.WithTimeout(rootCtx, 15*time.Second)
	defer authCancel()

	if err := tradingClient.PingAuth(authCtx); err != nil {
		panic(fmt.Errorf("auth check failed: %w", err))
	}

	modeText := "DEMO TRADER LIVE"
	if cfg.Execution.DryRun {
		modeText += " DRY RUN"
	} else {
		modeText += " EXECUTION"
	}

	fmt.Println(modeText)
	fmt.Println("--------------------------------------------------")
	fmt.Println("TIMEFRAME:", cfg.Timeframe)
	fmt.Println("SYMBOLS:", cfg.Symbols)
	fmt.Println("EXECUTION ENABLED:", cfg.Execution.Enabled)
	fmt.Println("DRY RUN:", cfg.Execution.DryRun)
	fmt.Println("USDT PER TRADE:", cfg.Execution.USDTPerTrade)
	fmt.Println("BOOTSTRAP DAYS:", cfg.Runtime.BootstrapDays)
	fmt.Println("RUN ON START:", cfg.Runtime.RunOnStart)
	fmt.Println("POLL INTERVAL SEC:", cfg.Runtime.PollIntervalSeconds)
	fmt.Println()

	fmt.Println("ACTIVE SETTINGS")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("timeframe=%s | candles=%d | markov_window=%d | vol_window=%d\n",
		cfg.Timeframe,
		cfg.Candles,
		cfg.MarkovWindow,
		cfg.VolWindow,
	)
	fmt.Printf("adaptive_mode=%v | regime_filter_enabled=%v | use_dynamic_zscore=%v\n",
		cfg.AdaptiveMode,
		cfg.RegimeFilterEnabled,
		cfg.UseDynamicZScore,
	)
	fmt.Printf("long_threshold=%.4f | short_threshold=%.4f | bootstrap_days=%d\n",
		cfg.LongScoreThreshold,
		cfg.ShortScoreThreshold,
		cfg.Runtime.BootstrapDays,
	)
	fmt.Printf("min_hold=%d | cooldown=%d | long_exit_confirm=%d | short_exit_confirm=%d\n",
		cfg.MinHoldBars,
		cfg.CooldownBars,
		cfg.LongExitConfirmBars,
		cfg.ShortExitConfirmBars,
	)
	fmt.Printf("usdt_per_trade=%.4f | stop_loss_percent=%.4f\n",
		cfg.Execution.USDTPerTrade,
		cfg.Execution.StopLossPercent,
	)
	fmt.Println("MONETAS / SYMBOLS:")
	for _, symbol := range cfg.Symbols {
		fmt.Printf(" - %s\n", symbol)
	}
	fmt.Println()

	states := bootstrapStates(rootCtx, marketClient, tradingClient, cfg)

	fmt.Println()
	fmt.Println("BOOTSTRAP COMPLETE")
	fmt.Println("--------------------------------------------------")
	for _, symbol := range cfg.Symbols {
		st := states[symbol]

		nextClose, err := nextExpectedClosedCandleTime(st.LastCandleTime, cfg.Timeframe)
		if err != nil {
			fmt.Printf("[%s] last_closed_open=%s | next_close_expected=ERR: %v\n",
				symbol,
				st.LastCandleTime.Format(time.RFC3339),
				err,
			)
			continue
		}

		fmt.Printf("[%s] last_closed_open=%s | next_close_expected=%s\n",
			symbol,
			st.LastCandleTime.Format(time.RFC3339),
			nextClose.Format(time.RFC3339),
		)
	}

	if cfg.Runtime.RunOnStart {
		fmt.Println()
		fmt.Println("INITIAL RUN")
		fmt.Println("--------------------------------------------------")
		initialAnchor, err := commonCycleCloseTime(rootCtx, marketClient, cfg, cfg.Symbols)
		if err != nil {
			panic(fmt.Errorf("determine initial cycle close time: %w", err))
		}

		runAndPrintCycle(rootCtx, marketClient, tradingClient, cfg, states, true, initialAnchor)
	}

	fmt.Println()
	fmt.Println("LIVE LOOP STARTED")
	fmt.Println("--------------------------------------------------")

	runLiveLoop(rootCtx, marketClient, tradingClient, cfg, states)
}

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

func runLiveLoop(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
	states map[string]*symbolRuntimeState,
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
		}
	}
}

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

func processSymbol(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	tradingClient exchange.TradingClient,
	cfg config.Config,
	state *symbolRuntimeState,
	symbol string,
	cycleCloseTime time.Time,
) tradeResult {
	ctx, cancel := context.WithTimeout(parentCtx, 45*time.Second)
	defer cancel()

	if state == nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("runtime state is nil"),
		}
	}

	if !state.InPosition && state.CooldownBarsLeft > 0 {
		state.CooldownBarsLeft--
	}

	tfDur, err := timeframeDuration(cfg.Timeframe)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("invalid timeframe: %w", err),
		}
	}

	lookbackBars := maxInt(cfg.Candles, cfg.MarkovWindow+cfg.VolWindow+20)
	bootstrapBars := barsForDays(cfg.Runtime.BootstrapDays, tfDur)
	if bootstrapBars > lookbackBars {
		lookbackBars = bootstrapBars
	}

	analysisEnd := cycleCloseTime.Add(tfDur)
	start := analysisEnd.Add(-time.Duration(lookbackBars) * tfDur)

	candles, err := marketClient.FetchOHLCVRange(ctx, symbol, cfg.Timeframe, start, analysisEnd)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("fetch candles: %w", err),
		}
	}

	filtered := make([]market.Candle, 0, len(candles))
	for _, c := range candles {
		if !c.Time.After(cycleCloseTime) {
			filtered = append(filtered, c)
		}
	}
	candles = filtered

	if len(candles) == 0 {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("no candles left after filtering to cycle close time %s", cycleCloseTime.Format(time.RFC3339)),
		}
	}

	score, err := markov.AnalyzeSymbol(symbol, candles, cfg)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("analyze symbol: %w", err),
		}
	}

	candidate, hasCandidate := buildTradeCandidate(score, cfg)
	desiredPos := desiredPositionFromCandidate(candidate, hasCandidate)

	position, err := tradingClient.GetPosition(ctx, symbol)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("get position: %w", err),
		}
	}

	syncNote := syncStateWithExchange(state, position, cfg, cycleCloseTime)

	// 1) если позиция уже открыта — живем по стратегии
	if state.InPosition {
		state.HoldBars++

		// 🔥 Mark III: Трейлинг-стоп
		currentPrice := score.LastPrice

		// TrailingStop
		manageTrailingStop(ctx, tradingClient, symbol, state, currentPrice)

		// ... дальше идёт адаптивный выход ...

		// <-- ADAPTIVE EXIT START
		// 🔒 ЗАЩИТА: Не выходим по адаптиву, пока не отдержали min_hold
		if state.HoldBars >= cfg.MinHoldBars {
			currentPrice := score.LastPrice
			currentScore := score.LongScore
			if state.Direction == -1 {
				currentScore = score.ShortScore
			}

			unrealizedPnlPct := (currentPrice - state.EntryPrice) / state.EntryPrice * 100.0 * float64(state.Direction)

			// Обновляем пик цены
			if state.Direction == 1 && currentPrice > state.PeakPrice {
				state.PeakPrice = currentPrice
			}
			if state.Direction == -1 && (currentPrice < state.PeakPrice || state.PeakPrice == 0) {
				state.PeakPrice = currentPrice
			}

			// 🟢 НОВЫЙ МОНИТОРИНГ ПОЗИЦИИ
			directionText := "LONG"
			if state.Direction == -1 {
				directionText = "SHORT"
			}

			thrusterStatus := "OFF"
			if state.TrailingActive {
				thrusterStatus = "🛡️ ON"
			}

			// Показываем лог ТОЛЬКО если:
			// 1. Включился трейлинг (важное событие)
			// 2. PnL стал положительным (первый раз)
			// 3. Скор сильно упал (модель теряет уверенность)
			shouldLog := false

			if state.TrailingActive && !state.PreviouslyLoggedThruster {
				shouldLog = true
				state.PreviouslyLoggedThruster = true
			}

			if unrealizedPnlPct > 0 && state.EntryPnlWasNegative {
				shouldLog = true
				state.EntryPnlWasNegative = false
			}

			// Всегда логируем если PnL > 0.5% или скор упал сильно
			if unrealizedPnlPct > 0.5 {
				shouldLog = true
			}

			if shouldLog {
				fmt.Printf("[MONITOR] %s %s | PnL: %.2f%% | Peak: %.4f | CurrScore: %.4f | Thrusters: %s\n",
					symbol, directionText, unrealizedPnlPct, state.PeakPrice, currentScore, thrusterStatus)
			}

			// Расчёт адаптивного выхода
			exitCtx := markov.ExitContext{
				EntryScore:       state.EntryScore,
				CurrentScore:     currentScore,
				UnrealizedPnlPct: unrealizedPnlPct,
				VolRatio:         1.0,
				BarsHeld:         state.HoldBars,
				Entropy:          score.Entropy,
				BaselineEntropy:  state.BaselineEntropy,
				MacroRegime:      string(score.RegimeClass),
			}
			confidence := markov.CalcAdaptiveExitConfidence(exitCtx)
			threshold := markov.CalcAdaptiveExitThreshold(exitCtx.VolRatio, 0.65)

			if confidence > threshold {
				closeSide := exchange.SideSell
				if state.Direction == -1 {
					closeSide = exchange.SideBuy
				}
				closeText, closeErr := closePositionOnExchange(
					ctx,
					tradingClient,
					symbol,
					position,
					closeSide,
					cycleCloseTime,
					fmt.Sprintf("adaptive_exit(conf=%.2f>thr=%.2f)", confidence, threshold),
				)
				if closeErr != nil {
					return tradeResult{Symbol: symbol, Err: closeErr}
				}
				resetStateAfterClose(state, cfg)
				return tradeResult{
					Symbol: symbol,
					Text: fmt.Sprintf(
						"[%s] ADAPTIVE EXIT | anchor=%s | pnl=%.2f%% | score_entry=%.2f->%.2f | entropy=%.2f | conf=%.2f>%.2f | %s",
						symbol,
						cycleCloseTime.Format(time.RFC3339),
						unrealizedPnlPct,
						state.EntryScore,
						currentScore,
						score.Entropy,
						confidence,
						threshold,
						closeText,
					),
				}
			}
		}
		// <-- ADAPTIVE EXIT END
	}

	if state.Direction == 1 {
		if state.HoldBars >= cfg.MinHoldBars {
			if desiredPos != 1 {
				state.LongBarsAgainst++
			} else {
				state.LongBarsAgainst = 0
			}
		}

		if state.HoldBars >= cfg.MinHoldBars && state.LongBarsAgainst >= cfg.LongExitConfirmBars {
			closeText, closeErr := closePositionOnExchange(
				ctx,
				tradingClient,
				symbol,
				position,
				exchange.SideSell,
				cycleCloseTime,
				"signal_exit_long",
			)
			if closeErr != nil {
				return tradeResult{
					Symbol: symbol,
					Err:    closeErr,
				}
			}

			resetStateAfterClose(state, cfg)

			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf(
					"[%s] CLOSED LONG | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f | %s",
					symbol,
					cycleCloseTime.Format(time.RFC3339),
					state.HoldBars,
					cfg.MinHoldBars,
					state.LongBarsAgainst,
					cfg.LongExitConfirmBars,
					score.HumanSignal,
					score.Score,
					score.LongScore,
					score.ShortScore,
					closeText,
				),
			}
		}

		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] HOLD LONG | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				state.HoldBars,
				cfg.MinHoldBars,
				state.LongBarsAgainst,
				cfg.LongExitConfirmBars,
				score.HumanSignal,
				score.Score,
				score.LongScore,
				score.ShortScore,
				formatNote(syncNote),
			),
		}
	}

	if state.Direction == -1 {
		if state.HoldBars >= cfg.MinHoldBars {
			if desiredPos != -1 {
				state.ShortBarsAgainst++
			} else {
				state.ShortBarsAgainst = 0
			}
		}

		if state.HoldBars >= cfg.MinHoldBars && state.ShortBarsAgainst >= cfg.ShortExitConfirmBars {
			closeText, closeErr := closePositionOnExchange(
				ctx,
				tradingClient,
				symbol,
				position,
				exchange.SideBuy,
				cycleCloseTime,
				"signal_exit_short",
			)
			if closeErr != nil {
				return tradeResult{
					Symbol: symbol,
					Err:    closeErr,
				}
			}

			resetStateAfterClose(state, cfg)

			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf(
					"[%s] CLOSED SHORT | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f | %s",
					symbol,
					cycleCloseTime.Format(time.RFC3339),
					state.HoldBars,
					cfg.MinHoldBars,
					state.ShortBarsAgainst,
					cfg.ShortExitConfirmBars,
					score.HumanSignal,
					score.Score,
					score.LongScore,
					score.ShortScore,
					closeText,
				),
			}
		}

		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] HOLD SHORT | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				state.HoldBars,
				cfg.MinHoldBars,
				state.ShortBarsAgainst,
				cfg.ShortExitConfirmBars,
				score.HumanSignal,
				score.Score,
				score.LongScore,
				score.ShortScore,
				formatNote(syncNote),
			),
		}
	}

	// 2) если нет позиции, но есть cooldown — не входим
	if state.CooldownBarsLeft > 0 {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] COOLDOWN | anchor=%s | remaining=%d | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				state.CooldownBarsLeft,
				score.HumanSignal,
				score.Score,
				score.LongScore,
				score.ShortScore,
				formatNote(syncNote),
			),
		}
	}

	// 3) если входа нет — просто SKIP
	if !hasCandidate {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] SKIP | anchor=%s | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				score.HumanSignal,
				score.Score,
				score.LongScore,
				score.ShortScore,
				formatNote(syncNote),
			),
		}
	}

	instrumentInfo, err := tradingClient.GetInstrumentInfo(ctx, symbol)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("get instrument info: %w", err),
		}
	}

	rawQty := calcRawQtyFromUSDT(cfg.Execution.USDTPerTrade, candidate.LastPrice)
	qty := normalizeQty(rawQty, instrumentInfo.QtyStep)

	if qty <= 0 {
		return tradeResult{
			Symbol: symbol,
			Err: fmt.Errorf(
				"normalized qty <= 0: raw=%.8f step=%.8f",
				rawQty,
				instrumentInfo.QtyStep,
			),
		}
	}

	if instrumentInfo.MinOrderQty > 0 && qty < instrumentInfo.MinOrderQty {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] SKIP | anchor=%s | qty too small: raw=%.8f normalized=%.8f min_qty=%.8f step=%.8f",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				rawQty,
				qty,
				instrumentInfo.MinOrderQty,
				instrumentInfo.QtyStep,
			),
		}
	}

	if instrumentInfo.MinNotional > 0 && qty*candidate.LastPrice < instrumentInfo.MinNotional {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] SKIP | anchor=%s | notional too small: qty=%.8f price=%.4f notional=%.8f min_notional=%.8f",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				qty,
				candidate.LastPrice,
				qty*candidate.LastPrice,
				instrumentInfo.MinNotional,
			),
		}
	}

	stopLoss := calcStopLoss(candidate.Side, candidate.LastPrice, cfg.Execution.StopLossPercent)

	openOrders, err := tradingClient.GetOpenOrders(ctx, symbol)
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("get open orders: %w", err),
		}
	}

	if hasActiveEntryOrder(openOrders) {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] SKIP | anchor=%s | active entry order already exists",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
			),
		}
	}

	clientOrderID := buildClientOrderID(symbol, candidate.Side, cycleCloseTime)

	if !cfg.Execution.Enabled || cfg.Execution.DryRun {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf(
				"[%s] DRY RUN | anchor=%s | would place %s %s | raw_qty=%.8f | qty=%.8f | step=%.8f | min_qty=%.8f | last=%.4f | stop=%.4f | score=%.4f | long=%.4f | short=%.4f | reason=%s%s",
				symbol,
				cycleCloseTime.Format(time.RFC3339),
				string(candidate.Side),
				symbol,
				rawQty,
				qty,
				instrumentInfo.QtyStep,
				instrumentInfo.MinOrderQty,
				candidate.LastPrice,
				stopLoss,
				candidate.Score,
				candidate.LongScore,
				candidate.ShortScore,
				candidate.Reason,
				formatNote(syncNote),
			),
		}
	}

	orderResult, err := tradingClient.PlaceOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        symbol,
		Side:          candidate.Side,
		OrderType:     exchange.OrderTypeMarket,
		Qty:           qty,
		TimeInForce:   exchange.TimeInForceIOC,
		ClientOrderID: clientOrderID,
	})
	if err != nil {
		return tradeResult{
			Symbol: symbol,
			Err:    fmt.Errorf("place order: %w", err),
		}
	}

	stopText := "stop=disabled"
	if cfg.Execution.SetStopLoss && stopLoss > 0 {
		if err := tradingClient.SetStopLoss(ctx, exchange.StopLossRequest{
			Symbol:   symbol,
			StopLoss: stopLoss,
		}); err != nil {
			return tradeResult{
				Symbol: symbol,
				Err: fmt.Errorf(
					"order placed but set stop loss failed: order_id=%s err=%w",
					orderResult.OrderID,
					err,
				),
			}
		}
		stopText = fmt.Sprintf("stop=%.4f", stopLoss)
	}

	// Инициализация новых полей при входе
	state.InPosition = true
	state.Direction = desiredPos
	state.EntryAnchor = cycleCloseTime
	state.EntryPrice = candidate.LastPrice
	state.EntryScore = candidate.Score    // <-- ADAPTIVE EXIT
	state.PeakPrice = candidate.LastPrice // <-- ADAPTIVE EXIT
	state.TrailingActive = false          // <-- ADAPTIVE EXIT
	state.BaselineEntropy = 1.05          // <-- ADAPTIVE EXIT
	state.HoldBars = 0
	state.LongBarsAgainst = 0
	state.ShortBarsAgainst = 0
	state.PreviouslyLoggedThruster = false
	state.EntryPnlWasNegative = true // считаем что на входе PnL = 0 (не отрицательный)

	return tradeResult{
		Symbol: symbol,
		Text: fmt.Sprintf(
			"[%s] EXECUTED | anchor=%s | placed %s %s | raw_qty=%.8f | qty=%.8f | step=%.8f | min_qty=%.8f | last=%.4f | %s | order_id=%s | score=%.4f | long=%.4f | short=%.4f | reason=%s%s",
			symbol,
			cycleCloseTime.Format(time.RFC3339),
			string(candidate.Side),
			symbol,
			rawQty,
			qty,
			instrumentInfo.QtyStep,
			instrumentInfo.MinOrderQty,
			candidate.LastPrice,
			stopText,
			orderResult.OrderID,
			candidate.Score,
			candidate.LongScore,
			candidate.ShortScore,
			candidate.Reason,
			formatNote(syncNote),
		),
	}
}

func fetchLastClosedCandleTime(
	ctx context.Context,
	marketClient exchange.MarketDataClient,
	cfg config.Config,
	symbol string,
) (time.Time, error) {
	tfDur, err := timeframeDuration(cfg.Timeframe)
	if err != nil {
		return time.Time{}, err
	}

	end := time.Now().UTC()
	start := end.Add(-3 * tfDur)

	candles, err := marketClient.FetchOHLCVRange(ctx, symbol, cfg.Timeframe, start, end)
	if err != nil {
		return time.Time{}, err
	}
	if len(candles) == 0 {
		return time.Time{}, fmt.Errorf("no candles returned")
	}

	closedBefore := time.Now().UTC().Add(-tfDur)
	var lastClosed time.Time

	for _, c := range candles {
		if !c.Time.After(closedBefore) && c.Time.After(lastClosed) {
			lastClosed = c.Time
		}
	}

	if lastClosed.IsZero() {
		lastClosed = candles[len(candles)-1].Time
	}

	return lastClosed, nil
}

func commonCycleCloseTime(
	parentCtx context.Context,
	marketClient exchange.MarketDataClient,
	cfg config.Config,
	symbols []string,
) (time.Time, error) {
	for _, symbol := range symbols {
		ctx, cancel := context.WithTimeout(parentCtx, 20*time.Second)
		lastClosed, err := fetchLastClosedCandleTime(ctx, marketClient, cfg, symbol)
		cancel()
		if err == nil && !lastClosed.IsZero() {
			return lastClosed, nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to determine common cycle close time")
}

func buildTradeCandidate(score market.SymbolScore, cfg config.Config) (tradeCandidate, bool) {
	if cfg.LongOnly && !cfg.ShortOnly {
		if score.LongScore >= cfg.LongScoreThreshold {
			return tradeCandidate{
				Symbol:     score.Symbol,
				Side:       exchange.SideBuy,
				LastPrice:  score.LastPrice,
				Score:      score.Score,
				LongScore:  score.LongScore,
				ShortScore: score.ShortScore,
				Reason:     fmt.Sprintf("long_score %.4f >= threshold %.4f", score.LongScore, cfg.LongScoreThreshold),
			}, true
		}
		return tradeCandidate{}, false
	}

	if cfg.ShortOnly && !cfg.LongOnly {
		if score.ShortScore >= cfg.ShortScoreThreshold {
			return tradeCandidate{
				Symbol:     score.Symbol,
				Side:       exchange.SideSell,
				LastPrice:  score.LastPrice,
				Score:      score.Score,
				LongScore:  score.LongScore,
				ShortScore: score.ShortScore,
				Reason:     fmt.Sprintf("short_score %.4f >= threshold %.4f", score.ShortScore, cfg.ShortScoreThreshold),
			}, true
		}
		return tradeCandidate{}, false
	}

	if score.LongScore >= cfg.LongScoreThreshold && score.LongScore >= score.ShortScore {
		return tradeCandidate{
			Symbol:     score.Symbol,
			Side:       exchange.SideBuy,
			LastPrice:  score.LastPrice,
			Score:      score.Score,
			LongScore:  score.LongScore,
			ShortScore: score.ShortScore,
			Reason:     fmt.Sprintf("long_score %.4f >= threshold %.4f", score.LongScore, cfg.LongScoreThreshold),
		}, true
	}

	if score.ShortScore >= cfg.ShortScoreThreshold && score.ShortScore > score.LongScore {
		return tradeCandidate{
			Symbol:     score.Symbol,
			Side:       exchange.SideSell,
			LastPrice:  score.LastPrice,
			Score:      score.Score,
			LongScore:  score.LongScore,
			ShortScore: score.ShortScore,
			Reason:     fmt.Sprintf("short_score %.4f >= threshold %.4f", score.ShortScore, cfg.ShortScoreThreshold),
		}, true
	}

	return tradeCandidate{}, false
}

func desiredPositionFromCandidate(candidate tradeCandidate, ok bool) int {
	if !ok {
		return 0
	}
	if candidate.Side == exchange.SideBuy {
		return 1
	}
	if candidate.Side == exchange.SideSell {
		return -1
	}
	return 0
}

func calcRawQtyFromUSDT(usdt, price float64) float64 {
	if usdt <= 0 || price <= 0 {
		return 0
	}
	return usdt / price
}

func normalizeQty(rawQty, qtyStep float64) float64 {
	if rawQty <= 0 || qtyStep <= 0 {
		return 0
	}
	// +1e-9 защищает от float-артефактов
	steps := math.Floor(rawQty/qtyStep + 1e-9)
	normalized := steps * qtyStep
	if normalized <= 0 {
		return qtyStep
	}

	// Точное форматирование под шаг биржи
	stepStr := fmt.Sprintf("%g", qtyStep)
	decimals := 0
	if idx := strings.Index(stepStr, "."); idx != -1 {
		decimals = len(stepStr) - idx - 1
	}
	safeStr := fmt.Sprintf("%.*f", decimals, normalized)
	safeQty, _ := strconv.ParseFloat(safeStr, 64)

	return safeQty
}

func calcStopLoss(side exchange.Side, price, stopLossPercent float64) float64 {
	if price <= 0 || stopLossPercent <= 0 {
		return 0
	}

	pct := stopLossPercent / 100.0

	switch side {
	case exchange.SideBuy:
		return price * (1.0 - pct)
	case exchange.SideSell:
		return price * (1.0 + pct)
	default:
		return 0
	}
}

func barsForDays(days int, tfDur time.Duration) int {
	if days <= 0 || tfDur <= 0 {
		return 0
	}
	return int((time.Duration(days) * 24 * time.Hour) / tfDur)
}

func isAfterCandleClose(now time.Time, tf string) bool {
	tfDur, err := timeframeDuration(tf)
	if err != nil {
		return false
	}
	lastClose := now.Truncate(tfDur)
	diff := now.Sub(lastClose)
	return diff >= 2*time.Second
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

func nextExpectedClosedCandleTime(lastClosedOpen time.Time, tf string) (time.Time, error) {
	tfDur, err := timeframeDuration(tf)
	if err != nil {
		return time.Time{}, err
	}
	return lastClosedOpen.Add(2 * tfDur), nil
}

func hasActiveEntryOrder(orders []exchange.Order) bool {
	for _, o := range orders {
		if o.ReduceOnly {
			continue
		}
		switch o.Status {
		case exchange.OrderStatusNew, exchange.OrderStatusPartiallyFilled:
			return true
		}
	}
	return false
}

func buildClientOrderID(symbol string, side exchange.Side, anchor time.Time) string {
	s := strings.ReplaceAll(symbol, "USDT", "")
	return fmt.Sprintf("mkv-%s-%s-%d", strings.ToLower(s), strings.ToLower(string(side)), anchor.Unix())
}

func buildCloseOrderID(symbol string, side exchange.Side, anchor time.Time) string {
	s := strings.ReplaceAll(symbol, "USDT", "")
	return fmt.Sprintf("mkv-close-%s-%s-%d", strings.ToLower(s), strings.ToLower(string(side)), anchor.Unix())
}

func directionFromPositionSide(side exchange.PositionSide) int {
	switch side {
	case exchange.PositionSideLong:
		return 1
	case exchange.PositionSideShort:
		return -1
	default:
		return 0
	}
}

func syncStateWithExchange(
	state *symbolRuntimeState,
	position exchange.Position,
	cfg config.Config,
	anchor time.Time,
) string {
	hasExchangePos := position.Side != exchange.PositionSideNone && position.Size > 0

	// 1. Биржа закрыла позицию, а бот ещё думает, что она открыта
	if state.InPosition && !hasExchangePos {
		resetStateAfterClose(state, cfg)
		return "detected_exchange_close"
	}

	// 2. Бот не знает о позиции, а на бирже она есть (перезапуск, ручной вход и т.д.)
	if !state.InPosition && hasExchangePos {
		state.InPosition = true
		state.Direction = directionFromPositionSide(position.Side)
		state.EntryPrice = position.AvgPrice
		state.EntryAnchor = anchor
		state.HoldBars = 0
		state.LongBarsAgainst = 0
		state.ShortBarsAgainst = 0
		state.PeakPrice = position.AvgPrice // 🔥 Для трейлинг-стопа
		return "detected_existing_exchange_position"
	}

	// 3. Позиция есть у всех, обновляем PeakPrice для трейлинга
	// ✅ ПРАВИЛЬНО: Сравниваем с числами
	// Обновляем пик цены в зависимости от направления
	if state.InPosition && hasExchangePos {
		if state.EntryPrice == 0 {
			state.EntryPrice = position.AvgPrice
		}

		// Long (1): новый пик - это максимум
		if state.Direction == 1 && position.MarkPrice > state.PeakPrice {
			state.PeakPrice = position.MarkPrice
		}
		// Short (-1): новый пик - это минимум
		if state.Direction == -1 && (position.MarkPrice < state.PeakPrice || state.PeakPrice == 0) {
			state.PeakPrice = position.MarkPrice
		}
	}

	return ""
}

func resetStateAfterClose(state *symbolRuntimeState, cfg config.Config) {
	state.InPosition = false
	state.Direction = 0
	state.EntryAnchor = time.Time{}
	state.EntryPrice = 0
	state.EntryScore = 0         // <-- ADAPTIVE EXIT
	state.PeakPrice = 0          // <-- ADAPTIVE EXIT
	state.TrailingActive = false // <-- ADAPTIVE EXIT
	state.BaselineEntropy = 1.05 // <-- ADAPTIVE EXIT
	state.HoldBars = 0
	state.LongBarsAgainst = 0
	state.ShortBarsAgainst = 0
	state.CooldownBarsLeft = cfg.CooldownBars
}

// manageTrailingStop - Mark III: Тюнинг реакторов
func manageTrailingStop(
	ctx context.Context,
	tradingClient exchange.TradingClient,
	symbol string,
	state *symbolRuntimeState,
	currentPrice float64,
) {

	// ... дальше твой код
	if !state.InPosition || state.EntryPrice == 0 {
		return
	}

	// Считаем PnL
	direction := float64(state.Direction)
	unrealizedPnlPct := (currentPrice - state.EntryPrice) / state.EntryPrice * 100.0 * direction

	// Обновляем PeakPrice
	if state.Direction == 1 && currentPrice > state.PeakPrice {
		state.PeakPrice = currentPrice
	}
	if state.Direction == -1 && (currentPrice < state.PeakPrice || state.PeakPrice == 0) {
		state.PeakPrice = currentPrice
	}

	// 1) Режим "Безубыток" (+0.3%)
	if unrealizedPnlPct >= 0.3 && !state.TrailingActive {

		// 🔥 NEW: Добавляем буфер на комиссии (0.1%)
		feeBuffer := 0.001

		var newStop float64
		if state.Direction == 1 {
			// Для LONG: стоп чуть выше входа
			newStop = state.EntryPrice * (1.0 + feeBuffer)
		} else {
			// Для SHORT: стоп чуть ниже входа
			newStop = state.EntryPrice * (1.0 - feeBuffer)
		}

		err := tradingClient.SetStopLoss(ctx, exchange.StopLossRequest{
			Symbol:   symbol,
			StopLoss: newStop,
		})
		if err == nil {
			state.TrailingActive = true
			// Логируем реальный стоп, чтобы видеть разницу
			fmt.Printf("[%s] 🛡️ THRUSTERS: SL moved to True BE (Entry: %.4f -> Stop: %.4f)\n",
				symbol, state.EntryPrice, newStop)
		}
	}

	// 2) Режим "Фиксация" (+0.6%) - тянем стоп за ценой
	if unrealizedPnlPct >= 0.6 && state.TrailingActive {
		var newStop float64
		if state.Direction == 1 {
			newStop = state.PeakPrice * 0.998 // -0.2% от пика
		} else {
			newStop = state.PeakPrice * 1.002 // +0.2% от пика
		}

		// Проверяем, что новый стоп лучше старого
		shouldUpdate := false
		if state.Direction == 1 && newStop > state.EntryPrice {
			shouldUpdate = true
		}
		if state.Direction == -1 && newStop < state.EntryPrice {
			shouldUpdate = true
		}

		if shouldUpdate {
			err := tradingClient.SetStopLoss(ctx, exchange.StopLossRequest{
				Symbol:   symbol,
				StopLoss: newStop,
			})
			if err == nil {
				fmt.Printf("[%s] 🔥 THRUSTERS MAX: Trailing SL at %.4f (PnL: %.2f%%)\n", symbol, newStop, unrealizedPnlPct)
			}
		}
	}
}

func closePositionOnExchange(
	ctx context.Context,
	tradingClient exchange.TradingClient,
	symbol string,
	position exchange.Position,
	closeSide exchange.Side,
	anchor time.Time,
	reason string,
) (string, error) {
	qty := position.Size
	if qty <= 0 {
		return "", fmt.Errorf("close position: exchange size <= 0")
	}

	orderResult, err := tradingClient.PlaceOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        symbol,
		Side:          closeSide,
		OrderType:     exchange.OrderTypeMarket,
		Qty:           qty,
		TimeInForce:   exchange.TimeInForceIOC,
		ClientOrderID: buildCloseOrderID(symbol, closeSide, anchor),
		ReduceOnly:    true,
	})
	if err != nil {
		return "", fmt.Errorf("close position order failed: %w", err)
	}

	return fmt.Sprintf("reduce_only_close order_id=%s reason=%s qty=%.8f", orderResult.OrderID, reason, qty), nil
}

func formatNote(note string) string {
	if note == "" {
		return ""
	}
	return " | note=" + note
}

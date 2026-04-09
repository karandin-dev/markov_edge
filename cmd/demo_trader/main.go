package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"
	"markov_screener/internal/market"
	"markov_screener/internal/markov"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/joho/godotenv"
)

type symbolRuntimeState struct {
	Symbol                   string
	LastCandleTime           time.Time
	LastRunAt                time.Time
	LastSignalText           string
	InPosition               bool
	Direction                int // 1 long, -1 short
	EntryAnchor              time.Time
	EntryPrice               float64
	EntryScore               float64
	PeakPrice                float64
	TrailingActive           bool
	BaselineEntropy          float64
	HoldBars                 int
	LongBarsAgainst          int
	ShortBarsAgainst         int
	CooldownBarsLeft         int
	PreviouslyLoggedThruster bool
	EntryPnlWasNegative      bool
	PartialProfitTaken       bool

	// 🔥 Новые поля для MAE/MFE
	MAE float64 // Maximum Adverse Excursion (%)
	MFE float64 // Maximum Favorable Excursion (%)

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

// getPositionType возвращает строку "LONG" или "SHORT"
func getPositionType(direction int) string {
	if direction == 1 {
		return "LONG"
	}
	return "SHORT"
}

func main() {
	_ = godotenv.Load()

	activePositions := make(map[string]*markov.Position)

	// 🔥 SETUP FILE LOGGING
	os.MkdirAll("logs", 0755)
	logPath := fmt.Sprintf("logs/massacre_%s.log", time.Now().Format("20060102_150405"))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Errorf("failed to create log file: %w", err))
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logPrintf := func(format string, args ...interface{}) {
		fmt.Fprintf(multiWriter, format, args...)
	}

	logPrintf("DEMO TRADER LIVE EXECUTION\n")
	logPrintf("--------------------------------------------------\n")

	configPath := flag.String("config", "configs/screener.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	fmt.Printf("🔵 SUBFRAME: enabled=%v | tf=%s | pnl_protect=%.2f%%\n",
		cfg.SubFrame.Enabled, cfg.SubFrame.Timeframe, cfg.SubFrame.Thresholds.PnlAcceleration)
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
		cfg.Timeframe, cfg.Candles, cfg.MarkovWindow, cfg.VolWindow)
	fmt.Printf("adaptive_mode=%v | regime_filter_enabled=%v | use_dynamic_zscore=%v\n",
		cfg.AdaptiveMode, cfg.RegimeFilterEnabled, cfg.UseDynamicZScore)
	fmt.Printf("long_threshold=%.4f | short_threshold=%.4f | bootstrap_days=%d\n",
		cfg.LongScoreThreshold, cfg.ShortScoreThreshold, cfg.Runtime.BootstrapDays)
	fmt.Printf("min_hold=%d | cooldown=%d | long_exit_confirm=%d | short_exit_confirm=%d\n",
		cfg.MinHoldBars, cfg.CooldownBars, cfg.LongExitConfirmBars, cfg.ShortExitConfirmBars)
	fmt.Printf("usdt_per_trade=%.4f | stop_loss_percent=%.4f\n",
		cfg.Execution.USDTPerTrade, cfg.Execution.StopLossPercent)
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
				symbol, st.LastCandleTime.Format(time.RFC3339), err)
			continue
		}
		fmt.Printf("[%s] last_closed_open=%s | next_close_expected=%s\n",
			symbol, st.LastCandleTime.Format(time.RFC3339), nextClose.Format(time.RFC3339))
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
	runLiveLoop(rootCtx, marketClient, tradingClient, cfg, states, activePositions)
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
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("runtime state is nil")}
	}

	if !state.InPosition && state.CooldownBarsLeft > 0 {
		state.CooldownBarsLeft--
	}

	tfDur, err := timeframeDuration(cfg.Timeframe)
	if err != nil {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("invalid timeframe: %w", err)}
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
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("fetch candles: %w", err)}
	}

	filtered := make([]market.Candle, 0, len(candles))
	for _, c := range candles {
		if !c.Time.After(cycleCloseTime) {
			filtered = append(filtered, c)
		}
	}
	candles = filtered

	if len(candles) == 0 {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("no candles left after filtering to cycle close time %s", cycleCloseTime.Format(time.RFC3339))}
	}

	score, err := markov.AnalyzeSymbol(symbol, candles, cfg)
	if err != nil {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("analyze symbol: %w", err)}
	}

	// 🔵 SUB-FRAME MONITORING (5m)
	var subFrameAnalysis *markov.SubFrameAnalysis
	if cfg.SubFrame.Enabled && state.InPosition {
		tf5mDur, _ := timeframeDuration("5m")
		subStart := cycleCloseTime.Add(-time.Duration(cfg.Candles) * tf5mDur)
		candles5m, err := marketClient.FetchOHLCVRange(ctx, symbol, "5m", subStart, cycleCloseTime)
		if err == nil && len(candles5m) >= 10 {
			analysis := markov.AnalyzeSubFrame(symbol, candles, candles5m, cfg, state.Direction, state.EntryPrice, score.LastPrice)
			subFrameAnalysis = &analysis
			if analysis.EarlyExitSignal {
				logger.Debugf("[%s] SUBFRAME ALERT: %s | trend5m=%s | scoreChange=%.3f",
					symbol, analysis.Reason, analysis.Trend5m, analysis.ScoreChange)
			}
		}
	}

	candidate, hasCandidate := buildTradeCandidate(score, cfg)
	desiredPos := desiredPositionFromCandidate(candidate, hasCandidate)

	position, err := tradingClient.GetPosition(ctx, symbol)
	if err != nil {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("get position: %w", err)}
	}

	syncNote := syncStateWithExchange(state, position, cfg, cycleCloseTime)

	// ============================================================================
	// 🔥 БЛОК УПРАВЛЕНИЯ ПОЗИЦИЕЙ (РЕФАКТОРИНГ: БЕЗ ДУБЛИРОВАНИЯ)
	// ============================================================================
	if state.InPosition {
		state.HoldBars++

		// 1. Базовые расчеты (ОДИН раз)
		currentPrice := score.LastPrice
		currentScore := score.LongScore
		if state.Direction == -1 {
			currentScore = score.ShortScore
		}
		unrealizedPnlPct := (currentPrice - state.EntryPrice) / state.EntryPrice * 100.0 * float64(state.Direction)

		//Обновление MAE/MFE
		if unrealizedPnlPct < 0 && unrealizedPnlPct < state.MAE {
			state.MAE = unrealizedPnlPct // запоминаем максимальный минус
		}
		if unrealizedPnlPct > 0 && unrealizedPnlPct > state.MFE {
			state.MFE = unrealizedPnlPct // запоминаем максимальный плюс
		}

		// 2. Обновление PeakPrice
		if state.Direction == 1 && currentPrice > state.PeakPrice {
			state.PeakPrice = currentPrice
		}
		if state.Direction == -1 && (currentPrice < state.PeakPrice || state.PeakPrice == 0) {
			state.PeakPrice = currentPrice
		}

		// 3. Проверка ATR TP
		atr14 := markov.CalculateATR(candles, 14)
		atrPercent := (atr14 / state.EntryPrice) * 100.0
		tpTargetPercent := math.Max(atrPercent*1.5, 0.8)

		if unrealizedPnlPct >= tpTargetPercent {
			closeSide := exchange.SideSell
			if state.Direction == -1 {
				closeSide = exchange.SideBuy
			}
			closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position, closeSide, cycleCloseTime, "ATR_TP")
			if closeErr != nil {
				return tradeResult{Symbol: symbol, Err: closeErr}
			}
			resetStateAfterClose(state, cfg)
			posType := getPositionType(state.Direction)

			// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
			logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
				symbol, getPositionType(state.Direction),
				state.MAE, state.MFE, unrealizedPnlPct,
				"Model_Decay_TP")

			resetStateAfterClose(state, cfg)

			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf("[%s] 🎯 ATR TP HIT | type=%s | PnL: %.2f%% (Target: %.2f%%) | entry=%.4f | peak=%.4f | %s",
					symbol, posType, unrealizedPnlPct, tpTargetPercent, state.EntryPrice, state.PeakPrice, closeText),
			}

		}

		// 4. Проверка Model Decay
		scoreDrop := state.EntryScore - currentScore
		if unrealizedPnlPct > 0.3 && scoreDrop > (state.EntryScore*0.30) {
			closeSide := exchange.SideSell
			if state.Direction == -1 {
				closeSide = exchange.SideBuy
			}
			closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position, closeSide, cycleCloseTime, "Model_Decay_TP")
			if closeErr != nil {
				return tradeResult{Symbol: symbol, Err: closeErr}
			}
			resetStateAfterClose(state, cfg)
			posType := getPositionType(state.Direction)

			// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
			logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
				symbol, getPositionType(state.Direction),
				state.MAE, state.MFE, unrealizedPnlPct,
				"Model_Decay_TP")

			resetStateAfterClose(state, cfg)
			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf("[%s] 📉 MODEL DECAY TP | type=%s | PnL: %.2f%% | Score dropped from %.3f to %.3f | entry=%.4f | peak=%.4f | %s",
					symbol, posType, unrealizedPnlPct, state.EntryScore, currentScore, state.EntryPrice, state.PeakPrice, closeText),
			}
		}

		// 5. Trailing Stop
		aggressiveOffset := 0.0015
		if cfg.TakeProfit.Enabled {
			aggressiveOffset = cfg.TakeProfit.Trailing.AggressiveOffset
		}
		manageTrailingStop(ctx, tradingClient, symbol, state, currentPrice, aggressiveOffset)

		// 6. Sub-Frame Exit
		if cfg.SubFrame.Enabled && subFrameAnalysis != nil && subFrameAnalysis.EarlyExitSignal {
			shouldExit := true
			exitReason := subFrameAnalysis.Reason

			if cfg.SubFrame.Constraints.RespectMinHold && state.HoldBars < cfg.MinHoldBars {
				if unrealizedPnlPct < -cfg.SubFrame.Constraints.MaxEarlyExitLoss {
					logger.Warnf("[%s] SUBFRAME critical loss: %.2f%% < -%.2f%% (min_hold not reached)",
						symbol, unrealizedPnlPct, cfg.SubFrame.Constraints.MaxEarlyExitLoss)
				} else {
					shouldExit = false
					exitReason = fmt.Sprintf("subframe_blocked(min_hold=%d, hold=%d)", cfg.MinHoldBars, state.HoldBars)
				}
			}

			if shouldExit && unrealizedPnlPct < -cfg.SubFrame.Thresholds.PnlAcceleration {
				closeSide := exchange.SideSell
				if state.Direction == -1 {
					closeSide = exchange.SideBuy
				}
				closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position, closeSide,
					cycleCloseTime, fmt.Sprintf("subframe_exit(%s)", exitReason))
				if closeErr != nil {
					return tradeResult{Symbol: symbol, Err: closeErr}
				}
				resetStateAfterClose(state, cfg)
				posType := getPositionType(state.Direction)

				// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
				logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
					symbol, getPositionType(state.Direction),
					state.MAE, state.MFE, unrealizedPnlPct,
					"Model_Decay_TP")

				resetStateAfterClose(state, cfg)
				return tradeResult{
					Symbol: symbol,
					Text: fmt.Sprintf("[%s] SUBFRAME EXIT | type=%s | anchor=%s | pnl=%.2f%% | reason=%s | trend5m=%s | scoreChange=%.3f | entry=%.4f | peak=%.4f | %s",
						symbol, posType, cycleCloseTime.Format(time.RFC3339), unrealizedPnlPct, exitReason,
						subFrameAnalysis.Trend5m, subFrameAnalysis.ScoreChange, state.EntryPrice, state.PeakPrice, closeText),
				}
			}
		}

		// 7. Partial TP (Скейлинг)
		if cfg.TakeProfit.Enabled && cfg.TakeProfit.Partial.Enabled && !state.PartialProfitTaken {
			if unrealizedPnlPct >= cfg.TakeProfit.Partial.AtPercent {
				closeQty := position.Size * cfg.TakeProfit.Partial.CloseRatio
				instrumentInfo, err := tradingClient.GetInstrumentInfo(ctx, symbol)
				if err != nil {
					logger.Warnf("[%s] Failed to get instrument info for partial TP: %v", symbol, err)
				} else {
					closeQty = normalizeQty(closeQty, instrumentInfo.QtyStep)
				}

				closeSide := exchange.SideSell
				if state.Direction == -1 {
					closeSide = exchange.SideBuy
				}

				partialText, partialErr := closePositionOnExchangeWithQty(ctx, tradingClient, symbol, closeSide,
					closeQty, cycleCloseTime, "Partial_TP")
				if partialErr == nil {
					state.PartialProfitTaken = true
					remaining := (1 - cfg.TakeProfit.Partial.CloseRatio) * 100

					// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
					logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
						symbol, getPositionType(state.Direction),
						state.MAE, state.MFE, unrealizedPnlPct,
						"Model_Decay_TP")

					resetStateAfterClose(state, cfg)
					// ✅ ЛОГ ФЛАГА (из картинки)
					logger.Infof("[%s] 🎯 PARTIAL TP: flag set, remaining=%.0f%%", symbol, remaining)

					fmt.Printf("[%s] 🎯 PARTIAL TP HIT: Closed %.0f%% at %.2f%% profit. Letting %.0f%% ride!\n",
						symbol, cfg.TakeProfit.Partial.CloseRatio*100, unrealizedPnlPct, remaining)
					fmt.Printf("[%s] %s\n", symbol, partialText)
				}
			}
		}

		// 8. Adaptive Exit
		if state.HoldBars >= cfg.MinHoldBars {
			exitCtx := markov.ExitContext{
				EntryScore:       state.EntryScore,
				CurrentScore:     currentScore,
				UnrealizedPnlPct: unrealizedPnlPct,
				VolRatio:         score.VolRatio,
				BarsHeld:         state.HoldBars,
				Entropy:          score.Entropy,
				BaselineEntropy:  state.BaselineEntropy,
				MacroRegime:      score.MacroRegime,
			}

			confidence := markov.CalcAdaptiveExitConfidence(exitCtx)
			entropyCut := cfg.GetEntropyCut(symbol)
			if score.Entropy > entropyCut {
				confidence *= 0.9
			}

			threshold := markov.CalcAdaptiveExitThreshold(exitCtx.VolRatio, cfg.Exit.Adaptive)

			if confidence > threshold {
				closeSide := exchange.SideSell
				if state.Direction == -1 {
					closeSide = exchange.SideBuy
				}
				closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position, closeSide,
					cycleCloseTime, fmt.Sprintf("adaptive_exit(conf=%.2f>thr=%.2f)", confidence, threshold))
				if closeErr != nil {
					return tradeResult{Symbol: symbol, Err: closeErr}
				}
				resetStateAfterClose(state, cfg)
				posType := getPositionType(state.Direction)

				// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
				logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
					symbol, getPositionType(state.Direction),
					state.MAE, state.MFE, unrealizedPnlPct,
					"Model_Decay_TP")

				resetStateAfterClose(state, cfg)

				return tradeResult{
					Symbol: symbol,
					Text: fmt.Sprintf("[%s] ADAPTIVE EXIT | type=%s | anchor=%s | pnl=%.2f%% | score_entry=%.2f->%.2f | entropy=%.2f | conf=%.2f>thr=%.2f | entry=%.4f | peak=%.4f | %s",
						symbol, posType, cycleCloseTime.Format(time.RFC3339), unrealizedPnlPct,
						state.EntryScore, currentScore, score.Entropy, confidence, threshold,
						state.EntryPrice, state.PeakPrice, closeText),
				}
			}
		}

		// 9. Signal Exit (ОДИН ЧИСТЫЙ БЛОК БЕЗ ДУБЛЕЙ)
		if state.Direction == 1 {
			if state.HoldBars >= cfg.MinHoldBars {
				if desiredPos != 1 {
					state.LongBarsAgainst++
				} else {
					state.LongBarsAgainst = 0
				}
			}

			if state.HoldBars >= cfg.MinHoldBars && state.LongBarsAgainst >= cfg.LongExitConfirmBars {
				closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position,
					exchange.SideSell, cycleCloseTime, "signal_exit_long")
				if closeErr != nil {
					return tradeResult{Symbol: symbol, Err: closeErr}
				}
				resetStateAfterClose(state, cfg)

				// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
				logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
					symbol, getPositionType(state.Direction),
					state.MAE, state.MFE, unrealizedPnlPct,
					"Model_Decay_TP")

				resetStateAfterClose(state, cfg)
				// ✅ ЛОГ С PeakPrice
				return tradeResult{
					Symbol: symbol,
					Text: fmt.Sprintf("[%s] CLOSED LONG | anchor=%s | hold=%d/%d | against=%d/%d | entry=%.4f | peak=%.4f | pnl=%.2f%% | signal=%s | score=%.4f | long=%.4f | short=%.4f | %s",
						symbol, cycleCloseTime.Format(time.RFC3339), state.HoldBars, cfg.MinHoldBars,
						state.LongBarsAgainst, cfg.LongExitConfirmBars, state.EntryPrice, state.PeakPrice,
						unrealizedPnlPct, score.HumanSignal, score.Score, score.LongScore, score.ShortScore, closeText),
				}
			}

			// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
			logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
				symbol, getPositionType(state.Direction),
				state.MAE, state.MFE, unrealizedPnlPct,
				"Model_Decay_TP")

			resetStateAfterClose(state, cfg)

			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf("[%s] HOLD LONG | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
					symbol, cycleCloseTime.Format(time.RFC3339), state.HoldBars, cfg.MinHoldBars,
					state.LongBarsAgainst, cfg.LongExitConfirmBars, score.HumanSignal,
					score.Score, score.LongScore, score.ShortScore, formatNote(syncNote)),
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
				closeText, closeErr := closePositionOnExchange(ctx, tradingClient, symbol, position,
					exchange.SideBuy, cycleCloseTime, "signal_exit_short")
				if closeErr != nil {
					return tradeResult{Symbol: symbol, Err: closeErr}
				}
				resetStateAfterClose(state, cfg)
				// ✅ ЛОГ С PeakPrice

				// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
				logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
					symbol, getPositionType(state.Direction),
					state.MAE, state.MFE, unrealizedPnlPct,
					"Model_Decay_TP")

				resetStateAfterClose(state, cfg)
				return tradeResult{
					Symbol: symbol,
					Text: fmt.Sprintf("[%s] CLOSED SHORT | anchor=%s | hold=%d/%d | against=%d/%d | entry=%.4f | peak=%.4f | pnl=%.2f%% | signal=%s | score=%.4f | long=%.4f | short=%.4f | %s",
						symbol, cycleCloseTime.Format(time.RFC3339), state.HoldBars, cfg.MinHoldBars,
						state.ShortBarsAgainst, cfg.ShortExitConfirmBars, state.EntryPrice, state.PeakPrice,
						unrealizedPnlPct, score.HumanSignal, score.Score, score.LongScore, score.ShortScore, closeText),
				}
			}

			// 🔥 НОВОЕ: [STATS] лог с MAE/MFE
			logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
				symbol, getPositionType(state.Direction),
				state.MAE, state.MFE, unrealizedPnlPct,
				"Model_Decay_TP")

			resetStateAfterClose(state, cfg)

			return tradeResult{
				Symbol: symbol,
				Text: fmt.Sprintf("[%s] HOLD SHORT | anchor=%s | hold=%d/%d | against=%d/%d | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
					symbol, cycleCloseTime.Format(time.RFC3339), state.HoldBars, cfg.MinHoldBars,
					state.ShortBarsAgainst, cfg.ShortExitConfirmBars, score.HumanSignal,
					score.Score, score.LongScore, score.ShortScore, formatNote(syncNote)),
			}
		}
	}

	// ============================================================================
	// 🔥 БЛОК ВХОДА В ПОЗИЦИЮ
	// ============================================================================
	var unrealizedPnlPct float64
	if state.InPosition && position.Size > 0 {
		currentPrice := score.LastPrice
		unrealizedPnlPct = (currentPrice - state.EntryPrice) / state.EntryPrice * 100.0 * float64(state.Direction)
	}

	if state.CooldownBarsLeft > 0 {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf("[%s] COOLDOWN | anchor=%s | remaining=%d | pnl=%.2f%% | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol, cycleCloseTime.Format(time.RFC3339), state.CooldownBarsLeft,
				unrealizedPnlPct, score.HumanSignal, score.Score, score.LongScore, score.ShortScore, formatNote(syncNote)),
		}
	}

	if !hasCandidate {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf("[%s] SKIP | anchor=%s | signal=%s | score=%.4f | long=%.4f | short=%.4f%s",
				symbol, cycleCloseTime.Format(time.RFC3339), score.HumanSignal,
				score.Score, score.LongScore, score.ShortScore, formatNote(syncNote)),
		}
	}

	instrumentInfo, err := tradingClient.GetInstrumentInfo(ctx, symbol)
	if err != nil {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("get instrument info: %w", err)}
	}

	rawQty := calcRawQtyFromUSDT(cfg.Execution.USDTPerTrade, candidate.LastPrice)
	qty := normalizeQty(rawQty, instrumentInfo.QtyStep)

	if qty <= 0 {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("normalized qty <= 0: raw=%.8f step=%.8f", rawQty, instrumentInfo.QtyStep)}
	}

	if instrumentInfo.MinOrderQty > 0 && qty < instrumentInfo.MinOrderQty {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf("[%s] SKIP | anchor=%s | qty too small: raw=%.8f normalized=%.8f min_qty=%.8f step=%.8f",
				symbol, cycleCloseTime.Format(time.RFC3339), rawQty, qty, instrumentInfo.MinOrderQty, instrumentInfo.QtyStep),
		}
	}

	if instrumentInfo.MinNotional > 0 && qty*candidate.LastPrice < instrumentInfo.MinNotional {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf("[%s] SKIP | anchor=%s | notional too small: qty=%.8f price=%.4f notional=%.8f min_notional=%.8f",
				symbol, cycleCloseTime.Format(time.RFC3339), qty, candidate.LastPrice, qty*candidate.LastPrice, instrumentInfo.MinNotional),
		}
	}

	stopLoss := calcStopLoss(candidate.Side, candidate.LastPrice, cfg.Execution.StopLossPercent)

	openOrders, err := tradingClient.GetOpenOrders(ctx, symbol)
	if err != nil {
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("get open orders: %w", err)}
	}

	if hasActiveEntryOrder(openOrders) {
		return tradeResult{Symbol: symbol, Text: fmt.Sprintf("[%s] SKIP | anchor=%s | active entry order already exists", symbol, cycleCloseTime.Format(time.RFC3339))}
	}

	clientOrderID := buildClientOrderID(symbol, candidate.Side, cycleCloseTime)

	if !cfg.Execution.Enabled || cfg.Execution.DryRun {
		return tradeResult{
			Symbol: symbol,
			Text: fmt.Sprintf("[%s] DRY RUN | anchor=%s | would place %s %s | raw_qty=%.8f | qty=%.8f | step=%.8f | min_qty=%.8f | last=%.4f | stop=%.4f | score=%.4f | long=%.4f | short=%.4f | reason=%s%s",
				symbol, cycleCloseTime.Format(time.RFC3339), string(candidate.Side), symbol,
				rawQty, qty, instrumentInfo.QtyStep, instrumentInfo.MinOrderQty,
				candidate.LastPrice, stopLoss, candidate.Score, candidate.LongScore,
				candidate.ShortScore, candidate.Reason, formatNote(syncNote)),
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
		return tradeResult{Symbol: symbol, Err: fmt.Errorf("place order: %w", err)}
	}

	stopText := "stop=disabled"
	if cfg.Execution.SetStopLoss && stopLoss > 0 {
		if err := tradingClient.SetStopLoss(ctx, exchange.StopLossRequest{
			Symbol:   symbol,
			StopLoss: stopLoss,
		}); err != nil {
			return tradeResult{Symbol: symbol, Err: fmt.Errorf("order placed but set stop loss failed: order_id=%s err=%w", orderResult.OrderID, err)}
		}
		stopText = fmt.Sprintf("stop=%.4f", stopLoss)
	}

	// Инициализация состояния
	state.InPosition = true
	state.Direction = desiredPos
	state.EntryAnchor = cycleCloseTime
	state.EntryPrice = candidate.LastPrice
	state.EntryScore = candidate.Score
	state.PeakPrice = candidate.LastPrice
	state.TrailingActive = false
	state.BaselineEntropy = 1.05
	state.HoldBars = 0
	state.LongBarsAgainst = 0
	state.ShortBarsAgainst = 0
	state.PreviouslyLoggedThruster = false
	state.EntryPnlWasNegative = true

	// ✅ ЛОГ ВХОДА С effectiveThr, Entropy, VolRatio (из картинки)
	effectiveThr := cfg.GetEffectiveLongThreshold(symbol)
	if candidate.Side == exchange.SideSell {
		effectiveThr = cfg.GetEffectiveShortThreshold(symbol)
	}

	return tradeResult{
		Symbol: symbol,
		Text: fmt.Sprintf("[%s] EXECUTED | anchor=%s | placed %s %s | raw_qty=%.8f | qty=%.8f | step=%.8f | min_qty=%.8f | last=%.4f | %s | order_id=%s | score=%.4f (thr=%.4f) | entropy=%.2f | vol=%.2f | long=%.4f | short=%.4f | reason=%s%s",
			symbol, cycleCloseTime.Format(time.RFC3339), string(candidate.Side), symbol,
			rawQty, qty, instrumentInfo.QtyStep, instrumentInfo.MinOrderQty,
			candidate.LastPrice, stopText, orderResult.OrderID,
			candidate.Score, effectiveThr, score.Entropy, score.VolRatio,
			candidate.LongScore, candidate.ShortScore, candidate.Reason, formatNote(syncNote)),
	}
}

func commonCycleCloseTime(parentCtx context.Context, marketClient exchange.MarketDataClient, cfg config.Config, symbols []string) (time.Time, error) {
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
	effectiveLongThr := cfg.GetEffectiveLongThreshold(score.Symbol)
	effectiveShortThr := cfg.GetEffectiveShortThreshold(score.Symbol)

	if cfg.LongOnly && !cfg.ShortOnly {
		if score.LongScore >= effectiveLongThr {
			return tradeCandidate{
				Symbol:     score.Symbol,
				Side:       exchange.SideBuy,
				LastPrice:  score.LastPrice,
				Score:      score.Score,
				LongScore:  score.LongScore,
				ShortScore: score.ShortScore,
				Reason:     fmt.Sprintf("long_score %.4f >= threshold %.4f (profile)", score.LongScore, effectiveLongThr),
			}, true
		}
		return tradeCandidate{}, false
	}

	if cfg.ShortOnly && !cfg.LongOnly {
		if score.ShortScore >= effectiveShortThr {
			return tradeCandidate{
				Symbol:     score.Symbol,
				Side:       exchange.SideSell,
				LastPrice:  score.LastPrice,
				Score:      score.Score,
				LongScore:  score.LongScore,
				ShortScore: score.ShortScore,
				Reason:     fmt.Sprintf("short_score %.4f >= threshold %.4f (profile)", score.ShortScore, effectiveShortThr),
			}, true
		}
		return tradeCandidate{}, false
	}

	if score.LongScore >= effectiveLongThr && score.LongScore >= score.ShortScore {
		return tradeCandidate{
			Symbol:     score.Symbol,
			Side:       exchange.SideBuy,
			LastPrice:  score.LastPrice,
			Score:      score.Score,
			LongScore:  score.LongScore,
			ShortScore: score.ShortScore,
			Reason:     fmt.Sprintf("long_score %.4f >= threshold %.4f (profile)", score.LongScore, effectiveLongThr),
		}, true
	}

	if score.ShortScore >= effectiveShortThr && score.ShortScore > score.LongScore {
		return tradeCandidate{
			Symbol:     score.Symbol,
			Side:       exchange.SideSell,
			LastPrice:  score.LastPrice,
			Score:      score.Score,
			LongScore:  score.LongScore,
			ShortScore: score.ShortScore,
			Reason:     fmt.Sprintf("short_score %.4f >= threshold %.4f (profile)", score.ShortScore, effectiveShortThr),
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

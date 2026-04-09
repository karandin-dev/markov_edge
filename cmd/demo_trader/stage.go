package main

import (
	"context"
	"fmt"
	"time"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"

	"github.com/bytedance/gopkg/util/logger"
)

// ============================================================================
// 🔄 Синхронизация и Управление состоянием
// ============================================================================

// syncStateWithExchange синхронизирует локальное состояние с реальной позицией на бирже
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

	// 2. Бот не знает о позиции, а на бирже она есть (перезапуск, ручной вход)
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

// resetStateAfterClose сбрасывает состояние после закрытия сделки
func resetStateAfterClose(state *symbolRuntimeState, cfg config.Config) {
	state.InPosition = false
	state.Direction = 0
	state.EntryAnchor = time.Time{}
	state.EntryPrice = 0
	state.EntryScore = 0
	state.PeakPrice = 0
	state.TrailingActive = false
	state.BaselineEntropy = 1.05
	state.HoldBars = 0
	state.LongBarsAgainst = 0
	state.ShortBarsAgainst = 0
	state.CooldownBarsLeft = cfg.CooldownBars
	state.PartialProfitTaken = false // Сброс флага частичного TP
}

// ============================================================================
// 🚀 Трейлинг-стоп (Thrusters)
// ============================================================================

// manageTrailingStop управляет трейлинг-стопом и переходом в безубыток
// aggressiveOffset: отступ для трейлинга после срабатывания частичного TP
func manageTrailingStop(
	ctx context.Context,
	tradingClient exchange.TradingClient,
	symbol string,
	state *symbolRuntimeState,
	currentPrice float64,
	aggressiveOffset float64,
) {
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

	// Определяем текущий отступ трейлинга
	trailOffset := 0.002 // Стандартный 0.2%
	if state.PartialProfitTaken {
		trailOffset = aggressiveOffset // Агрессивный режим (например, 0.15%)
	}

	// 1) Режим "Безубыток" (+0.3%)
	if unrealizedPnlPct >= 0.3 && !state.TrailingActive {
		feeBuffer := 0.001
		var newStop float64
		if state.Direction == 1 {
			newStop = state.EntryPrice * (1.0 + feeBuffer)
		} else {
			newStop = state.EntryPrice * (1.0 - feeBuffer)
		}

		err := tradingClient.SetStopLoss(ctx, exchange.StopLossRequest{
			Symbol:   symbol,
			StopLoss: newStop,
		})
		if err == nil {
			state.TrailingActive = true
			logger.Infof("[%s] 🛡️ THRUSTERS: SL moved to True BE (Entry: %.4f -> Stop: %.4f)",
				symbol, state.EntryPrice, newStop)
		}
	}

	// 2) Режим "Трейлинг" (+0.6%)
	if unrealizedPnlPct >= 0.6 && state.TrailingActive {
		var newStop float64
		// ⚠️ ИСПОЛЬЗУЕМ trailOffset вместо хардкода
		if state.Direction == 1 {
			newStop = state.PeakPrice * (1.0 - trailOffset)
		} else {
			newStop = state.PeakPrice * (1.0 + trailOffset)
		}

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
				trailType := "standard"
				if state.PartialProfitTaken {
					trailType = "aggressive"
				}
				logger.Infof("[%s] 🔥 THRUSTERS MAX [%s]: Trailing SL at %.4f (PnL: %.2f%%, offset: %.3f)",
					symbol, trailType, newStop, unrealizedPnlPct, trailOffset)
			}
		}
	}
}

// ============================================================================
// 🕰 Работа со временем свечей
// ============================================================================

// fetchLastClosedCandleTime получает время последней закрытой свечи
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

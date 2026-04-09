package markov

import (
	"markov_screener/internal/config"
	"math"

	"github.com/bytedance/gopkg/util/logger"
)

// ExitContext содержит все метрики, необходимые для расчёта уверенности в выходе
type ExitContext struct {
	EntryScore       float64 // score на момент входа
	CurrentScore     float64 // пересчитанный score на текущем баре
	UnrealizedPnlPct float64 // нереализованный PnL в % (с учётом направления)
	VolRatio         float64 // текущая волатильность / базовая (1.0 = норма)
	BarsHeld         int
	Entropy          float64
	BaselineEntropy  float64 // средняя энтропия за последние ~100 баров
	MacroRegime      string  // "bull", "bear", "transition_up", "transition_down", "neutral"
}

// CalcAdaptiveExitConfidence возвращает значение в диапазоне [0.0, 1.0]
// Чем выше значение, тем сильнее сигнал на закрытие позиции
func CalcAdaptiveExitConfidence(ctx ExitContext) float64 {
	// 1. Декей сигнала: насколько упала уверенность с момента входа
	signalDecay := 0.0
	if ctx.EntryScore > 0.01 {
		signalDecay = math.Max(0, (ctx.EntryScore-ctx.CurrentScore)/ctx.EntryScore)
	}

	// 2. Риск/Защита по PnL
	pnlRisk := 0.0
	if ctx.UnrealizedPnlPct > 0 {
		// В плюсе: чем больше прибыль, тем агрессивнее защита
		pnlRisk = math.Min(ctx.UnrealizedPnlPct/3.0, 1.0) * 0.6
	} else {
		// В минусе: терпим, но ограничиваем просадку
		pnlRisk = math.Min(math.Abs(ctx.UnrealizedPnlPct)/2.0, 0.5) * 0.3
	}

	// 3. Энтропийный шок: резкий рост неопределённости
	entropyShock := 0.0
	if ctx.BaselineEntropy > 0 {
		ratio := ctx.Entropy / ctx.BaselineEntropy
		if ratio > 1.3 {
			entropyShock = math.Min((ratio-1.3)/0.4, 1.0)
		}
	}

	// 4. Адаптивные веса (зависят от макро-режима и волатильности)
	wSig, wPnl, wEnt := calcAdaptiveWeights(ctx.MacroRegime, ctx.VolRatio)

	// 5. Итоговая уверенность
	confidence := wSig*signalDecay + wPnl*pnlRisk + wEnt*entropyShock
	return clampF64(confidence, 0.0, 1.0)
}

func CalcAdaptiveExitThreshold(volRatio float64, cfg config.AdaptiveExitConfig) float64 {
	baseThreshold := cfg.BaseConfThreshold

	if !cfg.VolAdjustment.Enabled {
		return baseThreshold
	}

	factor := cfg.VolAdjustment.Factor
	adj := baseThreshold * (1.0 - factor*(volRatio-1.0))

	return clampF64(adj, cfg.Clamp.Min, cfg.Clamp.Max)
}

func calcAdaptiveWeights(regime string, volRatio float64) (float64, float64, float64) {
	// Базовые веса
	wS, wP, wE := 0.5, 0.3, 0.2

	// Корректировка под режим
	switch regime {
	case "bull", "bear":
		// В тренде: больше доверяем сигналу, меньше реагируем на шум PnL
		wS, wP, wE = 0.6, 0.24, 0.2
	case "transition_up", "transition_down":
		// В переходе: баланс, чуть больше внимания энтропии
		wS, wP, wE = 0.5, 0.3, 0.26
	default:
		// Нейтрал/хаос: сигнал ненадёжен, защищаемся по PnL и энтропии
		wS, wP, wE = 0.2, 0.39, 0.3
	}

	// Корректировка под волатильность
	if volRatio > 1.5 {
		wS *= 0.8
		wP *= 1.2
		wE *= 1.2
	}

	// Нормализация весов до суммы = 1.0
	sum := wS + wP + wE
	return wS / sum, wP / sum, wE / sum
}

// clampF64 ограничивает значение в диапазоне [min, max]
func clampF64(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// exit.go — при любом типе выхода
func (p *Position) Close(exitPrice float64, reason string) {
	// Финальный расчет
	if p.Side == "Long" {
		p.FinalPnL = (exitPrice - p.EntryPrice) / p.EntryPrice
	} else {
		p.FinalPnL = (p.EntryPrice - exitPrice) / p.EntryPrice
	}
	p.ExitPrice = exitPrice
	p.ExitReason = reason

	// 🔥 Ключевой лог для анализа
	logger.Infof("[STATS] %s %s | MAE:%.2f%% MFE:%.2f%% Final:%.2f%% | Reason:%s",
		p.Symbol, p.Side,
		p.MAE*100, p.MFE*100, p.FinalPnL*100,
		reason)

	// 🔥 Дополнительно: экспортируем в CSV для бэктеста
	exportTradeStats(p)
}

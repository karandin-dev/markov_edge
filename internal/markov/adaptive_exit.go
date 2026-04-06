package markov

import (
	"math"
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

// CalcAdaptiveExitThreshold динамически подстраивает порог выхода под волатильность
func CalcAdaptiveExitThreshold(volRatio, baseThreshold float64) float64 {
	// Высокая волатильность → порог ниже (выходим раньше, фиксируем прибыль)
	// Низкая волатильность → порог выше (даём позиции "дышать")
	adj := baseThreshold * (1.0 - 0.25*(volRatio-1.0))
	return clampF64(adj, 0.45, 0.80)
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

func clampF64(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

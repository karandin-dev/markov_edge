package markov

import (
	"markov_screener/internal/config"
	"markov_screener/internal/market"
	"math"
)

// calculateVolRatio считает отношение текущей волатильности к базовой
// Использует cfg.VolWindow для базового окна, recentWindow фиксирован = 20
func CalculateVolRatio(candles []market.Candle, cfg config.Config) float64 {
	const recentWindow = 20 // окно для текущей волатильности

	if len(candles) < recentWindow {
		return 1.0
	}

	// Последние N свечей для текущей волатильности
	recent := candles[max(0, len(candles)-recentWindow):]
	currentVol := CalculateRealizedVol(recent)

	// Базовая волатильность: используем окно из конфига
	baseWindow := cfg.VolWindow
	if baseWindow < 50 {
		baseWindow = 100 // дефолт, если в конфиге мало
	}

	var baseCandles []market.Candle
	if len(candles) >= baseWindow {
		baseCandles = candles[len(candles)-baseWindow:]
	} else {
		baseCandles = candles
	}

	baseVol := CalculateRealizedVol(baseCandles)

	if baseVol == 0 {
		return 1.0
	}

	return currentVol / baseVol
}

// calculateRealizedVol считает среднее абсолютное изменение цены (realized volatility)
func CalculateRealizedVol(candles []market.Candle) float64 {
	if len(candles) < 2 {
		return 0
	}

	var returns []float64
	for i := 1; i < len(candles); i++ {
		if candles[i-1].Close == 0 {
			continue
		}
		ret := math.Abs((candles[i].Close - candles[i-1].Close) / candles[i-1].Close)
		returns = append(returns, ret)
	}

	return mean(returns)
}

// determineMacroRegime определяет макро-режим по тренду
func DetermineMacroRegime(candles []market.Candle) string {
	const (
		recentSpan  = 20
		olderSpan   = 20
		olderOffset = 30
	)

	if len(candles) < recentSpan+olderSpan+olderOffset {
		return "neutral"
	}

	recent := candles[len(candles)-recentSpan:]
	older := candles[len(candles)-olderSpan-olderOffset : len(candles)-olderOffset]

	recentAvg := MeanOfCloses(recent)
	olderAvg := MeanOfCloses(older)

	if olderAvg == 0 {
		return "neutral"
	}

	change := (recentAvg - olderAvg) / olderAvg

	// Пороги можно вынести в конфиг при необходимости
	if change > 0.02 {
		return "bull"
	} else if change < -0.02 {
		return "bear"
	} else if change > 0.005 {
		return "transition_up"
	} else if change < -0.005 {
		return "transition_down"
	}

	return "neutral"
}

// meanOfCloses считает среднее значение Close
func MeanOfCloses(candles []market.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	sum := 0.0
	for _, c := range candles {
		sum += c.Close
	}
	return sum / float64(len(candles))
}

// Вспомогательные функции
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

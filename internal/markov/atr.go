package markov

import (
	"markov_screener/internal/market"
	"math"
)

type Candle = market.Candle // ← добавь эту строку

// CalculateATR считает средний истинный диапазон за период (например, 14 свечей)
func CalculateATR(candles []Candle, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}

	var trueRanges []float64
	// Берем последние period+1 свечей
	startIdx := len(candles) - (period + 1)
	for i := startIdx + 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevClose := candles[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		tr := math.Max(tr1, math.Max(tr2, tr3))
		trueRanges = append(trueRanges, tr)
	}

	sum := 0.0
	for _, tr := range trueRanges {
		sum += tr
	}
	return sum / float64(len(trueRanges))
}

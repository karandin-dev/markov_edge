package markov

import (
	"markov_screener/internal/config"
	"markov_screener/internal/market"
	"math"
)

// SubFrameAnalysis результат анализа 5m
type SubFrameAnalysis struct {
	Trend5m         string  // "up", "down", "flat"
	ScoreChange     float64 // Изменение long/short score за последние 3 свечи 5m
	VolatilitySpike bool    // Резкий рост волатильности
	EarlyExitSignal bool    // Флаг раннего выхода
	Reason          string  // Причина сигнала
}

// AnalyzeSubFrame анализирует 5m данные для позиции на 15m
func AnalyzeSubFrame(
	symbol string,
	candles15m []market.Candle,
	candles5m []market.Candle,
	cfg config.Config,
	positionDirection int, // 1 = long, -1 = short
	entryPrice float64,
	currentPrice float64,
) SubFrameAnalysis {

	result := SubFrameAnalysis{}

	if !cfg.SubFrame.Enabled || len(candles5m) < 10 {
		return result
	}

	// 1. Определяем тренд на 5m (простая SMA)
	result.Trend5m = detectTrend5m(candles5m, 9)

	// 2. Считаем изменение скора (эвристика по цене)
	result.ScoreChange = calculateScoreChange5m(candles5m, positionDirection)

	// 3. Проверяем всплеск волатильности
	result.VolatilitySpike = checkVolatilitySpike(candles5m, cfg.SubFrame.Thresholds.ScoreChangeWarning)

	// 4. Логика раннего выхода
	if cfg.SubFrame.Exit.EarlyWarning {
		result.EarlyExitSignal, result.Reason = checkEarlyExit(
			positionDirection,
			entryPrice,
			currentPrice,
			result,
			cfg,
		)
	}

	return result
}

func detectTrend5m(candles []market.Candle, period int) string {
	if len(candles) < period+3 {
		return "flat"
	}

	recent := candles[len(candles)-period:]
	older := candles[len(candles)-period*2 : len(candles)-period]

	recentAvg := meanClose(recent)
	olderAvg := meanClose(older)

	change := (recentAvg - olderAvg) / olderAvg

	if change > 0.003 {
		return "up"
	} else if change < -0.003 {
		return "down"
	}
	return "flat"
}

func calculateScoreChange5m(candles []market.Candle, direction int) float64 {
	if len(candles) < 6 {
		return 0
	}

	// Эвристика: изменение цены как прокси для score
	recent := candles[len(candles)-3:]
	older := candles[len(candles)-6 : len(candles)-3]

	recentChange := (recent[len(recent)-1].Close - recent[0].Close) / recent[0].Close
	olderChange := (older[len(older)-1].Close - older[0].Close) / older[0].Close

	// Для long: падение цены = негативное изменение скора
	change := recentChange - olderChange
	if direction == -1 {
		change = -change // инвертируем для short
	}

	return math.Abs(change) * 10 // масштабируем к диапазону score
}

func checkVolatilitySpike(candles []market.Candle, threshold float64) bool {
	if len(candles) < 10 {
		return false
	}

	// Считаем ATR-подобную метрику за последние 3 свечи
	recent := candles[len(candles)-3:]
	var ranges []float64
	for _, c := range recent {
		rng := (c.High - c.Low) / c.Close
		ranges = append(ranges, rng)
	}

	avgRange := mean(ranges)

	// Сравниваем с предыдущими 5 свечами
	older := candles[len(candles)-8 : len(candles)-3]
	var olderRanges []float64
	for _, c := range older {
		rng := (c.High - c.Low) / c.Close
		olderRanges = append(olderRanges, rng)
	}
	olderAvg := mean(olderRanges)

	if olderAvg == 0 {
		return false
	}

	// 🔥 ИСПОЛЬЗУЕМ threshold для сравнения
	volatilityRatio := avgRange / olderAvg
	return volatilityRatio > threshold // ← Вот где используется threshold!
}

func checkEarlyExit(
	direction int,
	entryPrice, currentPrice float64,
	sub SubFrameAnalysis,
	cfg config.Config,
) (bool, string) {

	pnl := (currentPrice - entryPrice) / entryPrice * 100 * float64(direction)

	// Ускоренный выход при убытке
	if cfg.SubFrame.Exit.PnlProtection && pnl < -cfg.SubFrame.Thresholds.PnlAcceleration {
		if sub.Trend5m == oppositeTrend(direction) || sub.VolatilitySpike {
			return true, "subframe_pnl_protection"
		}
	}

	// Раннее предупреждение при развороте тренда
	if cfg.SubFrame.Exit.EarlyWarning {
		if sub.Trend5m == oppositeTrend(direction) &&
			sub.ScoreChange > cfg.SubFrame.Thresholds.ScoreChangeWarning {
			return true, "subframe_trend_reversal"
		}
	}

	return false, ""
}

func oppositeTrend(direction int) string {
	if direction == 1 {
		return "down"
	}
	return "up"
}

func meanClose(candles []market.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	sum := 0.0
	for _, c := range candles {
		sum += c.Close
	}
	return sum / float64(len(candles))
}

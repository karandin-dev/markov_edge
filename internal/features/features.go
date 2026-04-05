package features

import (
	"markov_screener/internal/market"
)

func BuildFeatures(
	candles []market.Candle,
	volWindow int,
	zWeak float64,
	zStrong float64,
	extremaWindow int,
) []market.FeatureRow {

	rows := ApplyReturns(candles)
	rows = ApplyRollingVolatility(rows, volWindow)
	rows = ApplyStates(rows, zWeak, zStrong)
	rows = ApplyExtrema(rows, extremaWindow)

	return rows
}

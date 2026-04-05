package features

import "markov_screener/internal/market"

func ApplyReturns(candles []market.Candle) []market.FeatureRow {
	if len(candles) == 0 {
		return nil
	}

	rows := make([]market.FeatureRow, len(candles))

	for i, c := range candles {
		rows[i] = market.FeatureRow{
			Time:  c.Time,
			Close: c.Close,
		}

		if i == 0 || candles[i-1].Close == 0 {
			rows[i].Return = 0
			continue
		}

		rows[i].Return = (c.Close / candles[i-1].Close) - 1.0
	}

	return rows
}

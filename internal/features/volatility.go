package features

import (
	"math"

	"markov_screener/internal/market"
)

func ApplyRollingVolatility(rows []market.FeatureRow, window int) []market.FeatureRow {
	if len(rows) == 0 || window <= 1 {
		return rows
	}

	for i := range rows {
		if i < window-1 {
			rows[i].Vol = 0
			rows[i].ZReturn = 0
			continue
		}

		start := i - window + 1
		values := make([]float64, 0, window)

		for j := start; j <= i; j++ {
			values = append(values, rows[j].Return)
		}

		std := stdDev(values)
		rows[i].Vol = std

		if std > 0 {
			rows[i].ZReturn = rows[i].Return / std
		} else {
			rows[i].ZReturn = 0
		}
	}

	return rows
}

func stdDev(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	var sum float64
	for _, v := range values {
		diff := v - mean
		sum += diff * diff
	}

	return math.Sqrt(sum / float64(n))
}

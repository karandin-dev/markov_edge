package features

import "markov_screener/internal/market"

func ApplyExtrema(rows []market.FeatureRow, window int) []market.FeatureRow {
	if len(rows) == 0 || window < 1 {
		return rows
	}

	for i := range rows {
		if i < window || i+window >= len(rows) {
			continue
		}

		isMin := true
		isMax := true
		center := rows[i].Close

		for j := i - window; j <= i+window; j++ {
			if j == i {
				continue
			}

			if rows[j].Close <= center {
				isMin = false
			}
			if rows[j].Close >= center {
				isMax = false
			}
		}

		rows[i].MinFlag = isMin
		rows[i].MaxFlag = isMax
	}

	return rows
}

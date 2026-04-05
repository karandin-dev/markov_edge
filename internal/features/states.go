package features

import "markov_screener/internal/market"

func ApplyStates(rows []market.FeatureRow, weakThreshold, strongThreshold float64) []market.FeatureRow {
	for i := range rows {
		z := rows[i].ZReturn

		switch {
		case z <= -strongThreshold:
			rows[i].State = market.StrongDown
		case z <= -weakThreshold:
			rows[i].State = market.WeakDown
		case z < weakThreshold:
			rows[i].State = market.Flat
		case z < strongThreshold:
			rows[i].State = market.WeakUp
		default:
			rows[i].State = market.StrongUp
		}
	}

	return rows
}

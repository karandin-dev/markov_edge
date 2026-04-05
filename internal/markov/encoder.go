package markov

import "markov_screener/internal/market"

func ContinuationProbability(tm TransitionMatrix, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	switch current {
	case market.StrongUp, market.WeakUp:
		return tm.Probs[i][int(market.WeakUp)] + tm.Probs[i][int(market.StrongUp)]
	case market.StrongDown, market.WeakDown:
		return tm.Probs[i][int(market.WeakDown)] + tm.Probs[i][int(market.StrongDown)]
	case market.Flat:
		return tm.Probs[i][int(market.Flat)]
	default:
		return 0
	}
}

func ReversalProbability(tm TransitionMatrix, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	switch current {
	case market.StrongUp, market.WeakUp:
		return tm.Probs[i][int(market.WeakDown)] + tm.Probs[i][int(market.StrongDown)]
	case market.StrongDown, market.WeakDown:
		return tm.Probs[i][int(market.WeakUp)] + tm.Probs[i][int(market.StrongUp)]
	case market.Flat:
		return tm.Probs[i][int(market.WeakDown)] +
			tm.Probs[i][int(market.StrongDown)] +
			tm.Probs[i][int(market.WeakUp)] +
			tm.Probs[i][int(market.StrongUp)]
	default:
		return 0
	}
}

func PersistenceProbability(tm TransitionMatrix, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	return tm.Probs[i][i]
}

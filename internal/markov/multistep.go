package markov

import "markov_screener/internal/market"

func MatrixPower2(tm TransitionMatrix) [NumStates][NumStates]float64 {
	return multiplyMatrices(tm.Probs, tm.Probs)
}

func multiplyMatrices(a, b [NumStates][NumStates]float64) [NumStates][NumStates]float64 {
	var result [NumStates][NumStates]float64

	for i := 0; i < NumStates; i++ {
		for j := 0; j < NumStates; j++ {
			sum := 0.0
			for k := 0; k < NumStates; k++ {
				sum += a[i][k] * b[k][j]
			}
			result[i][j] = sum
		}
	}

	return result
}

func ContinuationProbabilityFromMatrix(matrix [NumStates][NumStates]float64, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	switch current {
	case market.StrongUp, market.WeakUp:
		return matrix[i][int(market.WeakUp)] + matrix[i][int(market.StrongUp)]
	case market.StrongDown, market.WeakDown:
		return matrix[i][int(market.WeakDown)] + matrix[i][int(market.StrongDown)]
	case market.Flat:
		return matrix[i][int(market.Flat)]
	default:
		return 0
	}
}

func ReversalProbabilityFromMatrix(matrix [NumStates][NumStates]float64, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	switch current {
	case market.StrongUp, market.WeakUp:
		return matrix[i][int(market.WeakDown)] + matrix[i][int(market.StrongDown)]
	case market.StrongDown, market.WeakDown:
		return matrix[i][int(market.WeakUp)] + matrix[i][int(market.StrongUp)]
	case market.Flat:
		return matrix[i][int(market.WeakDown)] +
			matrix[i][int(market.StrongDown)] +
			matrix[i][int(market.WeakUp)] +
			matrix[i][int(market.StrongUp)]
	default:
		return 0
	}
}

func PersistenceProbabilityFromMatrix(matrix [NumStates][NumStates]float64, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	return matrix[i][i]
}

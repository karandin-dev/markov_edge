package markov

import (
	"math"

	"markov_screener/internal/market"
)

func UpProbability(tm TransitionMatrix, current market.State) float64 {
	return UpProbabilityFromMatrix(tm.Probs, current)
}

func DownProbability(tm TransitionMatrix, current market.State) float64 {
	return DownProbabilityFromMatrix(tm.Probs, current)
}

func UpProbabilityFromMatrix(matrix [NumStates][NumStates]float64, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	return matrix[i][int(market.WeakUp)] + matrix[i][int(market.StrongUp)]
}

func DownProbabilityFromMatrix(matrix [NumStates][NumStates]float64, current market.State) float64 {
	i := int(current)
	if i < 0 || i >= NumStates {
		return 0
	}

	return matrix[i][int(market.WeakDown)] + matrix[i][int(market.StrongDown)]
}

func CalculateLongScore(
	current market.State,
	probUp1 float64,
	probDown1 float64,
	probUp2 float64,
	probDown2 float64,
	persistence float64,
	entropy float64,
) float64 {
	edge1 := probUp1 - probDown1
	edge2 := probUp2 - probDown2

	persistenceAdj := directionalPersistenceLong(current, persistence)
	entropyNorm := normalizeEntropy(entropy)

	raw :=
		0.60*edge1 +
			0.45*edge2 +
			0.10*persistenceAdj -
			0.05*entropyNorm

	raw *= 1.8

	return clamp(raw, -1.0, 1.0)
}

func CalculateShortScore(
	current market.State,
	probUp1 float64,
	probDown1 float64,
	probUp2 float64,
	probDown2 float64,
	persistence float64,
	entropy float64,
) float64 {
	edge1 := probDown1 - probUp1
	edge2 := probDown2 - probUp2

	persistenceAdj := directionalPersistenceShort(current, persistence)
	entropyNorm := normalizeEntropy(entropy)

	raw :=
		0.30*edge1 +
			0.75*edge2 +
			0.10*persistenceAdj -
			0.05*entropyNorm

	raw *= 1.8

	return clamp(raw, -1.0, 1.0)
}

func directionalPersistenceLong(current market.State, persistence float64) float64 {
	switch current {
	case market.StrongUp, market.WeakUp:
		return persistence
	case market.Flat:
		return -0.25 * persistence
	case market.WeakDown, market.StrongDown:
		return -persistence
	default:
		return 0
	}
}

func directionalPersistenceShort(current market.State, persistence float64) float64 {
	switch current {
	case market.StrongDown, market.WeakDown:
		return persistence
	case market.Flat:
		return -0.25 * persistence
	case market.WeakUp, market.StrongUp:
		return -persistence
	default:
		return 0
	}
}

func normalizeEntropy(entropy float64) float64 {
	maxEntropy := math.Log(float64(NumStates))
	if maxEntropy <= 0 {
		return 0
	}

	x := entropy / maxEntropy
	return clamp(x, 0.0, 1.0)
}

func clamp(x, minVal, maxVal float64) float64 {
	if x < minVal {
		return minVal
	}
	if x > maxVal {
		return maxVal
	}
	return x
}

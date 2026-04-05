package markov

import "math"

func RowEntropy(row [NumStates]float64) float64 {
	entropy := 0.0

	for _, p := range row {
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}

	return entropy
}

func MatrixEntropy(tm TransitionMatrix) float64 {
	total := 0.0

	for i := 0; i < NumStates; i++ {
		total += RowEntropy(tm.Probs[i])
	}

	return total / float64(NumStates)
}

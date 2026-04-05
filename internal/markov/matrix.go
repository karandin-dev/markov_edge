package markov

import (
	"errors"

	"markov_screener/internal/market"
)

const NumStates = 5

type TransitionMatrix struct {
	Counts [NumStates][NumStates]float64
	Probs  [NumStates][NumStates]float64
	Alpha  float64
	Window int
}

func BuildTransitionMatrix(rows []market.FeatureRow, window int, alpha float64) (TransitionMatrix, error) {
	var tm TransitionMatrix

	if len(rows) < 2 {
		return tm, errors.New("not enough rows to build transition matrix")
	}
	if window < 2 {
		return tm, errors.New("window must be at least 2")
	}
	if alpha < 0 {
		return tm, errors.New("alpha must be >= 0")
	}

	if window > len(rows) {
		window = len(rows)
	}

	tm.Alpha = alpha
	tm.Window = window

	start := len(rows) - window

	for i := start; i < len(rows)-1; i++ {
		from := int(rows[i].State)
		to := int(rows[i+1].State)

		if from < 0 || from >= NumStates || to < 0 || to >= NumStates {
			continue
		}

		tm.Counts[from][to]++
	}

	for i := 0; i < NumStates; i++ {
		rowSum := 0.0
		for j := 0; j < NumStates; j++ {
			rowSum += tm.Counts[i][j]
		}

		denom := rowSum + alpha*NumStates
		if denom == 0 {
			for j := 0; j < NumStates; j++ {
				tm.Probs[i][j] = 1.0 / float64(NumStates)
			}
			continue
		}

		for j := 0; j < NumStates; j++ {
			tm.Probs[i][j] = (tm.Counts[i][j] + alpha) / denom
		}
	}

	return tm, nil
}

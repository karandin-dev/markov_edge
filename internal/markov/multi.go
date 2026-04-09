package markov

import (
	"fmt"
	"math"

	"markov_screener/internal/config"
	"markov_screener/internal/features"
	"markov_screener/internal/market"
)

func AnalyzeSymbol(symbol string, candles []market.Candle, cfg config.Config) (market.SymbolScore, error) {
	var result market.SymbolScore

	if len(candles) < cfg.VolWindow+5 {
		return result, fmt.Errorf("not enough candles for %s", symbol)
	}

	rows := features.ApplyReturns(candles)
	rows = features.ApplyRollingVolatility(rows, cfg.VolWindow)
	rows = features.ApplyStates(rows, cfg.ZWeakThreshold, cfg.ZStrongThreshold)
	rows = features.ApplyExtrema(rows, cfg.ExtremaWindow)

	tm, err := BuildTransitionMatrix(rows, cfg.MarkovWindow, 0.5)
	if err != nil {
		return result, err
	}

	last := rows[len(rows)-1]

	continuation := ContinuationProbability(tm, last.State)
	reversal := ReversalProbability(tm, last.State)
	persistence := PersistenceProbability(tm, last.State)

	m2 := MatrixPower2(tm)
	continuation2 := ContinuationProbabilityFromMatrix(m2, last.State)
	reversal2 := ReversalProbabilityFromMatrix(m2, last.State)
	persistence2 := PersistenceProbabilityFromMatrix(m2, last.State)

	entropy := RowEntropy(tm.Probs[int(last.State)])

	regimeClass := ClassifyRegime(
		last.State,
		continuation,
		reversal,
		persistence,
		continuation2,
		reversal2,
		persistence2,
		entropy,
	)

	humanSignal := HumanSignal(
		last.State,
		regimeClass,
		continuation,
		reversal,
		continuation2,
		reversal2,
		entropy,
	)

	probUp1 := UpProbability(tm, last.State)
	probDown1 := DownProbability(tm, last.State)
	probUp2 := UpProbabilityFromMatrix(m2, last.State)
	probDown2 := DownProbabilityFromMatrix(m2, last.State)

	longScore := CalculateLongScore(
		last.State,
		probUp1,
		probDown1,
		probUp2,
		probDown2,
		persistence,
		entropy,
	)

	shortScore := CalculateShortScore(
		last.State,
		probUp1,
		probDown1,
		probUp2,
		probDown2,
		persistence,
		entropy,
	)

	score := math.Max(longScore, shortScore)

	result = market.SymbolScore{
		Symbol:            symbol,
		LastPrice:         last.Close,
		LastState:         last.State,
		ContinuationProb:  continuation,
		ReversalProb:      reversal,
		PersistenceProb:   persistence,
		ContinuationProb2: continuation2,
		ReversalProb2:     reversal2,
		PersistenceProb2:  persistence2,
		Entropy:           entropy,
		RegimeClass:       regimeClass,
		HumanSignal:       humanSignal,
		Score:             score,
		LongScore:         longScore,
		ShortScore:        shortScore,
		ProbUp1:           probUp1,
		ProbDown1:         probDown1,
		ProbUp2:           probUp2,
		ProbDown2:         probDown2,

		VolRatio:    CalculateVolRatio(candles, cfg), // см. ниже
		MacroRegime: DetermineMacroRegime(candles),   // см. ниже
	}

	return result, nil
}

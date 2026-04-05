package backtest

import (
	"fmt"
	"math"
	"sort"

	"markov_screener/internal/domain"
	"markov_screener/internal/dzs"
	"markov_screener/internal/features"
	"markov_screener/internal/market"
	"markov_screener/internal/markov"
)

type Observation = domain.Observation

type SignalStats = domain.SignalStats

type EquityPoint = domain.EquityPoint

type StrategyStats = domain.StrategyStats

type StrategyOptions = domain.StrategyOptions

type MacroRegime string

const (
	MacroBull           MacroRegime = "bull"
	MacroBear           MacroRegime = "bear"
	MacroTransitionUp   MacroRegime = "transition_up"
	MacroTransitionDown MacroRegime = "transition_down"
	MacroNeutral        MacroRegime = "neutral"
)

func BuildObservations(symbol string, candles []market.Candle) []Observation {
	results := make([]Observation, 0)

	const (
		startIndex         = 300
		volWindow          = 14
		extremaWindow      = 3
		markovWindow       = 200
		alpha              = 0.5
		emaPeriod          = 200
		macroStateLookback = 30
	)

	if len(candles) < startIndex+10 {
		return results
	}

	profile := ProfileForSymbol(symbol)

	for t := startIndex; t < len(candles)-6; t++ {
		history := candles[:t+1]

		rows := features.BuildFeatures(
			history,
			volWindow,
			profile.ZWeakThreshold,
			profile.ZStrongThreshold,
			extremaWindow,
		)
		if len(rows) < 50 {
			continue
		}

		tm, err := markov.BuildTransitionMatrix(rows, markovWindow, alpha)
		if err != nil {
			continue
		}

		last := rows[len(rows)-1]

		cont1 := markov.ContinuationProbability(tm, last.State)
		rev1 := markov.ReversalProbability(tm, last.State)
		pers1 := markov.PersistenceProbability(tm, last.State)

		m2 := markov.MatrixPower2(tm)
		cont2 := markov.ContinuationProbabilityFromMatrix(m2, last.State)
		rev2 := markov.ReversalProbabilityFromMatrix(m2, last.State)
		pers2 := markov.PersistenceProbabilityFromMatrix(m2, last.State)

		entropy := markov.RowEntropy(tm.Probs[int(last.State)])

		regime := markov.ClassifyRegime(
			last.State,
			cont1,
			rev1,
			pers1,
			cont2,
			rev2,
			pers2,
			entropy,
		)

		signal := markov.HumanSignal(
			last.State,
			regime,
			cont1,
			rev1,
			cont2,
			rev2,
			entropy,
		)

		probUp1 := markov.UpProbability(tm, last.State)
		probDown1 := markov.DownProbability(tm, last.State)
		probUp2 := markov.UpProbabilityFromMatrix(m2, last.State)
		probDown2 := markov.DownProbabilityFromMatrix(m2, last.State)

		longScore := markov.CalculateLongScore(
			last.State,
			probUp1,
			probDown1,
			probUp2,
			probDown2,
			pers1,
			entropy,
		)

		shortScore := markov.CalculateShortScore(
			last.State,
			probUp1,
			probDown1,
			probUp2,
			probDown2,
			pers1,
			entropy,
		)

		phase := detectMarketPhase(history, emaPeriod)
		macroRegime := detectMacroRegime(history, rows, emaPeriod, macroStateLookback)

		entryPrice := candles[t].Close
		if entryPrice == 0 {
			continue
		}

		ret1 := candles[t+1].Close/entryPrice - 1.0
		ret3 := candles[t+3].Close/entryPrice - 1.0
		ret6 := candles[t+6].Close/entryPrice - 1.0

		score := math.Max(longScore, shortScore)
		if signal == "strong long" || signal == "weak long" {
			score = longScore
		} else if signal == "strong short" || signal == "weak short" {
			score = shortScore
		}

		results = append(results, Observation{
			Time:        candles[t].Time.Unix(),
			Symbol:      symbol,
			Signal:      signal,
			State:       last.State.String(),
			Regime:      regime,
			Phase:       phase,
			MacroRegime: string(macroRegime),
			ZScore:      last.ZReturn,
			Score:       score,
			LongScore:   longScore,
			ShortScore:  shortScore,
			Entropy:     entropy,
			Return1:     ret1,
			Return3:     ret3,
			Return6:     ret6,
		})
	}

	return results
}

func AnalyzeBySignal(results []Observation) []SignalStats {
	type acc struct {
		count int
		sum1  float64
		sum3  float64
		sum6  float64
		win1  int
		win3  int
		win6  int
	}

	buckets := map[string]*acc{}

	for _, r := range results {
		if _, ok := buckets[r.Signal]; !ok {
			buckets[r.Signal] = &acc{}
		}

		a := buckets[r.Signal]
		a.count++
		a.sum1 += r.Return1
		a.sum3 += r.Return3
		a.sum6 += r.Return6

		if r.Return1 > 0 {
			a.win1++
		}
		if r.Return3 > 0 {
			a.win3++
		}
		if r.Return6 > 0 {
			a.win6++
		}
	}

	stats := make([]SignalStats, 0, len(buckets))
	for signal, a := range buckets {
		if a.count == 0 {
			continue
		}

		stats = append(stats, SignalStats{
			Signal:   signal,
			Count:    a.count,
			AvgRet1:  a.sum1 / float64(a.count),
			AvgRet3:  a.sum3 / float64(a.count),
			AvgRet6:  a.sum6 / float64(a.count),
			WinRate1: float64(a.win1) / float64(a.count),
			WinRate3: float64(a.win3) / float64(a.count),
			WinRate6: float64(a.win6) / float64(a.count),
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].AvgRet3 > stats[j].AvgRet3
	})

	return stats
}

func PrintSignalStats(stats []SignalStats) {
	fmt.Println()
	fmt.Println("BACKTEST RESULTS BY SIGNAL")
	fmt.Println("------------------------------------------------------------------------------------------------")
	fmt.Printf("%-15s %-8s %-12s %-12s %-12s %-12s %-12s %-12s\n",
		"SIGNAL", "COUNT", "AVG_RET_1", "AVG_RET_3", "AVG_RET_6", "WINRATE_1", "WINRATE_3", "WINRATE_6")
	fmt.Println("------------------------------------------------------------------------------------------------")

	for _, s := range stats {
		fmt.Printf("%-15s %-8d %-12.5f %-12.5f %-12.5f %-12.2f %-12.2f %-12.2f\n",
			s.Signal,
			s.Count,
			s.AvgRet1,
			s.AvgRet3,
			s.AvgRet6,
			s.WinRate1*100,
			s.WinRate3*100,
			s.WinRate6*100,
		)
	}
}

func detectMarketPhase(candles []market.Candle, emaPeriod int) string {
	if len(candles) < emaPeriod {
		return "neutral"
	}

	ema := emaFromCandles(candles, emaPeriod)
	lastClose := candles[len(candles)-1].Close

	if lastClose > ema {
		return "bull"
	}
	if lastClose < ema {
		return "bear"
	}
	return "neutral"
}

func emaFromCandles(candles []market.Candle, period int) float64 {
	if len(candles) == 0 {
		return 0
	}
	if len(candles) < period {
		return candles[len(candles)-1].Close
	}

	alpha := 2.0 / float64(period+1)

	start := len(candles) - period
	ema := candles[start].Close

	for i := start + 1; i < len(candles); i++ {
		ema = alpha*candles[i].Close + (1.0-alpha)*ema
	}

	return ema
}

func detectMacroRegime(candles []market.Candle, rows []market.FeatureRow, emaPeriod int, stateLookback int) MacroRegime {
	if len(candles) < emaPeriod+10 || len(rows) < stateLookback {
		return MacroNeutral
	}

	currentEMA := emaFromCandles(candles, emaPeriod)

	slopeLookback := 10
	prevEnd := len(candles) - slopeLookback
	if prevEnd <= emaPeriod {
		return MacroNeutral
	}

	prevEMA := emaFromCandles(candles[:prevEnd], emaPeriod)

	lastClose := candles[len(candles)-1].Close
	emaSlope := currentEMA - prevEMA

	start := len(rows) - stateLookback
	upCount := 0
	downCount := 0
	flatCount := 0

	for i := start; i < len(rows); i++ {
		switch rows[i].State {
		case market.StrongUp, market.WeakUp:
			upCount++
		case market.StrongDown, market.WeakDown:
			downCount++
		case market.Flat:
			flatCount++
		}
	}

	total := upCount + downCount + flatCount
	if total == 0 {
		return MacroNeutral
	}

	upRatio := float64(upCount) / float64(total)
	downRatio := float64(downCount) / float64(total)
	flatRatio := float64(flatCount) / float64(total)

	if lastClose < currentEMA && emaSlope < 0 && downRatio >= 0.38 && downRatio > upRatio {
		return MacroBear
	}

	if lastClose > currentEMA && emaSlope > 0 && upRatio >= 0.38 && upRatio > downRatio {
		return MacroBull
	}

	if emaSlope >= 0 && upRatio >= 0.28 && upRatio > downRatio*0.8 {
		return MacroTransitionUp
	}

	if emaSlope <= 0 && downRatio >= 0.28 && downRatio > upRatio*0.8 {
		return MacroTransitionDown
	}

	if flatRatio >= 0.45 {
		return MacroNeutral
	}

	return MacroNeutral
}

func dynamicZConfig(opts StrategyOptions) dzs.Config {
	return dzs.Config{
		Percentile: opts.DynamicZPercentile,
		Window:     opts.DynamicZWindow,
		Fallback:   dynamicZFallbackCut(opts),
		MinCut:     dynamicZMinCut(opts),
		MaxCut:     dynamicZMaxCut(opts),
		MinSamples: dynamicZMinSamples(opts),
	}
}

func dynamicZFallbackCut(opts StrategyOptions) float64 {
	if opts.DynamicZFallbackCut > 0 {
		return opts.DynamicZFallbackCut
	}
	if opts.ExtremeZScoreCut > 0 {
		return opts.ExtremeZScoreCut
	}
	return 2.20
}

func dynamicZMinCut(opts StrategyOptions) float64 {
	if opts.DynamicZMinCut > 0 {
		return opts.DynamicZMinCut
	}
	return 1.70
}

func dynamicZMaxCut(opts StrategyOptions) float64 {
	if opts.DynamicZMaxCut > 0 {
		return opts.DynamicZMaxCut
	}
	return 3.00
}

func dynamicZMinSamples(opts StrategyOptions) int {
	if opts.DynamicZMinSamples > 0 {
		return opts.DynamicZMinSamples
	}
	return 50
}

func effectiveExtremeZCut(obs Observation, recent []Observation, opts StrategyOptions) float64 {
	if !opts.UseDynamicZScore {
		return opts.ExtremeZScoreCut
	}
	history := make([]float64, 0, len(recent)+1)
	for _, r := range recent {
		history = append(history, r.ZScore)
	}
	history = append(history, obs.ZScore)
	return dzs.DynamicZCutWithConfig(history, dynamicZConfig(opts))
}

func regimeEntropyCut(obs Observation, opts StrategyOptions) float64 {
	if opts.RegimeEntropyCut > 0 {
		return opts.RegimeEntropyCut
	}
	profile := ProfileForSymbol(obs.Symbol)
	if profile.EntropyCut > 0 {
		return profile.EntropyCut
	}
	return 0
}

func passesRegimeFilter(obs Observation, recent []Observation, opts StrategyOptions, zCut float64) bool {
	if !opts.RegimeFilterEnabled {
		return true
	}

	if cut := regimeEntropyCut(obs, opts); cut > 0 && obs.Entropy >= cut {
		return false
	}

	if zCut > 0 && math.Abs(obs.ZScore) >= zCut {
		return false
	}

	if zCut > 0 && opts.PostExtremeBars > 0 && len(recent) > 0 {
		start := len(recent) - opts.PostExtremeBars
		if start < 0 {
			start = 0
		}
		for i := start; i < len(recent); i++ {
			if math.Abs(recent[i].ZScore) >= zCut {
				return false
			}
		}
	}

	return true
}

func getDesiredPositionWithContext(obs Observation, recent []Observation, opts StrategyOptions) int {
	zCut := effectiveExtremeZCut(obs, recent, opts)
	if !passesRegimeFilter(obs, recent, opts, zCut) {
		return 0
	}
	return getDesiredPosition(obs, opts)
}

func getDesiredPosition(obs Observation, opts StrategyOptions) int {
	profile := ProfileForSymbol(obs.Symbol)

	entropyCut := opts.EntropyCut
	if profile.EntropyCut > 0 {
		entropyCut = profile.EntropyCut
	}

	longThr := opts.LongScoreThreshold
	if profile.LongScoreThreshold > 0 {
		longThr = profile.LongScoreThreshold
	}

	shortThr := opts.ShortScoreThreshold
	if profile.ShortScoreThreshold > 0 {
		shortThr = profile.ShortScoreThreshold
	}

	if obs.Entropy > entropyCut {
		return 0
	}

	signalIsLong := obs.Signal == "strong long" || obs.Signal == "weak long"
	signalIsShort := obs.Signal == "strong short" || obs.Signal == "weak short"

	if signalIsLong && obs.LongScore < longThr {
		return 0
	}
	if signalIsShort && obs.ShortScore < shortThr {
		return 0
	}

	base := 0

	if opts.AdaptiveMode {
		switch obs.MacroRegime {
		case string(MacroBear):
			if signalIsShort {
				base = -1
			}

		case string(MacroTransitionUp):
			if obs.Signal == "strong long" {
				base = 1
			} else if obs.Signal == "weak long" && (obs.Regime == "up_unstable" || obs.Regime == "trend_long") {
				base = 1
			} else {
				base = 0
			}

		case string(MacroBull):
			if signalIsLong {
				base = 1
			}

		case string(MacroTransitionDown):
			if obs.Signal == "strong short" {
				base = -1
			} else if obs.Signal == "weak short" && (obs.Regime == "down_unstable" || obs.Regime == "trend_short") {
				base = -1
			} else {
				base = 0
			}

		default:
			base = 0
		}
	} else {
		switch obs.Signal {
		case "strong long", "weak long":
			base = 1
		case "strong short", "weak short":
			base = -1
		default:
			base = 0
		}
	}

	if opts.ShortOnly && base == 1 {
		return 0
	}
	if opts.LongOnly && base == -1 {
		return 0
	}

	if opts.UsePhaseFilter && !opts.AdaptiveMode {
		if obs.Phase == "bear" && base == 1 {
			return 0
		}
		if obs.Phase == "bull" && base == -1 {
			return 0
		}
	}

	return base
}

func RunStrategyBacktest(candles []market.Candle, observations []Observation, opts StrategyOptions) ([]EquityPoint, StrategyStats) {
	points := make([]EquityPoint, 0)
	stats := StrategyStats{}

	if opts.MinHoldBars < 0 {
		opts.MinHoldBars = 0
	}
	if opts.CooldownBars < 0 {
		opts.CooldownBars = 0
	}
	if opts.LongExitConfirmBars < 1 {
		opts.LongExitConfirmBars = 1
	}
	if opts.ShortExitConfirmBars < 1 {
		opts.ShortExitConfirmBars = 1
	}

	if len(candles) < 2 || len(observations) < 2 {
		return points, stats
	}

	obsMap := make(map[int64]Observation, len(observations))
	obsIndexMap := make(map[int64]int, len(observations))
	for i, obs := range observations {
		obsMap[obs.Time] = obs
		obsIndexMap[obs.Time] = i
	}

	equity := 1.0
	peak := 1.0

	position := 0
	entryPrice := 0.0

	totalFees := 0.0
	tradeReturns := make([]float64, 0)
	tradeBars := make([]int, 0)

	tradeOpen := false
	tradeEquity := 1.0
	currentTradeBars := 0

	cooldownLeft := 0

	sumWins := 0.0
	sumLosses := 0.0
	winCount := 0
	lossCount := 0

	longBarsAgainst := 0
	shortBarsAgainst := 0

	for i := 1; i < len(candles); i++ {
		if cooldownLeft > 0 {
			cooldownLeft--
		}

		prevCandle := candles[i-1]
		currCandle := candles[i]

		obs, hasObs := obsMap[prevCandle.Time.Unix()]
		desiredPosition := 0
		signal := ""
		regime := ""
		phase := "neutral"

		if hasObs {
			recent := make([]Observation, 0)
			if idx, ok := obsIndexMap[prevCandle.Time.Unix()]; ok {
				lookback := opts.PostExtremeBars
				if opts.UseDynamicZScore && opts.DynamicZWindow > lookback {
					lookback = opts.DynamicZWindow
				}
				if lookback > 0 {
					start := idx - lookback
					if start < 0 {
						start = 0
					}
					recent = observations[start:idx]
				}
			}
			desiredPosition = getDesiredPositionWithContext(obs, recent, opts)
			signal = obs.Signal
			regime = obs.Regime
			phase = obs.Phase
		}

		if tradeOpen && position != 0 {
			currentTradeBars++
		}

		if position != 0 && entryPrice > 0 {
			if position == 1 {
				stopPrice := entryPrice * (1.0 - opts.StopLossPct)
				if currCandle.Low <= stopPrice {
					barRet := stopPrice/prevCandle.Close - 1.0
					netRet := barRet - opts.Commission

					totalFees += opts.Commission
					equity *= (1.0 + netRet)
					tradeEquity *= (1.0 + netRet)

					points = append(points, EquityPoint{
						Time:       currCandle.Time.Unix(),
						Signal:     signal,
						Regime:     regime,
						Phase:      phase,
						Position:   position,
						Price:      stopPrice,
						BarReturn:  barRet,
						NetReturn:  netRet,
						Equity:     equity,
						TradeFee:   opts.Commission,
						StopHit:    true,
						ExitReason: "stop_loss",
					})

					finalTradeRet := tradeEquity - 1.0
					tradeReturns = append(tradeReturns, finalTradeRet)
					tradeBars = append(tradeBars, currentTradeBars)

					if finalTradeRet > 0 {
						stats.WinningTrades++
						sumWins += finalTradeRet
						winCount++
					} else {
						sumLosses += -finalTradeRet
						lossCount++
					}

					stats.Trades++
					stats.StoppedTrades++

					position = 0
					entryPrice = 0.0
					tradeOpen = false
					tradeEquity = 1.0
					currentTradeBars = 0
					cooldownLeft = opts.CooldownBars
					longBarsAgainst = 0
					shortBarsAgainst = 0

					updateDrawdown(&peak, equity, &stats.MaxDrawdown)
					continue
				}
			}

			if position == -1 {
				stopPrice := entryPrice * (1.0 + opts.StopLossPct)
				if currCandle.High >= stopPrice {
					barRet := prevCandle.Close/stopPrice - 1.0
					netRet := barRet - opts.Commission

					totalFees += opts.Commission
					equity *= (1.0 + netRet)
					tradeEquity *= (1.0 + netRet)

					points = append(points, EquityPoint{
						Time:       currCandle.Time.Unix(),
						Signal:     signal,
						Regime:     regime,
						Phase:      phase,
						Position:   position,
						Price:      stopPrice,
						BarReturn:  barRet,
						NetReturn:  netRet,
						Equity:     equity,
						TradeFee:   opts.Commission,
						StopHit:    true,
						ExitReason: "stop_loss",
					})

					finalTradeRet := tradeEquity - 1.0
					tradeReturns = append(tradeReturns, finalTradeRet)
					tradeBars = append(tradeBars, currentTradeBars)

					if finalTradeRet > 0 {
						stats.WinningTrades++
						sumWins += finalTradeRet
						winCount++
					} else {
						sumLosses += -finalTradeRet
						lossCount++
					}

					stats.Trades++
					stats.StoppedTrades++

					position = 0
					entryPrice = 0.0
					tradeOpen = false
					tradeEquity = 1.0
					currentTradeBars = 0
					cooldownLeft = opts.CooldownBars
					longBarsAgainst = 0
					shortBarsAgainst = 0

					updateDrawdown(&peak, equity, &stats.MaxDrawdown)
					continue
				}
			}
		}

		canExitBySignal := currentTradeBars >= opts.MinHoldBars

		effectiveDesiredPosition := desiredPosition

		if position != 0 {
			if !canExitBySignal {
				effectiveDesiredPosition = position
			} else {
				switch position {
				case 1:
					if desiredPosition != 1 {
						longBarsAgainst++
					} else {
						longBarsAgainst = 0
					}

					if longBarsAgainst < opts.LongExitConfirmBars {
						effectiveDesiredPosition = 1
					}

				case -1:
					if desiredPosition != -1 {
						shortBarsAgainst++
					} else {
						shortBarsAgainst = 0
					}

					if shortBarsAgainst < opts.ShortExitConfirmBars {
						effectiveDesiredPosition = -1
					}
				}
			}
		} else {
			longBarsAgainst = 0
			shortBarsAgainst = 0
		}

		if position != effectiveDesiredPosition {
			if position != 0 {
				var barRet float64
				if position == 1 {
					barRet = currCandle.Close/prevCandle.Close - 1.0
				} else {
					barRet = prevCandle.Close/currCandle.Close - 1.0
				}

				netRet := barRet - opts.Commission
				totalFees += opts.Commission
				equity *= (1.0 + netRet)
				tradeEquity *= (1.0 + netRet)

				points = append(points, EquityPoint{
					Time:       currCandle.Time.Unix(),
					Signal:     signal,
					Regime:     regime,
					Phase:      phase,
					Position:   0,
					Price:      currCandle.Close,
					BarReturn:  barRet,
					NetReturn:  netRet,
					Equity:     equity,
					TradeFee:   opts.Commission,
					StopHit:    false,
					ExitReason: "signal_exit",
				})

				finalTradeRet := tradeEquity - 1.0
				tradeReturns = append(tradeReturns, finalTradeRet)
				tradeBars = append(tradeBars, currentTradeBars)

				if finalTradeRet > 0 {
					stats.WinningTrades++
					sumWins += finalTradeRet
					winCount++
				} else {
					sumLosses += -finalTradeRet
					lossCount++
				}

				stats.Trades++

				position = 0
				entryPrice = 0.0
				tradeOpen = false
				tradeEquity = 1.0
				currentTradeBars = 0
				cooldownLeft = opts.CooldownBars
				longBarsAgainst = 0
				shortBarsAgainst = 0
			}

			if effectiveDesiredPosition != 0 && cooldownLeft == 0 {
				position = effectiveDesiredPosition
				entryPrice = currCandle.Close
				tradeOpen = true
				tradeEquity = 1.0
				currentTradeBars = 0
				longBarsAgainst = 0
				shortBarsAgainst = 0

				equity *= (1.0 - opts.Commission)
				tradeEquity *= (1.0 - opts.Commission)
				totalFees += opts.Commission

				points = append(points, EquityPoint{
					Time:       currCandle.Time.Unix(),
					Signal:     signal,
					Regime:     regime,
					Phase:      phase,
					Position:   position,
					Price:      currCandle.Close,
					BarReturn:  0,
					NetReturn:  -opts.Commission,
					Equity:     equity,
					TradeFee:   opts.Commission,
					StopHit:    false,
					ExitReason: "entry",
				})
			}
		} else if position != 0 && tradeOpen {
			var barRet float64
			if position == 1 {
				barRet = currCandle.Close/prevCandle.Close - 1.0
			} else {
				barRet = prevCandle.Close/currCandle.Close - 1.0
			}

			netRet := barRet
			equity *= (1.0 + netRet)
			tradeEquity *= (1.0 + netRet)

			points = append(points, EquityPoint{
				Time:       currCandle.Time.Unix(),
				Signal:     signal,
				Regime:     regime,
				Phase:      phase,
				Position:   position,
				Price:      currCandle.Close,
				BarReturn:  barRet,
				NetReturn:  netRet,
				Equity:     equity,
				TradeFee:   0,
				StopHit:    false,
				ExitReason: "",
			})
		}

		updateDrawdown(&peak, equity, &stats.MaxDrawdown)
	}

	stats.FinalEquity = equity
	stats.TotalReturn = equity - 1.0
	stats.TotalFees = totalFees

	if stats.Trades > 0 {
		stats.AvgFeePerTrade = totalFees / float64(stats.Trades)
	}

	if len(tradeReturns) > 0 {
		sumRet := 0.0
		sumBars := 0
		for i := range tradeReturns {
			sumRet += tradeReturns[i]
			sumBars += tradeBars[i]
		}
		stats.AvgTradeReturn = sumRet / float64(len(tradeReturns))
		stats.WinRate = float64(stats.WinningTrades) / float64(len(tradeReturns))
		stats.AvgTradeBars = float64(sumBars) / float64(len(tradeBars))
	}

	if winCount > 0 {
		stats.AvgWin = sumWins / float64(winCount)
	}
	if lossCount > 0 {
		stats.AvgLoss = sumLosses / float64(lossCount)
	}
	if sumLosses > 0 {
		stats.ProfitFactor = sumWins / sumLosses
	}

	return points, stats
}

func updateDrawdown(peak *float64, equity float64, maxDD *float64) {
	if equity > *peak {
		*peak = equity
	}
	dd := 1.0 - equity/(*peak)
	if dd > *maxDD {
		*maxDD = dd
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func PrintStrategyStats(stats StrategyStats) {
	fmt.Println()
	fmt.Println("STRATEGY BACKTEST")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Trades:            %d\n", stats.Trades)
	fmt.Printf("Winning Trades:    %d\n", stats.WinningTrades)
	fmt.Printf("Stopped Trades:    %d\n", stats.StoppedTrades)
	fmt.Printf("Total Return:      %.2f%%\n", stats.TotalReturn*100)
	fmt.Printf("Final Equity:      %.4f\n", stats.FinalEquity)
	fmt.Printf("Win Rate:          %.2f%%\n", stats.WinRate*100)
	fmt.Printf("Avg Trade Return:  %.2f%%\n", stats.AvgTradeReturn*100)
	fmt.Printf("Avg Trade Bars:    %.2f\n", stats.AvgTradeBars)
	fmt.Printf("Avg Win:           %.2f%%\n", stats.AvgWin*100)
	fmt.Printf("Avg Loss:          %.2f%%\n", stats.AvgLoss*100)
	fmt.Printf("Profit Factor:     %.2f\n", stats.ProfitFactor)
	fmt.Printf("Max Drawdown:      %.2f%%\n", stats.MaxDrawdown*100)
	fmt.Printf("Total Fees:        %.2f%%\n", stats.TotalFees*100)
	fmt.Printf("Avg Fee / Trade:   %.4f%%\n", stats.AvgFeePerTrade*100)
}

func PrintEligibleSignalStats(observations []Observation, opts StrategyOptions) {
	rawLong := 0
	rawShort := 0
	finalLong := 0
	finalShort := 0

	macroBull := 0
	macroBear := 0
	macroNeutral := 0

	for i, obs := range observations {
		if obs.Signal == "strong long" || obs.Signal == "weak long" {
			rawLong++
		}
		if obs.Signal == "strong short" || obs.Signal == "weak short" {
			rawShort++
		}

		switch obs.MacroRegime {
		case string(MacroBull):
			macroBull++
		case string(MacroBear):
			macroBear++
		default:
			macroNeutral++
		}

		lookback := opts.PostExtremeBars
		if opts.UseDynamicZScore && opts.DynamicZWindow > lookback {
			lookback = opts.DynamicZWindow
		}
		start := i - lookback
		if start < 0 {
			start = 0
		}

		recent := observations[start:i]

		pos := getDesiredPositionWithContext(obs, recent, opts)
		if pos == 1 {
			finalLong++
		}
		if pos == -1 {
			finalShort++
		}
	}

	fmt.Println()
	fmt.Println("FILTER DEBUG")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Raw long signals:           %d\n", rawLong)
	fmt.Printf("Raw short signals:          %d\n", rawShort)
	fmt.Printf("Macro bull bars:            %d\n", macroBull)
	fmt.Printf("Macro bear bars:            %d\n", macroBear)
	fmt.Printf("Macro neutral bars:         %d\n", macroNeutral)
	fmt.Printf("Final tradable long sigs:   %d\n", finalLong)
	fmt.Printf("Final tradable short sigs:  %d\n", finalShort)
}

func PrintScoreStats(observations []Observation) {
	var shortScores []float64
	var longScores []float64

	for _, obs := range observations {
		if obs.Signal == "strong short" || obs.Signal == "weak short" {
			shortScores = append(shortScores, obs.ShortScore)
		}
		if obs.Signal == "strong long" || obs.Signal == "weak long" {
			longScores = append(longScores, obs.LongScore)
		}
	}

	printOne := func(name string, vals []float64) {
		if len(vals) == 0 {
			fmt.Printf("%s: no data\n", name)
			return
		}

		sort.Float64s(vals)

		sum := 0.0
		for _, v := range vals {
			sum += v
		}

		p25 := vals[len(vals)/4]
		p50 := vals[len(vals)/2]
		p75 := vals[(len(vals)*3)/4]

		fmt.Printf("%s\n", name)
		fmt.Printf("  count: %d\n", len(vals))
		fmt.Printf("  min:   %.4f\n", vals[0])
		fmt.Printf("  p25:   %.4f\n", p25)
		fmt.Printf("  p50:   %.4f\n", p50)
		fmt.Printf("  p75:   %.4f\n", p75)
		fmt.Printf("  max:   %.4f\n", vals[len(vals)-1])
		fmt.Printf("  avg:   %.4f\n", sum/float64(len(vals)))
	}

	fmt.Println()
	fmt.Println("SCORE DEBUG")
	fmt.Println("--------------------------------------------------")
	printOne("SHORT scores", shortScores)
	printOne("LONG scores", longScores)
}

func PrintShortPipelineDebug(observations []Observation, opts StrategyOptions) {
	rawShort := 0
	afterEntropy := 0
	afterMacro := 0
	afterScore := 0
	final := 0

	for _, obs := range observations {
		isShort := obs.Signal == "strong short" || obs.Signal == "weak short"
		if !isShort {
			continue
		}
		rawShort++

		profile := ProfileForSymbol(obs.Symbol)
		entropyCut := opts.EntropyCut
		if profile.EntropyCut > 0 {
			entropyCut = profile.EntropyCut
		}
		shortThr := opts.ShortScoreThreshold
		if profile.ShortScoreThreshold > 0 {
			shortThr = profile.ShortScoreThreshold
		}

		if obs.Entropy > entropyCut {
			continue
		}
		afterEntropy++

		macroOK := false
		if opts.AdaptiveMode {
			switch obs.MacroRegime {
			case string(MacroBear), string(MacroTransitionDown):
				macroOK = true
			default:
				macroOK = false
			}
		} else {
			macroOK = true
		}
		if !macroOK {
			continue
		}
		afterMacro++

		if obs.ShortScore < shortThr {
			continue
		}
		afterScore++

		final++
	}

	fmt.Println()
	fmt.Println("SHORT PIPELINE DEBUG")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Raw short:        %d\n", rawShort)
	fmt.Printf("After entropy:    %d\n", afterEntropy)
	fmt.Printf("After macro:      %d\n", afterMacro)
	fmt.Printf("After score:      %d\n", afterScore)
	fmt.Printf("Final short:      %d\n", final)
}

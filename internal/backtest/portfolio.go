package backtest

import (
	"fmt"
	"sort"

	"markov_screener/internal/domain"
	"markov_screener/internal/dzs"
	"markov_screener/internal/market"
)

type PortfolioOptions = domain.PortfolioOptions

type PortfolioPosition = domain.PortfolioPosition

type PortfolioTrade = domain.PortfolioTrade

type PortfolioEquityPoint = domain.PortfolioEquityPoint

type PortfolioStats = domain.PortfolioStats

type symbolState struct {
	Symbol       string
	Candles      []market.Candle
	Observations []Observation
	ObsMap       map[int64]Observation
	ObsIndexMap  map[int64]int
}

type portfolioEvent struct {
	Time   int64
	Symbol string
	Index  int
}

func RunPortfolioBacktest(
	data map[string][]market.Candle,
	observations map[string][]Observation,
	popts PortfolioOptions,
) ([]PortfolioTrade, []PortfolioEquityPoint, PortfolioStats) {
	collector := dzs.NewCollector()
	zHistory := make(map[string][]float64)

	stats := PortfolioStats{
		InitialBalance: popts.InitialBalance,
		FinalBalance:   popts.InitialBalance,
	}

	if popts.InitialBalance <= 0 {
		return nil, nil, stats
	}
	if popts.RiskPerTrade <= 0 {
		return nil, nil, stats
	}
	if popts.MaxPortfolioRisk <= 0 {
		return nil, nil, stats
	}

	states := make(map[string]*symbolState)
	events := make([]portfolioEvent, 0)

	for symbol, candles := range data {
		obsList := observations[symbol]
		if len(candles) < 2 || len(obsList) == 0 {
			continue
		}

		obsMap := make(map[int64]Observation, len(obsList))
		obsIndexMap := make(map[int64]int, len(obsList))
		for i, o := range obsList {
			obsMap[o.Time] = o
			obsIndexMap[o.Time] = i
		}

		states[symbol] = &symbolState{
			Symbol:       symbol,
			Candles:      candles,
			Observations: obsList,
			ObsMap:       obsMap,
			ObsIndexMap:  obsIndexMap,
		}

		for i := 1; i < len(candles); i++ {
			events = append(events, portfolioEvent{
				Time:   candles[i].Time.Unix(),
				Symbol: symbol,
				Index:  i,
			})
		}
	}

	if len(events) == 0 {
		return nil, nil, stats
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Time == events[j].Time {
			return events[i].Symbol < events[j].Symbol
		}
		return events[i].Time < events[j].Time
	})

	balance := popts.InitialBalance
	peakBalance := balance

	openPositions := make(map[string]*PortfolioPosition)
	cooldowns := make(map[string]int)

	trades := make([]PortfolioTrade, 0)
	equity := make([]PortfolioEquityPoint, 0)

	totalFees := 0.0
	sumWins := 0.0
	sumLossesAbs := 0.0
	bestTrade := 0.0
	worstTrade := 0.0
	firstTrade := true

	for _, ev := range events {
		st := states[ev.Symbol]
		if st == nil {
			continue
		}
		if ev.Index <= 0 || ev.Index >= len(st.Candles) {
			continue
		}

		prev := st.Candles[ev.Index-1]
		curr := st.Candles[ev.Index]

		if cooldowns[ev.Symbol] > 0 {
			cooldowns[ev.Symbol]--
		}

		obs, hasObs := st.ObsMap[prev.Time.Unix()]
		desiredPos := 0

		if hasObs {
			symbol := ev.Symbol

			// DZS по каждому символу отдельно, в том же пространстве abs(Z), что и regime filter
			zHistory[symbol] = append(zHistory[symbol], obs.ZScore)
			zCut := dzs.DynamicZCutWithConfig(zHistory[symbol], dynamicZConfig(popts.StrategyOptions))
			collector.Add(symbol, prev.Time, zCut)

			recent := make([]Observation, 0)
			if idx, ok := st.ObsIndexMap[prev.Time.Unix()]; ok && popts.StrategyOptions.PostExtremeBars > 0 {
				start := idx - popts.StrategyOptions.PostExtremeBars
				if start < 0 {
					start = 0
				}
				recent = st.Observations[start:idx]
			}

			desiredPos = getDesiredPositionWithContext(obs, recent, popts.StrategyOptions)

			if !popts.AllowLong && desiredPos == 1 {
				desiredPos = 0
			}
			if !popts.AllowShort && desiredPos == -1 {
				desiredPos = 0
			}
		}

		pos := openPositions[ev.Symbol]

		// 1. Обновляем/закрываем открытую позицию
		if pos != nil {
			pos.HoldBars++

			closed, trade, feeUSD := processPortfolioPosition(
				pos,
				curr,
				prev,
				desiredPos,
				popts.StrategyOptions,
			)

			if feeUSD > 0 {
				balance -= feeUSD
				totalFees += feeUSD
			}

			if closed {
				balance += trade.PnlUSD
				trades = append(trades, trade)

				if trade.PnlUSD >= 0 {
					stats.WinningTrades++
					sumWins += trade.PnlUSD
				} else {
					stats.LosingTrades++
					sumLossesAbs += -trade.PnlUSD
				}

				if firstTrade {
					bestTrade = trade.PnlUSD
					worstTrade = trade.PnlUSD
					firstTrade = false
				} else {
					if trade.PnlUSD > bestTrade {
						bestTrade = trade.PnlUSD
					}
					if trade.PnlUSD < worstTrade {
						worstTrade = trade.PnlUSD
					}
				}

				delete(openPositions, ev.Symbol)
				cooldowns[ev.Symbol] = popts.StrategyOptions.CooldownBars
				pos = nil
			}
		}

		// 2. Открываем новую позицию, если можно
		if pos == nil && desiredPos != 0 && cooldowns[ev.Symbol] == 0 {
			currentReserved := totalReservedRisk(openPositions)
			riskAmount := balance * popts.RiskPerTrade
			maxAllowedRisk := balance * popts.MaxPortfolioRisk

			if currentReserved+riskAmount <= maxAllowedRisk {
				sizeUSD := riskAmount / popts.StrategyOptions.StopLossPct
				if sizeUSD > balance {
					sizeUSD = balance
				}

				if sizeUSD > 0 {
					entryFee := sizeUSD * popts.StrategyOptions.Commission
					if entryFee < balance {
						balance -= entryFee
						totalFees += entryFee

						openPositions[ev.Symbol] = &PortfolioPosition{
							Symbol:          ev.Symbol,
							Direction:       desiredPos,
							EntryTime:       curr.Time.Unix(),
							EntryPrice:      curr.Close,
							EntryIndex:      ev.Index,
							SizeUSD:         sizeUSD,
							RiskAmountUSD:   riskAmount,
							ReservedRisk:    riskAmount,
							HoldBars:        0,
							EntryZScore:     obs.ZScore,
							EntryEntropy:    obs.Entropy,
							EntryState:      obs.State,
							EntryLongScore:  obs.LongScore,
							EntryShortScore: obs.ShortScore,
						}
					}
				}
			}
		}

		if balance > peakBalance {
			peakBalance = balance
		}
		dd := 0.0
		if peakBalance > 0 {
			dd = 1.0 - balance/peakBalance
		}
		if dd > stats.MaxDrawdownPct {
			stats.MaxDrawdownPct = dd
		}

		equity = append(equity, PortfolioEquityPoint{
			Time:            curr.Time.Unix(),
			Balance:         balance,
			OpenPositions:   len(openPositions),
			ReservedRiskUSD: totalReservedRisk(openPositions),
			RealizedPnlUSD:  balance - popts.InitialBalance,
		})
	}

	stats.Trades = len(trades)
	stats.FinalBalance = balance
	stats.TotalFeesUSD = totalFees
	stats.TotalReturnPct = 0
	if popts.InitialBalance > 0 {
		stats.TotalReturnPct = (balance/popts.InitialBalance - 1.0) * 100
	}
	if stats.Trades > 0 {
		stats.WinRatePct = float64(stats.WinningTrades) / float64(stats.Trades) * 100
		sumTrades := 0.0
		for _, t := range trades {
			sumTrades += t.PnlUSD
		}
		stats.AvgTradeUSD = sumTrades / float64(stats.Trades)
	}
	if stats.WinningTrades > 0 {
		stats.AvgWinUSD = sumWins / float64(stats.WinningTrades)
	}
	if stats.LosingTrades > 0 {
		stats.AvgLossUSD = sumLossesAbs / float64(stats.LosingTrades)
	}
	if sumLossesAbs > 0 {
		stats.ProfitFactor = sumWins / sumLossesAbs
	}
	stats.BestTradeUSD = bestTrade
	stats.WorstTradeUSD = worstTrade

	fmt.Println("\nDYNAMIC Z-SCORE STATS")
	collector.Print()

	return trades, equity, stats
}

func processPortfolioPosition(
	pos *PortfolioPosition,
	curr market.Candle,
	prev market.Candle,
	desiredPos int,
	opts StrategyOptions,
) (bool, PortfolioTrade, float64) {
	var trade PortfolioTrade
	exitFee := 0.0

	canExitBySignal := pos.HoldBars >= opts.MinHoldBars

	// stop-loss
	if pos.Direction == 1 {
		stopPrice := pos.EntryPrice * (1.0 - opts.StopLossPct)
		if curr.Low <= stopPrice {
			pnlPct := stopPrice/pos.EntryPrice - 1.0
			pnlUSD := pos.SizeUSD * pnlPct
			exitFee = pos.SizeUSD * opts.Commission

			trade = PortfolioTrade{
				Symbol:          pos.Symbol,
				Direction:       pos.Direction,
				EntryTime:       pos.EntryTime,
				ExitTime:        curr.Time.Unix(),
				EntryPrice:      pos.EntryPrice,
				ExitPrice:       stopPrice,
				SizeUSD:         pos.SizeUSD,
				PnlUSD:          pnlUSD,
				PnlPctOnPos:     pnlPct * 100,
				Reason:          "stop_loss",
				HoldBars:        pos.HoldBars,
				FeeUSD:          exitFee,
				EntryZScore:     pos.EntryZScore,
				EntryEntropy:    pos.EntryEntropy,
				EntryState:      pos.EntryState,
				EntryLongScore:  pos.EntryLongScore,
				EntryShortScore: pos.EntryShortScore,
			}
			return true, trade, exitFee
		}
	} else {
		stopPrice := pos.EntryPrice * (1.0 + opts.StopLossPct)
		if curr.High >= stopPrice {
			pnlPct := pos.EntryPrice/stopPrice - 1.0
			pnlUSD := pos.SizeUSD * pnlPct
			exitFee = pos.SizeUSD * opts.Commission

			trade = PortfolioTrade{
				Symbol:          pos.Symbol,
				Direction:       pos.Direction,
				EntryTime:       pos.EntryTime,
				ExitTime:        curr.Time.Unix(),
				EntryPrice:      pos.EntryPrice,
				ExitPrice:       stopPrice,
				SizeUSD:         pos.SizeUSD,
				PnlUSD:          pnlUSD,
				PnlPctOnPos:     pnlPct * 100,
				Reason:          "stop_loss",
				HoldBars:        pos.HoldBars,
				FeeUSD:          exitFee,
				EntryZScore:     pos.EntryZScore,
				EntryEntropy:    pos.EntryEntropy,
				EntryState:      pos.EntryState,
				EntryLongScore:  pos.EntryLongScore,
				EntryShortScore: pos.EntryShortScore,
			}
			return true, trade, exitFee
		}
	}

	// exit confirm logic
	effectiveDesired := desiredPos
	if !canExitBySignal {
		effectiveDesired = pos.Direction
	} else {
		if pos.Direction == 1 {
			if desiredPos != 1 {
				pos.LongBarsAgainst++
			} else {
				pos.LongBarsAgainst = 0
			}
			if pos.LongBarsAgainst < opts.LongExitConfirmBars {
				effectiveDesired = 1
			}
		} else {
			if desiredPos != -1 {
				pos.ShortBarsAgainst++
			} else {
				pos.ShortBarsAgainst = 0
			}
			if pos.ShortBarsAgainst < opts.ShortExitConfirmBars {
				effectiveDesired = -1
			}
		}
	}

	if effectiveDesired != pos.Direction {
		var exitPrice float64
		var pnlPct float64

		exitPrice = curr.Close
		if pos.Direction == 1 {
			pnlPct = exitPrice/pos.EntryPrice - 1.0
		} else {
			pnlPct = pos.EntryPrice/exitPrice - 1.0
		}

		pnlUSD := pos.SizeUSD * pnlPct
		exitFee = pos.SizeUSD * opts.Commission

		trade = PortfolioTrade{
			Symbol:          pos.Symbol,
			Direction:       pos.Direction,
			EntryTime:       pos.EntryTime,
			ExitTime:        curr.Time.Unix(),
			EntryPrice:      pos.EntryPrice,
			ExitPrice:       exitPrice,
			SizeUSD:         pos.SizeUSD,
			PnlUSD:          pnlUSD,
			PnlPctOnPos:     pnlPct * 100,
			Reason:          "signal_exit",
			HoldBars:        pos.HoldBars,
			FeeUSD:          exitFee,
			EntryZScore:     pos.EntryZScore,
			EntryEntropy:    pos.EntryEntropy,
			EntryState:      pos.EntryState,
			EntryLongScore:  pos.EntryLongScore,
			EntryShortScore: pos.EntryShortScore,
		}
		return true, trade, exitFee
	}

	_ = prev
	return false, PortfolioTrade{}, 0
}

func totalReservedRisk(open map[string]*PortfolioPosition) float64 {
	sum := 0.0
	for _, p := range open {
		sum += p.ReservedRisk
	}
	return sum
}

func PrintPortfolioStats(stats PortfolioStats) {
	fmt.Println()
	fmt.Println("PORTFOLIO BACKTEST")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("Initial Balance:    $%.2f\n", stats.InitialBalance)
	fmt.Printf("Final Balance:      $%.2f\n", stats.FinalBalance)
	fmt.Printf("Total Return:       %.2f%%\n", stats.TotalReturnPct)
	fmt.Printf("Max Drawdown:       %.2f%%\n", stats.MaxDrawdownPct*100)
	fmt.Printf("Trades:             %d\n", stats.Trades)
	fmt.Printf("Winning Trades:     %d\n", stats.WinningTrades)
	fmt.Printf("Losing Trades:      %d\n", stats.LosingTrades)
	fmt.Printf("Win Rate:           %.2f%%\n", stats.WinRatePct)
	fmt.Printf("Profit Factor:      %.2f\n", stats.ProfitFactor)
	fmt.Printf("Avg Trade:          $%.2f\n", stats.AvgTradeUSD)
	fmt.Printf("Avg Win:            $%.2f\n", stats.AvgWinUSD)
	fmt.Printf("Avg Loss:           $%.2f\n", stats.AvgLossUSD)
	fmt.Printf("Total Fees:         $%.2f\n", stats.TotalFeesUSD)
	fmt.Printf("Best Trade:         $%.2f\n", stats.BestTradeUSD)
	fmt.Printf("Worst Trade:        $%.2f\n", stats.WorstTradeUSD)
}

func PrintPortfolioTradeBreakdown(trades []PortfolioTrade) {
	if len(trades) == 0 {
		fmt.Println("No trades.")
		return
	}

	type row struct {
		Symbol string
		Trades int
		Wins   int
		PnlUSD float64
		Fees   float64
	}

	perSymbol := make(map[string]*row)

	for _, t := range trades {
		r, ok := perSymbol[t.Symbol]
		if !ok {
			r = &row{Symbol: t.Symbol}
			perSymbol[t.Symbol] = r
		}

		r.Trades++
		if t.PnlUSD >= 0 {
			r.Wins++
		}

		r.PnlUSD += t.PnlUSD
		r.Fees += t.FeeUSD
	}

	rows := make([]row, 0, len(perSymbol))
	for _, r := range perSymbol {
		rows = append(rows, *r)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].PnlUSD > rows[j].PnlUSD
	})

	fmt.Println()
	fmt.Println("PORTFOLIO SYMBOL BREAKDOWN")
	fmt.Println("--------------------------------------------------------------------------")
	fmt.Printf("%-10s %-10s %-10s %-12s %-12s %-12s\n",
		"SYMBOL", "TRADES", "WIN%", "GROSS_PNL", "FEES", "NET_PNL")

	for _, r := range rows {
		winRate := 0.0
		if r.Trades > 0 {
			winRate = float64(r.Wins) / float64(r.Trades) * 100
		}

		net := r.PnlUSD - r.Fees

		fmt.Printf("%-10s %-10d %-10.2f %-12.2f %-12.2f %-12.2f\n",
			r.Symbol,
			r.Trades,
			winRate,
			r.PnlUSD,
			r.Fees,
			net,
		)
	}

	fmt.Println()
	fmt.Println("PORTFOLIO SYMBOL BREAKDOWN")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("%-10s %-10s %-10s %-10s\n", "SYMBOL", "TRADES", "WIN%", "PNL_USD")
	for _, r := range rows {
		winRate := 0.0
		if r.Trades > 0 {
			winRate = float64(r.Wins) / float64(r.Trades) * 100
		}
		fmt.Printf("%-10s %-10d %-10.2f %-10.2f\n",
			r.Symbol, r.Trades, winRate, r.PnlUSD)
	}
}

package backtest

func (c StrategyConfig) ToOptions() StrategyOptions {
	return StrategyOptions{
		Commission:           c.Commission,
		StopLossPct:          c.StopLossPct,
		UsePhaseFilter:       c.UsePhaseFilter,
		ShortOnly:            c.ShortOnly,
		LongOnly:             c.LongOnly,
		AdaptiveMode:         c.AdaptiveMode,
		MinHoldBars:          c.MinHoldBars,
		CooldownBars:         c.CooldownBars,
		LongExitConfirmBars:  c.LongExitConfirmBars,
		ShortExitConfirmBars: c.ShortExitConfirmBars,

		RegimeFilterEnabled: c.RegimeFilterEnabled,
		ExtremeZScoreCut:    c.ExtremeZScoreCut,
		RegimeEntropyCut:    c.RegimeEntropyCut,
		PostExtremeBars:     c.PostExtremeBars,

		UseDynamicZScore:    c.UseDynamicZScore,
		DynamicZWindow:      c.DynamicZWindow,
		DynamicZPercentile:  c.DynamicZPercentile,
		DynamicZFallbackCut: c.DynamicZFallbackCut,
		DynamicZMinCut:      c.DynamicZMinCut,
		DynamicZMaxCut:      c.DynamicZMaxCut,
		DynamicZMinSamples:  c.DynamicZMinSamples,
	}
}

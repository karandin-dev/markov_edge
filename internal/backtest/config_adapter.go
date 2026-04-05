package backtest

import "markov_screener/internal/config"

func NewStrategyOptionsFromConfig(cfg config.Config) StrategyOptions {
	return StrategyOptions{
		Commission:           cfg.Commission,
		StopLossPct:          cfg.StopLossPct,
		EntropyCut:           cfg.RegimeEntropyCut,
		UsePhaseFilter:       cfg.UsePhaseFilter,
		ShortOnly:            cfg.ShortOnly,
		LongOnly:             cfg.LongOnly,
		AdaptiveMode:         cfg.AdaptiveMode,
		MinHoldBars:          cfg.MinHoldBars,
		CooldownBars:         cfg.CooldownBars,
		LongExitConfirmBars:  cfg.LongExitConfirmBars,
		ShortExitConfirmBars: cfg.ShortExitConfirmBars,

		ShortScoreThreshold: cfg.ShortScoreThreshold,
		LongScoreThreshold:  cfg.LongScoreThreshold,

		RegimeFilterEnabled: cfg.RegimeFilterEnabled,
		ExtremeZScoreCut:    cfg.ExtremeZScoreCut,
		RegimeEntropyCut:    cfg.RegimeEntropyCut,
		PostExtremeBars:     cfg.PostExtremeBars,

		UseDynamicZScore:    cfg.UseDynamicZScore,
		DynamicZWindow:      cfg.DynamicZWindow,
		DynamicZPercentile:  cfg.DynamicZPercentile,
		DynamicZFallbackCut: cfg.DynamicZFallbackCut,
		DynamicZMinCut:      cfg.DynamicZMinCut,
		DynamicZMaxCut:      cfg.DynamicZMaxCut,
		DynamicZMinSamples:  cfg.DynamicZMinSamples,
	}
}

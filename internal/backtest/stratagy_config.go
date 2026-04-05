package backtest

type StrategyConfig struct {
	Profile string `yaml:"profile"`

	Commission           float64 `yaml:"commission"`
	StopLossPct          float64 `yaml:"stop_loss_pct"`
	UsePhaseFilter       bool    `yaml:"use_phase_filter"`
	ShortOnly            bool    `yaml:"short_only"`
	LongOnly             bool    `yaml:"long_only"`
	AdaptiveMode         bool    `yaml:"adaptive_mode"`
	MinHoldBars          int     `yaml:"min_hold_bars"`
	CooldownBars         int     `yaml:"cooldown_bars"`
	LongExitConfirmBars  int     `yaml:"long_exit_confirm_bars"`
	ShortExitConfirmBars int     `yaml:"short_exit_confirm_bars"`

	RegimeFilterEnabled bool    `yaml:"regime_filter_enabled"`
	ExtremeZScoreCut    float64 `yaml:"extreme_zscore_cut"`
	RegimeEntropyCut    float64 `yaml:"regime_entropy_cut"`
	PostExtremeBars     int     `yaml:"post_extreme_bars"`

	UseDynamicZScore    bool    `yaml:"use_dynamic_zscore"`
	DynamicZWindow      int     `yaml:"dynamic_z_window"`
	DynamicZPercentile  float64 `yaml:"dynamic_z_percentile"`
	DynamicZFallbackCut float64 `yaml:"dynamic_z_fallback_cut"`
	DynamicZMinCut      float64 `yaml:"dynamic_z_min_cut"`
	DynamicZMaxCut      float64 `yaml:"dynamic_z_max_cut"`
	DynamicZMinSamples  int     `yaml:"dynamic_z_min_samples"`
}

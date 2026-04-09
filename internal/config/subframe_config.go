package config

type SubFrameConfig struct {
	Enabled     bool                `yaml:"enabled"`
	Timeframe   string              `yaml:"timeframe"`
	Exit        SubFrameExitConfig  `yaml:"exit"`
	Thresholds  SubFrameThresholds  `yaml:"thresholds"`
	Constraints SubFrameConstraints `yaml:"constraints"`
}

type SubFrameExitConfig struct {
	EarlyWarning    bool `yaml:"early_warning"`
	PnlProtection   bool `yaml:"pnl_protection"`
	TrailRefinement bool `yaml:"trail_refinement"`
}

type SubFrameThresholds struct {
	ScoreChangeWarning float64 `yaml:"score_change_warning"`
	PnlAcceleration    float64 `yaml:"pnl_acceleration"`
	ConfirmBars        int     `yaml:"confirm_bars"`
}

type SubFrameConstraints struct {
	RespectMinHold   bool    `yaml:"respect_min_hold"`
	MaxEarlyExitLoss float64 `yaml:"max_early_exit_loss"`
}

package config

type ExitConfig struct {
	Adaptive AdaptiveExitConfig `yaml:"adaptive"`
	Signal   SignalExitConfig   `yaml:"signal"`
	MinHold  int                `yaml:"min_hold_bars"`
}

type AdaptiveExitConfig struct {
	Enabled           bool                `yaml:"enabled"`
	BaseConfThreshold float64             `yaml:"base_conf_threshold"`
	VolAdjustment     VolAdjustmentConfig `yaml:"vol_adjustment"`
	Clamp             ClampConfig         `yaml:"clamp"`
}

type VolAdjustmentConfig struct {
	Enabled bool    `yaml:"enabled"`
	Factor  float64 `yaml:"factor"`
}

type ClampConfig struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type SignalExitConfig struct {
	LongConfirmBars  int `yaml:"long_confirm_bars"`
	ShortConfirmBars int `yaml:"short_confirm_bars"`
}

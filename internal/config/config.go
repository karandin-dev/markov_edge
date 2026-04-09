package config

import "strings"

type Config struct {
	Timeframe        string   `yaml:"timeframe"`
	Symbols          []string `yaml:"symbols"`
	Candles          int      `yaml:"candles"`
	VolWindow        int      `yaml:"vol_window"`
	MarkovWindow     int      `yaml:"markov_window"`
	ExtremaWindow    int      `yaml:"extrema_window"`
	IncludeShorts    bool     `yaml:"include_shorts"`
	Commission       float64  `yaml:"commission"`
	ZStrongThreshold float64  `yaml:"z_strong_threshold"`
	ZWeakThreshold   float64  `yaml:"z_weak_threshold"`
	Workers          int      `yaml:"workers"`

	StopLossPct          float64 `yaml:"stop_loss_pct"`
	UsePhaseFilter       bool    `yaml:"use_phase_filter"`
	ShortOnly            bool    `yaml:"short_only"`
	LongOnly             bool    `yaml:"long_only"`
	AdaptiveMode         bool    `yaml:"adaptive_mode"`
	MinHoldBars          int     `yaml:"min_hold_bars"`
	CooldownBars         int     `yaml:"cooldown_bars"`
	LongExitConfirmBars  int     `yaml:"long_exit_confirm_bars"`
	ShortExitConfirmBars int     `yaml:"short_exit_confirm_bars"`

	ShortScoreThreshold float64 `yaml:"short_score_threshold"`
	LongScoreThreshold  float64 `yaml:"long_score_threshold"`

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

	Exchange  ExchangeConfig  `yaml:"exchange"`
	Execution ExecutionConfig `yaml:"execution"`

	Runtime RuntimeConfig `yaml:"runtime"`
	Exit    ExitConfig    `yaml:"exit"` //

	SubFrame SubFrameConfig `yaml:"subframe"`

	// 🔵 Take Profit настройки
	TakeProfit TakeProfitConfig `yaml:"take_profit"`

	// Индивидуальные профили
	Profiles map[string]CoinProfile `yaml:"Profile"`
}

type RuntimeConfig struct {
	BootstrapDays       int  `yaml:"bootstrap_days"`
	RunOnStart          bool `yaml:"run_on_start"`
	PollIntervalSeconds int  `yaml:"poll_interval_seconds"`
}

type ExchangeConfig struct {
	Provider string `yaml:"provider"`
	Env      string `yaml:"env"`
	Category string `yaml:"category"`
	BaseURL  string `yaml:"base_url"`
}

type ExecutionConfig struct {
	Enabled         bool    `yaml:"enabled"`
	DryRun          bool    `yaml:"dry_run"`
	PositionMode    string  `yaml:"position_mode"`
	OrderType       string  `yaml:"order_type"`
	USDTPerTrade    float64 `yaml:"usdt_per_trade"`
	ReduceOnlyExit  bool    `yaml:"reduce_only_exit"`
	SetStopLoss     bool    `yaml:"set_stop_loss"`
	StopLossPercent float64 `yaml:"stop_loss_percent"`
}

func Default() Config {
	cfg := Config{
		Timeframe:        "1h",
		Candles:          300,
		VolWindow:        14,
		MarkovWindow:     200,
		ExtremaWindow:    3,
		IncludeShorts:    true,
		Commission:       0.001,
		ZStrongThreshold: 1.2,
		ZWeakThreshold:   0.3,
		Workers:          4,

		Exchange: ExchangeConfig{
			Provider: "bybit",
			Env:      "mainnet",
			Category: "linear",
			BaseURL:  "https://api.bybit.com",
		},
		Execution: ExecutionConfig{
			Enabled:         false,
			DryRun:          true,
			PositionMode:    "one_way",
			OrderType:       "market",
			USDTPerTrade:    50,
			ReduceOnlyExit:  true,
			SetStopLoss:     true,
			StopLossPercent: 1.5,
		},
		Runtime: RuntimeConfig{
			BootstrapDays:       7,
			RunOnStart:          true,
			PollIntervalSeconds: 5,
		},
	}

	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Runtime.BootstrapDays <= 0 {
		cfg.Runtime.BootstrapDays = 7
	}
	if cfg.Runtime.PollIntervalSeconds <= 0 {
		cfg.Runtime.PollIntervalSeconds = 5
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	if cfg.Exchange.Provider == "" {
		cfg.Exchange.Provider = "bybit"
	}
	if cfg.Exchange.Env == "" {
		cfg.Exchange.Env = "mainnet"
	}
	if cfg.Exchange.Category == "" {
		cfg.Exchange.Category = "linear"
	}
	if cfg.Exchange.BaseURL == "" {
		switch cfg.Exchange.Env {
		case "demo":
			cfg.Exchange.BaseURL = "https://api-demo.bybit.com"
		default:
			cfg.Exchange.BaseURL = "https://api.bybit.com"
		}
	}

	if cfg.Execution.PositionMode == "" {
		cfg.Execution.PositionMode = "one_way"
	}
	if cfg.Execution.OrderType == "" {
		cfg.Execution.OrderType = "market"
	}
	if cfg.Execution.USDTPerTrade <= 0 {
		cfg.Execution.USDTPerTrade = 50
	}
	if cfg.Execution.StopLossPercent <= 0 && cfg.StopLossPct > 0 {
		cfg.Execution.StopLossPercent = cfg.StopLossPct
	}
}

type TakeProfitConfig struct {
	Enabled  bool             `yaml:"enabled"`
	Partial  PartialTPConfig  `yaml:"partial"`
	Trailing TrailingTPConfig `yaml:"trailing"`
}

type PartialTPConfig struct {
	Enabled    bool    `yaml:"enabled"`
	AtPercent  float64 `yaml:"at_percent"`
	CloseRatio float64 `yaml:"close_ratio"`
}

type TrailingTPConfig struct {
	AggressiveOffset float64 `yaml:"aggressive_offset"`
}

// CoinProfile — индивидуальные настройки для монеты
type CoinProfile struct {
	ZWeakThreshold      float64 `yaml:"ZWeakThreshold"`
	ZStrongThreshold    float64 `yaml:"ZStrongThreshold"`
	EntropyCut          float64 `yaml:"EntropyCut"`
	ShortScoreThreshold float64 `yaml:"ShortScoreThreshold"`
	LongScoreThreshold  float64 `yaml:"LongScoreThreshold"`
}

// GetEffectiveLongThreshold возвращает порог для long с учётом профиля монеты
func (cfg *Config) GetEffectiveLongThreshold(symbol string) float64 {
	// Убираем USDT суффикс для поиска в профиле
	symbolKey := strings.TrimSuffix(symbol, "USDT")

	if profile, ok := cfg.Profiles[symbolKey]; ok && profile.LongScoreThreshold > 0 {
		return profile.LongScoreThreshold
	}
	// Фоллбэк на глобальный порог
	return cfg.LongScoreThreshold
}

// GetEffectiveShortThreshold — аналогично для short
func (cfg *Config) GetEffectiveShortThreshold(symbol string) float64 {
	symbolKey := strings.TrimSuffix(symbol, "USDT")

	if profile, ok := cfg.Profiles[symbolKey]; ok && profile.ShortScoreThreshold > 0 {
		return profile.ShortScoreThreshold
	}
	return cfg.ShortScoreThreshold
}

// GetEntropyCut — порог энтропии для монеты
func (cfg *Config) GetEntropyCut(symbol string) float64 {
	symbolKey := strings.TrimSuffix(symbol, "USDT")

	if profile, ok := cfg.Profiles[symbolKey]; ok && profile.EntropyCut > 0 {
		return profile.EntropyCut
	}
	return 1.6 // дефолт
}

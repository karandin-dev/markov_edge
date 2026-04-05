package backtest

type AssetProfile struct {
	ZWeakThreshold      float64
	ZStrongThreshold    float64
	EntropyCut          float64
	ShortScoreThreshold float64
	LongScoreThreshold  float64
}

type ProfileConfig struct {
	BTC  AssetProfile
	ETH  AssetProfile
	SOL  AssetProfile
	LINK AssetProfile
	HBAR AssetProfile
	ALT  AssetProfile
}

var currentProfileConfig = DefaultProfileConfig()

func DefaultProfileConfig() ProfileConfig {
	return ConservativeProfileConfig()
}

func ConservativeProfileConfig() ProfileConfig {
	return ProfileConfig{
		BTC: AssetProfile{
			ZWeakThreshold:      0.30,
			ZStrongThreshold:    1.20,
			EntropyCut:          1.55,
			ShortScoreThreshold: 0.10,
			LongScoreThreshold:  0.12,
		},
		ETH: AssetProfile{
			ZWeakThreshold:      0.55,
			ZStrongThreshold:    1.70,
			EntropyCut:          1.30,
			ShortScoreThreshold: 0.14,
			LongScoreThreshold:  0.17,
		},
		SOL: AssetProfile{
			ZWeakThreshold:      0.55,
			ZStrongThreshold:    1.70,
			EntropyCut:          1.30,
			ShortScoreThreshold: 0.14,
			LongScoreThreshold:  0.17,
		},
		LINK: AssetProfile{
			ZWeakThreshold:      0.55,
			ZStrongThreshold:    1.70,
			EntropyCut:          1.30,
			ShortScoreThreshold: 0.14,
			LongScoreThreshold:  0.17,
		},
		HBAR: AssetProfile{
			ZWeakThreshold:      0.55,
			ZStrongThreshold:    1.70,
			EntropyCut:          1.30,
			ShortScoreThreshold: 0.14,
			LongScoreThreshold:  0.17,
		},
		ALT: AssetProfile{
			ZWeakThreshold:      0.55,
			ZStrongThreshold:    1.70,
			EntropyCut:          1.30,
			ShortScoreThreshold: 0.14,
			LongScoreThreshold:  0.17,
		},
	}
}

func SetProfileConfig(cfg ProfileConfig) {
	currentProfileConfig = cfg
}

func GetProfileConfig() ProfileConfig {
	return currentProfileConfig
}

func ProfileForSymbol(symbol string) AssetProfile {
	switch symbol {
	case "BTCUSDT":
		return currentProfileConfig.BTC
	case "ETHUSDT":
		return currentProfileConfig.ETH
	case "SOLUSDT":
		return currentProfileConfig.SOL
	case "LINKUSDT":
		return currentProfileConfig.LINK
	case "HBARUSDT":
		return currentProfileConfig.HBAR
	default:
		return currentProfileConfig.ALT
	}
}

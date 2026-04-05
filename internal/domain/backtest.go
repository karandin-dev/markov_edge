package domain

type Observation struct {
	Time        int64
	Symbol      string
	Signal      string
	State       string
	Regime      string
	Phase       string
	MacroRegime string
	ZScore      float64
	Score       float64
	LongScore   float64
	ShortScore  float64
	Entropy     float64
	Return1     float64
	Return3     float64
	Return6     float64
}

type SignalStats struct {
	Signal   string
	Count    int
	AvgRet1  float64
	AvgRet3  float64
	AvgRet6  float64
	WinRate1 float64
	WinRate3 float64
	WinRate6 float64
}

type EquityPoint struct {
	Time       int64
	Signal     string
	Regime     string
	Phase      string
	Position   int
	Price      float64
	BarReturn  float64
	NetReturn  float64
	Equity     float64
	TradeFee   float64
	StopHit    bool
	ExitReason string
}

type StrategyStats struct {
	Trades         int
	WinningTrades  int
	StoppedTrades  int
	TotalReturn    float64
	FinalEquity    float64
	WinRate        float64
	AvgTradeReturn float64
	AvgTradeBars   float64
	MaxDrawdown    float64
	TotalFees      float64
	AvgFeePerTrade float64
	AvgWin         float64
	AvgLoss        float64
	ProfitFactor   float64
}

type StrategyOptions struct {
	Commission           float64
	StopLossPct          float64
	EntropyCut           float64
	UsePhaseFilter       bool
	ShortOnly            bool
	LongOnly             bool
	AdaptiveMode         bool
	MinHoldBars          int
	CooldownBars         int
	LongExitConfirmBars  int
	ShortExitConfirmBars int
	ShortScoreThreshold  float64
	LongScoreThreshold   float64
	RegimeFilterEnabled  bool
	ExtremeZScoreCut     float64
	RegimeEntropyCut     float64
	PostExtremeBars      int
	UseDynamicZScore     bool
	DynamicZWindow       int
	DynamicZPercentile   float64
	DynamicZFallbackCut  float64
	DynamicZMinCut       float64
	DynamicZMaxCut       float64
	DynamicZMinSamples   int
}

package domain

type PortfolioOptions struct {
	InitialBalance   float64
	RiskPerTrade     float64
	MaxPortfolioRisk float64
	AllowLong        bool
	AllowShort       bool
	StrategyOptions  StrategyOptions
}

type PortfolioPosition struct {
	Symbol           string
	Direction        int
	EntryTime        int64
	EntryPrice       float64
	EntryIndex       int
	SizeUSD          float64
	RiskAmountUSD    float64
	ReservedRisk     float64
	HoldBars         int
	LongBarsAgainst  int
	ShortBarsAgainst int
	EntryZScore      float64
	EntryEntropy     float64
	EntryState       string
	EntryLongScore   float64
	EntryShortScore  float64
}

type PortfolioTrade struct {
	Symbol          string
	Direction       int
	EntryTime       int64
	ExitTime        int64
	EntryPrice      float64
	ExitPrice       float64
	SizeUSD         float64
	PnlUSD          float64
	PnlPctOnPos     float64
	Reason          string
	HoldBars        int
	FeeUSD          float64
	EntryZScore     float64
	EntryEntropy    float64
	EntryState      string
	EntryLongScore  float64
	EntryShortScore float64
}

type PortfolioEquityPoint struct {
	Time            int64
	Balance         float64
	OpenPositions   int
	ReservedRiskUSD float64
	RealizedPnlUSD  float64
}

type PortfolioStats struct {
	InitialBalance float64
	FinalBalance   float64
	TotalReturnPct float64
	MaxDrawdownPct float64
	Trades         int
	WinningTrades  int
	LosingTrades   int
	WinRatePct     float64
	ProfitFactor   float64
	AvgTradeUSD    float64
	AvgWinUSD      float64
	AvgLossUSD     float64
	TotalFeesUSD   float64
	BestTradeUSD   float64
	WorstTradeUSD  float64
}

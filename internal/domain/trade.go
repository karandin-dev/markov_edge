package domain

type Trade struct {
	ID              int64
	RunID           string
	StrategyVersion string
	Symbol          string
	Direction       int
	EntryTime       int64
	ExitTime        int64
	EntryPrice      float64
	ExitPrice       float64
	SizeUSD         float64
	PnlUSD          float64
	PnlPct          float64
	FeeUSD          float64
	HoldBars        int
	Reason          string
	EntryZScore     float64
	EntryEntropy    float64
	EntryState      string
	EntryLongScore  float64
	EntryShortScore float64
}

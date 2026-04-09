package market

import "time"

type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type State int

const (
	StrongDown State = iota
	WeakDown
	Flat
	WeakUp
	StrongUp
)

func (s State) String() string {
	switch s {
	case StrongDown:
		return "strong_down"
	case WeakDown:
		return "weak_down"
	case Flat:
		return "flat"
	case WeakUp:
		return "weak_up"
	case StrongUp:
		return "strong_up"
	default:
		return "unknown"
	}
}

type FeatureRow struct {
	Time    time.Time
	Close   float64
	Return  float64
	Vol     float64
	ZReturn float64
	State   State
	MinFlag bool
	MaxFlag bool
}

type SymbolScore struct {
	Symbol            string
	LastPrice         float64
	LastState         State
	ContinuationProb  float64
	ReversalProb      float64
	PersistenceProb   float64
	ContinuationProb2 float64
	ReversalProb2     float64
	PersistenceProb2  float64
	Entropy           float64
	RegimeClass       string
	HumanSignal       string
	Score             float64
	LongScore         float64
	ShortScore        float64
	ProbUp1           float64
	ProbDown1         float64
	ProbUp2           float64
	ProbDown2         float64

	VolRatio    float64 // Текущая волатильность / базовая
	MacroRegime string  // "bull", "bear", "neutral", etc.
}

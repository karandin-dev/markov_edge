package domain

type ChartCandle struct {
	Index   int
	Time    string
	Open    float64
	High    float64
	Low     float64
	Close   float64
	Volume  float64
	IsEntry bool
	IsExit  bool
}

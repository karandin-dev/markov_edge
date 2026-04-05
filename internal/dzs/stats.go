package dzs

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type QuarterStats struct {
	Values []float64
}

type Collector struct {
	Data map[string]map[string]*QuarterStats
}

func NewCollector() *Collector {
	return &Collector{Data: make(map[string]map[string]*QuarterStats)}
}

func quarterKey(t time.Time) string {
	month := int(t.Month())
	switch {
	case month <= 3:
		return "Q1"
	case month <= 6:
		return "Q2"
	case month <= 9:
		return "Q3"
	default:
		return "Q4"
	}
}

func (c *Collector) Add(symbol string, t time.Time, z float64) {
	yearQuarter := fmt.Sprintf("%d-%s", t.Year(), quarterKey(t))
	if _, ok := c.Data[symbol]; !ok {
		c.Data[symbol] = make(map[string]*QuarterStats)
	}
	if _, ok := c.Data[symbol][yearQuarter]; !ok {
		c.Data[symbol][yearQuarter] = &QuarterStats{}
	}
	c.Data[symbol][yearQuarter].Values = append(c.Data[symbol][yearQuarter].Values, z)
}

func (c *Collector) Print() {
	symbols := make([]string, 0, len(c.Data))
	for symbol := range c.Data {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	for _, symbol := range symbols {
		fmt.Println("\n====", symbol, "====")

		quarters := c.Data[symbol]
		keys := make([]string, 0, len(quarters))
		for key := range quarters {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			stats := quarters[key]
			if len(stats.Values) == 0 {
				continue
			}
			minVal := math.MaxFloat64
			maxVal := -math.MaxFloat64
			sum := 0.0
			for _, value := range stats.Values {
				sum += value
				if value < minVal {
					minVal = value
				}
				if value > maxVal {
					maxVal = value
				}
			}
			avg := sum / float64(len(stats.Values))
			fmt.Printf("%s | avg: %.2f | min: %.2f | max: %.2f | n: %d\n", key, avg, minVal, maxVal, len(stats.Values))
		}
	}
}

package dzs

import (
	"math"
	"sort"
)

type Config struct {
	Percentile float64
	Window     int
	Fallback   float64
	MinCut     float64
	MaxCut     float64
	MinSamples int
}

func DefaultConfig() Config {
	return Config{
		Percentile: 90,
		Window:     400,
		Fallback:   2.2,
		MinCut:     2.1,
		MaxCut:     3.00,
		MinSamples: 50,
	}
}

func Percentile(arr []float64, p float64) float64 {
	if len(arr) == 0 {
		return 0
	}

	cp := make([]float64, len(arr))
	copy(cp, arr)
	sort.Float64s(cp)

	if p <= 0 {
		return cp[0]
	}
	if p >= 100 {
		return cp[len(cp)-1]
	}

	k := int(float64(len(cp)-1) * p / 100.0)
	return cp[k]
}

func normalize(history []float64, window int) []float64 {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if window > 0 && len(history) > window {
		start = len(history) - window
	}
	out := make([]float64, 0, len(history)-start)
	for _, v := range history[start:] {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, math.Abs(v))
	}
	return out
}

func DynamicZCut(z []float64) float64 {

	return DynamicZCutWithConfig(z, DefaultConfig())
}

var dzsDebugCounter int

func DynamicZCutWithConfig(z []float64, cfg Config) float64 {
	if cfg.Percentile <= 0 {
		cfg.Percentile = 90
	}
	if cfg.Fallback <= 0 {
		cfg.Fallback = 2.2
	}
	if cfg.MinCut <= 0 {
		cfg.MinCut = 1.7
	}
	if cfg.MaxCut <= 0 {
		cfg.MaxCut = 3.5
	}
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 50
	}

	values := normalize(z, cfg.Window)

	if len(values) < cfg.MinSamples {
		finalCut := round2(clamp(cfg.Fallback, cfg.MinCut, cfg.MaxCut))

		dzsDebugCounter++

		return finalCut
	}

	rawCut := Percentile(values, cfg.Percentile)
	finalCut := round2(clamp(rawCut, cfg.MinCut, cfg.MaxCut))

	dzsDebugCounter++

	return finalCut
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

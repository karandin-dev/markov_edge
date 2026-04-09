package main

import (
	"fmt"
	"markov_screener/internal/exchange"
	"math"
	"strconv"
	"strings"
	"time"
)

// timeframeDuration возвращает duration для строкового таймфрейма
func timeframeDuration(tf string) (time.Duration, error) {
	switch tf {
	case "1m":
		return time.Minute, nil
	case "3m":
		return 3 * time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "2h":
		return 2 * time.Hour, nil
	case "4h":
		return 4 * time.Hour, nil
	case "6h":
		return 6 * time.Hour, nil
	case "12h":
		return 12 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "1w":
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported timeframe: %s", tf)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func barsForDays(days int, tfDur time.Duration) int {
	if days <= 0 || tfDur <= 0 {
		return 0
	}
	return int((time.Duration(days) * 24 * time.Hour) / tfDur)
}

func isAfterCandleClose(now time.Time, tf string) bool {
	tfDur, err := timeframeDuration(tf)
	if err != nil {
		return false
	}
	lastClose := now.Truncate(tfDur)
	diff := now.Sub(lastClose)
	return diff >= 2*time.Second
}

func nextExpectedClosedCandleTime(lastClosedOpen time.Time, tf string) (time.Time, error) {
	tfDur, err := timeframeDuration(tf)
	if err != nil {
		return time.Time{}, err
	}
	return lastClosedOpen.Add(2 * tfDur), nil
}

func calcRawQtyFromUSDT(usdt, price float64) float64 {
	if usdt <= 0 || price <= 0 {
		return 0
	}
	return usdt / price
}

func normalizeQty(rawQty, qtyStep float64) float64 {
	if rawQty <= 0 || qtyStep <= 0 {
		return 0
	}
	steps := math.Floor(rawQty/qtyStep + 1e-9)
	normalized := steps * qtyStep
	if normalized <= 0 {
		return qtyStep
	}
	stepStr := fmt.Sprintf("%g", qtyStep)
	decimals := 0
	if idx := strings.Index(stepStr, "."); idx != -1 {
		decimals = len(stepStr) - idx - 1
	}
	safeStr := fmt.Sprintf("%.*f", decimals, normalized)
	safeQty, _ := strconv.ParseFloat(safeStr, 64)
	return safeQty
}

func calcStopLoss(side exchange.Side, price, stopLossPercent float64) float64 {
	if price <= 0 || stopLossPercent <= 0 {
		return 0
	}
	pct := stopLossPercent / 100.0
	switch side {
	case exchange.SideBuy:
		return price * (1.0 - pct)
	case exchange.SideSell:
		return price * (1.0 + pct)
	default:
		return 0
	}
}

func formatNote(note string) string {
	if note == "" {
		return ""
	}
	return " | note=" + note
}

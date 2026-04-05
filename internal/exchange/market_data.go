package exchange

import (
	"context"
	"time"

	"markov_screener/internal/market"
)

type MarketDataClient interface {
	FetchOHLCVRange(
		ctx context.Context,
		symbol string,
		timeframe string,
		start time.Time,
		end time.Time,
	) ([]market.Candle, error)
}

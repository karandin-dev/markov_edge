package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const DefaultBybitBaseURL = "https://api.bybit.com"

type BybitClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewBybitClient() *BybitClient {
	return NewBybitClientWithBaseURL(DefaultBybitBaseURL)
}

func NewBybitClientWithBaseURL(baseURL string) *BybitClient {
	if baseURL == "" {
		baseURL = DefaultBybitBaseURL
	}

	return &BybitClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

type bybitKlineResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		Category string     `json:"category"`
		Symbol   string     `json:"symbol"`
		List     [][]string `json:"list"`
	} `json:"result"`
	Time int64 `json:"time"`
}

func (c *BybitClient) FetchOHLCVRange(
	ctx context.Context,
	symbol string,
	timeframe string,
	start time.Time,
	end time.Time,
) ([]Candle, error) {
	if !start.Before(end) {
		return nil, fmt.Errorf("start must be before end")
	}

	tfDur, err := timeframeDuration(timeframe)
	if err != nil {
		return nil, err
	}

	const pageLimit = 1000

	all := make([]Candle, 0, 8000)
	seen := make(map[int64]struct{})

	currentEnd := end

	for {
		if !currentEnd.After(start) {
			break
		}

		page, err := c.fetchOHLCVPage(
			ctx,
			symbol,
			timeframe,
			start.UnixMilli(),
			currentEnd.UnixMilli(),
			pageLimit,
		)
		if err != nil {
			return nil, err
		}

		if len(page) == 0 {
			break
		}

		added := 0
		for _, candle := range page {
			ts := candle.Time.UnixMilli()
			if _, ok := seen[ts]; ok {
				continue
			}
			seen[ts] = struct{}{}
			all = append(all, candle)
			added++
		}

		if added == 0 {
			break
		}

		oldest := page[0].Time
		nextEnd := oldest.Add(-tfDur)

		if !nextEnd.Before(currentEnd) {
			break
		}
		currentEnd = nextEnd

		time.Sleep(120 * time.Millisecond)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.Before(all[j].Time)
	})

	result := make([]Candle, 0, len(all))
	for _, candle := range all {
		if (candle.Time.Equal(start) || candle.Time.After(start)) &&
			(candle.Time.Equal(end) || candle.Time.Before(end)) {
			result = append(result, candle)
		}
	}

	return result, nil
}

func (c *BybitClient) fetchOHLCVPage(
	ctx context.Context,
	symbol string,
	timeframe string,
	startMs int64,
	endMs int64,
	limit int,
) ([]Candle, error) {
	interval, err := toBybitInterval(timeframe)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	u, err := url.Parse(c.baseURL + "/v5/market/kline")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	q := u.Query()
	q.Set("category", "linear")
	q.Set("symbol", symbol)
	q.Set("interval", interval)
	q.Set("limit", strconv.Itoa(limit))

	if startMs > 0 {
		q.Set("start", strconv.FormatInt(startMs, 10))
	}
	if endMs > 0 {
		q.Set("end", strconv.FormatInt(endMs, 10))
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp bybitKlineResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.RetCode != 0 {
		return nil, fmt.Errorf("bybit error: retCode=%d retMsg=%s", apiResp.RetCode, apiResp.RetMsg)
	}

	candles := make([]Candle, 0, len(apiResp.Result.List))

	for _, row := range apiResp.Result.List {
		if len(row) < 6 {
			continue
		}

		tsMs, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp: %w", err)
		}
		open, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parse open: %w", err)
		}
		high, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse high: %w", err)
		}
		low, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, fmt.Errorf("parse low: %w", err)
		}
		closePrice, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return nil, fmt.Errorf("parse close: %w", err)
		}
		volume, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, fmt.Errorf("parse volume: %w", err)
		}

		candles = append(candles, Candle{
			Time:   time.UnixMilli(tsMs),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}

	reverseCandles(candles)

	return candles, nil
}

func toBybitInterval(tf string) (string, error) {
	switch tf {
	case "1m":
		return "1", nil
	case "3m":
		return "3", nil
	case "5m":
		return "5", nil
	case "15m":
		return "15", nil
	case "30m":
		return "30", nil
	case "1h":
		return "60", nil
	case "2h":
		return "120", nil
	case "4h":
		return "240", nil
	case "6h":
		return "360", nil
	case "12h":
		return "720", nil
	case "1d":
		return "D", nil
	case "1w":
		return "W", nil
	default:
		return "", fmt.Errorf("unsupported timeframe: %s", tf)
	}
}

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

func reverseCandles(c []Candle) {
	for i, j := 0, len(c)-1; i < j; i, j = i+1, j-1 {
		c[i], c[j] = c[j], c[i]
	}
}

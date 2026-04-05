package exchange

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBybitRecvWindow = "5000"
)

type BybitPrivateClient struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	category   string
	httpClient *http.Client
	recvWindow string
}

func NewBybitPrivateClient(baseURL, apiKey, apiSecret, category string) *BybitPrivateClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.bybit.com"
	}
	if strings.TrimSpace(category) == "" {
		category = "linear"
	}

	return &BybitPrivateClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		apiSecret: apiSecret,
		category:  category,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		recvWindow: DefaultBybitRecvWindow,
	}
}

type bybitAPIResponse struct {
	RetCode int             `json:"retCode"`
	RetMsg  string          `json:"retMsg"`
	Result  json.RawMessage `json:"result"`
	Time    int64           `json:"time"`
}

type bybitPositionListResult struct {
	List []bybitPositionItem `json:"list"`
}

type bybitPositionItem struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Size          string `json:"size"`
	AvgPrice      string `json:"avgPrice"`
	MarkPrice     string `json:"markPrice"`
	UnrealisedPnl string `json:"unrealisedPnl"`
	Leverage      string `json:"leverage"`
}

type bybitOpenOrdersResult struct {
	List []bybitOrderItem `json:"list"`
}

type bybitOrderItem struct {
	OrderID     string `json:"orderId"`
	OrderLinkID string `json:"orderLinkId"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	OrderType   string `json:"orderType"`
	Price       string `json:"price"`
	Qty         string `json:"qty"`
	CumExecQty  string `json:"cumExecQty"`
	OrderStatus string `json:"orderStatus"`
	ReduceOnly  bool   `json:"reduceOnly"`
}

type bybitCreateOrderResult struct {
	OrderID     string `json:"orderId"`
	OrderLinkID string `json:"orderLinkId"`
}

type bybitCancelOrderResult struct {
	OrderID     string `json:"orderId"`
	OrderLinkID string `json:"orderLinkId"`
}

type bybitInstrumentInfoResult struct {
	List []bybitInstrumentItem `json:"list"`
}

type bybitInstrumentItem struct {
	Symbol        string `json:"symbol"`
	LotSizeFilter struct {
		MinOrderQty string `json:"minOrderQty"`
		QtyStep     string `json:"qtyStep"`
		MinNotional string `json:"minNotionalValue"`
	} `json:"lotSizeFilter"`
}

func (c *BybitPrivateClient) GetPosition(ctx context.Context, symbol string) (Position, error) {
	params := url.Values{}
	params.Set("category", c.category)
	params.Set("symbol", symbol)

	var resp bybitPositionListResult
	if err := c.doSignedGET(ctx, "/v5/position/list", params, &resp); err != nil {
		return Position{}, err
	}

	if len(resp.List) == 0 {
		return Position{
			Symbol: symbol,
			Side:   PositionSideNone,
		}, nil
	}

	item := resp.List[0]

	size, err := parseFloat(item.Size)
	if err != nil {
		return Position{}, fmt.Errorf("parse position size: %w", err)
	}
	avgPrice, err := parseFloat(item.AvgPrice)
	if err != nil {
		return Position{}, fmt.Errorf("parse avg price: %w", err)
	}
	markPrice, err := parseFloat(item.MarkPrice)
	if err != nil {
		return Position{}, fmt.Errorf("parse mark price: %w", err)
	}
	unrealizedPnL, err := parseFloat(item.UnrealisedPnl)
	if err != nil {
		return Position{}, fmt.Errorf("parse unrealised pnl: %w", err)
	}
	leverage, err := parseFloat(item.Leverage)
	if err != nil {
		return Position{}, fmt.Errorf("parse leverage: %w", err)
	}

	side := mapBybitPositionSide(item.Side, size)

	return Position{
		Symbol:        item.Symbol,
		Side:          side,
		Size:          size,
		AvgPrice:      avgPrice,
		MarkPrice:     markPrice,
		UnrealizedPnL: unrealizedPnL,
		Leverage:      leverage,
	}, nil
}

func (c *BybitPrivateClient) GetOpenOrders(ctx context.Context, symbol string) ([]Order, error) {
	params := url.Values{}
	params.Set("category", c.category)
	params.Set("symbol", symbol)
	params.Set("openOnly", "0")

	var resp bybitOpenOrdersResult
	if err := c.doSignedGET(ctx, "/v5/order/realtime", params, &resp); err != nil {
		return nil, err
	}

	orders := make([]Order, 0, len(resp.List))
	for _, item := range resp.List {
		price, err := parseFloat(item.Price)
		if err != nil {
			return nil, fmt.Errorf("parse order price: %w", err)
		}
		qty, err := parseFloat(item.Qty)
		if err != nil {
			return nil, fmt.Errorf("parse order qty: %w", err)
		}
		filledQty, err := parseFloat(item.CumExecQty)
		if err != nil {
			return nil, fmt.Errorf("parse filled qty: %w", err)
		}

		orders = append(orders, Order{
			OrderID:       item.OrderID,
			ClientOrderID: item.OrderLinkID,
			Symbol:        item.Symbol,
			Side:          Side(item.Side),
			Type:          OrderType(item.OrderType),
			Price:         price,
			Qty:           qty,
			FilledQty:     filledQty,
			Status:        mapBybitOrderStatus(item.OrderStatus),
			ReduceOnly:    item.ReduceOnly,
		})
	}

	return orders, nil
}

func (c *BybitPrivateClient) GetInstrumentInfo(ctx context.Context, symbol string) (InstrumentInfo, error) {
	params := url.Values{}
	params.Set("category", c.category)
	params.Set("symbol", symbol)

	var resp bybitInstrumentInfoResult
	if err := c.doPublicGET(ctx, "/v5/market/instruments-info", params, &resp); err != nil {
		return InstrumentInfo{}, err
	}

	if len(resp.List) == 0 {
		return InstrumentInfo{}, fmt.Errorf("instrument info not found for %s", symbol)
	}

	item := resp.List[0]

	minOrderQty, err := parseFloat(item.LotSizeFilter.MinOrderQty)
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("parse minOrderQty: %w", err)
	}

	qtyStep, err := parseFloat(item.LotSizeFilter.QtyStep)
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("parse qtyStep: %w", err)
	}

	minNotional, err := parseFloat(item.LotSizeFilter.MinNotional)
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("parse minNotionalValue: %w", err)
	}

	return InstrumentInfo{
		Symbol:      item.Symbol,
		MinOrderQty: minOrderQty,
		QtyStep:     qtyStep,
		MinNotional: minNotional,
	}, nil
}

func (c *BybitPrivateClient) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (OrderResult, error) {
	body := map[string]any{
		"category":  c.category,
		"symbol":    req.Symbol,
		"side":      string(req.Side),
		"orderType": string(req.OrderType),
		"qty":       formatFloat(req.Qty),
	}

	if req.ClientOrderID != "" {
		body["orderLinkId"] = req.ClientOrderID
	}
	if req.ReduceOnly {
		body["reduceOnly"] = true
	}
	if req.TimeInForce != "" && req.OrderType == OrderTypeLimit {
		body["timeInForce"] = string(req.TimeInForce)
	}
	if req.OrderType == OrderTypeLimit {
		body["price"] = formatFloat(req.Price)
	}
	if req.StopLoss > 0 {
		body["stopLoss"] = formatFloat(req.StopLoss)
	}
	if req.TakeProfit > 0 {
		body["takeProfit"] = formatFloat(req.TakeProfit)
	}

	var resp bybitCreateOrderResult
	if err := c.doSignedPOST(ctx, "/v5/order/create", body, &resp); err != nil {
		return OrderResult{}, err
	}

	return OrderResult{
		OrderID:       resp.OrderID,
		ClientOrderID: resp.OrderLinkID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		OrderType:     req.OrderType,
		Qty:           req.Qty,
		Price:         req.Price,
		Status:        OrderStatusNew,
		RawStatus:     "accepted",
	}, nil
}

func (c *BybitPrivateClient) CancelOrder(ctx context.Context, req CancelOrderRequest) error {
	body := map[string]any{
		"category": c.category,
		"symbol":   req.Symbol,
		"orderId":  req.OrderID,
	}

	var resp bybitCancelOrderResult
	if err := c.doSignedPOST(ctx, "/v5/order/cancel", body, &resp); err != nil {
		return err
	}

	return nil
}

func (c *BybitPrivateClient) CancelAllOrders(ctx context.Context, symbol string) error {
	body := map[string]any{
		"category": c.category,
		"symbol":   symbol,
	}

	var resp map[string]any
	if err := c.doSignedPOST(ctx, "/v5/order/cancel-all", body, &resp); err != nil {
		return err
	}

	return nil
}

func (c *BybitPrivateClient) SetStopLoss(ctx context.Context, req StopLossRequest) error {
	body := map[string]any{
		"category": c.category,
		"symbol":   req.Symbol,
		"stopLoss": formatFloat(req.StopLoss),
		"tpslMode": "Full",
	}

	var resp map[string]any
	if err := c.doSignedPOST(ctx, "/v5/position/trading-stop", body, &resp); err != nil {
		return err
	}

	return nil
}

func (c *BybitPrivateClient) PingAuth(ctx context.Context) error {
	var resp map[string]any
	return c.doSignedGET(ctx, "/v5/user/query-api", nil, &resp)
}

func (c *BybitPrivateClient) doSignedGET(ctx context.Context, path string, params url.Values, out any) error {
	query := ""
	if params != nil {
		query = params.Encode()
	}

	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signPayload := ts + c.apiKey + c.recvWindow + query
	signature := signBybit(signPayload, c.apiSecret)

	fullURL := c.baseURL + path
	if query != "" {
		fullURL += "?" + query
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("create GET request: %w", err)
	}

	c.applyAuthHeaders(req, ts, signature)

	return c.do(req, out)
}

func (c *BybitPrivateClient) doPublicGET(ctx context.Context, path string, params url.Values, out any) error {
	query := ""
	if params != nil {
		query = params.Encode()
	}

	fullURL := c.baseURL + path
	if query != "" {
		fullURL += "?" + query
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("create public GET request: %w", err)
	}

	return c.do(req, out)
}

func (c *BybitPrivateClient) doSignedPOST(ctx context.Context, path string, body any, out any) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signPayload := ts + c.apiKey + c.recvWindow + string(bodyBytes)
	signature := signBybit(signPayload, c.apiSecret)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+path,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return fmt.Errorf("create POST request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.applyAuthHeaders(req, ts, signature)

	return c.do(req, out)
}

func (c *BybitPrivateClient) applyAuthHeaders(req *http.Request, ts, signature string) {
	req.Header.Set("X-BAPI-API-KEY", c.apiKey)
	req.Header.Set("X-BAPI-TIMESTAMP", ts)
	req.Header.Set("X-BAPI-RECV-WINDOW", c.recvWindow)
	req.Header.Set("X-BAPI-SIGN", signature)
}

func (c *BybitPrivateClient) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp bybitAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("decode api response: %w", err)
	}

	if apiResp.RetCode != 0 {
		return fmt.Errorf("bybit error retCode=%d retMsg=%s", apiResp.RetCode, apiResp.RetMsg)
	}

	if out == nil {
		return nil
	}
	if len(apiResp.Result) == 0 || string(apiResp.Result) == "null" {
		return nil
	}

	if err := json.Unmarshal(apiResp.Result, out); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}

	return nil
}

func signBybit(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func parseFloat(v string) (float64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, nil
	}
	return strconv.ParseFloat(v, 64)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func mapBybitPositionSide(side string, size float64) PositionSide {
	if size == 0 {
		return PositionSideNone
	}

	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return PositionSideLong
	case "sell":
		return PositionSideShort
	default:
		return PositionSideNone
	}
}

func mapBybitOrderStatus(s string) OrderStatus {
	switch s {
	case "New", "Created", "Untriggered":
		return OrderStatusNew
	case "PartiallyFilled":
		return OrderStatusPartiallyFilled
	case "Filled":
		return OrderStatusFilled
	case "Cancelled", "Deactivated":
		return OrderStatusCancelled
	case "Rejected":
		return OrderStatusRejected
	default:
		return OrderStatus(s)
	}
}

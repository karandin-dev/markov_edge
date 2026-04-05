package exchange

import "context"

type Side string
type OrderType string
type PositionSide string
type TimeInForce string
type OrderStatus string

const (
	SideBuy  Side = "Buy"
	SideSell Side = "Sell"
)

const (
	OrderTypeMarket OrderType = "Market"
	OrderTypeLimit  OrderType = "Limit"
)

const (
	PositionSideLong  PositionSide = "Long"
	PositionSideShort PositionSide = "Short"
	PositionSideNone  PositionSide = "None"
)

const (
	TimeInForceGTC TimeInForce = "GTC"
	TimeInForceIOC TimeInForce = "IOC"
	TimeInForceFOK TimeInForce = "FOK"
)

const (
	OrderStatusNew             OrderStatus = "New"
	OrderStatusPartiallyFilled OrderStatus = "PartiallyFilled"
	OrderStatusFilled          OrderStatus = "Filled"
	OrderStatusCancelled       OrderStatus = "Cancelled"
	OrderStatusRejected        OrderStatus = "Rejected"
)

type Position struct {
	Symbol        string
	Side          PositionSide
	Size          float64
	AvgPrice      float64
	MarkPrice     float64
	UnrealizedPnL float64
	Leverage      float64
}

type Order struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Side          Side
	Type          OrderType
	Price         float64
	Qty           float64
	FilledQty     float64
	Status        OrderStatus
	ReduceOnly    bool
}

type PlaceOrderRequest struct {
	Symbol        string
	Side          Side
	OrderType     OrderType
	Qty           float64
	Price         float64
	ReduceOnly    bool
	TimeInForce   TimeInForce
	ClientOrderID string
	StopLoss      float64
	TakeProfit    float64
}

type OrderResult struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Side          Side
	OrderType     OrderType
	Qty           float64
	Price         float64
	Status        OrderStatus
	RawStatus     string
}

type CancelOrderRequest struct {
	Symbol  string
	OrderID string
}

type StopLossRequest struct {
	Symbol   string
	StopLoss float64
}

type InstrumentInfo struct {
	Symbol      string
	MinOrderQty float64
	QtyStep     float64
	MinNotional float64
}

type TradingClient interface {
	GetPosition(ctx context.Context, symbol string) (Position, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]Order, error)
	GetInstrumentInfo(ctx context.Context, symbol string) (InstrumentInfo, error)
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (OrderResult, error)
	CancelOrder(ctx context.Context, req CancelOrderRequest) error
	CancelAllOrders(ctx context.Context, symbol string) error
	SetStopLoss(ctx context.Context, req StopLossRequest) error
}

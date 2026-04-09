package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"markov_screener/internal/exchange"
)

// ============================================================================
// 🆔 ID генераторы
// ============================================================================

// buildClientOrderID создаёт уникальный client_order_id для входа
func buildClientOrderID(symbol string, side exchange.Side, anchor time.Time) string {
	s := strings.ReplaceAll(symbol, "USDT", "")
	return fmt.Sprintf("mkv-%s-%s-%d", strings.ToLower(s), strings.ToLower(string(side)), anchor.Unix())
}

// buildCloseOrderID создаёт уникальный client_order_id для выхода
func buildCloseOrderID(symbol string, side exchange.Side, anchor time.Time) string {
	s := strings.ReplaceAll(symbol, "USDT", "")
	return fmt.Sprintf("mkv-close-%s-%s-%d", strings.ToLower(s), strings.ToLower(string(side)), anchor.Unix())
}

// ============================================================================
// 🔄 Закрытие позиций
// ============================================================================

// closePositionOnExchange закрывает ВСЮ позицию по рынку
func closePositionOnExchange(
	ctx context.Context,
	tradingClient exchange.TradingClient,
	symbol string,
	position exchange.Position,
	closeSide exchange.Side,
	anchor time.Time,
	reason string,
) (string, error) {
	qty := position.Size
	if qty <= 0 {
		return "", fmt.Errorf("close position: exchange size <= 0")
	}

	orderResult, err := tradingClient.PlaceOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        symbol,
		Side:          closeSide,
		OrderType:     exchange.OrderTypeMarket,
		Qty:           qty,
		TimeInForce:   exchange.TimeInForceIOC,
		ClientOrderID: buildCloseOrderID(symbol, closeSide, anchor),
		ReduceOnly:    true,
	})
	if err != nil {
		return "", fmt.Errorf("close position order failed: %w", err)
	}

	return fmt.Sprintf("reduce_only_close order_id=%s reason=%s qty=%.8f", orderResult.OrderID, reason, qty), nil
}

// closePositionOnExchangeWithQty закрывает КОНКРЕТНОЕ количество (для частичного TP)
func closePositionOnExchangeWithQty(
	ctx context.Context,
	tradingClient exchange.TradingClient,
	symbol string,
	closeSide exchange.Side,
	qty float64,
	anchor time.Time,
	reason string,
) (string, error) {
	if qty <= 0 {
		return "", fmt.Errorf("close position: qty <= 0")
	}

	orderResult, err := tradingClient.PlaceOrder(ctx, exchange.PlaceOrderRequest{
		Symbol:        symbol,
		Side:          closeSide,
		OrderType:     exchange.OrderTypeMarket,
		Qty:           qty,
		TimeInForce:   exchange.TimeInForceIOC,
		ClientOrderID: buildCloseOrderID(symbol, closeSide, anchor),
		ReduceOnly:    true,
	})
	if err != nil {
		return "", fmt.Errorf("partial close position order failed: %w", err)
	}

	return fmt.Sprintf("partial_close order_id=%s reason=%s qty=%.8f", orderResult.OrderID, reason, qty), nil
}

// ============================================================================
//#️⃣ Вспомогательные функции для ордеров
// ============================================================================

// hasActiveEntryOrder проверяет, есть ли активный ордер на вход (не ReduceOnly)
func hasActiveEntryOrder(orders []exchange.Order) bool {
	for _, o := range orders {
		if o.ReduceOnly {
			continue
		}
		switch o.Status {
		case exchange.OrderStatusNew, exchange.OrderStatusPartiallyFilled:
			return true
		}
	}
	return false
}

// directionFromPositionSide конвертирует PositionSide в числовое направление
func directionFromPositionSide(side exchange.PositionSide) int {
	switch side {
	case exchange.PositionSideLong:
		return 1
	case exchange.PositionSideShort:
		return -1
	default:
		return 0
	}
}

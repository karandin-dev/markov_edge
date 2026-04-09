package markov

import "time"

// Position представляет открытую позицию
type Position struct {
	Symbol     string
	Side       string // "Long" или "Short"
	EntryPrice float64
	EntryScore float64
	EntryTime  time.Time
	MAE        float64 // Maximum Adverse Excursion (в % от entry)
	MFE        float64 // Maximum Favorable Excursion (в % от entry)
	LastUpdate time.Time
	ExitPrice  float64
	ExitReason string
	FinalPnL   float64
}

// NewPosition создаёт новую позицию с инициализацией метрик
func NewPosition(symbol, side string, entryPrice, entryScore float64) *Position {
	return &Position{
		Symbol:     symbol,
		Side:       side,
		EntryPrice: entryPrice,
		EntryScore: entryScore,
		EntryTime:  time.Now(),
		MAE:        0.0, // начинаем с нуля
		MFE:        0.0,
		LastUpdate: time.Now(),
	}
}

// UpdateMetrics обновляет MAE/MFE для позиции
func (p *Position) UpdateMetrics(currentPrice float64) {
	// Рассчитываем текущий PnL в % от входа
	var currentPnL float64
	if p.Side == "Long" {
		currentPnL = (currentPrice - p.EntryPrice) / p.EntryPrice
	} else {
		currentPnL = (p.EntryPrice - currentPrice) / p.EntryPrice
	}

	// 🔥 Обновляем MAE (отслеживаем максимальный минус)
	// MAE инициализирован 0, поэтому первый отрицательный PnL запишется автоматически
	if currentPnL < 0 && currentPnL < p.MAE {
		p.MAE = currentPnL
	}

	// 🔥 Обновляем MFE (отслеживаем максимальный плюс)
	// MFE инициализирован 0, поэтому первый положительный PnL запишется автоматически
	if currentPnL > 0 && currentPnL > p.MFE {
		p.MFE = currentPnL
	}

	p.LastUpdate = time.Now()
}

// IsProfitable возвращает true, если позиция в плюсе
func (p *Position) IsProfitable(currentPrice float64) bool {
	if p.Side == "Long" {
		return currentPrice > p.EntryPrice
	}
	return currentPrice < p.EntryPrice
}

// GetCurrentPnL возвращает текущий нереализованный PnL в %
func (p *Position) GetCurrentPnL(currentPrice float64) float64 {
	if p.Side == "Long" {
		return (currentPrice - p.EntryPrice) / p.EntryPrice
	}
	return (p.EntryPrice - currentPrice) / p.EntryPrice
}

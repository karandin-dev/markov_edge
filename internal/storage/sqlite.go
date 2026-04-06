package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// TradeLog структура для записи в БД
type TradeLog struct {
	Timestamp  time.Time
	Symbol     string
	EventType  string // EXECUTED, HOLD, ADAPTIVE_EXIT, SIGNAL_EXIT, ERROR, SKIP
	Direction  string // LONG, SHORT, FLAT
	PnlPercent float64
	ScoreEntry float64
	ScoreExit  float64
	Entropy    float64
	Confidence float64
	Threshold  float64
	HoldBars   int
	OrderID    string
	Reason     string
	RawLog     string
}

// InitDB открывает SQLite и создаёт таблицу
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// WAL-режим для лучшей конкурентности и безопасности
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA synchronous=NORMAL;")

	schema := `
	CREATE TABLE IF NOT EXISTS trade_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		symbol TEXT,
		event_type TEXT,
		direction TEXT,
		pnl_percent REAL,
		score_entry REAL,
		score_exit REAL,
		entropy REAL,
		confidence REAL,
		threshold REAL,
		hold_bars INTEGER,
		order_id TEXT,
		reason TEXT,
		raw_log TEXT
	);`

	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema creation failed: %w", err)
	}
	return db, nil
}

// LogTrade сохраняет запись в БД
func LogTrade(db *sql.DB, log TradeLog) error {
	query := `INSERT INTO trade_logs (timestamp, symbol, event_type, direction, pnl_percent, 
		score_entry, score_exit, entropy, confidence, threshold, hold_bars, order_id, reason, raw_log)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(query, log.Timestamp, log.Symbol, log.EventType, log.Direction,
		log.PnlPercent, log.ScoreEntry, log.ScoreExit, log.Entropy, log.Confidence,
		log.Threshold, log.HoldBars, log.OrderID, log.Reason, log.RawLog)
	return err
}

// ParseEventType извлекает тип события из текста лога
func ParseEventType(text string) string {
	switch {
	case len(text) == 0:
		return "EMPTY"
	case contains(text, "EXECUTED"):
		return "EXECUTED"
	case contains(text, "ADAPTIVE EXIT"):
		return "ADAPTIVE_EXIT"
	case contains(text, "CLOSED"):
		return "SIGNAL_EXIT"
	case contains(text, "HOLD"):
		return "HOLD"
	case contains(text, "SKIP"):
		return "SKIP"
	case contains(text, "ERROR"):
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}
func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

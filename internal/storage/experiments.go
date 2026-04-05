package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func InitDB(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	db := &DB{conn: conn}

	if err := db.createTables(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	if db == nil || db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

func (db *DB) createTables() error {
	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TEXT NOT NULL,
			mode TEXT NOT NULL,
			strategy_version TEXT NOT NULL,
			notes TEXT,
			timeframe TEXT NOT NULL,
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			symbols_json TEXT NOT NULL,
			config_json TEXT NOT NULL,

			initial_balance REAL,
			final_balance REAL,
			return_pct REAL,
			max_drawdown_pct REAL,
			trades INTEGER,
			winning_trades INTEGER,
			losing_trades INTEGER,
			win_rate REAL,
			profit_factor REAL,
			avg_trade REAL,
			avg_win REAL,
			avg_loss REAL,
			total_fees REAL
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS symbol_summaries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			period_label TEXT NOT NULL,
			symbol TEXT NOT NULL,
			trades INTEGER,
			winning_trades INTEGER,
			losing_trades INTEGER,
			win_rate REAL,
			gross_pnl REAL,
			fees REAL,
			net_pnl REAL,
			avg_trade REAL,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);
		`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("create tables: %w", err)
		}
	}

	return nil
}

type RunRecord struct {
	CreatedAt       string
	Mode            string
	StrategyVersion string
	Notes           string
	Timeframe       string
	StartDate       string
	EndDate         string
	SymbolsJSON     string
	ConfigJSON      string

	InitialBalance float64
	FinalBalance   float64
	ReturnPct      float64
	MaxDrawdownPct float64
	Trades         int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64
	ProfitFactor   float64
	AvgTrade       float64
	AvgWin         float64
	AvgLoss        float64
	TotalFees      float64
}

type SymbolSummaryRecord struct {
	RunID         int64
	PeriodLabel   string
	Symbol        string
	Trades        int
	WinningTrades int
	LosingTrades  int
	WinRate       float64
	GrossPNL      float64
	Fees          float64
	NetPNL        float64
	AvgTrade      float64
}

func (db *DB) SaveRun(r RunRecord) (int64, error) {
	query := `
	INSERT INTO runs (
		created_at,
		mode,
		strategy_version,
		notes,
		timeframe,
		start_date,
		end_date,
		symbols_json,
		config_json,
		initial_balance,
		final_balance,
		return_pct,
		max_drawdown_pct,
		trades,
		winning_trades,
		losing_trades,
		win_rate,
		profit_factor,
		avg_trade,
		avg_win,
		avg_loss,
		total_fees
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := db.conn.Exec(
		query,
		r.CreatedAt,
		r.Mode,
		r.StrategyVersion,
		r.Notes,
		r.Timeframe,
		r.StartDate,
		r.EndDate,
		r.SymbolsJSON,
		r.ConfigJSON,
		r.InitialBalance,
		r.FinalBalance,
		r.ReturnPct,
		r.MaxDrawdownPct,
		r.Trades,
		r.WinningTrades,
		r.LosingTrades,
		r.WinRate,
		r.ProfitFactor,
		r.AvgTrade,
		r.AvgWin,
		r.AvgLoss,
		r.TotalFees,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}

	runID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get run id: %w", err)
	}

	return runID, nil
}

func (db *DB) SaveSymbolSummaries(rows []SymbolSummaryRecord) error {
	query := `
	INSERT INTO symbol_summaries (
		run_id,
		period_label,
		symbol,
		trades,
		winning_trades,
		losing_trades,
		win_rate,
		gross_pnl,
		fees,
		net_pnl,
		avg_trade
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	stmt, err := tx.Prepare(query)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		_, err := stmt.Exec(
			row.RunID,
			row.PeriodLabel,
			row.Symbol,
			row.Trades,
			row.WinningTrades,
			row.LosingTrades,
			row.WinRate,
			row.GrossPNL,
			row.Fees,
			row.NetPNL,
			row.AvgTrade,
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert symbol summary: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
func (db *DB) Conn() *sql.DB {
	if db == nil {
		return nil
	}
	return db.conn
}

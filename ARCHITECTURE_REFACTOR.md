# Markov Screener: refactor notes

This refactor keeps the project behavior intact while making the structure easier to navigate.

## What changed

- Added `internal/domain/` for shared core types
  - `Observation`, `StrategyOptions`, `StrategyStats`
  - `PortfolioOptions`, `PortfolioTrade`, `PortfolioStats`
  - `Trade`, `ChartCandle`, `PageData`
- Added `internal/dzs/` for Dynamic Z-Score logic and quarterly stats
- Kept `internal/analysis/` as a thin compatibility layer so older imports keep working
- Renamed `internal/backtest/progile.go` -> `internal/backtest/profile.go`
- `cmd/show_trade` now reads shared types from `internal/domain` instead of defining them in `main.go`
- `internal/backtest` now uses shared domain aliases instead of owning all structs inline

## Why this helps

- Core structs are no longer hidden inside entrypoints
- Dynamic Z-Score logic is isolated in one place
- Backtest logic and shared models are easier to reason about
- Future moves toward `internal/regime/`, `internal/portfolio/`, and `internal/optimize/` will be simpler

## Recommended next step

1. Move regime-filter logic into `internal/regime/`
2. Split portfolio engine from generic backtest engine
3. Consolidate optimization runners behind shared services

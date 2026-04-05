# DZS refactor notes

Что изменено:

1. Dynamic Z-Score теперь считается по `abs(z-score)`, а не по raw `z-score`.
2. Введён отдельный конфиг DZS в `StrategyOptions`.
3. Conservative preset настроен под текущее плато:
   - fallback = 2.20
   - clamp = [2.20, 3.00]
   - percentile = 90
   - window = 400
4. `cmd/portfolio_backtest` переведён с aggressive на conservative preset.
5. DZS теперь реально участвует в regime filter через `effectiveExtremeZCut(...)`.

Что это даёт:
- нет рассинхрона между DZS и `abs(obs.ZScore)`
- portfolio backtest запускается не в aggressive-режиме
- dynamic threshold живёт в диапазоне найденного плато, а не около 1.30

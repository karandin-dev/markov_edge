package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"markov_screener/internal/domain"
	"markov_screener/internal/market"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", filepath.Join("data", "experiments.db"), "path to sqlite db")
	cacheDir := flag.String("cache-dir", filepath.Join("data", "cache"), "directory with candle json cache")
	tradeID := flag.Int64("trade-id", 0, "trade id from trades table")
	before := flag.Int("before", 48, "candles before entry")
	after := flag.Int("after", 16, "candles after entry")
	outPath := flag.String("out", "", "output html path")
	flag.Parse()

	if *tradeID <= 0 {
		fmt.Println("usage: go run cmd/show_trade/main.go --trade-id 123")
		os.Exit(1)
	}

	trade, err := loadTrade(*dbPath, *tradeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load trade: %v\n", err)
		os.Exit(1)
	}

	candles, err := loadSymbolCandles(*cacheDir, trade.Symbol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load candles: %v\n", err)
		os.Exit(1)
	}
	if len(candles) == 0 {
		fmt.Fprintf(os.Stderr, "no candles found for %s\n", trade.Symbol)
		os.Exit(1)
	}

	window, entryIndex, exitIndex, err := buildWindow(candles, trade, *before, *after)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build window: %v\n", err)
		os.Exit(1)
	}

	if *outPath == "" {
		*outPath = fmt.Sprintf("trade_%d_%s.html", trade.ID, strings.ToLower(trade.Symbol))
	}

	if err := renderHTML(*outPath, trade, window, entryIndex, exitIndex, *before, *after); err != nil {
		fmt.Fprintf(os.Stderr, "render html: %v\n", err)
		os.Exit(1)
	}

	abs, _ := filepath.Abs(*outPath)
	fmt.Printf("saved: %s\n", abs)
}

func loadTrade(dbPath string, tradeID int64) (domain.Trade, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return domain.Trade{}, err
	}
	defer db.Close()

	var t domain.Trade
	row := db.QueryRow(`
SELECT
    id,
    run_id,
    strategy_version,
    symbol,
    direction,
    entry_time,
    exit_time,
    entry_price,
    exit_price,
    size_usd,
    pnl_usd,
    pnl_pct,
    fee_usd,
    hold_bars,
    reason,
    entry_zscore,
    entry_entropy,
    entry_state,
    entry_long_score,
    entry_short_score
FROM trades
WHERE id = ?
`, tradeID)

	err = row.Scan(
		&t.ID,
		&t.RunID,
		&t.StrategyVersion,
		&t.Symbol,
		&t.Direction,
		&t.EntryTime,
		&t.ExitTime,
		&t.EntryPrice,
		&t.ExitPrice,
		&t.SizeUSD,
		&t.PnlUSD,
		&t.PnlPct,
		&t.FeeUSD,
		&t.HoldBars,
		&t.Reason,
		&t.EntryZScore,
		&t.EntryEntropy,
		&t.EntryState,
		&t.EntryLongScore,
		&t.EntryShortScore,
	)
	if err != nil {
		return domain.Trade{}, err
	}
	return t, nil
}

func loadSymbolCandles(cacheDir, symbol string) ([]market.Candle, error) {
	pattern := filepath.Join(cacheDir, symbol+"_*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no cache files match %s", pattern)
	}

	sort.Strings(paths)

	seen := make(map[int64]market.Candle)
	for _, path := range paths {
		candles, err := loadCandlesJSON(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		for _, c := range candles {
			seen[c.Time.Unix()] = c
		}
	}

	merged := make([]market.Candle, 0, len(seen))
	for _, c := range seen {
		merged = append(merged, c)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Time.Before(merged[j].Time)
	})
	return merged, nil
}

func loadCandlesJSON(path string) ([]market.Candle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var candles []market.Candle
	if err := json.NewDecoder(f).Decode(&candles); err != nil {
		return nil, err
	}
	return candles, nil
}

func buildWindow(candles []market.Candle, trade domain.Trade, before, after int) ([]domain.ChartCandle, int, int, error) {
	if len(candles) == 0 {
		return nil, 0, 0, fmt.Errorf("no candles")
	}

	entryIdx := nearestIndex(candles, trade.EntryTime)
	exitIdx := nearestIndex(candles, trade.ExitTime)
	if entryIdx < 0 {
		return nil, 0, 0, fmt.Errorf("entry candle not found")
	}
	if exitIdx < 0 {
		exitIdx = entryIdx
	}

	start := maxInt(0, entryIdx-before)
	end := minInt(len(candles)-1, entryIdx+after)
	if exitIdx > end {
		end = minInt(len(candles)-1, exitIdx+2)
	}

	window := make([]domain.ChartCandle, 0, end-start+1)
	for i := start; i <= end; i++ {
		c := candles[i]
		window = append(window, domain.ChartCandle{
			Index:   len(window),
			Time:    c.Time.UTC().Format("2006-01-02 15:04"),
			Open:    c.Open,
			High:    c.High,
			Low:     c.Low,
			Close:   c.Close,
			Volume:  c.Volume,
			IsEntry: i == entryIdx,
			IsExit:  i == exitIdx,
		})
	}

	return window, entryIdx - start, exitIdx - start, nil
}

func nearestIndex(candles []market.Candle, targetUnix int64) int {
	target := time.Unix(targetUnix, 0).UTC()

	i := sort.Search(len(candles), func(i int) bool {
		return !candles[i].Time.Before(target)
	})
	if i < len(candles) && candles[i].Time.Equal(target) {
		return i
	}
	if i == 0 {
		if withinOneBar(candles[0].Time, target) {
			return 0
		}
		return -1
	}
	if i >= len(candles) {
		last := len(candles) - 1
		if withinOneBar(candles[last].Time, target) {
			return last
		}
		return -1
	}

	prevDelta := absDuration(candles[i-1].Time.Sub(target))
	nextDelta := absDuration(candles[i].Time.Sub(target))
	if prevDelta <= nextDelta {
		if prevDelta <= 15*time.Minute {
			return i - 1
		}
		return -1
	}
	if nextDelta <= 15*time.Minute {
		return i
	}
	return -1
}

func renderHTML(outPath string, trade domain.Trade, window []domain.ChartCandle, entryIndex, exitIndex, before, after int) error {
	b, err := json.Marshal(window)
	if err != nil {
		return err
	}

	priceMin := math.Inf(1)
	priceMax := math.Inf(-1)
	for _, c := range window {
		if c.Low < priceMin {
			priceMin = c.Low
		}
		if c.High > priceMax {
			priceMax = c.High
		}
	}
	pad := (priceMax - priceMin) * 0.08
	if pad == 0 {
		pad = priceMax * 0.02
	}
	priceMin -= pad
	priceMax += pad

	dirLabel := "LONG"
	if trade.Direction < 0 {
		dirLabel = "SHORT"
	}

	data := domain.PageData{
		Trade:          trade,
		EntryTime:      time.Unix(trade.EntryTime, 0).UTC().Format("2006-01-02 15:04:05 UTC"),
		ExitTime:       time.Unix(trade.ExitTime, 0).UTC().Format("2006-01-02 15:04:05 UTC"),
		DirectionLabel: dirLabel,
		WindowSummary:  fmt.Sprintf("%d свечей до входа, %d после", before, after),
		CandlesJSON:    template.JS(string(b)),
		EntryIndex:     entryIndex,
		ExitIndex:      exitIndex,
		PriceMin:       priceMin,
		PriceMax:       priceMax,
		ContextBefore:  before,
		ContextAfter:   after,
		WindowStart:    window[0].Time,
		WindowEnd:      window[len(window)-1].Time,
		GeneratedAt:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	}

	tmpl := template.Must(template.New("page").Parse(pageHTML))

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

func withinOneBar(a, b time.Time) bool {
	return absDuration(a.Sub(b)) <= 15*time.Minute
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const pageHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Trade {{.Trade.ID}} - {{.Trade.Symbol}}</title>
  <style>
    :root {
      --bg: #0f1115;
      --card: #171a21;
      --muted: #9aa4b2;
      --text: #ecf0f7;
      --grid: #2b3240;
      --up: #2ecc71;
      --down: #ff5c5c;
      --accent: #4aa8ff;
      --warn: #ffcd10;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 24px;
      font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--text);
    }
    .wrap {
      max-width: 1400px;
      margin: 0 auto;
      display: grid;
      grid-template-columns: 1.6fr 0.9fr;
      gap: 18px;
    }
    .card {
      background: var(--card);
      border: 1px solid #232834;
      border-radius: 18px;
      box-shadow: 0 12px 30px rgba(0,0,0,.22);
      padding: 18px;
    }
    h1, h2, h3, p { margin-top: 0; }
    h1 { font-size: 24px; margin-bottom: 8px; }
    .sub { color: var(--muted); margin-bottom: 12px; }
    .badges { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 14px; }
    .badge {
      padding: 6px 10px;
      border-radius: 999px;
      font-size: 12px;
      background: #202735;
      border: 1px solid #303a4d;
    }
    .badge.win { color: var(--up); }
    .badge.loss { color: var(--down); }
    .badge.warn { color: var(--warn); }
    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 10px;
    }
    .kv {
      background: #121722;
      border: 1px solid #252c39;
      border-radius: 14px;
      padding: 12px;
    }
    .kv .k { color: var(--muted); font-size: 12px; margin-bottom: 6px; }
    .kv .v { font-size: 15px; font-weight: 600; }
    svg {
      width: 100%;
      height: auto;
      background: #0d1118;
      border-radius: 16px;
      border: 1px solid #252c39;
      display: block;
    }
    .legend {
      margin-top: 12px;
      display: flex;
      gap: 14px;
      flex-wrap: wrap;
      color: var(--muted);
      font-size: 13px;
    }
    .legend span::before {
      content: "";
      display: inline-block;
      width: 10px;
      height: 10px;
      border-radius: 50%;
      margin-right: 6px;
      vertical-align: middle;
    }
    .legend .entry::before { background: var(--accent); }
    .legend .exit::before { background: var(--warn); }
    .legend .up::before { background: var(--up); }
    .legend .down::before { background: var(--down); }
    .note {
      color: var(--muted);
      line-height: 1.5;
      font-size: 14px;
    }
    @media (max-width: 980px) {
      .wrap { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>{{.Trade.Symbol}} · сделка #{{.Trade.ID}}</h1>
      <div class="sub">Окно: {{.WindowStart}} → {{.WindowEnd}} · {{.WindowSummary}}</div>
      <div class="badges">
        <div class="badge">{{.DirectionLabel}}</div>
        <div class="badge">state: {{.Trade.EntryState}}</div>
        <div class="badge">reason: {{.Trade.Reason}}</div>
        <div class="badge {{if ge .Trade.PnlUSD 0.0}}win{{else}}loss{{end}}">PnL USD: {{printf "%.2f" .Trade.PnlUSD}}</div>
        <div class="badge {{if ge .Trade.PnlPct 0.0}}win{{else}}loss{{end}}">PnL %%: {{printf "%.3f" .Trade.PnlPct}}</div>
      </div>
      <svg id="chart" viewBox="0 0 1200 720" preserveAspectRatio="xMidYMid meet"></svg>
      <div class="legend">
        <span class="entry">вход</span>
        <span class="exit">выход</span>
        <span class="up">бычья свеча</span>
        <span class="down">медвежья свеча</span>
      </div>
    </div>

    <div class="card">
      <h2>Параметры входа</h2>
      <div class="grid">
        <div class="kv"><div class="k">Вход</div><div class="v">{{.EntryTime}}</div></div>
        <div class="kv"><div class="k">Выход</div><div class="v">{{.ExitTime}}</div></div>
        <div class="kv"><div class="k">Entry price</div><div class="v">{{printf "%.6f" .Trade.EntryPrice}}</div></div>
        <div class="kv"><div class="k">Exit price</div><div class="v">{{printf "%.6f" .Trade.ExitPrice}}</div></div>
        <div class="kv"><div class="k">Z-score</div><div class="v">{{printf "%.4f" .Trade.EntryZScore}}</div></div>
        <div class="kv"><div class="k">Entropy</div><div class="v">{{printf "%.4f" .Trade.EntryEntropy}}</div></div>
        <div class="kv"><div class="k">Long score</div><div class="v">{{printf "%.4f" .Trade.EntryLongScore}}</div></div>
        <div class="kv"><div class="k">Short score</div><div class="v">{{printf "%.4f" .Trade.EntryShortScore}}</div></div>
        <div class="kv"><div class="k">Hold bars</div><div class="v">{{.Trade.HoldBars}}</div></div>
        <div class="kv"><div class="k">Fee USD</div><div class="v">{{printf "%.4f" .Trade.FeeUSD}}</div></div>
      </div>
      <h3 style="margin-top:18px;">Как читать</h3>
      <div class="note">
        Синяя вертикаль — момент входа. Жёлтая — выход. Сверху над входом есть подпись с state, z-score и entropy. Окно вокруг сделки помогает понять, вход был в начале движения или уже в экстремуме.
      </div>
      <div class="note" style="margin-top:10px;">
        Сгенерировано: {{.GeneratedAt}}
      </div>
    </div>
  </div>

<script>
const candles = {{.CandlesJSON}};
const entryIndex = {{.EntryIndex}};
const exitIndex = {{.ExitIndex}};
const priceMin = {{printf "%.12f" .PriceMin}};
const priceMax = {{printf "%.12f" .PriceMax}};
const tradeMeta = {
  symbol: {{printf "%q" .Trade.Symbol}},
  direction: {{printf "%q" .DirectionLabel}},
  state: {{printf "%q" .Trade.EntryState}},
  z: {{printf "%.6f" .Trade.EntryZScore}},
  entropy: {{printf "%.6f" .Trade.EntryEntropy}},
  pnlUsd: {{printf "%.6f" .Trade.PnlUSD}},
  pnlPct: {{printf "%.6f" .Trade.PnlPct}},
  reason: {{printf "%q" .Trade.Reason}}
};

const svg = document.getElementById('chart');
const W = 1200;
const H = 720;
const margin = {top: 42, right: 70, bottom: 76, left: 70};
const plotW = W - margin.left - margin.right;
const plotH = H - margin.top - margin.bottom;
const xStep = plotW / Math.max(candles.length, 1);
const candleBody = Math.max(5, xStep * 0.58);

function y(price) {
  return margin.top + (priceMax - price) / (priceMax - priceMin) * plotH;
}
function x(index) {
  return margin.left + index * xStep + xStep / 2;
}
function line(x1, y1, x2, y2, stroke, width=1, dash='') {
  const el = document.createElementNS('http://www.w3.org/2000/svg', 'line');
  el.setAttribute('x1', x1); el.setAttribute('y1', y1);
  el.setAttribute('x2', x2); el.setAttribute('y2', y2);
  el.setAttribute('stroke', stroke); el.setAttribute('stroke-width', width);
  if (dash) el.setAttribute('stroke-dasharray', dash);
  svg.appendChild(el);
  return el;
}
function text(str, xPos, yPos, fill, size=12, anchor='middle', weight='500') {
  const el = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  el.textContent = str;
  el.setAttribute('x', xPos); el.setAttribute('y', yPos);
  el.setAttribute('fill', fill); el.setAttribute('font-size', size);
  el.setAttribute('text-anchor', anchor);
  el.setAttribute('font-family', 'Inter, system-ui, sans-serif');
  el.setAttribute('font-weight', weight);
  svg.appendChild(el);
  return el;
}
function rect(xPos, yPos, w, h, fill, stroke='none', rx=0, opacity='1') {
  const el = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  el.setAttribute('x', xPos); el.setAttribute('y', yPos);
  el.setAttribute('width', w); el.setAttribute('height', h);
  el.setAttribute('fill', fill); el.setAttribute('stroke', stroke);
  el.setAttribute('rx', rx);
  el.setAttribute('opacity', opacity);
  svg.appendChild(el);
  return el;
}
function circle(cx, cy, r, fill, stroke='none') {
  const el = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  el.setAttribute('cx', cx); el.setAttribute('cy', cy);
  el.setAttribute('r', r); el.setAttribute('fill', fill); el.setAttribute('stroke', stroke);
  svg.appendChild(el);
  return el;
}

rect(0, 0, W, H, '#0d1118');
for (let i = 0; i <= 5; i++) {
  const yy = margin.top + i * plotH / 5;
  line(margin.left, yy, margin.left + plotW, yy, '#2b3240', 1);
  const price = priceMax - (priceMax - priceMin) * (i / 5);
  text(price.toFixed(2), W - 8, yy + 4, '#9aa4b2', 12, 'end', '400');
}

candles.forEach((c, idx) => {
  const cx = x(idx);
  const openY = y(c.Open);
  const closeY = y(c.Close);
  const highY = y(c.High);
  const lowY = y(c.Low);
  const bullish = c.Close >= c.Open;
  const color = bullish ? '#2ecc71' : '#ff5c5c';
  line(cx, highY, cx, lowY, color, 1.5);
  const top = Math.min(openY, closeY);
  const bodyH = Math.max(2, Math.abs(closeY - openY));
  rect(cx - candleBody / 2, top, candleBody, bodyH, color, color, 2);
});

const entryX = x(entryIndex);
const exitX = x(exitIndex);
line(entryX, margin.top, entryX, margin.top + plotH, '#4aa8ff', 2, '6 5');
line(exitX, margin.top, exitX, margin.top + plotH, '#ffcd10', 2, '6 5');

const entryCandle = candles[entryIndex];
const exitCandle = candles[exitIndex];
const entryY = y(entryCandle.Close);
const exitY = y(exitCandle.Close);
circle(entryX, entryY, 7, '#4aa8ff', '#d5e9ff');
circle(exitX, exitY, 7, '#ffcd10', '#fff2b8');

const label =
  tradeMeta.direction + ' | ' +
  tradeMeta.state + ' | z=' +
  tradeMeta.z.toFixed(2) + ' | e=' +
  tradeMeta.entropy.toFixed(2);

text(
  'PnL ' + tradeMeta.pnlUsd.toFixed(2) +
  ' USD (' + tradeMeta.pnlPct.toFixed(2) + '%) · ' +
  tradeMeta.reason,
  Math.min(W - 170, Math.max(200, exitX)),
  Math.min(H - 20, y(exitCandle.Low) + 28),
  '#9aa4b2',
  12,
  'middle',
  '500'
);
const tickStep = Math.max(1, Math.ceil(candles.length / 8));
for (let i = 0; i < candles.length; i += tickStep) {
  const xx = x(i);
  line(xx, margin.top + plotH, xx, margin.top + plotH + 6, '#576175', 1);
  text(candles[i].Time.slice(5), xx, H - 20, '#9aa4b2', 11, 'middle', '400');
}
line(margin.left, margin.top + plotH, margin.left + plotW, margin.top + plotH, '#576175', 1);
line(margin.left, margin.top, margin.left, margin.top + plotH, '#576175', 1);
</script>
</body>
</html>`

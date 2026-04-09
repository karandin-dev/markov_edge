// stats.go
package markov

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/logger"
)

var statsMutex sync.Mutex

// stats.go
func exportTradeStats(pos *Position) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	file, err := os.OpenFile("trade_stats.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Errorf("Failed to open stats file: %v", err)
		return
	}
	defer file.Close()

	// Если файл пустой — пишем заголовок
	if fileInfo, _ := file.Stat(); fileInfo.Size() == 0 {
		file.WriteString("Symbol,Side,EntryScore,MAE_pct,MFE_pct,FinalPnL_pct,ExitReason,Duration_min\n")
	}

	duration := time.Since(pos.EntryTime).Minutes()
	line := fmt.Sprintf("%s,%s,%.4f,%.2f,%.2f,%.2f,%s,%.1f\n",
		pos.Symbol, pos.Side, pos.EntryScore,
		pos.MAE*100, pos.MFE*100, pos.FinalPnL*100,
		pos.ExitReason, duration)

	file.WriteString(line)
}

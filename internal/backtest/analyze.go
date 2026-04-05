package backtest

import "fmt"

func Analyze(results []Observation) {
	type stats struct {
		count int
		sum3  float64
		win3  int
	}

	m := map[string]*stats{}

	for _, r := range results {
		s, ok := m[r.Signal]
		if !ok {
			s = &stats{}
			m[r.Signal] = s
		}

		s.count++
		s.sum3 += r.Return3
		if r.Return3 > 0 {
			s.win3++
		}
	}

	fmt.Println("\nRESULTS (Return over 3 bars):")
	fmt.Println("-------------------------------------")
	fmt.Printf("%-15s %-8s %-10s %-10s\n", "Signal", "Count", "AvgRet", "Winrate")

	for k, v := range m {
		avg := v.sum3 / float64(v.count)
		winrate := float64(v.win3) / float64(v.count)

		fmt.Printf("%-15s %-8d %-10.4f %-10.2f\n",
			k, v.count, avg, winrate)
	}
}

package config

import (
	"fmt"
	"os"
)

type ExchangeSecrets struct {
	APIKey    string
	APISecret string
}

func LoadExchangeSecrets(cfg Config) (ExchangeSecrets, error) {
	var s ExchangeSecrets

	switch cfg.Exchange.Provider {
	case "", "bybit":
		switch cfg.Exchange.Env {
		case "demo":
			s.APIKey = os.Getenv("BYBIT_DEMO_API_KEY")
			s.APISecret = os.Getenv("BYBIT_DEMO_API_SECRET")
		default:
			s.APIKey = os.Getenv("BYBIT_API_KEY")
			s.APISecret = os.Getenv("BYBIT_API_SECRET")
		}
	default:
		return s, fmt.Errorf("unsupported exchange provider: %s", cfg.Exchange.Provider)
	}

	return s, nil
}

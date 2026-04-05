package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/joho/godotenv"

	"markov_screener/internal/config"
	"markov_screener/internal/exchange"
)

func main() {
	_ = godotenv.Load()

	configPath := flag.String("config", "configs/screener.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	secrets, err := config.LoadExchangeSecrets(cfg)
	if err != nil {
		panic(fmt.Errorf("load exchange secrets: %w", err))
	}

	if secrets.APIKey == "" || secrets.APISecret == "" {
		panic("demo api key/secret are empty")
	}

	client := exchange.NewBybitPrivateClient(
		cfg.Exchange.BaseURL,
		secrets.APIKey,
		secrets.APISecret,
		cfg.Exchange.Category,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.PingAuth(ctx); err != nil {
		panic(fmt.Errorf("auth check failed: %w", err))
	}

	fmt.Println("BYBIT DEMO AUTH OK")
}

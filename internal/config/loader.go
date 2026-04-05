package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	applyDefaults(&cfg)

	return cfg, nil
}

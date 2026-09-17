package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port          int    `yaml:"port"`
	MaxToken      string `yaml:"max_token"`
	MaxDeviceID   string `yaml:"max_device_id"`
	APIToken      string `yaml:"api_token"`
	DefaultChatID int64  `yaml:"default_chat_id"`
}

func Load(path string) *Config {
	cfg := &Config{
		Port: 8000,
	}

	data, err := os.ReadFile(path)
	if err == nil {
		yaml.Unmarshal(data, cfg)
	}

	if v := os.Getenv("MAX_TOKEN"); v != "" {
		cfg.MaxToken = v
	}
	if v := os.Getenv("MAX_DEVICE_ID"); v != "" {
		cfg.MaxDeviceID = v
	}
	if v := os.Getenv("API_TOKEN"); v != "" {
		cfg.APIToken = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("DEFAULT_CHAT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.DefaultChatID = id
		}
	}

	return cfg
}

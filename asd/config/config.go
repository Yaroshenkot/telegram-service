package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppID   int
	AppHash string
	Port    int
}

func Load() (*Config, error) {
	appIDStr := os.Getenv("APP_ID")
	if appIDStr == "" {
		return nil, errors.New("APP_ID is required")
	}
	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return nil, err
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return nil, errors.New("APP_HASH is required")
	}

	port := 50051
	if p := os.Getenv("PORT"); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("PORT must be anumber: %w", err)
		}
	}

	return &Config{
		AppID:   appID,
		AppHash: appHash,
		Port:    port,
	}, nil

}
func (c *Config) PortStr() string {
	return fmt.Sprintf(":%d", c.Port)
}

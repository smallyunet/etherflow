package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the EtherFlow indexer
type Config struct {
	RPCURL          string
	DBPath          string
	PollingInterval time.Duration
	StartBlock      uint64
	MaxRetries      int
	RetryDelay      time.Duration
}

// Load loads configuration from environment variables
// In a real app, this might also load from a YAML file
func Load() (*Config, error) {
	cfg := &Config{
		RPCURL:          getEnv("ETHERFLOW_RPC_URL", "http://localhost:8545"),
		DBPath:          getEnv("ETHERFLOW_DB_PATH", "etherflow.db"),
		PollingInterval: getEnvDuration("ETHERFLOW_POLLING_INTERVAL", 2*time.Second),
		StartBlock:      getEnvUint64("ETHERFLOW_START_BLOCK", 0),
		MaxRetries:      getEnvInt("ETHERFLOW_MAX_RETRIES", 5),
		RetryDelay:      getEnvDuration("ETHERFLOW_RETRY_DELAY", 1*time.Second),
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return fallback
}

func getEnvUint64(key string, fallback uint64) uint64 {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseUint(value, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

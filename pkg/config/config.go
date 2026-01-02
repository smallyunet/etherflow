package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for the EtherFlow indexer
type Config struct {
	RPCURL             string        `yaml:"rpc_url"`
	DBDriver           string        `yaml:"db_driver"` // sqlite or postgres
	DBPath             string        `yaml:"db_path"`   // Path for sqlite, DSN for postgres
	PollingInterval    time.Duration `yaml:"polling_interval"`
	StartBlock         uint64        `yaml:"start_block"`
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	SafeWindowSize     uint64        `yaml:"safe_window_size"`
	ParallelProcessing bool          `yaml:"parallel_processing"`
}

// Load loads configuration from environment variables or a config file
func Load() (*Config, error) {
	// 1. Check if config file is specified
	if configPath := os.Getenv("ETHERFLOW_CONFIG_PATH"); configPath != "" {
		return LoadFromFile(configPath)
	}

	// 2. Fallback to env vars
	cfg := &Config{
		RPCURL:             getEnv("ETHERFLOW_RPC_URL", "http://localhost:8545"),
		DBDriver:           getEnv("ETHERFLOW_DB_DRIVER", "sqlite"),
		DBPath:             getEnv("ETHERFLOW_DB_PATH", "etherflow.db"),
		PollingInterval:    getEnvDuration("ETHERFLOW_POLLING_INTERVAL", 2*time.Second),
		StartBlock:         getEnvUint64("ETHERFLOW_START_BLOCK", 0),
		MaxRetries:         getEnvInt("ETHERFLOW_MAX_RETRIES", 5),
		RetryDelay:         getEnvDuration("ETHERFLOW_RETRY_DELAY", 1*time.Second),
		SafeWindowSize:     getEnvUint64("ETHERFLOW_SAFE_WINDOW_SIZE", 128),
		ParallelProcessing: getEnvBool("ETHERFLOW_PARALLEL_PROCESSING", false),
	}
	return cfg, nil
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Set defaults if missing (optional, simplistic approach)
	if cfg.PollingInterval == 0 {
		cfg.PollingInterval = 2 * time.Second
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 1 * time.Second
	}
	if cfg.DBDriver == "" {
		cfg.DBDriver = "sqlite"
	}
	if cfg.SafeWindowSize == 0 {
		cfg.SafeWindowSize = 128
	}

	return &cfg, nil
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

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return fallback
}

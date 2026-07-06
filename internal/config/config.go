// Package config loads runtime settings from env
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL   string
	HTTPAddr      string
	NodeID        int64
	LogLevel      string
	LogFormat     string
	Env           string
	JWTPrivateKey string // PEM PKCS8 ed25519 (empty in dev == ephemeral)
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

// Load reads config from env and validates it
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HTTPAddr:      getDefault("HTTP_ADDR", ":8080"),
		LogLevel:      strings.ToLower(getDefault("LOG_LEVEL", "info")),
		LogFormat:     strings.ToLower(getDefault("LOG_FORMAT", "text")),
		Env:           strings.ToLower(getDefault("ENV", "dev")),
		JWTPrivateKey: os.Getenv("JWT_PRIVATE_KEY"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required and currently not set")
	}

	node, err := getInt("NODE_ID", 0)
	if err != nil {
		return nil, err
	}
	if node < 0 || node > 1023 {
		return nil, fmt.Errorf("config: NODE_ID must be 0..1023, got %d", node)
	}
	c.NodeID = node

	if c.AccessTTL, err = getDuration("ACCESS_TTL", 15*time.Minute); err != nil {
		return nil, err
	}
	if c.RefreshTTL, err = getDuration("REFRESH_TTL", 720*time.Hour); err != nil {
		return nil, err
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("config: bad LOG_LEVEL %q", c.LogLevel)
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return nil, fmt.Errorf("config: bad LOG_FORMAT %q", c.LogFormat)
	}

	if c.Env != "dev" && c.JWTPrivateKey == "" {
		return nil, fmt.Errorf("config: JWT_PRIVATE_KEY is required outside dev")
	}

	return c, nil
}

func (c *Config) IsDev() bool { return c.Env == "dev" }

func getDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q", key, v)
	}
	return n, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration, to %q", key, v)
	}
	return d, nil
}

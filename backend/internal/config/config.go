// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	JWTSecret       []byte
	JWTTTL          time.Duration
	WSTicketTTL     time.Duration
	BcryptCost      int
	AllowedOrigins  []string // for CORS and WebSocket Origin checks; empty = same-origin / native clients only
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

const minSecretLen = 32

// Load reads the configuration and reports all problems at once.
func Load() (Config, error) {
	var errs []error

	cfg := Config{
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   []byte(os.Getenv("JWT_SECRET")),
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < minSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes", minSecretLen))
	}

	cfg.JWTTTL = parse(&errs, "JWT_TTL", "24h", time.ParseDuration)
	cfg.WSTicketTTL = parse(&errs, "WS_TICKET_TTL", "30s", time.ParseDuration)
	cfg.ShutdownTimeout = parse(&errs, "SHUTDOWN_TIMEOUT", "15s", time.ParseDuration)
	cfg.BcryptCost = parse(&errs, "BCRYPT_COST", "12", strconv.Atoi)
	if cfg.BcryptCost < 10 || cfg.BcryptCost > 15 {
		errs = append(errs, errors.New("BCRYPT_COST must be between 10 and 15"))
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(getEnv("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}

	return cfg, errors.Join(errs...)
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func parse[T any](errs *[]error, key, def string, fn func(string) (T, error)) T {
	v, err := fn(getEnv(key, def))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
	}
	return v
}

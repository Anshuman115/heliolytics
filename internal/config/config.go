package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Addr              string
	SigningSecret     string
	DBURL             string
	RateLimitPerMin   int
	ReparseEnabled    bool
	ReparseSecret     string
	TrustProxy        bool
}

var weakSigningSecrets = []string{
	"REDACTED-EXAMPLE-PLACEHOLDER",
	"REDACTED-PUBLISHED-DEV-KEY",
}

func Load() Config {
	cfg := Config{
		Addr:            env("ADDR", ":8080"),
		SigningSecret:   env("HELIOLYTICS_SIGNING_SECRET", ""),
		DBURL:           env("DATABASE_URL", ""),
		RateLimitPerMin: EnvInt("RATE_LIMIT_PER_MIN", 120),
		ReparseEnabled:  env("REPARSE_ENABLED", "") == "true",
		ReparseSecret:   env("REPARSE_SECRET", ""),
		TrustProxy:      env("TRUST_PROXY", "") == "true",
	}
	if cfg.DBURL == "" {
		cfg.DBURL = "set DATABASE_URL in environment"
	}
	for _, weak := range weakSigningSecrets {
		if cfg.SigningSecret == weak {
			log.Printf("WARNING: HELIOLYTICS_SIGNING_SECRET matches a known default — change before production")
			break
		}
	}
	return cfg
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func EnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

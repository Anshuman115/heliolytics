package config

import (
	"crypto/sha256"
	"encoding/hex"
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

// SHA-256 digests of known development signing secrets. Digests are used so the
// literal values are not repeated in the repository, since both were published in
// its history and remain recoverable from it.
var weakSigningSecretDigests = []string{
	"b3cf5152a10b7a596ff461bfb6e04203341d08239fc80a56ce135f187ef29e6e",
	"93ec5f0a32098f9e89c82e40979731bbe9cf5f391008333cd96ed586187d4d36",
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
		log.Fatal("DATABASE_URL is required. Set it in the environment or deploy/.env")
	}
	for _, digest := range weakSigningSecretDigests {
		sum := sha256.Sum256([]byte(cfg.SigningSecret))
		if hex.EncodeToString(sum[:]) == digest {
			log.Printf("WARNING: HELIOLYTICS_SIGNING_SECRET matches a published default, change it before production")
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

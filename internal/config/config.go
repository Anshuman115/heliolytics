// Package config loads the C7 API settings. Loading never terminates the
// process, never prints secrets and never falls back on a malformed value.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
)

// Defaults from the C7 contract.
const (
	DefaultRateLimitPerMin        = 120
	DefaultRateLimitPerKeyPerMin  = 600
	DefaultRequestTimeoutSeconds  = 90
	DefaultMaxConcurrentBodyReads = 2

	maxRequestTimeoutSeconds = 90
	defaultAddr              = ":8080"
)

// Config is the full API configuration. Commit and BuiltAt are injected by
// cmd/server from linker flags; an empty value means the binary was built
// without them and /version reports null.
type Config struct {
	Addr                   string
	DBURL                  string
	Keys                   auth.KeySet
	RateLimitPerMin        int
	RateLimitPerKeyPerMin  int
	RequestTimeout         time.Duration
	MaxConcurrentBodyReads int
	TrustProxy             bool
	TrustedProxyCIDRs      []netip.Prefix
	Commit                 string
	BuiltAt                string
}

// Load reads the environment once. Errors name the variable they came from
// and never include the supplied value.
func Load() (Config, error) {
	cfg := Config{
		Addr:                   defaultAddr,
		RateLimitPerMin:        DefaultRateLimitPerMin,
		RateLimitPerKeyPerMin:  DefaultRateLimitPerKeyPerMin,
		RequestTimeout:         DefaultRequestTimeoutSeconds * time.Second,
		MaxConcurrentBodyReads: DefaultMaxConcurrentBodyReads,
	}

	var err error
	if cfg.DBURL, err = requiredEnv("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	keys, err := requiredEnv("HELIOLYTICS_API_KEYS")
	if err != nil {
		return Config{}, err
	}
	if cfg.Keys, err = auth.ParseKeySet(keys); err != nil {
		return Config{}, fmt.Errorf("config: HELIOLYTICS_API_KEYS: %w", err)
	}
	if cfg.Addr, err = stringEnv("ADDR", defaultAddr); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitPerMin, err = positiveIntEnv("RATE_LIMIT_PER_MIN", DefaultRateLimitPerMin, 0); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitPerKeyPerMin, err = positiveIntEnv("RATE_LIMIT_PER_KEY_PER_MIN", DefaultRateLimitPerKeyPerMin, 0); err != nil {
		return Config{}, err
	}
	timeout, err := positiveIntEnv("REQUEST_TIMEOUT_SECONDS", DefaultRequestTimeoutSeconds, maxRequestTimeoutSeconds)
	if err != nil {
		return Config{}, err
	}
	cfg.RequestTimeout = time.Duration(timeout) * time.Second
	if cfg.MaxConcurrentBodyReads, err = positiveIntEnv("MAX_CONCURRENT_BODY_READS", DefaultMaxConcurrentBodyReads, 0); err != nil {
		return Config{}, err
	}
	if cfg.TrustProxy, err = boolEnv("TRUST_PROXY", false); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxyCIDRs, err = cidrEnv("TRUSTED_PROXY_CIDRS"); err != nil {
		return Config{}, err
	}
	if cfg.TrustProxy && len(cfg.TrustedProxyCIDRs) == 0 {
		return Config{}, errors.New("config: TRUSTED_PROXY_CIDRS is required when TRUST_PROXY is true")
	}
	return cfg, nil
}

// requiredEnv returns a non-empty variable and refuses an unset one.
func requiredEnv(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("config: %s is required", name)
	}
	return value, nil
}

// stringEnv applies a default only when the variable is unset, so an explicit
// empty override is an error rather than a silent default.
func stringEnv(name, def string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return def, nil
	}
	if value == "" {
		return "", fmt.Errorf("config: %s must not be empty", name)
	}
	return value, nil
}

// positiveIntEnv parses an integer above zero, refusing overflow and an
// optional upper bound.
func positiveIntEnv(name string, def, max int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return def, nil
	}
	if value == "" {
		return 0, fmt.Errorf("config: %s must not be empty", name)
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer", name)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", name)
	}
	if max > 0 && n > max {
		return 0, fmt.Errorf("config: %s must be at most %d", name, max)
	}
	return n, nil
}

// boolEnv accepts exactly true or false.
func boolEnv(name string, def bool) (bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return def, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("config: %s must be true or false", name)
}

// cidrEnv parses a comma-separated prefix list. An empty entry or a bad
// prefix is an error.
func cidrEnv(name string) ([]netip.Prefix, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("config: %s has an empty entry", name)
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil || prefix.Addr().Zone() != "" {
			return nil, fmt.Errorf("config: %s has an invalid CIDR", name)
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

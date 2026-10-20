package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

var configEnvs = []string{
	"DATABASE_URL",
	"HELIOLYTICS_WEB_API_KEY",
	"HELIOLYTICS_PHONE_API_KEY",
	"ADDR",
	"RATE_LIMIT_PER_MIN",
	"RATE_LIMIT_PER_KEY_PER_MIN",
	"REQUEST_TIMEOUT_SECONDS",
	"MAX_CONCURRENT_BODY_READS",
	"TRUST_PROXY",
	"TRUSTED_PROXY_CIDRS",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range configEnvs {
		t.Setenv(name, "unused")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv(%s): %v", name, err)
		}
	}
}

func loadWith(t *testing.T, overrides map[string]string) error {
	t.Helper()
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "phone."+strings.Repeat("11", 32))
	for name, value := range overrides {
		t.Setenv(name, value)
	}
	_, err := Load()
	return err
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "phone."+strings.Repeat("11", 32))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
	if cfg.RateLimitPerMin != DefaultRateLimitPerMin {
		t.Fatalf("RateLimitPerMin = %d", cfg.RateLimitPerMin)
	}
	if cfg.RateLimitPerKeyPerMin != DefaultRateLimitPerKeyPerMin {
		t.Fatalf("RateLimitPerKeyPerMin = %d", cfg.RateLimitPerKeyPerMin)
	}
	if cfg.RequestTimeout != DefaultRequestTimeoutSeconds*time.Second {
		t.Fatalf("RequestTimeout = %s", cfg.RequestTimeout)
	}
	if cfg.MaxConcurrentBodyReads != DefaultMaxConcurrentBodyReads {
		t.Fatalf("MaxConcurrentBodyReads = %d", cfg.MaxConcurrentBodyReads)
	}
	if cfg.TrustProxy {
		t.Fatal("TrustProxy defaulted to true")
	}
	if len(cfg.TrustedProxyCIDRs) != 0 {
		t.Fatalf("TrustedProxyCIDRs = %v", cfg.TrustedProxyCIDRs)
	}
	if cfg.Commit != "" || cfg.BuiltAt != "" {
		t.Fatalf("build metadata defaulted: %q %q", cfg.Commit, cfg.BuiltAt)
	}
	if cfg.Keys.Empty() {
		t.Fatal("keys not loaded")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "phone."+strings.Repeat("11", 32))
	t.Setenv("ADDR", "127.0.0.1:9090")
	t.Setenv("RATE_LIMIT_PER_MIN", "5")
	t.Setenv("RATE_LIMIT_PER_KEY_PER_MIN", "7")
	t.Setenv("REQUEST_TIMEOUT_SECONDS", "30")
	t.Setenv("MAX_CONCURRENT_BODY_READS", "3")
	t.Setenv("TRUST_PROXY", "true")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32, ::1/128")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9090" || cfg.RateLimitPerMin != 5 || cfg.RateLimitPerKeyPerMin != 7 {
		t.Fatalf("address or rates ignored: %+v", cfg)
	}
	if cfg.RequestTimeout != 30*time.Second || cfg.MaxConcurrentBodyReads != 3 {
		t.Fatalf("timeout or slots ignored: %+v", cfg)
	}
	if !cfg.TrustProxy || len(cfg.TrustedProxyCIDRs) != 2 {
		t.Fatalf("proxy settings ignored: %v %v", cfg.TrustProxy, cfg.TrustedProxyCIDRs)
	}
	if got := cfg.TrustedProxyCIDRs[0].String(); got != "127.0.0.1/32" {
		t.Fatalf("first CIDR = %q", got)
	}
}

func TestLoadRequiresBaseSettings(t *testing.T) {
	clearEnv(t)
	t.Setenv("HELIOLYTICS_WEB_API_KEY", "web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", "phone."+strings.Repeat("11", 32))
	if _, err := Load(); err == nil {
		t.Fatal("missing DATABASE_URL accepted")
	}

	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	if _, err := Load(); err == nil {
		t.Fatal("missing API keys accepted")
	}
}

func TestLoadRejectsInvalidOverrides(t *testing.T) {
	publishedSecret := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	cases := map[string]map[string]string{
		"zero rate":             {"RATE_LIMIT_PER_MIN": "0"},
		"negative rate":         {"RATE_LIMIT_PER_KEY_PER_MIN": "-5"},
		"non numeric rate":      {"RATE_LIMIT_PER_MIN": "fast"},
		"empty rate":            {"RATE_LIMIT_PER_MIN": ""},
		"non numeric timeout":   {"REQUEST_TIMEOUT_SECONDS": "soon"},
		"zero timeout":          {"REQUEST_TIMEOUT_SECONDS": "0"},
		"ninety one timeout":    {"REQUEST_TIMEOUT_SECONDS": "91"},
		"overflow slots":        {"MAX_CONCURRENT_BODY_READS": "99999999999999999999999"},
		"zero slots":            {"MAX_CONCURRENT_BODY_READS": "0"},
		"bad boolean":           {"TRUST_PROXY": "TRUE"},
		"trusted without CIDR":  {"TRUST_PROXY": "true"},
		"bad CIDR":              {"TRUSTED_PROXY_CIDRS": "127.0.0.1"},
		"empty CIDR entry":      {"TRUSTED_PROXY_CIDRS": "127.0.0.1/32,,"},
		"empty address":         {"ADDR": ""},
		"wrong prefix":          {"HELIOLYTICS_PHONE_API_KEY": "web." + strings.Repeat("11", 32)},
		"published example key": {"HELIOLYTICS_PHONE_API_KEY": "phone." + publishedSecret},
		"short secret":          {"HELIOLYTICS_PHONE_API_KEY": "phone." + strings.Repeat("11", 31)},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			if err := loadWith(t, overrides); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestLoadErrorsNeverEchoSecrets(t *testing.T) {
	publishedSecret := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	err := loadWith(t, map[string]string{
		"HELIOLYTICS_PHONE_API_KEY": "phone." + publishedSecret,
	})
	if err == nil {
		t.Fatal("published example key accepted")
	}
	if strings.Contains(err.Error(), publishedSecret) {
		t.Fatalf("error echoed the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "HELIOLYTICS_PHONE_API_KEY") {
		t.Fatalf("error does not name the variable: %v", err)
	}
}

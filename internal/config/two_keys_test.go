package config

import (
	"os"
	"strings"
	"testing"
)

func setTwoKeys(t *testing.T, web, phone string) {
	t.Helper()
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	t.Setenv("HELIOLYTICS_WEB_API_KEY", web)
	t.Setenv("HELIOLYTICS_PHONE_API_KEY", phone)
}

func TestLoadTwoAPIKeys(t *testing.T) {
	setTwoKeys(t, "web."+strings.Repeat("22", 32), "phone."+strings.Repeat("11", 32))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid two-key configuration rejected: %v", err)
	}
	for _, tc := range []struct {
		kid, secret string
		canWrite    bool
	}{
		{"web", strings.Repeat("22", 32), false},
		{"phone", strings.Repeat("11", 32), true},
	} {
		key, err := cfg.Keys.Lookup(tc.kid)
		if err != nil {
			t.Fatalf("%s key not loaded: %v", tc.kid, err)
		}
		if key.SecretHex() != tc.secret || key.CanWrite() != tc.canWrite {
			t.Fatalf("%s key has the wrong secret or permissions", tc.kid)
		}
	}
}

func TestLoadRejectsInvalidTwoAPIKeys(t *testing.T) {
	web := "web." + strings.Repeat("22", 32)
	phone := "phone." + strings.Repeat("11", 32)
	published := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	cases := []struct {
		name, web, phone, variable string
	}{
		{"empty web", "", phone, "HELIOLYTICS_WEB_API_KEY"},
		{"empty phone", web, "", "HELIOLYTICS_PHONE_API_KEY"},
		{"web wrong prefix", phone, phone, "HELIOLYTICS_WEB_API_KEY"},
		{"phone wrong prefix", web, web, "HELIOLYTICS_PHONE_API_KEY"},
		{"swapped", phone, web, "HELIOLYTICS_WEB_API_KEY"},
		{"web malformed", "web.not-a-secret", phone, "HELIOLYTICS_WEB_API_KEY"},
		{"phone malformed", web, "phone.not-a-secret", "HELIOLYTICS_PHONE_API_KEY"},
		{"web uppercase", "web." + strings.Repeat("AB", 32), phone, "HELIOLYTICS_WEB_API_KEY"},
		{"phone uppercase", web, "phone." + strings.Repeat("AB", 32), "HELIOLYTICS_PHONE_API_KEY"},
		{"web list", web + "," + phone, phone, "HELIOLYTICS_WEB_API_KEY"},
		{"phone list", web, phone + "," + web, "HELIOLYTICS_PHONE_API_KEY"},
		{"same secret", web, "phone." + strings.Repeat("22", 32), "HELIOLYTICS_PHONE_API_KEY"},
		{"web fixture", "web." + published, phone, "HELIOLYTICS_WEB_API_KEY"},
		{"phone fixture", web, "phone." + published, "HELIOLYTICS_PHONE_API_KEY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setTwoKeys(t, tc.web, tc.phone)
			_, err := Load()
			if err == nil {
				t.Fatal("invalid keys accepted")
			}
			if !strings.Contains(err.Error(), tc.variable) {
				t.Fatalf("error does not identify %s: %v", tc.variable, err)
			}
			for _, value := range []string{tc.web, tc.phone} {
				if value != "" && strings.Contains(err.Error(), value) {
					t.Fatalf("error exposed key: %v", err)
				}
			}
		})
	}
	for _, variable := range []string{"HELIOLYTICS_WEB_API_KEY", "HELIOLYTICS_PHONE_API_KEY"} {
		t.Run("missing "+variable, func(t *testing.T) {
			setTwoKeys(t, web, phone)
			if err := os.Unsetenv(variable); err != nil {
				t.Fatal(err)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), variable) {
				t.Fatalf("missing key not identified: %v", err)
			}
		})
	}
}

func TestLoadRejectsOldKeysOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://app@127.0.0.1:5432/heliolytics")
	t.Setenv("HELIOLYTICS_API_KEYS", "phone."+strings.Repeat("11", 32)+",web."+strings.Repeat("22", 32))
	t.Setenv("HELIOLYTICS_API_KEY", "web."+strings.Repeat("22", 32))
	for _, variable := range []string{"HELIOLYTICS_WEB_API_KEY", "HELIOLYTICS_PHONE_API_KEY"} {
		t.Setenv(variable, "")
		if err := os.Unsetenv(variable); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(); err == nil {
		t.Fatal("old variables started the API without the two required keys")
	}
}

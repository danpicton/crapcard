package main

import "testing"

func TestConfigFromEnvDefaults(t *testing.T) {
	cfg, addr, dbPath := configFrom(func(string) string { return "" })

	if addr != defaultAddr {
		t.Fatalf("addr = %q, want %q", addr, defaultAddr)
	}
	if dbPath != defaultDBPath {
		t.Fatalf("db path = %q, want %q", dbPath, defaultDBPath)
	}
	if cfg.TrustProxy {
		t.Fatalf("TrustProxy defaults to true — forwarded headers must be opt-in")
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	env := map[string]string{
		"CRAPCARD_ADDR":    ":9000",
		"CRAPCARD_DB_PATH": "/data/cards.db",
		"TRUST_PROXY":      "true",
	}
	cfg, addr, dbPath := configFrom(func(k string) string { return env[k] })

	if addr != ":9000" {
		t.Fatalf("addr = %q", addr)
	}
	if dbPath != "/data/cards.db" {
		t.Fatalf("db path = %q", dbPath)
	}
	if !cfg.TrustProxy {
		t.Fatalf("TRUST_PROXY=true was not honoured")
	}
}

func TestTrustProxyOnlyAcceptsAffirmativeValues(t *testing.T) {
	for _, v := range []string{"false", "0", "no", "", "maybe"} {
		cfg, _, _ := configFrom(func(k string) string {
			if k == "TRUST_PROXY" {
				return v
			}
			return ""
		})
		if cfg.TrustProxy {
			t.Fatalf("TRUST_PROXY=%q enabled proxy trust", v)
		}
	}
	for _, v := range []string{"true", "1", "yes"} {
		cfg, _, _ := configFrom(func(k string) string {
			if k == "TRUST_PROXY" {
				return v
			}
			return ""
		})
		if !cfg.TrustProxy {
			t.Fatalf("TRUST_PROXY=%q did not enable proxy trust", v)
		}
	}
}

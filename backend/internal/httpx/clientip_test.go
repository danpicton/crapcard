package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danpicton/crapcard/internal/httpx"
)

func TestClientIPUsesRemoteAddrByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:54321"

	if got := httpx.ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want 203.0.113.7", got)
	}
}

func TestClientIPIgnoresForwardedHeadersWhenProxyNotTrusted(t *testing.T) {
	// Any client can set these; honouring them unconditionally would let an
	// attacker forge the IP written to the audit log.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")

	if got := httpx.ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want the real peer 203.0.113.7", got)
	}
	if httpx.ProxyTrusted(req) {
		t.Fatalf("ProxyTrusted = true without the TrustProxy middleware")
	}
}

func TestTrustProxyHonoursForwardedHeaders(t *testing.T) {
	cases := map[string]struct {
		headers map[string]string
		want    string
	}{
		"x-forwarded-for":       {map[string]string{"X-Forwarded-For": "198.51.100.1"}, "198.51.100.1"},
		"x-forwarded-for chain": {map[string]string{"X-Forwarded-For": "198.51.100.1, 10.0.0.1"}, "198.51.100.1"},
		"x-real-ip":             {map[string]string{"X-Real-IP": "198.51.100.9"}, "198.51.100.9"},
		"neither":               {nil, "203.0.113.7"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotIP string
			var gotTrusted bool
			h := httpx.TrustProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotIP = httpx.ClientIP(r)
				gotTrusted = httpx.ProxyTrusted(r)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			if gotIP != tc.want {
				t.Fatalf("ClientIP = %q, want %q", gotIP, tc.want)
			}
			if !gotTrusted {
				t.Fatalf("ProxyTrusted = false behind TrustProxy")
			}
		})
	}
}

// Package httpx holds small HTTP helpers shared across feature packages.
package httpx

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// clientIPKey carries the proxy-resolved client IP in the request context.
type clientIPKey struct{}

// proxyTrustedKey marks a request as having arrived via a trusted proxy, so
// other forwarded headers (e.g. X-Forwarded-Proto) may be honoured too.
type proxyTrustedKey struct{}

// ClientIP returns a stable key identifying the requesting client.
//
// By default it is the host part of r.RemoteAddr — the connection's real
// peer. X-Forwarded-For and X-Real-IP are deliberately ignored because any
// client can set them: trusting them lets an attacker forge the IP recorded
// in audit logs.
//
// Deployments behind a trusted reverse proxy opt in by wrapping the handler
// chain with TrustProxy (TRUST_PROXY env var); ClientIP then returns the IP
// that middleware resolved from the forwarded headers.
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok && ip != "" {
		return ip
	}
	return remoteHost(r)
}

// ProxyTrusted reports whether the request passed through the TrustProxy
// middleware, i.e. forwarded headers set by the proxy may be honoured.
func ProxyTrusted(r *http.Request) bool {
	trusted, _ := r.Context().Value(proxyTrustedKey{}).(bool)
	return trusted
}

// TrustProxy is middleware for deployments sitting behind a reverse proxy the
// operator controls. It resolves the client IP from X-Forwarded-For (first
// entry) or X-Real-IP and stores it on the context, and marks the request as
// proxied. Enable it only when the proxy strips inbound copies of those
// headers, otherwise clients can spoof them.
func TrustProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), proxyTrustedKey{}, true)
		if ip := forwardedIP(r); ip != "" {
			ctx = context.WithValue(ctx, clientIPKey{}, ip)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// forwardedIP extracts the originating client IP from proxy headers.
func forwardedIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// The left-most entry is the original client; the rest are proxies.
		first, _, _ := strings.Cut(xff, ",")
		if ip := strings.TrimSpace(first); ip != "" {
			return ip
		}
	}
	return strings.TrimSpace(r.Header.Get("X-Real-IP"))
}

// remoteHost strips the port from r.RemoteAddr.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

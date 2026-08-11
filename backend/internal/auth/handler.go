package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danpicton/crapcard/internal/httpx"
)

// sessionCookieName is the cookie the browser session is carried in.
const sessionCookieName = "session"

// MinPasswordLength is the shortest password the setup and change-password
// flows accept.
const MinPasswordLength = 8

// Handler holds HTTP handlers for auth endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a new auth Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// isHTTPS reports whether the request arrived over HTTPS — either directly
// (r.TLS != nil) or via a trusted reverse proxy that signals it with
// X-Forwarded-Proto. The header is only honoured when the deployment has
// opted in via httpx.TrustProxy, since any client can set it otherwise.
//
// This gates the cookie's Secure flag: hardcoding Secure:true breaks
// plain-HTTP deployments, because browsers silently discard secure cookies
// sent over HTTP and every session is invalid the moment it is issued.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil ||
		(httpx.ProxyTrusted(r) && r.Header.Get("X-Forwarded-Proto") == "https")
}

// Login handles POST /api/auth/login.
// Body: {"username": "...", "password": "..."}
// Sets an HttpOnly session cookie on success.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sess, err := h.svc.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		slog.Warn("audit: login failed",
			"event", "login_failed",
			"username", req.Username,
			"ip", httpx.ClientIP(r),
		)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		slog.Error("audit: login error", "event", "login_error", "err", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	slog.Info("audit: login succeeded",
		"event", "login_succeeded",
		"username", req.Username,
		"ip", httpx.ClientIP(r),
	)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Logout handles POST /api/auth/logout: deletes the session and expires the
// cookie. It succeeds even without a valid session so the client can always
// clear its state.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			slog.Error("logout: delete session", "err", err)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Me handles GET /api/auth/me, returning the authenticated user. It must be
// mounted behind RequireAuth.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(u))
}

// SetupStatus handles GET /api/auth/setup-status, telling an unauthenticated
// client whether the instance still needs its first admin.
func (h *Handler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	needs, err := h.svc.NeedsSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": needs})
}

// Setup handles POST /api/auth/setup, creating the first admin user. It is
// the only unauthenticated write endpoint, and is refused with 409 once any
// user exists so it cannot be replayed to add a second admin.
func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < MinPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	needs, err := h.svc.NeedsSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read setup status")
		return
	}
	if !needs {
		writeError(w, http.StatusConflict, "setup has already been completed")
		return
	}

	if err := h.svc.SeedAdmin(r.Context(), req.Username, req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, "setup failed")
		return
	}

	slog.Info("audit: setup completed",
		"event", "setup_completed",
		"username", req.Username,
		"ip", httpx.ClientIP(r),
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// publicUser projects a User onto the fields safe to send to a client —
// notably excluding the password hash.
func publicUser(u *User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"is_admin":   u.IsAdmin,
		"created_at": u.CreatedAt,
	}
}

// writeJSON and writeError are package-local names for the shared httpx
// helpers, keeping the handler bodies terse.
func writeJSON(w http.ResponseWriter, status int, v any) {
	httpx.WriteJSON(w, status, v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	httpx.WriteError(w, status, msg)
}

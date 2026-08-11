// Package images stores the pictures pasted into a card, as blobs in the same
// SQLite file as everything else. One file is the whole backup.
package images

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/httpx"
)

// Config bounds what a user may store.
type Config struct {
	// MaxImageSize is the largest single upload, in bytes.
	MaxImageSize int64
	// MaxTotalBytes is the most one user may store in total.
	MaxTotalBytes int64
}

// DefaultConfig returns the shipped limits: 10 MB per image, 500 MB per user.
func DefaultConfig() Config {
	return Config{
		MaxImageSize:  10 << 20,
		MaxTotalBytes: 500 << 20,
	}
}

// allowedImageMIMEs enumerates the types accepted. It is deliberately a
// closed list of raster formats.
//
// SVG is excluded on purpose: it is a document format that can carry script,
// and serving one from the app's own origin would be a stored XSS.
var allowedImageMIMEs = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/gif":  {},
	"image/webp": {},
}

// Handler serves image upload and retrieval.
type Handler struct {
	db  *db.DB
	cfg Config
}

// NewHandler creates a Handler with the default limits.
func NewHandler(database *db.DB) *Handler {
	return NewHandlerWith(database, DefaultConfig())
}

// NewHandlerWith creates a Handler with explicit limits.
func NewHandlerWith(database *db.DB, cfg Config) *Handler {
	return &Handler{db: database, cfg: cfg}
}

// Middleware wraps a handler, typically with authentication.
type Middleware func(http.Handler) http.Handler

// Register mounts the image routes.
func (h *Handler) Register(mux *http.ServeMux, requireAuth Middleware) {
	mux.Handle("POST /api/images", requireAuth(http.HandlerFunc(h.Upload)))
	mux.Handle("GET /api/images/{id}", requireAuth(http.HandlerFunc(h.Serve)))
}

// Upload handles POST /api/images with the raw image bytes as the body, which
// is what a clipboard paste produces. It responds with the URL to embed in
// the note's markdown.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	// Read one byte past the limit so an oversized body is detected rather
	// than silently truncated into a valid-looking image.
	limited := io.LimitReader(r.Body, h.cfg.MaxImageSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read image")
		return
	}
	if int64(len(data)) > h.cfg.MaxImageSize {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image is too large")
		return
	}
	if len(data) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "empty image")
		return
	}

	// Sniff the bytes rather than trusting the declared Content-Type: a
	// client can label anything image/png.
	mime := http.DetectContentType(data)
	if _, ok := allowedImageMIMEs[mime]; !ok {
		httpx.WriteError(w, http.StatusBadRequest,
			"unsupported image type (png, jpeg, gif and webp are accepted)")
		return
	}

	used, err := currentImageBytes(r.Context(), h.db, u.ID)
	if err != nil {
		slog.Error("image quota check", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "upload failed")
		return
	}
	if used+int64(len(data)) > h.cfg.MaxTotalBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "image storage quota exceeded")
		return
	}

	id, err := newID()
	if err != nil {
		slog.Error("image id", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO images(id, user_id, mime_type, byte_size, data) VALUES(?, ?, ?, ?, ?)`,
		id, u.ID, mime, len(data), data,
	); err != nil {
		slog.Error("store image", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":        id,
		"url":       "/api/images/" + id,
		"mime_type": mime,
		"size":      len(data),
	})
}

// Serve handles GET /api/images/{id}, returning the bytes to their owner.
func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, http.StatusNotFound, "image not found")
		return
	}

	var mime string
	var data []byte
	err := h.db.QueryRowContext(r.Context(),
		`SELECT mime_type, data FROM images WHERE id=? AND user_id=?`, id, u.ID,
	).Scan(&mime, &data)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "image not found")
		return
	}
	if err != nil {
		slog.Error("read image", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not read image")
		return
	}

	// These bytes came from a user and are served from the app's own origin.
	// nosniff stops the browser second-guessing the type into something
	// executable, and the sandbox CSP neuters anything that slips through.
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	// Content is immutable: the id is derived from random bytes, never reused.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Error("write image", "err", err)
	}
}

// currentImageBytes totals what the user already stores.
func currentImageBytes(ctx context.Context, database *db.DB, userID int64) (int64, error) {
	var total sql.NullInt64
	if err := database.QueryRowContext(ctx,
		`SELECT SUM(byte_size) FROM images WHERE user_id=?`, userID,
	).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum image bytes: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

// newID returns a random, unguessable image id.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

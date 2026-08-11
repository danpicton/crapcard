// Command server runs crapcard: the API and the embedded single-page app,
// backed by one SQLite file.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danpicton/crapcard/internal/db"
)

const (
	defaultAddr   = ":8080"
	defaultDBPath = "crapcard.db"
)

// configFrom reads settings from the environment. The lookup function is a
// parameter so it can be tested without touching the process environment.
func configFrom(getenv func(string) string) (Config, string, string) {
	cfg := Config{
		// Forwarded headers are spoofable by any client, so honouring them
		// has to be an explicit deployment decision.
		TrustProxy: isAffirmative(getenv("TRUST_PROXY")),
	}

	addr := getenv("CRAPCARD_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	dbPath := getenv("CRAPCARD_DB_PATH")
	if dbPath == "" {
		dbPath = defaultDBPath
	}
	return cfg, addr, dbPath
}

func isAffirmative(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, addr, dbPath := configFrom(os.Getenv)

	database, err := db.Open(db.Config{SQLitePath: dbPath})
	if err != nil {
		slog.Error("open database", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer database.Close()

	handler, err := newServer(database, cfg)
	if err != nil {
		slog.Error("build server", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: image uploads and slow clients would otherwise be
		// cut off mid-transfer.
		IdleTimeout: 120 * time.Second,
	}

	// Sweep expired sessions on start-up rather than on every request.
	go func() {
		if err := purgeExpiredSessions(database); err != nil {
			slog.Error("purge expired sessions", "err", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("crapcard listening", "addr", addr, "db", dbPath, "trust_proxy", cfg.TrustProxy)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}

// purgeExpiredSessions clears sessions whose expiry has passed.
func purgeExpiredSessions(database *db.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := database.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	return err
}

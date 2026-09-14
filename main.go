// Command minecraft-whitelist-sync runs the whitelistsync.Syncer as a long
// -lived service: an HTTP server for Authentik's webhook, plus the
// background debounce and reconcile loop.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/minz1/minecraft-whitelist-sync/internal/whitelistsync"
)

const (
	defaultRCONTimeout       = 10 * time.Second
	defaultHTTPTimeout       = 15 * time.Second
	defaultDebounce          = 5 * time.Second
	defaultMinInterval       = 30 * time.Second
	defaultReconcileInterval = 15 * time.Minute
	readHeaderTimeout        = 10 * time.Second
	shutdownTimeout          = 5 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("exiting", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := loadConfig(log)
	syncer := whitelistsync.New(cfg, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go syncer.Run(ctx)

	server := &http.Server{
		Addr:              envOr("LISTEN_ADDR", ":8765"),
		Handler:           syncer.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()

	log.Info("listening", "addr", server.Addr)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func loadConfig(log *slog.Logger) whitelistsync.Config {
	return whitelistsync.Config{
		RCONHost:     mustEnv(log, "RCON_HOST"),
		RCONPort:     envOr("RCON_PORT", "25575"),
		RCONPassword: mustEnv(log, "RCON_PASSWORD"),
		RCONTimeout:  defaultRCONTimeout,

		AuthentikURL:   mustEnv(log, "AUTHENTIK_URL"),
		AuthentikToken: mustEnv(log, "AUTHENTIK_TOKEN"),
		ClientCertPath: mustEnv(log, "AUTHENTIK_CLIENT_CERT"),
		ClientKeyPath:  mustEnv(log, "AUTHENTIK_CLIENT_KEY"),

		WhitelistFile: envOr("WHITELIST_FILE", "/persist/atm10/whitelist.json"),
		WebhookToken:  mustEnv(log, "WEBHOOK_TOKEN"),

		Debounce:          envDuration(log, "DEBOUNCE", defaultDebounce),
		MinInterval:       envDuration(log, "MIN_INTERVAL", defaultMinInterval),
		ReconcileInterval: envDuration(log, "RECONCILE_INTERVAL", defaultReconcileInterval),
		HTTPTimeout:       defaultHTTPTimeout,
	}
}

func mustEnv(log *slog.Logger, name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Error("missing required env var", "name", name)
		os.Exit(1)
	}
	return v
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func envDuration(log *slog.Logger, name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, parseErr := time.ParseDuration(v)
	if parseErr != nil {
		log.Warn("invalid duration, using default", "name", name, "value", v, "default", fallback)
		return fallback
	}
	return d
}

package whitelistsync

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Syncer drives the whitelist sync loop and serves the webhook endpoint
// that triggers it.
type Syncer struct {
	cfg     Config
	log     *slog.Logger
	client  *http.Client
	trigger chan struct{}
}

// New builds a Syncer. The returned client presents cfg's client
// certificate to Authentik, re-reading it from disk on every TLS handshake
// so a certificate renewed by an external process (e.g. ACME) takes effect
// without a restart.
func New(cfg Config, log *slog.Logger) *Syncer {
	reloader := newCertReloader(cfg.ClientCertPath, cfg.ClientKeyPath)
	client := &http.Client{
		Timeout: cfg.HTTPTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				GetClientCertificate: reloader.getCertificate,
			},
		},
	}

	return &Syncer{
		cfg:    cfg,
		log:    log,
		client: client,
		// Buffered 1: a queued sync already covers any signal that arrives
		// while it waits, so extra triggers are dropped rather than piling up.
		trigger: make(chan struct{}, 1),
	}
}

// Handler serves POST /whitelist/notify: a bearer-token-gated trigger that
// ignores its body and always re-syncs the full whitelist.
func (s *Syncer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /whitelist/notify", func(w http.ResponseWriter, r *http.Request) {
		if !validToken(r, s.cfg.WebhookToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.requestSync()
		w.WriteHeader(http.StatusAccepted)
	})
	return mux
}

func validToken(r *http.Request, want string) bool {
	const prefix = "Bearer "
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, prefix) {
		return false
	}
	got = strings.TrimPrefix(got, prefix)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Syncer) requestSync() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Run performs an immediate sync, then drives the debounce loop and
// periodic reconcile ticker until ctx is done.
func (s *Syncer) Run(ctx context.Context) {
	s.requestSync()

	ticker := time.NewTicker(s.cfg.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.requestSync()
		case <-s.trigger:
			s.debounce(ctx)
		}
	}
}

// debounce waits for a burst of triggers to settle, enforces the minimum
// gap since the last sync, then runs one. Authentik's audit log fires
// model_updated on every login, not just real whitelist changes, so
// coalescing here is what keeps this from syncing constantly.
func (s *Syncer) debounce(ctx context.Context) {
	timer := time.NewTimer(s.cfg.Debounce)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	select {
	case <-s.trigger: // drop a second signal that arrived during the wait
	default:
	}

	if syncErr := s.Sync(ctx); syncErr != nil {
		s.log.ErrorContext(ctx, "sync failed", "error", syncErr)
	}

	select {
	case <-ctx.Done():
	case <-time.After(s.cfg.MinInterval):
	}
}

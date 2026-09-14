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

type Syncer struct {
	cfg     Config
	log     *slog.Logger
	client  *http.Client
	trigger chan struct{}
}

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
		cfg:     cfg,
		log:     log,
		client:  client,
		trigger: make(chan struct{}, 1),
	}
}

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

func (s *Syncer) debounce(ctx context.Context) {
	timer := time.NewTimer(s.cfg.Debounce)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	select {
	case <-s.trigger:
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

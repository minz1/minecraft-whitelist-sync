package whitelistsync_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minz1/minecraft-whitelist-sync/internal/whitelistsync"
)

func newTestSyncer(
	t *testing.T,
	authentikURL string,
	debounce, minInterval, reconcile time.Duration,
) *whitelistsync.Syncer {
	t.Helper()
	cfg := whitelistsync.Config{
		RCONHost: "127.0.0.1", RCONPort: "1", RCONPassword: "x", RCONTimeout: 200 * time.Millisecond,
		AuthentikURL: authentikURL, AuthentikToken: "test-token",
		WhitelistFile:     filepath.Join(t.TempDir(), "whitelist.json"),
		WebhookToken:      "webhook-token",
		Debounce:          debounce,
		MinInterval:       minInterval,
		ReconcileInterval: reconcile,
		HTTPTimeout:       5 * time.Second,
	}
	return whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
}

func TestHandlerRejectsMissingOrWrongToken(t *testing.T) {
	t.Parallel()

	syncer := newTestSyncer(t, "http://127.0.0.1:1", time.Hour, time.Hour, time.Hour)
	handler := syncer.Handler()

	for _, auth := range []string{"", "Bearer wrong-token", "webhook-token"} {
		req := httptest.NewRequest(http.MethodPost, "/whitelist/notify", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Authorization=%q: status = %d, want 401", auth, rec.Code)
		}
	}
}

func TestHandlerAcceptsCorrectToken(t *testing.T) {
	t.Parallel()

	syncer := newTestSyncer(t, "http://127.0.0.1:1", time.Hour, time.Hour, time.Hour)
	handler := syncer.Handler()

	req := httptest.NewRequest(http.MethodPost, "/whitelist/notify", nil)
	req.Header.Set("Authorization", "Bearer webhook-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rec.Code)
	}
}

func TestRunDebouncesBurstIntoOneSync(t *testing.T) {
	t.Parallel()

	var syncCount atomic.Int32
	authentik := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		syncCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pagination":{"total_pages":1},"results":[]}`))
	}))
	t.Cleanup(authentik.Close)

	// Long reconcile interval so only the webhook-triggered syncs count.
	syncer := newTestSyncer(t, authentik.URL, 50*time.Millisecond, 10*time.Millisecond, time.Hour)
	handler := syncer.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go syncer.Run(ctx)

	// Startup sync fires immediately; let it land before the burst.
	time.Sleep(20 * time.Millisecond)
	startupCount := syncCount.Load()

	for range 5 {
		req := httptest.NewRequest(http.MethodPost, "/whitelist/notify", nil)
		req.Header.Set("Authorization", "Bearer webhook-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	time.Sleep(150 * time.Millisecond)

	got := syncCount.Load() - startupCount
	if got != 1 {
		t.Errorf("syncs after burst = %d, want 1 (startup sync excluded)", got)
	}
}

func TestRunReconcileTicksTriggerSync(t *testing.T) {
	t.Parallel()

	var syncCount atomic.Int32
	authentik := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		syncCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pagination":{"total_pages":1},"results":[]}`))
	}))
	t.Cleanup(authentik.Close)

	syncer := newTestSyncer(t, authentik.URL, time.Millisecond, time.Millisecond, 30*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go syncer.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Startup sync plus at least two reconcile ticks in 100ms at a 30ms period.
	if got := syncCount.Load(); got < 3 {
		t.Errorf("sync count = %d, want at least 3 (startup + reconcile ticks)", got)
	}
}

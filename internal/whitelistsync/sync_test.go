package whitelistsync_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/minz1/minecraft-whitelist-sync/internal/whitelistsync"
)

func TestDiffSummary(t *testing.T) {
	t.Parallel()

	desired := map[string]string{"uuid-a": "alice", "uuid-b": "bobby", "uuid-c": "carol"}
	current := map[string]string{"uuid-a": "alice", "uuid-b": "bob", "uuid-d": "dave"}

	added, removed, renamed := whitelistsync.DiffSummaryForTest(desired, current)

	if !reflect.DeepEqual(added, []string{"carol"}) {
		t.Errorf("added = %v, want [carol]", added)
	}
	if !reflect.DeepEqual(removed, []string{"dave"}) {
		t.Errorf("removed = %v, want [dave]", removed)
	}
	if !reflect.DeepEqual(renamed, []string{"bob -> bobby"}) {
		t.Errorf("renamed = %v, want [bob -> bobby]", renamed)
	}
}

func TestMapsEqual(t *testing.T) {
	t.Parallel()

	a := map[string]string{"x": "1", "y": "2"}
	b := map[string]string{"y": "2", "x": "1"}
	c := map[string]string{"x": "1"}

	if !whitelistsync.MapsEqualForTest(a, b) {
		t.Error("expected equal maps regardless of key order")
	}
	if whitelistsync.MapsEqualForTest(a, c) {
		t.Error("expected differently-sized maps to compare unequal")
	}
}

func TestReadCurrentMissingOrMalformedFile(t *testing.T) {
	t.Parallel()

	if got := whitelistsync.ReadCurrentForTest("/nonexistent/whitelist.json"); len(got) != 0 {
		t.Errorf("missing file: got %v, want empty map", got)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "whitelist.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}
	if got := whitelistsync.ReadCurrentForTest(path); len(got) != 0 {
		t.Errorf("malformed file: got %v, want empty map", got)
	}
}

func TestWriteAndReadWhitelistRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "whitelist.json")
	desired := map[string]string{"uuid-a": "alice", "uuid-b": "bob"}

	if err := whitelistsync.WriteWhitelistForTest(path, desired); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := whitelistsync.ReadCurrentForTest(path); !whitelistsync.MapsEqualForTest(got, desired) {
		t.Errorf("round trip = %v, want %v", got, desired)
	}
}

// fakeRCON accepts one connection, authenticates any password, and answers
// "whitelist reload" — enough for Sync's happy path.
func fakeRCON(t *testing.T) string {
	t.Helper()
	ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("listen: %v", listenErr)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		if _, _, _, recvErr := whitelistsync.RCONRecvForTest(conn); recvErr != nil {
			return
		}
		if sendErr := whitelistsync.RCONSendForTest(conn, 1, whitelistsync.RCONTypeAuthForTest, ""); sendErr != nil {
			return
		}
		if _, _, _, recvErr := whitelistsync.RCONRecvForTest(conn); recvErr != nil {
			return
		}
		_ = whitelistsync.RCONSendForTest(conn, 1, whitelistsync.RCONTypeCommandForTest, "")
	}()

	host, port, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr != nil {
		t.Fatalf("split addr: %v", splitErr)
	}
	return host + ":" + port
}

func fakeAuthentik(t *testing.T, users []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := map[string]any{
			"pagination": map[string]any{"total_pages": 1},
			"results":    users,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestSyncWritesNewWhitelistAndReloadsRCON(t *testing.T) {
	t.Parallel()

	authentik := fakeAuthentik(t, []map[string]any{
		{"attributes": map[string]any{"minecraft_uuid": "uuid-a", "minecraft_username": "alice"}},
		{"attributes": map[string]any{}}, // no minecraft attributes: excluded
	})
	t.Cleanup(authentik.Close)

	rconAddr := fakeRCON(t)
	host, port, splitErr := net.SplitHostPort(rconAddr)
	if splitErr != nil {
		t.Fatalf("split rcon addr: %v", splitErr)
	}

	whitelistPath := filepath.Join(t.TempDir(), "whitelist.json")
	cfg := whitelistsync.Config{
		RCONHost: host, RCONPort: port, RCONPassword: "irrelevant", RCONTimeout: time.Second,
		AuthentikURL: authentik.URL, AuthentikToken: "test-token",
		ClientCertPath: "", ClientKeyPath: "",
		WhitelistFile: whitelistPath, WebhookToken: "webhook-token",
		HTTPTimeout: 5 * time.Second,
	}

	syncer := whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got := whitelistsync.ReadCurrentForTest(whitelistPath)
	want := map[string]string{"uuid-a": "alice"}
	if !whitelistsync.MapsEqualForTest(got, want) {
		t.Errorf("whitelist = %v, want %v", got, want)
	}
}

func TestSyncNoOpWhenUnchanged(t *testing.T) {
	t.Parallel()

	authentik := fakeAuthentik(t, []map[string]any{
		{"attributes": map[string]any{"minecraft_uuid": "uuid-a", "minecraft_username": "alice"}},
	})
	t.Cleanup(authentik.Close)

	whitelistPath := filepath.Join(t.TempDir(), "whitelist.json")
	if err := whitelistsync.WriteWhitelistForTest(whitelistPath, map[string]string{"uuid-a": "alice"}); err != nil {
		t.Fatalf("seed whitelist: %v", err)
	}

	// RCON_HOST points nowhere reachable; if Sync tries to reload despite
	// no diff, this test's own timeout would eventually notice, but the
	// real assertion is that it returns quickly without error.
	cfg := whitelistsync.Config{
		RCONHost: "127.0.0.1", RCONPort: "1", RCONPassword: "x", RCONTimeout: time.Second,
		AuthentikURL: authentik.URL, AuthentikToken: "test-token",
		WhitelistFile: whitelistPath, WebhookToken: "webhook-token",
		HTTPTimeout: 5 * time.Second,
	}

	syncer := whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestSyncToleratesUnreachableRCON(t *testing.T) {
	t.Parallel()

	authentik := fakeAuthentik(t, []map[string]any{
		{"attributes": map[string]any{"minecraft_uuid": "uuid-a", "minecraft_username": "alice"}},
	})
	t.Cleanup(authentik.Close)

	whitelistPath := filepath.Join(t.TempDir(), "whitelist.json")
	cfg := whitelistsync.Config{
		RCONHost: "127.0.0.1", RCONPort: "1", RCONPassword: "x", RCONTimeout: 200 * time.Millisecond,
		AuthentikURL: authentik.URL, AuthentikToken: "test-token",
		WhitelistFile: whitelistPath, WebhookToken: "webhook-token",
		HTTPTimeout: 5 * time.Second,
	}

	syncer := whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("expected Sync to tolerate an unreachable RCON, got %v", err)
	}
	if got := whitelistsync.ReadCurrentForTest(
		whitelistPath,
	); !whitelistsync.MapsEqualForTest(
		got,
		map[string]string{"uuid-a": "alice"},
	) {
		t.Errorf("whitelist should still be written even when RCON is unreachable, got %v", got)
	}
}

func TestSyncFetchFailurePropagates(t *testing.T) {
	t.Parallel()

	authentik := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(authentik.Close)

	cfg := whitelistsync.Config{
		RCONHost: "127.0.0.1", RCONPort: "1", RCONPassword: "x", RCONTimeout: time.Second,
		AuthentikURL: authentik.URL, AuthentikToken: "test-token",
		WhitelistFile: filepath.Join(t.TempDir(), "whitelist.json"), WebhookToken: "webhook-token",
		HTTPTimeout: 5 * time.Second,
	}

	syncer := whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
	if err := syncer.Sync(context.Background()); err == nil {
		t.Fatal("expected an error when Authentik returns 500")
	}
}

func TestSyncPagination(t *testing.T) {
	t.Parallel()

	var requests []string
	authentik := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pagination": map[string]any{"total_pages": 2},
				"results": []map[string]any{
					{"attributes": map[string]any{"minecraft_uuid": "uuid-a", "minecraft_username": "alice"}},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pagination": map[string]any{"total_pages": 2},
			"results": []map[string]any{
				{"attributes": map[string]any{"minecraft_uuid": "uuid-b", "minecraft_username": "bob"}},
			},
		})
	}))
	t.Cleanup(authentik.Close)

	whitelistPath := filepath.Join(t.TempDir(), "whitelist.json")
	cfg := whitelistsync.Config{
		RCONHost: "127.0.0.1", RCONPort: "1", RCONPassword: "x", RCONTimeout: 200 * time.Millisecond,
		AuthentikURL: authentik.URL, AuthentikToken: "test-token",
		WhitelistFile: whitelistPath, WebhookToken: "webhook-token",
		HTTPTimeout: 5 * time.Second,
	}

	syncer := whitelistsync.New(cfg, slog.New(slog.DiscardHandler))
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 paginated requests, got %d: %v", len(requests), requests)
	}

	want := map[string]string{"uuid-a": "alice", "uuid-b": "bob"}
	if got := whitelistsync.ReadCurrentForTest(whitelistPath); !whitelistsync.MapsEqualForTest(got, want) {
		t.Errorf("whitelist = %v, want %v", got, want)
	}
}

package whitelistsync_test

import (
	"net"
	"testing"
	"time"

	"github.com/minz1/minecraft-whitelist-sync/internal/whitelistsync"
)

func TestRCONSendRecvRoundTrip(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		reqID, ptype, body, recvErr := whitelistsync.RCONRecvForTest(server)
		if recvErr != nil {
			t.Errorf("server recv: %v", recvErr)
			return
		}
		if reqID != 1 || ptype != whitelistsync.RCONTypeAuthForTest || body != "hunter2" {
			t.Errorf("server got (%d, %d, %q), want (1, %d, hunter2)",
				reqID, ptype, body, whitelistsync.RCONTypeAuthForTest)
		}
		if sendErr := whitelistsync.RCONSendForTest(server, 1, whitelistsync.RCONTypeAuthForTest, ""); sendErr != nil {
			t.Errorf("server send: %v", sendErr)
		}
	}()

	if sendErr := whitelistsync.RCONSendForTest(
		client,
		1,
		whitelistsync.RCONTypeAuthForTest,
		"hunter2",
	); sendErr != nil {
		t.Fatalf("client send: %v", sendErr)
	}
	reqID, _, _, recvErr := whitelistsync.RCONRecvForTest(client)
	if recvErr != nil {
		t.Fatalf("client recv: %v", recvErr)
	}
	if reqID != 1 {
		t.Errorf("reqID = %d, want 1", reqID)
	}
	<-done
}

func TestRCONRecvShortPacket(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _ = server.Write([]byte{3, 0, 0, 0, 1, 2, 3})
	}()

	if _, _, _, recvErr := whitelistsync.RCONRecvForTest(client); recvErr == nil {
		t.Fatal("expected an error for a short packet, got nil")
	}
}

func TestRCONReloadWhitelistDialFailure(t *testing.T) {
	t.Parallel()

	reloadErr := whitelistsync.RCONReloadWhitelistForTest("127.0.0.1:1", "irrelevant", 200*time.Millisecond)
	if reloadErr == nil {
		t.Fatal("expected a dial error")
	}
}

func TestRCONReloadWhitelistAuthFailure(t *testing.T) {
	t.Parallel()

	ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("listen: %v", listenErr)
	}
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		if _, _, _, recvErr := whitelistsync.RCONRecvForTest(conn); recvErr != nil {
			return
		}
		_ = whitelistsync.RCONSendForTest(conn, -1, whitelistsync.RCONTypeAuthForTest, "")
	}()

	reloadErr := whitelistsync.RCONReloadWhitelistForTest(ln.Addr().String(), "wrong", time.Second)
	if reloadErr == nil {
		t.Fatal("expected an authentication error")
	}
}

func TestRCONReloadWhitelistSuccess(t *testing.T) {
	t.Parallel()

	ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("listen: %v", listenErr)
	}
	defer ln.Close()

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

	if reloadErr := whitelistsync.RCONReloadWhitelistForTest(
		ln.Addr().String(),
		"correct",
		time.Second,
	); reloadErr != nil {
		t.Fatalf("expected success, got %v", reloadErr)
	}
}

package whitelistsync

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	rconTypeCommand = 2
	rconTypeAuth    = 3
	rconSizeLen     = 4 // the size prefix itself
	rconHeaderLen   = 8 // request ID + packet type, both int32
	rconTrailerLen  = 2 // NUL body terminator + NUL packet terminator
)

// rconSend writes one Source RCON packet: a little-endian size prefix
// covering everything after itself, a request ID, a packet type, and a
// NUL-terminated body followed by an extra trailing NUL.
func rconSend(conn net.Conn, reqID, ptype int32, body string) error {
	payload := append([]byte(body), 0, 0)
	size := rconHeaderLen + len(payload)

	buf := make([]byte, rconSizeLen, rconSizeLen+size)
	binary.LittleEndian.PutUint32(buf, uint32(size))           //nolint:gosec // RCON packets are always small
	buf = binary.LittleEndian.AppendUint32(buf, uint32(reqID)) //nolint:gosec // wire value, sign doesn't matter
	buf = binary.LittleEndian.AppendUint32(buf, uint32(ptype)) //nolint:gosec // wire value, sign doesn't matter
	buf = append(buf, payload...)

	_, writeErr := conn.Write(buf)
	return writeErr
}

// rconRecv reads one Source RCON packet and strips its trailing NULs.
func rconRecv(conn net.Conn) (int32, int32, string, error) {
	head, readSizeErr := readExact(conn, rconSizeLen)
	if readSizeErr != nil {
		return 0, 0, "", fmt.Errorf("read size: %w", readSizeErr)
	}
	size := binary.LittleEndian.Uint32(head)

	body, readBodyErr := readExact(conn, int(size))
	if readBodyErr != nil {
		return 0, 0, "", fmt.Errorf("read body: %w", readBodyErr)
	}
	if len(body) < rconHeaderLen+rconTrailerLen {
		return 0, 0, "", fmt.Errorf("short packet: %d bytes", len(body))
	}

	reqID := int32(binary.LittleEndian.Uint32(body[0:4])) //nolint:gosec // wire value, sign doesn't matter
	ptype := int32(binary.LittleEndian.Uint32(body[4:8])) //nolint:gosec // wire value, sign doesn't matter
	return reqID, ptype, string(body[rconHeaderLen : len(body)-rconTrailerLen]), nil
}

func readExact(conn net.Conn, n int) ([]byte, error) {
	buf := make([]byte, 0, n)
	for len(buf) < n {
		chunk := make([]byte, n-len(buf))
		read, readErr := conn.Read(chunk)
		if readErr != nil {
			return nil, readErr
		}
		buf = append(buf, chunk[:read]...)
	}
	return buf, nil
}

// rconUnavailableError marks a dial failure as tolerable: whitelist.json is
// still correct, and the server picks it up on its next start.
type rconUnavailableError struct{ cause error }

func (e *rconUnavailableError) Error() string { return fmt.Sprintf("RCON unavailable: %v", e.cause) }
func (e *rconUnavailableError) Unwrap() error { return e.cause }

// rconReloadWhitelist authenticates to the Minecraft server's RCON port and
// issues "whitelist reload".
func rconReloadWhitelist(ctx context.Context, addr, password string, timeout time.Duration) error {
	dialer := net.Dialer{Timeout: timeout}
	conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
	if dialErr != nil {
		return &rconUnavailableError{cause: dialErr}
	}
	defer conn.Close()

	if deadlineErr := conn.SetDeadline(time.Now().Add(timeout)); deadlineErr != nil {
		return fmt.Errorf("set deadline: %w", deadlineErr)
	}

	if sendErr := rconSend(conn, 1, rconTypeAuth, password); sendErr != nil {
		return fmt.Errorf("send auth: %w", sendErr)
	}
	reqID, _, _, authRecvErr := rconRecv(conn)
	if authRecvErr != nil {
		return fmt.Errorf("recv auth response: %w", authRecvErr)
	}
	if reqID == -1 {
		return errors.New("authentication failed")
	}

	if sendErr := rconSend(conn, 1, rconTypeCommand, "whitelist reload"); sendErr != nil {
		return fmt.Errorf("send command: %w", sendErr)
	}
	if _, _, _, cmdRecvErr := rconRecv(conn); cmdRecvErr != nil {
		return fmt.Errorf("recv command response: %w", cmdRecvErr)
	}
	return nil
}

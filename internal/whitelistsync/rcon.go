package whitelistsync

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	rconTypeCommand = 2
	rconTypeAuth    = 3
	rconSizeLen     = 4
	rconHeaderLen   = 8
	rconTrailerLen  = 2
	rconFullHeader  = rconSizeLen + rconHeaderLen
)

func rconSend(conn net.Conn, reqID, ptype int32, body string) error {
	payload := append([]byte(body), 0, 0)
	bodyLen := rconHeaderLen + len(payload)
	if bodyLen > math.MaxInt32 {
		return fmt.Errorf("rcon body too large: %d bytes", len(payload))
	}
	size := int32(bodyLen)

	var header [rconFullHeader]byte
	if _, err := binary.Encode(header[0:4], binary.LittleEndian, size); err != nil {
		return fmt.Errorf("encode size: %w", err)
	}
	if _, err := binary.Encode(header[4:8], binary.LittleEndian, reqID); err != nil {
		return fmt.Errorf("encode request id: %w", err)
	}
	if _, err := binary.Encode(header[8:12], binary.LittleEndian, ptype); err != nil {
		return fmt.Errorf("encode packet type: %w", err)
	}

	_, writeErr := conn.Write(append(header[:], payload...))
	return writeErr
}

func rconRecv(conn net.Conn) (int32, int32, string, error) {
	sizeBuf, readSizeErr := readExact(conn, rconSizeLen)
	if readSizeErr != nil {
		return 0, 0, "", fmt.Errorf("read size: %w", readSizeErr)
	}
	var size int32
	if _, err := binary.Decode(sizeBuf, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", fmt.Errorf("decode size: %w", err)
	}
	if size < 0 {
		return 0, 0, "", fmt.Errorf("negative packet size: %d", size)
	}

	body, readBodyErr := readExact(conn, int(size))
	if readBodyErr != nil {
		return 0, 0, "", fmt.Errorf("read body: %w", readBodyErr)
	}
	if len(body) < rconHeaderLen+rconTrailerLen {
		return 0, 0, "", fmt.Errorf("short packet: %d bytes", len(body))
	}

	var reqID, ptype int32
	if _, err := binary.Decode(body[0:4], binary.LittleEndian, &reqID); err != nil {
		return 0, 0, "", fmt.Errorf("decode request id: %w", err)
	}
	if _, err := binary.Decode(body[4:8], binary.LittleEndian, &ptype); err != nil {
		return 0, 0, "", fmt.Errorf("decode packet type: %w", err)
	}
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

type rconUnavailableError struct{ cause error }

func (e *rconUnavailableError) Error() string { return fmt.Sprintf("RCON unavailable: %v", e.cause) }
func (e *rconUnavailableError) Unwrap() error { return e.cause }

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

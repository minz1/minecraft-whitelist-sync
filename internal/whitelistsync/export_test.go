package whitelistsync

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

// RCONTypeAuthForTest and RCONTypeCommandForTest expose the RCON packet
// type constants for rcon_test.go (package whitelistsync_test).
const (
	RCONTypeAuthForTest    = rconTypeAuth
	RCONTypeCommandForTest = rconTypeCommand
)

// RCONSendForTest exposes rconSend for rcon_test.go, which drives it
// directly against a [net.Pipe] rather than a real Minecraft server.
func RCONSendForTest(conn net.Conn, reqID, ptype int32, body string) error {
	return rconSend(conn, reqID, ptype, body)
}

// RCONRecvForTest exposes rconRecv for rcon_test.go.
func RCONRecvForTest(conn net.Conn) (int32, int32, string, error) {
	return rconRecv(conn)
}

// RCONReloadWhitelistForTest exposes the dial+auth+command flow for
// rcon_test.go's dial-failure and auth-failure cases.
func RCONReloadWhitelistForTest(addr, password string, timeout time.Duration) error {
	return rconReloadWhitelist(context.Background(), addr, password, timeout)
}

// DiffSummaryForTest exposes diffSummary for sync_test.go, which otherwise
// only exercises it indirectly through Syncer.Sync.
func DiffSummaryForTest(desired, current map[string]string) ([]string, []string, []string) {
	return diffSummary(desired, current)
}

// MapsEqualForTest exposes mapsEqual for sync_test.go.
func MapsEqualForTest(a, b map[string]string) bool {
	return mapsEqual(a, b)
}

// ReadCurrentForTest exposes readCurrent for sync_test.go, so it can assert
// whitelist.json contents without also exercising the Authentik fetch.
func ReadCurrentForTest(path string) map[string]string {
	return readCurrent(path)
}

// WriteWhitelistForTest exposes writeWhitelist for sync_test.go.
func WriteWhitelistForTest(path string, desired map[string]string) error {
	return writeWhitelist(path, desired)
}

// CertReloaderForTest wraps the unexported certReloader for
// certreload_test.go.
type CertReloaderForTest struct{ r *certReloader }

// NewCertReloaderForTest builds a CertReloaderForTest.
func NewCertReloaderForTest(certPath, keyPath string) *CertReloaderForTest {
	return &CertReloaderForTest{r: newCertReloader(certPath, keyPath)}
}

// GetCertificate re-reads the keypair if either file's mtime changed since
// the last call.
func (c *CertReloaderForTest) GetCertificate() (*tls.Certificate, error) {
	return c.r.getCertificate(nil)
}

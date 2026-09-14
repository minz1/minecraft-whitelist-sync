package whitelistsync

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

const (
	RCONTypeAuthForTest    = rconTypeAuth
	RCONTypeCommandForTest = rconTypeCommand
)

func RCONSendForTest(conn net.Conn, reqID, ptype int32, body string) error {
	return rconSend(conn, reqID, ptype, body)
}

func RCONRecvForTest(conn net.Conn) (int32, int32, string, error) {
	return rconRecv(conn)
}

func RCONReloadWhitelistForTest(addr, password string, timeout time.Duration) error {
	return rconReloadWhitelist(context.Background(), addr, password, timeout)
}

func DiffSummaryForTest(desired, current map[string]string) ([]string, []string, []string) {
	return diffSummary(desired, current)
}

func MapsEqualForTest(a, b map[string]string) bool {
	return mapsEqual(a, b)
}

func ReadCurrentForTest(path string) map[string]string {
	return readCurrent(path)
}

func WriteWhitelistForTest(path string, desired map[string]string) error {
	return writeWhitelist(path, desired)
}

type CertReloaderForTest struct{ r *certReloader }

func NewCertReloaderForTest(certPath, keyPath string) *CertReloaderForTest {
	return &CertReloaderForTest{r: newCertReloader(certPath, keyPath)}
}

func (c *CertReloaderForTest) GetCertificate() (*tls.Certificate, error) {
	return c.r.getCertificate(nil)
}

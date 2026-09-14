package whitelistsync

import (
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"
)

type certReloader struct {
	certPath, keyPath string

	mu          sync.Mutex
	cert        *tls.Certificate
	certModTime time.Time
	keyModTime  time.Time
}

func newCertReloader(certPath, keyPath string) *certReloader {
	return &certReloader{certPath: certPath, keyPath: keyPath}
}

func (r *certReloader) getCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	certInfo, statCertErr := os.Stat(r.certPath)
	if statCertErr != nil {
		return nil, fmt.Errorf("stat client cert: %w", statCertErr)
	}
	keyInfo, statKeyErr := os.Stat(r.keyPath)
	if statKeyErr != nil {
		return nil, fmt.Errorf("stat client key: %w", statKeyErr)
	}

	if r.cert != nil && certInfo.ModTime().Equal(r.certModTime) && keyInfo.ModTime().Equal(r.keyModTime) {
		return r.cert, nil
	}

	cert, loadErr := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if loadErr != nil {
		return nil, fmt.Errorf("load client keypair: %w", loadErr)
	}

	r.cert = &cert
	r.certModTime = certInfo.ModTime()
	r.keyModTime = keyInfo.ModTime()
	return r.cert, nil
}

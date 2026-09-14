package whitelistsync_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minz1/minecraft-whitelist-sync/internal/whitelistsync"
)

func TestCertReloaderPicksUpRotatedKeypair(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	writeSelfSignedKeypair(t, certPath, keyPath, "first")
	reloader := whitelistsync.NewCertReloaderForTest(certPath, keyPath)

	first, err := reloader.GetCertificate()
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	second, err := reloader.GetCertificate()
	if err != nil {
		t.Fatalf("cached load: %v", err)
	}
	if first != second {
		t.Error("expected an unchanged keypair to return the cached certificate, not reload")
	}

	writeSelfSignedKeypair(t, certPath, keyPath, "second")
	future := time.Now().Add(time.Minute)
	if chtimesErr := os.Chtimes(certPath, future, future); chtimesErr != nil {
		t.Fatalf("chtimes cert: %v", chtimesErr)
	}
	if chtimesErr := os.Chtimes(keyPath, future, future); chtimesErr != nil {
		t.Fatalf("chtimes key: %v", chtimesErr)
	}

	third, err := reloader.GetCertificate()
	if err != nil {
		t.Fatalf("reload after rotation: %v", err)
	}
	if third == first {
		t.Error("expected a reloaded certificate after mtime changed, got the cached one")
	}
}

func TestCertReloaderMissingFile(t *testing.T) {
	t.Parallel()

	reloader := whitelistsync.NewCertReloaderForTest("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if _, err := reloader.GetCertificate(); err == nil {
		t.Fatal("expected an error for a missing cert file")
	}
}

func writeSelfSignedKeypair(t *testing.T, certPath, keyPath, commonName string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if writeErr := os.WriteFile(certPath, certPEM, 0o600); writeErr != nil {
		t.Fatalf("write cert: %v", writeErr)
	}
	if writeErr := os.WriteFile(keyPath, keyPEM, 0o600); writeErr != nil {
		t.Fatalf("write key: %v", writeErr)
	}
}

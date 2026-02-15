package tls

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGenerateSelfSigned_ValidCertificate(t *testing.T) {
	dir := t.TempDir()

	cert, err := generateSelfSigned(dir)
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("certificate should have at least one certificate in chain")
	}

	// Parse the DER-encoded certificate to verify it's valid x509.
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}

	if x509Cert.Subject.CommonName != "wgpilot self-signed" {
		t.Errorf("expected CN 'wgpilot self-signed', got %q", x509Cert.Subject.CommonName)
	}

	if len(x509Cert.IPAddresses) == 0 {
		t.Error("certificate should include IP SANs")
	}

	if len(x509Cert.DNSNames) == 0 || x509Cert.DNSNames[0] != "localhost" {
		t.Error("certificate should include 'localhost' DNS SAN")
	}

	if !x509Cert.NotAfter.After(x509Cert.NotBefore) {
		t.Error("NotAfter should be after NotBefore")
	}
}

func TestSelfSignedExists_NoFiles(t *testing.T) {
	dir := t.TempDir()
	if selfSignedExists(dir) {
		t.Error("should return false when no cert files exist")
	}
}

func TestSelfSignedExists_WithFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := generateSelfSigned(dir)
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}

	if !selfSignedExists(dir) {
		t.Error("should return true after generating certs")
	}
}

func TestLoadSelfSigned_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original, err := generateSelfSigned(dir)
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}

	loaded, err := loadSelfSigned(dir)
	if err != nil {
		t.Fatalf("loadSelfSigned: %v", err)
	}

	// Both should have the same leaf certificate.
	if len(original.Certificate) != len(loaded.Certificate) {
		t.Fatalf("certificate chain length mismatch: %d vs %d",
			len(original.Certificate), len(loaded.Certificate))
	}
}

func TestNewManager_SelfSigned(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(Config{
		Mode:    "self-signed",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if mgr.ActiveMode() != ModeSelfSigned {
		t.Errorf("expected mode self-signed, got %s", mgr.ActiveMode())
	}

	if mgr.TLSConfig() == nil {
		t.Fatal("TLSConfig should not be nil")
	}

	if mgr.TLSConfig().MinVersion != tls.VersionTLS12 {
		t.Error("MinVersion should be TLS 1.2")
	}
}

func TestNewManager_DefaultMode(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(Config{
		Mode:    "",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if mgr.ActiveMode() != ModeSelfSigned {
		t.Errorf("expected default mode self-signed, got %s", mgr.ActiveMode())
	}
}

func TestNewManager_ACMEFallbackToSelfSigned(t *testing.T) {
	dir := t.TempDir()

	// ACME without a domain should fail and fall back to self-signed.
	mgr, err := NewManager(Config{
		Mode:    "acme",
		Domain:  "",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if mgr.ActiveMode() != ModeSelfSigned {
		t.Errorf("expected fallback to self-signed, got %s", mgr.ActiveMode())
	}

	if mgr.TLSConfig() == nil {
		t.Fatal("TLSConfig should not be nil after fallback")
	}
}

func TestNewManager_ManualMode(t *testing.T) {
	dir := t.TempDir()
	certDir := filepath.Join(dir, "manual")

	// Generate a cert to use as manual cert/key.
	_, err := generateSelfSigned(certDir)
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}

	certFile := filepath.Join(certDir, selfSignedCertFile)
	keyFile := filepath.Join(certDir, selfSignedKeyFile)

	mgr, err := NewManager(Config{
		Mode:     "manual",
		CertFile: certFile,
		KeyFile:  keyFile,
		DataDir:  dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if mgr.ActiveMode() != ModeManual {
		t.Errorf("expected mode manual, got %s", mgr.ActiveMode())
	}

	if mgr.TLSConfig() == nil {
		t.Fatal("TLSConfig should not be nil")
	}
}

func TestNewManager_ManualMode_MissingFiles(t *testing.T) {
	dir := t.TempDir()

	_, err := NewManager(Config{
		Mode:     "manual",
		CertFile: "",
		KeyFile:  "",
		DataDir:  dir,
	}, testLogger())
	if err == nil {
		t.Fatal("NewManager should fail with missing cert/key files")
	}
}

func TestNewManager_ManualMode_InvalidFiles(t *testing.T) {
	dir := t.TempDir()

	// Write invalid data as cert/key
	certFile := filepath.Join(dir, "bad.crt")
	keyFile := filepath.Join(dir, "bad.key")
	os.WriteFile(certFile, []byte("not a cert"), 0644)
	os.WriteFile(keyFile, []byte("not a key"), 0600)

	_, err := NewManager(Config{
		Mode:     "manual",
		CertFile: certFile,
		KeyFile:  keyFile,
		DataDir:  dir,
	}, testLogger())
	if err == nil {
		t.Fatal("NewManager should fail with invalid cert/key files")
	}
}

func TestNewManager_ACMEWithDomain(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(Config{
		Mode:    "acme",
		Domain:  "example.com",
		Email:   "admin@example.com",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if mgr.ActiveMode() != ModeACME {
		t.Errorf("expected mode acme, got %s", mgr.ActiveMode())
	}
}

func TestNewManager_SelfSigned_ReusesExisting(t *testing.T) {
	dir := t.TempDir()

	// First creation generates a cert.
	mgr1, err := NewManager(Config{
		Mode:    "self-signed",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager (1st): %v", err)
	}

	// Second creation should load the existing cert.
	mgr2, err := NewManager(Config{
		Mode:    "self-signed",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager (2nd): %v", err)
	}

	// Both should have valid TLS configs.
	if mgr1.TLSConfig() == nil || mgr2.TLSConfig() == nil {
		t.Fatal("TLSConfig should not be nil")
	}
}

func TestHTTPHandler_NonACME(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(Config{
		Mode:    "self-signed",
		DataDir: dir,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// For non-ACME, HTTPHandler should return a non-nil handler.
	handler := mgr.HTTPHandler(http.NotFoundHandler())
	if handler == nil {
		t.Error("HTTPHandler should return a non-nil handler")
	}
}

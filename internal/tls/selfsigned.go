package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	selfSignedCertFile = "selfsigned.crt"
	selfSignedKeyFile  = "selfsigned.key"
	selfSignedValidity = 365 * 24 * time.Hour // 1 year
)

// generateSelfSigned creates a self-signed TLS certificate and key,
// stores them in certDir, and returns the loaded tls.Certificate.
func generateSelfSigned(certDir string) (tls.Certificate, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert dir %s: %w", certDir, err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate ecdsa key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"wgpilot"},
			CommonName:   "wgpilot self-signed",
		},
		NotBefore:             now,
		NotAfter:              now.Add(selfSignedValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		// Allow connections via any IP.
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback, net.IPv4zero},
		DNSNames:    []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	certPath := filepath.Join(certDir, selfSignedCertFile)
	keyPath := filepath.Join(certDir, selfSignedKeyFile)

	// Write cert PEM
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create cert file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return tls.Certificate{}, fmt.Errorf("write cert pem: %w", err)
	}

	// Write key PEM
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create key file: %w", err)
	}
	defer keyFile.Close()

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal ec private key: %w", err)
	}

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key pem: %w", err)
	}

	return tls.LoadX509KeyPair(certPath, keyPath)
}

// loadSelfSigned loads an existing self-signed cert from certDir, or
// returns an error if the files don't exist.
func loadSelfSigned(certDir string) (tls.Certificate, error) {
	certPath := filepath.Join(certDir, selfSignedCertFile)
	keyPath := filepath.Join(certDir, selfSignedKeyFile)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load self-signed cert: %w", err)
	}
	return cert, nil
}

// selfSignedExists checks whether a self-signed cert already exists in certDir.
func selfSignedExists(certDir string) bool {
	certPath := filepath.Join(certDir, selfSignedCertFile)
	keyPath := filepath.Join(certDir, selfSignedKeyFile)

	_, errCert := os.Stat(certPath)
	_, errKey := os.Stat(keyPath)
	return errCert == nil && errKey == nil
}

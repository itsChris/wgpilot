package tls

import (
	"crypto/tls"
	"fmt"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// newACMEManager creates an autocert.Manager for automatic Let's Encrypt
// certificate provisioning. Certificates are cached in certDir.
func newACMEManager(domain, email, certDir string) (*autocert.Manager, error) {
	if domain == "" {
		return nil, fmt.Errorf("acme: domain is required")
	}

	cacheDir := filepath.Join(certDir, "acme")

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domain),
		Cache:      autocert.DirCache(cacheDir),
		Email:      email,
	}

	return m, nil
}

// acmeTLSConfig returns a *tls.Config that uses ACME for certificate management.
func acmeTLSConfig(mgr *autocert.Manager) *tls.Config {
	return mgr.TLSConfig()
}

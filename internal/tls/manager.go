package tls

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"

	"golang.org/x/crypto/acme/autocert"
)

// Mode represents the TLS certificate mode.
type Mode string

const (
	ModeSelfSigned Mode = "self-signed"
	ModeACME       Mode = "acme"
	ModeManual     Mode = "manual"
)

// Config holds the TLS configuration for the Manager.
type Config struct {
	Mode       string // "self-signed", "acme", or "manual"
	Domain     string // required for acme mode
	Email      string // optional contact email for acme
	CertFile   string // required for manual mode
	KeyFile    string // required for manual mode
	DataDir    string // base data directory (certs stored in DataDir/certs/)
}

// Manager handles TLS certificate provisioning and configuration.
type Manager struct {
	cfg         Config
	logger      *slog.Logger
	certDir     string
	tlsConfig   *tls.Config
	acmeManager *autocert.Manager
	activeMode  Mode
}

// NewManager creates a TLS manager that provisions certificates based on the
// configured mode. If ACME mode fails (e.g., missing domain), it falls back
// to self-signed mode with a warning log.
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	certDir := filepath.Join(cfg.DataDir, "certs")
	m := &Manager{
		cfg:     cfg,
		logger:  logger,
		certDir: certDir,
	}

	mode := Mode(cfg.Mode)
	if mode == "" {
		mode = ModeSelfSigned
	}

	switch mode {
	case ModeManual:
		tlsCfg, err := m.setupManual()
		if err != nil {
			return nil, fmt.Errorf("tls manual setup: %w", err)
		}
		m.tlsConfig = tlsCfg
		m.activeMode = ModeManual
		logger.Info("tls_configured",
			"mode", "manual",
			"cert_file", cfg.CertFile,
			"key_file", cfg.KeyFile,
			"component", "tls",
		)

	case ModeACME:
		tlsCfg, err := m.setupACME()
		if err != nil {
			// Fallback to self-signed
			logger.Warn("tls_acme_fallback",
				"error", err,
				"reason", "falling back to self-signed",
				"component", "tls",
			)
			tlsCfg, err = m.setupSelfSigned()
			if err != nil {
				return nil, fmt.Errorf("tls self-signed fallback: %w", err)
			}
			m.tlsConfig = tlsCfg
			m.activeMode = ModeSelfSigned
		} else {
			m.tlsConfig = tlsCfg
			m.activeMode = ModeACME
			logger.Info("tls_configured",
				"mode", "acme",
				"domain", cfg.Domain,
				"component", "tls",
			)
		}

	default:
		// Default: self-signed
		tlsCfg, err := m.setupSelfSigned()
		if err != nil {
			return nil, fmt.Errorf("tls self-signed setup: %w", err)
		}
		m.tlsConfig = tlsCfg
		m.activeMode = ModeSelfSigned
		logger.Info("tls_configured",
			"mode", "self-signed",
			"cert_dir", certDir,
			"component", "tls",
		)
	}

	return m, nil
}

// TLSConfig returns the tls.Config to use with the HTTP server.
func (m *Manager) TLSConfig() *tls.Config {
	return m.tlsConfig
}

// ActiveMode returns the TLS mode that is actually in use.
func (m *Manager) ActiveMode() Mode {
	return m.activeMode
}

// HTTPHandler returns an HTTP handler for ACME HTTP-01 challenges.
// For non-ACME modes, it returns a simple redirect to HTTPS.
func (m *Manager) HTTPHandler(fallback http.Handler) http.Handler {
	if m.acmeManager != nil {
		return m.acmeManager.HTTPHandler(fallback)
	}
	return fallback
}

func (m *Manager) setupSelfSigned() (*tls.Config, error) {
	var cert tls.Certificate
	var err error

	if selfSignedExists(m.certDir) {
		cert, err = loadSelfSigned(m.certDir)
		if err != nil {
			// Existing cert is corrupted — regenerate
			m.logger.Warn("tls_selfsigned_reload_failed",
				"error", err,
				"action", "regenerating",
				"component", "tls",
			)
			cert, err = generateSelfSigned(m.certDir)
			if err != nil {
				return nil, fmt.Errorf("generate self-signed cert: %w", err)
			}
		}
		m.logger.Info("tls_selfsigned_loaded",
			"cert_dir", m.certDir,
			"component", "tls",
		)
	} else {
		cert, err = generateSelfSigned(m.certDir)
		if err != nil {
			return nil, fmt.Errorf("generate self-signed cert: %w", err)
		}
		m.logger.Info("tls_selfsigned_generated",
			"cert_dir", m.certDir,
			"component", "tls",
		)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func (m *Manager) setupACME() (*tls.Config, error) {
	mgr, err := newACMEManager(m.cfg.Domain, m.cfg.Email, m.certDir)
	if err != nil {
		return nil, fmt.Errorf("create acme manager: %w", err)
	}
	m.acmeManager = mgr
	tlsCfg := acmeTLSConfig(mgr)
	tlsCfg.MinVersion = tls.VersionTLS12
	return tlsCfg, nil
}

func (m *Manager) setupManual() (*tls.Config, error) {
	if m.cfg.CertFile == "" || m.cfg.KeyFile == "" {
		return nil, fmt.Errorf("manual tls mode requires cert_file and key_file")
	}

	cert, err := tls.LoadX509KeyPair(m.cfg.CertFile, m.cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load manual cert: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

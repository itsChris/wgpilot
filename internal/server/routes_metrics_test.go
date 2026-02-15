package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsChris/wgpilot/internal/db"
)

func TestHandleMetrics_NoNetworks(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", ct)
	}

	body := w.Body.String()
	// Should have HELP/TYPE lines even with no networks.
	if !strings.Contains(body, "# HELP wg_peers_total") {
		t.Error("expected HELP wg_peers_total in output")
	}
	if !strings.Contains(body, "# TYPE wg_interface_up gauge") {
		t.Error("expected TYPE wg_interface_up in output")
	}
}

func TestHandleMetrics_WithNetwork_ReturnsAllMetrics(t *testing.T) {
	srv := newTestServerForMonitoring(t)
	ctx := context.Background()

	_, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name: "Test VPN", Interface: "wg0", Mode: "gateway",
		Subnet: "10.0.0.0/24", ListenPort: 51820,
		PrivateKey: "priv", PublicKey: "pub",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	_, err = srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID: 1, Name: "My Phone", PublicKey: "peer-public-key",
		PrivateKey: "peer-priv", AllowedIPs: "10.0.0.2/32", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()

	// Check all expected metrics are present.
	expectations := []string{
		`wg_interface_up{network="wg0"} 1`,
		`wg_peers_total{network="wg0"}`,
		`wg_peers_online{network="wg0"}`,
		`wg_transfer_bytes_total{network="wg0",direction="rx"}`,
		`wg_transfer_bytes_total{network="wg0",direction="tx"}`,
		`wg_peer_last_handshake_seconds{network="wg0"`,
	}
	for _, exp := range expectations {
		if !strings.Contains(body, exp) {
			t.Errorf("expected metrics output to contain %q, got:\n%s", exp, body)
		}
	}

	// Verify peer name appears in labels (should use DB name "My Phone").
	if !strings.Contains(body, `peer="My Phone"`) {
		t.Errorf("expected peer label 'My Phone' in metrics, got:\n%s", body)
	}
}

func TestHandleMetrics_DisabledNetwork_ShowsDown(t *testing.T) {
	srv := newTestServerForMonitoring(t)
	ctx := context.Background()

	_, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name: "Disabled VPN", Interface: "wg1", Mode: "gateway",
		Subnet: "10.0.1.0/24", ListenPort: 51821,
		PrivateKey: "priv", PublicKey: "pub",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `wg_interface_up{network="wg1"} 0`) {
		t.Errorf("expected wg_interface_up=0 for disabled network, got:\n%s", body)
	}
}

func TestHandleMetrics_NoAuth_Required(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	// Metrics endpoint should work without auth.
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without auth, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleMetrics_TransferValues(t *testing.T) {
	srv := newTestServerForMonitoring(t)
	ctx := context.Background()

	_, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name: "Test", Interface: "wg0", Mode: "gateway",
		Subnet: "10.0.0.0/24", ListenPort: 51820,
		PrivateKey: "priv", PublicKey: "pub",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	body := w.Body.String()

	// Mock returns ReceiveBytes=5000, TransmitBytes=3000.
	if !strings.Contains(body, `wg_transfer_bytes_total{network="wg0",direction="rx"} 5000`) {
		t.Errorf("expected rx=5000, got:\n%s", body)
	}
	if !strings.Contains(body, `wg_transfer_bytes_total{network="wg0",direction="tx"} 3000`) {
		t.Errorf("expected tx=3000, got:\n%s", body)
	}
}

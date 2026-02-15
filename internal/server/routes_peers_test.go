package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itsChris/wgpilot/internal/db"
)

// createTestNetwork creates a network in the database for peer tests.
func createTestNetwork(t *testing.T, srv *Server) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Test Network",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "server-priv-key",
		PublicKey:  "server-pub-key",
		DNSServers: "1.1.1.1",
		NATEnabled: true,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create test network: %v", err)
	}
	return id
}

func TestCreatePeer_Success(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	body := `{
		"name": "My Phone",
		"email": "chris@example.com",
		"role": "client",
		"persistent_keepalive": 25
	}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp peerResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Name != "My Phone" {
		t.Errorf("expected name='My Phone', got %q", resp.Name)
	}
	if resp.Email != "chris@example.com" {
		t.Errorf("expected email='chris@example.com', got %q", resp.Email)
	}
	if resp.Role != "client" {
		t.Errorf("expected role='client', got %q", resp.Role)
	}
	if resp.PersistentKeepalive != 25 {
		t.Errorf("expected persistent_keepalive=25, got %d", resp.PersistentKeepalive)
	}
	if resp.PublicKey == "" {
		t.Error("expected non-empty public key")
	}
	if !strings.Contains(resp.AllowedIPs, "10.0.0.2/32") {
		t.Errorf("expected allowed_ips to contain 10.0.0.2/32, got %q", resp.AllowedIPs)
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}
	if resp.NetworkID != netID {
		t.Errorf("expected network_id=%d, got %d", netID, resp.NetworkID)
	}
}

func TestCreatePeer_ValidationErrors(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	body := `{
		"name": "",
		"role": "invalid",
		"persistent_keepalive": -1
	}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp validationErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %q", resp.Code)
	}

	fieldNames := map[string]bool{}
	for _, f := range resp.Fields {
		fieldNames[f.Field] = true
	}
	if !fieldNames["name"] {
		t.Error("expected field error for 'name'")
	}
	if !fieldNames["role"] {
		t.Error("expected field error for 'role'")
	}
}

func TestCreatePeer_NetworkNotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	body := `{"name": "Test", "role": "client"}`
	req := httptest.NewRequest("POST", "/api/networks/999/peers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreatePeer_IPExhaustion(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	// Create network with /30 (only 2 usable IPs: .1 for server, .2 for one peer).
	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Tiny",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/30",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	// First peer should succeed (gets .2).
	body := `{"name": "Peer 1", "role": "client"}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for first peer, got %d: %s", w.Code, w.Body.String())
	}

	// Second peer should fail (no more IPs).
	body = `{"name": "Peer 2", "role": "client"}`
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for IP exhaustion, got %d: %s", w.Code, w.Body.String())
	}

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "IP_POOL_EXHAUSTED" {
		t.Errorf("expected IP_POOL_EXHAUSTED, got %q", resp.Error.Code)
	}
}

func TestListPeers_Empty(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers", netID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []peerResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestGetPeer_Found(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)
	ctx := context.Background()

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:  netID,
		Name:       "Test Peer",
		PublicKey:  "peer-pub-key",
		AllowedIPs: "10.0.0.2/32",
		Role:       "client",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp peerResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "Test Peer" {
		t.Errorf("expected name='Test Peer', got %q", resp.Name)
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/999", netID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPeer_WrongNetwork(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	net1ID := createTestNetwork(t, srv)
	net2ID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Other Network",
		Interface:  "wg1",
		Mode:       "gateway",
		Subnet:     "10.1.0.0/24",
		ListenPort: 51821,
		PublicKey:   "pub-key-2",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:  net1ID,
		Name:       "Peer in Net1",
		PublicKey:  "peer-pub-key",
		AllowedIPs: "10.0.0.2/32",
		Role:       "client",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	// Try to access peer via wrong network.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d", net2ID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for peer in wrong network, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdatePeer_Success(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)
	ctx := context.Background()

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:           netID,
		Name:                "Original",
		PublicKey:           "peer-pub-key",
		PresharedKey:        "peer-psk",
		AllowedIPs:          "10.0.0.2/32",
		PersistentKeepalive: 0,
		Role:                "client",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	body := `{"name": "Renamed", "persistent_keepalive": 25, "enabled": false}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/networks/%d/peers/%d", netID, peerID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp peerResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "Renamed" {
		t.Errorf("expected name=Renamed, got %q", resp.Name)
	}
	if resp.PersistentKeepalive != 25 {
		t.Errorf("expected persistent_keepalive=25, got %d", resp.PersistentKeepalive)
	}
	if resp.Enabled {
		t.Error("expected enabled=false after update")
	}
}

func TestUpdatePeer_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	body := `{"name": "Updated"}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/networks/%d/peers/999", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeletePeer_Success(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)
	ctx := context.Background()

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:  netID,
		Name:       "ToDelete",
		PublicKey:  "peer-pub-key",
		AllowedIPs: "10.0.0.2/32",
		Role:       "client",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d/peers/%d", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	peer, err := srv.db.GetPeerByID(ctx, peerID)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer != nil {
		t.Error("expected peer to be deleted")
	}
}

func TestDeletePeer_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d/peers/999", netID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPeerConfig_Download(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)
	ctx := context.Background()

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:           netID,
		Name:                "Config Peer",
		PrivateKey:          "cHJpdmF0ZS1rZXktZm9yLXRlc3RpbmctcHVycG9zZXM=", // base64 placeholder
		PublicKey:           "peer-pub-key",
		PresharedKey:        "cHJlc2hhcmVkLWtleS1mb3ItdGVzdGluZy1wdXJwb3Nlcw==",
		AllowedIPs:          "10.0.0.2/32",
		PersistentKeepalive: 25,
		Role:                "client",
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	// Set public_ip so the endpoint is deterministic.
	if err := srv.db.SetSetting(ctx, "public_ip", "203.0.113.45"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d/config", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("expected attachment disposition, got %q", disposition)
	}
	if !strings.Contains(disposition, "wgpilot-Config-Peer.conf") {
		t.Errorf("expected filename in disposition, got %q", disposition)
	}

	conf := w.Body.String()
	if !strings.Contains(conf, "[Interface]") {
		t.Error("config missing [Interface] section")
	}
	if !strings.Contains(conf, "[Peer]") {
		t.Error("config missing [Peer] section")
	}
	if !strings.Contains(conf, "PrivateKey") {
		t.Error("config missing PrivateKey")
	}
	if !strings.Contains(conf, "server-pub-key") {
		t.Errorf("config missing server public key, got:\n%s", conf)
	}
	if !strings.Contains(conf, "203.0.113.45:51820") {
		t.Errorf("config missing server endpoint, got:\n%s", conf)
	}
	// Gateway mode should have 0.0.0.0/0.
	if !strings.Contains(conf, "0.0.0.0/0") {
		t.Errorf("gateway mode config should have 0.0.0.0/0 in AllowedIPs, got:\n%s", conf)
	}
	if !strings.Contains(conf, "PersistentKeepalive = 25") {
		t.Errorf("config missing PersistentKeepalive, got:\n%s", conf)
	}
}

func TestPeerConfig_HubRoutedMode(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:             "Hub",
		Interface:        "wg0",
		Mode:             "hub-routed",
		Subnet:           "10.0.0.0/24",
		ListenPort:       51820,
		PrivateKey:       "server-priv-key",
		PublicKey:        "server-pub-key",
		DNSServers:       "1.1.1.1",
		InterPeerRouting: true,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:    netID,
		Name:         "Hub Peer",
		PrivateKey:   "cHJpdmF0ZS1rZXktZm9yLXRlc3RpbmctcHVycG9zZXM=",
		PublicKey:    "peer-pub-key",
		PresharedKey: "cHJlc2hhcmVkLWtleS1mb3ItdGVzdGluZy1wdXJwb3Nlcw==",
		AllowedIPs:   "10.0.0.2/32",
		Role:         "client",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d/config", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conf := w.Body.String()
	// Hub-routed mode should have network subnet as AllowedIPs.
	if !strings.Contains(conf, "AllowedIPs = 10.0.0.0/24") {
		t.Errorf("hub-routed config should have subnet in AllowedIPs, got:\n%s", conf)
	}
}

func TestPeerConfig_SiteToSiteMode(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "S2S",
		Interface:  "wg0",
		Mode:       "site-to-site",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "server-priv-key",
		PublicKey:  "server-pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:    netID,
		Name:         "Site Peer",
		PrivateKey:   "cHJpdmF0ZS1rZXktZm9yLXRlc3RpbmctcHVycG9zZXM=",
		PublicKey:    "peer-pub-key",
		PresharedKey: "cHJlc2hhcmVkLWtleS1mb3ItdGVzdGluZy1wdXJwb3Nlcw==",
		AllowedIPs:   "10.0.0.2/32, 192.168.1.0/24",
		SiteNetworks: "192.168.1.0/24",
		Role:         "site-gateway",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d/config", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	conf := w.Body.String()
	// Site-to-site mode should have the site networks in AllowedIPs.
	if !strings.Contains(conf, "AllowedIPs = 192.168.1.0/24") {
		t.Errorf("site-to-site config should have site_networks in AllowedIPs, got:\n%s", conf)
	}
}

func TestPeerQR_ReturnsPNG(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netID := createTestNetwork(t, srv)
	ctx := context.Background()

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:    netID,
		Name:         "QR Peer",
		PrivateKey:   "cHJpdmF0ZS1rZXktZm9yLXRlc3RpbmctcHVycG9zZXM=",
		PublicKey:    "peer-pub-key",
		PresharedKey: "cHJlc2hhcmVkLWtleS1mb3ItdGVzdGluZy1wdXJwb3Nlcw==",
		AllowedIPs:   "10.0.0.2/32",
		Role:         "client",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers/%d/qr", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "image/png" {
		t.Errorf("expected image/png, got %q", contentType)
	}

	// PNG files start with magic bytes.
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47}
	body := w.Body.Bytes()
	if len(body) < 4 {
		t.Fatal("response too short to be a PNG")
	}
	for i, b := range pngHeader {
		if body[i] != b {
			t.Fatalf("expected PNG header at byte %d: got %x, want %x", i, body[i], b)
		}
	}
}

func TestFullCRUDLifecycle(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	// Step 1: Create network.
	body := `{
		"name": "Lifecycle Network",
		"mode": "gateway",
		"subnet": "10.0.0.0/24",
		"listen_port": 51820,
		"dns_servers": "1.1.1.1",
		"nat_enabled": true
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create network: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var netResp networkResponse
	json.NewDecoder(w.Body).Decode(&netResp)
	netID := netResp.ID

	// Step 2: Add peer.
	body = `{"name": "Peer 1", "role": "client", "persistent_keepalive": 25}`
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create peer: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var peerResp peerResponse
	json.NewDecoder(w.Body).Decode(&peerResp)
	peerID := peerResp.ID

	// Step 3: Update peer.
	body = `{"name": "Peer 1 Updated"}`
	req = httptest.NewRequest("PUT", fmt.Sprintf("/api/networks/%d/peers/%d", netID, peerID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update peer: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updatedPeer peerResponse
	json.NewDecoder(w.Body).Decode(&updatedPeer)
	if updatedPeer.Name != "Peer 1 Updated" {
		t.Errorf("expected updated name, got %q", updatedPeer.Name)
	}

	// Step 4: List peers (should have 1).
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers", netID), nil)
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list peers: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var peers []peerResponse
	json.NewDecoder(w.Body).Decode(&peers)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}

	// Step 5: Delete peer.
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d/peers/%d", netID, peerID), nil)
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete peer: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Step 6: List peers (should be empty).
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d/peers", netID), nil)
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&peers)
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after delete, got %d", len(peers))
	}

	// Step 7: Delete network.
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d", netID), nil)
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete network: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Step 8: Verify network is gone.
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d", netID), nil)
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreatePeer_SiteGateway_AllowedIPs(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "S2S Network",
		Interface:  "wg0",
		Mode:       "site-to-site",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	body := `{
		"name": "Site Gateway",
		"role": "site-gateway",
		"site_networks": "192.168.1.0/24, 192.168.2.0/24"
	}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/networks/%d/peers", netID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp peerResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Server-side AllowedIPs should include peer IP + site networks.
	if !strings.Contains(resp.AllowedIPs, "10.0.0.2/32") {
		t.Errorf("expected AllowedIPs to contain peer IP, got %q", resp.AllowedIPs)
	}
	if !strings.Contains(resp.AllowedIPs, "192.168.1.0/24") {
		t.Errorf("expected AllowedIPs to contain site network, got %q", resp.AllowedIPs)
	}
	if resp.SiteNetworks != "192.168.1.0/24, 192.168.2.0/24" {
		t.Errorf("expected site_networks preserved, got %q", resp.SiteNetworks)
	}
}

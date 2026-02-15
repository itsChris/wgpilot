package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/testutil"
	"github.com/itsChris/wgpilot/internal/wg"
)

// newTestServerWithWG creates a test server with mocked WG and NFT managers.
func newTestServerWithWG(t *testing.T) (*Server, *testutil.MockWireGuardController, *testutil.MockNFTManager) {
	t.Helper()
	ctx := context.Background()
	logger := newDiscardLogger()

	database, err := db.New(ctx, ":memory:", logger, true)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(ctx, database, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	jwtSvc, err := auth.NewJWTService(newTestJWTSecret(), 24*time.Hour, logger)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}

	sessions, err := auth.NewSessionManager(false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	limiter, err := auth.NewLoginRateLimiter(5, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter: %v", err)
	}
	t.Cleanup(func() { limiter.Stop() })

	mockWG := &testutil.MockWireGuardController{}
	mockLink := &testutil.MockLinkManager{}
	mockNFT := testutil.NewMockNFTManager()

	wgMgr, err := wg.NewManager(mockWG, mockLink, logger)
	if err != nil {
		t.Fatalf("wg.NewManager: %v", err)
	}

	srv, err := New(Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: limiter,
		WGManager:   wgMgr,
		NFTManager:  mockNFT,
		DevMode:     true,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv, mockWG, mockNFT
}

// authRequest adds a valid JWT cookie to a request.
func authRequest(t *testing.T, srv *Server, req *http.Request) *http.Request {
	t.Helper()
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	return req
}

func TestCreateNetwork_Success(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)

	body := `{
		"name": "Home VPN",
		"mode": "gateway",
		"subnet": "10.0.0.0/24",
		"listen_port": 51820,
		"dns_servers": "1.1.1.1,8.8.8.8",
		"nat_enabled": true,
		"inter_peer_routing": false
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp networkResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Name != "Home VPN" {
		t.Errorf("expected name='Home VPN', got %q", resp.Name)
	}
	if resp.Mode != "gateway" {
		t.Errorf("expected mode='gateway', got %q", resp.Mode)
	}
	if resp.Subnet != "10.0.0.0/24" {
		t.Errorf("expected subnet='10.0.0.0/24', got %q", resp.Subnet)
	}
	if resp.ListenPort != 51820 {
		t.Errorf("expected listen_port=51820, got %d", resp.ListenPort)
	}
	if resp.Interface != "wg0" {
		t.Errorf("expected interface='wg0', got %q", resp.Interface)
	}
	if resp.PublicKey == "" {
		t.Error("expected non-empty public key")
	}
	if !resp.NATEnabled {
		t.Error("expected nat_enabled=true")
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}

	// Check that NAT was applied.
	if _, ok := mockNFT.NATRules["wg0"]; !ok {
		t.Error("expected NAT masquerade rule for wg0")
	}
}

func TestCreateNetwork_ValidationErrors(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	body := `{
		"name": "",
		"mode": "invalid",
		"subnet": "300.0.0.0/24",
		"listen_port": 80,
		"dns_servers": "not-an-ip"
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
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
	if len(resp.Fields) < 4 {
		t.Errorf("expected at least 4 field errors, got %d: %+v", len(resp.Fields), resp.Fields)
	}

	fieldNames := map[string]bool{}
	for _, f := range resp.Fields {
		fieldNames[f.Field] = true
	}
	for _, expected := range []string{"name", "mode", "subnet", "listen_port", "dns_servers"} {
		if !fieldNames[expected] {
			t.Errorf("expected field error for %q", expected)
		}
	}
}

func TestCreateNetwork_SubnetConflict(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	_, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Existing",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "existing-pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	body := `{
		"name": "Conflicting",
		"mode": "gateway",
		"subnet": "10.0.0.0/24",
		"listen_port": 51821
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "SUBNET_CONFLICT" {
		t.Errorf("expected SUBNET_CONFLICT, got %q", resp.Error.Code)
	}
}

func TestCreateNetwork_PortConflict(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	_, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Existing",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "existing-pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	body := `{
		"name": "Port Conflict",
		"mode": "gateway",
		"subnet": "10.1.0.0/24",
		"listen_port": 51820
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "PORT_IN_USE" {
		t.Errorf("expected PORT_IN_USE, got %q", resp.Error.Code)
	}
}

func TestListNetworks_Empty(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("GET", "/api/networks", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []networkListItem
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestListNetworks_WithPeerCounts(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "TestNet",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	for _, name := range []string{"Peer A", "Peer B"} {
		_, err := srv.db.CreatePeer(ctx, &db.Peer{
			NetworkID:  netID,
			Name:       name,
			PublicKey:  "pk-" + name,
			AllowedIPs: "10.0.0.2/32",
			Role:       "client",
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("create peer: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/api/networks", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []networkListItem
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 network, got %d", len(result))
	}
	if result[0].PeerCount != 2 {
		t.Errorf("expected peer_count=2, got %d", result[0].PeerCount)
	}
}

func TestGetNetwork_Found(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	id, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "TestNet",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/networks/%d", id), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp networkResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "TestNet" {
		t.Errorf("expected name=TestNet, got %q", resp.Name)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("GET", "/api/networks/999", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNetwork_Success(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)
	ctx := context.Background()

	id, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Original",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		NATEnabled: false,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	body := `{"name": "Updated", "nat_enabled": true, "dns_servers": "1.1.1.1"}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/networks/%d", id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp networkResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "Updated" {
		t.Errorf("expected name=Updated, got %q", resp.Name)
	}
	if !resp.NATEnabled {
		t.Error("expected nat_enabled=true after update")
	}
	if resp.DNSServers != "1.1.1.1" {
		t.Errorf("expected dns_servers=1.1.1.1, got %q", resp.DNSServers)
	}

	if _, ok := mockNFT.NATRules["wg0"]; !ok {
		t.Error("expected NAT masquerade rule for wg0 after enabling NAT")
	}
}

func TestUpdateNetwork_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	body := `{"name": "Updated"}`
	req := httptest.NewRequest("PUT", "/api/networks/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteNetwork_Success(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	id, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "ToDelete",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		NATEnabled: true,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d", id), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	n, err := srv.db.GetNetworkByID(ctx, id)
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if n != nil {
		t.Error("expected network to be deleted")
	}
}

func TestDeleteNetwork_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("DELETE", "/api/networks/999", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteNetwork_CascadeDeletesPeers(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	ctx := context.Background()

	netID, err := srv.db.CreateNetwork(ctx, &db.Network{
		Name:       "Network",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PublicKey:   "pub-key",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	peerID, err := srv.db.CreatePeer(ctx, &db.Peer{
		NetworkID:  netID,
		Name:       "Peer",
		PublicKey:  "peer-pub-key",
		AllowedIPs: "10.0.0.2/32",
		Role:       "client",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d", netID), nil)
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
		t.Error("expected peer to be cascade-deleted")
	}
}

func TestCreateNetwork_InterPeerRouting(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)

	body := `{
		"name": "Hub Network",
		"mode": "hub-routed",
		"subnet": "10.0.0.0/24",
		"listen_port": 51820,
		"nat_enabled": false,
		"inter_peer_routing": true
	}`
	req := httptest.NewRequest("POST", "/api/networks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if !mockNFT.ForwardRules["wg0"] {
		t.Error("expected inter-peer forwarding rule for wg0")
	}
	if _, ok := mockNFT.NATRules["wg0"]; ok {
		t.Error("expected no NAT rule for hub-routed mode")
	}
}

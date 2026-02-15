package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	"github.com/itsChris/wgpilot/internal/logging"
	"github.com/itsChris/wgpilot/internal/testutil"
	"github.com/itsChris/wgpilot/internal/wg"
)

func newTestServerForMonitoring(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	logger := newDiscardLogger()
	ring := logging.NewRingBuffer(100)

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

	mockWG := &testutil.MockWireGuardController{
		DeviceFn: func(name string) (*wg.DeviceInfo, error) {
			return &wg.DeviceInfo{
				Name:       name,
				PublicKey:  "server-pub-key",
				ListenPort: 51820,
				Peers: []wg.WGPeerInfo{
					{
						PublicKey:     "peer-public-key",
						Endpoint:      "1.2.3.4:12345",
						LastHandshake: time.Now().Add(-30 * time.Second),
						ReceiveBytes:  5000,
						TransmitBytes: 3000,
					},
				},
			}, nil
		},
	}
	mockLink := &testutil.MockLinkManager{}

	wgMgr, err := wg.NewManager(mockWG, mockLink, logger)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	srv, err := New(Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: limiter,
		WGManager:   wgMgr,
		DevMode:     true,
		Ring:        ring,
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func authCookie(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: token}
}

func req_ctx() context.Context {
	return context.Background()
}

func newTestNetwork() *db.Network {
	return &db.Network{
		Name: "Test VPN", Interface: "wg0", Mode: "gateway",
		Subnet: "10.0.0.0/24", ListenPort: 51820,
		PrivateKey: "priv", PublicKey: "pub",
		Enabled: true,
	}
}

func newProdModeServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	logger := newDiscardLogger()
	ring := logging.NewRingBuffer(100)

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

	srv, err := New(Config{
		DB:          database,
		Logger:      logger,
		JWTService:  jwtSvc,
		Sessions:    sessions,
		RateLimiter: limiter,
		DevMode:     false, // Production mode.
		Ring:        ring,
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func TestHandleStatus_ReturnsCorrectPeerData(t *testing.T) {
	srv := newTestServerForMonitoring(t)
	ctx := context.Background()

	// Create a network and peer in DB.
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

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(resp.Networks))
	}

	net := resp.Networks[0]
	if net.Name != "Test VPN" {
		t.Errorf("expected name='Test VPN', got %q", net.Name)
	}
	if !net.Up {
		t.Error("expected interface to be up")
	}
	if len(net.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(net.Peers))
	}

	peer := net.Peers[0]
	if peer.Name != "My Phone" {
		t.Errorf("expected peer name='My Phone', got %q", peer.Name)
	}
	if !peer.Online {
		t.Error("expected peer to be online")
	}
	if peer.TransferRx != 5000 {
		t.Errorf("expected transfer_rx=5000, got %d", peer.TransferRx)
	}
}

func TestHandleStatus_EmptyNetworks(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Networks) != 0 {
		t.Errorf("expected 0 networks, got %d", len(resp.Networks))
	}
}

func TestHandleSSEEvents_ReceivesUpdates(t *testing.T) {
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

	// Use httptest.NewServer for real HTTP connection.
	ts := httptest.NewServer(srv)
	defer ts.Close()

	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", ts.URL+"/api/networks/1/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream, got %q", ct)
	}

	// Read the first SSE event (sent immediately on connect).
	scanner := bufio.NewScanner(resp.Body)
	var eventType, eventData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if eventType != "status" {
		t.Errorf("expected event type 'status', got %q", eventType)
	}
	if eventData == "" {
		t.Fatal("expected event data")
	}

	var events []map[string]any
	if err := json.Unmarshal([]byte(eventData), &events); err != nil {
		t.Fatalf("parse event data: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestHandleSSEEvents_InvalidNetworkID(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/api/networks/abc/events", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSSEEvents_NetworkNotFound(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/api/networks/999/events", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

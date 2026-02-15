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

func createTwoNetworks(t *testing.T, database *db.DB) (int64, int64) {
	t.Helper()
	ctx := context.Background()

	netAID, err := database.CreateNetwork(ctx, &db.Network{
		Name:       "Network A",
		Interface:  "wg0",
		Mode:       "gateway",
		Subnet:     "10.0.0.0/24",
		ListenPort: 51820,
		PrivateKey: "priv-a",
		PublicKey:  "pub-a",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network A: %v", err)
	}

	netBID, err := database.CreateNetwork(ctx, &db.Network{
		Name:       "Network B",
		Interface:  "wg1",
		Mode:       "gateway",
		Subnet:     "10.1.0.0/24",
		ListenPort: 51821,
		PrivateKey: "priv-b",
		PublicKey:  "pub-b",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create network B: %v", err)
	}

	return netAID, netBID
}

func TestCreateBridge_Success(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	body := fmt.Sprintf(`{
		"network_a_id": %d,
		"network_b_id": %d,
		"direction": "bidirectional"
	}`, netAID, netBID)
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp bridgeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.NetworkAID != netAID {
		t.Errorf("expected network_a_id=%d, got %d", netAID, resp.NetworkAID)
	}
	if resp.NetworkBID != netBID {
		t.Errorf("expected network_b_id=%d, got %d", netBID, resp.NetworkBID)
	}
	if resp.Direction != "bidirectional" {
		t.Errorf("expected direction=bidirectional, got %q", resp.Direction)
	}
	if resp.InterfaceA != "wg0" {
		t.Errorf("expected interface_a=wg0, got %q", resp.InterfaceA)
	}
	if resp.InterfaceB != "wg1" {
		t.Errorf("expected interface_b=wg1, got %q", resp.InterfaceB)
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}

	// Check that nftables rules were applied.
	if _, ok := mockNFT.BridgeRules["wg0:wg1"]; !ok {
		t.Error("expected bridge rule for wg0:wg1")
	}
}

func TestCreateBridge_InvalidDirection(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	body := fmt.Sprintf(`{
		"network_a_id": %d,
		"network_b_id": %d,
		"direction": "invalid"
	}`, netAID, netBID)
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateBridge_DuplicateReturns409(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	body := fmt.Sprintf(`{
		"network_a_id": %d,
		"network_b_id": %d,
		"direction": "bidirectional"
	}`, netAID, netBID)

	// First creation should succeed.
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Second creation should fail with 409.
	req = httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	var resp errorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "BRIDGE_ALREADY_EXISTS" {
		t.Errorf("expected BRIDGE_ALREADY_EXISTS, got %q", resp.Error.Code)
	}
}

func TestCreateBridge_NonexistentNetworkReturns404(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	body := `{
		"network_a_id": 999,
		"network_b_id": 998,
		"direction": "bidirectional"
	}`
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateBridge_SelfReferenceReturns400(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netAID, _ := createTwoNetworks(t, srv.db)

	body := fmt.Sprintf(`{
		"network_a_id": %d,
		"network_b_id": %d,
		"direction": "bidirectional"
	}`, netAID, netAID)
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListBridges_Empty(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("GET", "/api/bridges", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []bridgeResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestListBridges_WithBridges(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	ctx := context.Background()
	if _, err := srv.db.CreateBridge(ctx, &db.Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	}); err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/bridges", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []bridgeResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 bridge, got %d", len(result))
	}
	if result[0].NetworkAName != "Network A" {
		t.Errorf("expected network_a_name='Network A', got %q", result[0].NetworkAName)
	}
	if result[0].NetworkBName != "Network B" {
		t.Errorf("expected network_b_name='Network B', got %q", result[0].NetworkBName)
	}
}

func TestGetBridge_Found(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	ctx := context.Background()
	id, err := srv.db.CreateBridge(ctx, &db.Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "a_to_b", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/bridges/%d", id), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp bridgeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Direction != "a_to_b" {
		t.Errorf("expected direction=a_to_b, got %q", resp.Direction)
	}
}

func TestGetBridge_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("GET", "/api/bridges/999", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteBridge_Success(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	ctx := context.Background()
	id, err := srv.db.CreateBridge(ctx, &db.Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	// Add the NFT rule so we can verify it gets removed.
	mockNFT.AddNetworkBridge("wg0", "wg1", "bidirectional")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/bridges/%d", id), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Bridge should be gone from DB.
	got, err := srv.db.GetBridgeByID(ctx, id)
	if err != nil {
		t.Fatalf("get bridge: %v", err)
	}
	if got != nil {
		t.Error("expected bridge to be deleted")
	}

	// NFT bridge rule should be removed.
	if _, ok := mockNFT.BridgeRules["wg0:wg1"]; ok {
		t.Error("expected bridge NFT rule to be removed")
	}
}

func TestDeleteBridge_NotFound(t *testing.T) {
	srv, _, _ := newTestServerWithWG(t)

	req := httptest.NewRequest("DELETE", "/api/bridges/999", nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteNetwork_CascadeDeletesBridges(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	ctx := context.Background()
	bridgeID, err := srv.db.CreateBridge(ctx, &db.Bridge{
		NetworkAID: netAID, NetworkBID: netBID, Direction: "bidirectional", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create bridge: %v", err)
	}

	// Add the NFT rule.
	mockNFT.AddNetworkBridge("wg0", "wg1", "bidirectional")

	// Delete network A — bridge should cascade-delete and NFT rules should be removed.
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/networks/%d", netAID), nil)
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Bridge should be gone.
	bridge, err := srv.db.GetBridgeByID(ctx, bridgeID)
	if err != nil {
		t.Fatalf("get bridge: %v", err)
	}
	if bridge != nil {
		t.Error("expected bridge to be cascade-deleted")
	}

	// NFT bridge rule should be removed.
	if _, ok := mockNFT.BridgeRules["wg0:wg1"]; ok {
		t.Error("expected bridge NFT rule to be removed after network deletion")
	}
}

func TestCreateBridge_DirectionAToB(t *testing.T) {
	srv, _, mockNFT := newTestServerWithWG(t)
	netAID, netBID := createTwoNetworks(t, srv.db)

	body := fmt.Sprintf(`{
		"network_a_id": %d,
		"network_b_id": %d,
		"direction": "a_to_b"
	}`, netAID, netBID)
	req := httptest.NewRequest("POST", "/api/bridges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authRequest(t, srv, req)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Check direction in NFT mock.
	dir, ok := mockNFT.BridgeRules["wg0:wg1"]
	if !ok {
		t.Fatal("expected bridge rule")
	}
	if dir != "a_to_b" {
		t.Errorf("expected direction a_to_b in NFT, got %q", dir)
	}
}

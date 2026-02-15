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
)

// ── Helpers ──────────────────────────────────────────────────────────

// newSetupTestServer creates a test server with setup NOT complete.
// The default newTestServer marks setup as complete; this undoes that.
func newSetupTestServer(t *testing.T) *Server {
	t.Helper()
	srv := newTestServer(t)
	ctx := context.Background()
	// Clear setup_complete so we start in setup mode.
	if err := srv.db.DeleteSetting(ctx, "setup_complete"); err != nil {
		t.Fatalf("DeleteSetting setup_complete: %v", err)
	}
	return srv
}

// setupOTP stores a hashed OTP in the test database.
func setupOTP(t *testing.T, srv *Server, otp string) {
	t.Helper()
	hash, err := auth.HashPassword(otp)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := srv.db.SetSetting(context.Background(), "setup_otp", hash); err != nil {
		t.Fatalf("SetSetting setup_otp: %v", err)
	}
}

// doStep1 executes setup step 1 and returns the session cookie.
func doStep1(t *testing.T, srv *Server, otp string) *http.Cookie {
	t.Helper()
	body := fmt.Sprintf(`{"otp":%q,"username":"admin","password":"securepassword123"}`, otp)
	req := httptest.NewRequest("POST", "/api/setup/step/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("step 1: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	t.Fatal("step 1: no session cookie")
	return nil
}

// doStep2 executes setup step 2.
func doStep2(t *testing.T, srv *Server, cookie *http.Cookie) {
	t.Helper()
	body := `{"public_ip":"203.0.113.1","hostname":"","dns_servers":"1.1.1.1,8.8.8.8"}`
	req := httptest.NewRequest("POST", "/api/setup/step/2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("step 2: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// doStep3 executes setup step 3 and returns the network ID.
func doStep3(t *testing.T, srv *Server, cookie *http.Cookie) int64 {
	t.Helper()
	body := `{"name":"Home VPN","mode":"gateway","subnet":"10.0.0.0/24","listen_port":51820,"nat_enabled":true,"inter_peer_routing":false}`
	req := httptest.NewRequest("POST", "/api/setup/step/3", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("step 3: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupStep3Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("step 3: decode: %v", err)
	}
	return resp.Network.ID
}

// doStep4 executes setup step 4.
func doStep4(t *testing.T, srv *Server, cookie *http.Cookie) {
	t.Helper()
	body := `{"name":"My Laptop","role":"client","tunnel_type":"full"}`
	req := httptest.NewRequest("POST", "/api/setup/step/4", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("step 4: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Tests ────────────────────────────────────────────────────────────

func TestSetupStatus_InitialState(t *testing.T) {
	srv := newSetupTestServer(t)

	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Complete {
		t.Error("expected complete=false initially")
	}
	if resp.CurrentStep != 1 {
		t.Errorf("expected current_step=1, got %d", resp.CurrentStep)
	}
}

func TestSetupStatus_AfterComplete(t *testing.T) {
	srv := newSetupTestServer(t)
	ctx := context.Background()

	if err := srv.db.SetSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var resp setupStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Complete {
		t.Error("expected complete=true")
	}
}

func TestSetupStep1_Success(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "install-password-123"
	setupOTP(t, srv, otp)

	cookie := doStep1(t, srv, otp)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}

	// Verify admin was created.
	ctx := context.Background()
	user, err := srv.db.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected admin user")
	}

	// Verify OTP was deleted.
	otpVal, _ := srv.db.GetSetting(ctx, "setup_otp")
	if otpVal != "" {
		t.Error("OTP should be deleted")
	}

	// Verify step marker is set.
	step1, _ := srv.db.GetSetting(ctx, "setup_step1_done")
	if step1 != "true" {
		t.Errorf("expected setup_step1_done=true, got %q", step1)
	}

	// Verify setup is NOT complete yet (only step 1 done).
	complete, _ := srv.db.GetSetting(ctx, "setup_complete")
	if complete == "true" {
		t.Error("setup should not be complete after step 1")
	}
}

func TestSetupStep1_InvalidOTP(t *testing.T) {
	srv := newSetupTestServer(t)
	setupOTP(t, srv, "correct-password")

	body := `{"otp":"wrong-password","username":"admin","password":"securepassword123"}`
	req := httptest.NewRequest("POST", "/api/setup/step/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep1_AlreadyUsed(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "install-password"
	setupOTP(t, srv, otp)

	// Complete step 1.
	doStep1(t, srv, otp)

	// Try step 1 again.
	body := `{"otp":"install-password","username":"admin2","password":"securepassword123"}`
	req := httptest.NewRequest("POST", "/api/setup/step/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for re-used OTP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep1_ShortPassword(t *testing.T) {
	srv := newSetupTestServer(t)
	setupOTP(t, srv, "test-otp")

	body := `{"otp":"test-otp","username":"admin","password":"short"}`
	req := httptest.NewRequest("POST", "/api/setup/step/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep2_Success(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)

	doStep2(t, srv, cookie)

	// Verify settings saved.
	ctx := context.Background()
	publicIP, _ := srv.db.GetSetting(ctx, "public_ip")
	if publicIP != "203.0.113.1" {
		t.Errorf("expected public_ip=203.0.113.1, got %q", publicIP)
	}
	step2, _ := srv.db.GetSetting(ctx, "setup_step2_done")
	if step2 != "true" {
		t.Error("expected setup_step2_done=true")
	}
}

func TestSetupStep2_RequiresStep1(t *testing.T) {
	srv := newSetupTestServer(t)

	// Create a valid token to bypass auth but not setup state.
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	body := `{"public_ip":"1.2.3.4","dns_servers":"1.1.1.1"}`
	req := httptest.NewRequest("POST", "/api/setup/step/2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for skipped step, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep3_Success(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)

	networkID := doStep3(t, srv, cookie)
	if networkID == 0 {
		t.Error("expected non-zero network ID")
	}

	// Verify network was created in DB.
	ctx := context.Background()
	network, err := srv.db.GetNetworkByID(ctx, networkID)
	if err != nil {
		t.Fatalf("GetNetworkByID: %v", err)
	}
	if network == nil {
		t.Fatal("expected network to exist")
	}
	if network.Name != "Home VPN" {
		t.Errorf("expected name=Home VPN, got %q", network.Name)
	}
	if network.Mode != "gateway" {
		t.Errorf("expected mode=gateway, got %q", network.Mode)
	}

	step3, _ := srv.db.GetSetting(ctx, "setup_step3_done")
	if step3 != "true" {
		t.Error("expected setup_step3_done=true")
	}
}

func TestSetupStep3_RequiresStep2(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)

	// Skip step 2, try step 3.
	body := `{"name":"Test","mode":"gateway","subnet":"10.0.0.0/24","listen_port":51820,"nat_enabled":true}`
	req := httptest.NewRequest("POST", "/api/setup/step/3", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for skipped step, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep4_Success(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)
	doStep3(t, srv, cookie)

	body := `{"name":"My Laptop","role":"client","tunnel_type":"full"}`
	req := httptest.NewRequest("POST", "/api/setup/step/4", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp setupStep4Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Peer.Name != "My Laptop" {
		t.Errorf("expected peer name=My Laptop, got %q", resp.Peer.Name)
	}
	if resp.Config == "" {
		t.Error("expected non-empty config")
	}
	if resp.QRData == "" {
		t.Error("expected non-empty QR data")
	}

	// Verify setup_complete is true.
	ctx := context.Background()
	complete, _ := srv.db.GetSetting(ctx, "setup_complete")
	if complete != "true" {
		t.Error("expected setup_complete=true after step 4")
	}
}

func TestSetupStep4_RequiresStep3(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)

	// Skip step 3, try step 4.
	body := `{"name":"My Laptop","role":"client","tunnel_type":"full"}`
	req := httptest.NewRequest("POST", "/api/setup/step/4", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for skipped step, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupFullFlow(t *testing.T) {
	srv := newSetupTestServer(t)
	ctx := context.Background()
	otp := "full-flow-otp"
	setupOTP(t, srv, otp)

	// Step 1: Create admin.
	cookie := doStep1(t, srv, otp)

	// Verify status shows step 2.
	statusReq := httptest.NewRequest("GET", "/api/setup/status", nil)
	statusW := httptest.NewRecorder()
	srv.ServeHTTP(statusW, statusReq)
	var status setupStatusResponse
	json.NewDecoder(statusW.Body).Decode(&status)
	if status.CurrentStep != 2 {
		t.Errorf("after step 1: expected current_step=2, got %d", status.CurrentStep)
	}

	// Step 2: Server settings.
	doStep2(t, srv, cookie)

	// Step 3: Create network.
	doStep3(t, srv, cookie)

	// Step 4: Create peer → setup complete.
	doStep4(t, srv, cookie)

	// Verify setup is complete.
	complete, _ := srv.db.GetSetting(ctx, "setup_complete")
	if complete != "true" {
		t.Fatal("expected setup_complete=true")
	}

	// Verify status endpoint reports complete.
	statusReq2 := httptest.NewRequest("GET", "/api/setup/status", nil)
	statusW2 := httptest.NewRecorder()
	srv.ServeHTTP(statusW2, statusReq2)
	var status2 setupStatusResponse
	json.NewDecoder(statusW2.Body).Decode(&status2)
	if !status2.Complete {
		t.Error("expected complete=true in status")
	}
}

func TestSetupResumeAfterInterruption(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "resume-otp"
	setupOTP(t, srv, otp)

	// Complete steps 1 and 2.
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)

	// "Browser closed" — check status to see where to resume.
	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var status setupStatusResponse
	json.NewDecoder(w.Body).Decode(&status)
	if status.Complete {
		t.Error("should not be complete yet")
	}
	if status.CurrentStep != 3 {
		t.Errorf("expected current_step=3 after step 2, got %d", status.CurrentStep)
	}

	// Resume at step 3.
	doStep3(t, srv, cookie)

	// Check status again.
	req2 := httptest.NewRequest("GET", "/api/setup/status", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	var status2 setupStatusResponse
	json.NewDecoder(w2.Body).Decode(&status2)
	if status2.CurrentStep != 4 {
		t.Errorf("expected current_step=4 after step 3, got %d", status2.CurrentStep)
	}
}

func TestSetupEndpoints_409AfterCompletion(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "complete-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)
	doStep3(t, srv, cookie)
	doStep4(t, srv, cookie)

	// All step endpoints should return 409.
	tests := []struct {
		method string
		path   string
		body   string
		auth   bool
	}{
		{"POST", "/api/setup/step/1", `{"otp":"x","username":"a","password":"longpassword123"}`, false},
		{"POST", "/api/setup/step/2", `{"public_ip":"1.2.3.4"}`, true},
		{"POST", "/api/setup/step/3", `{"name":"X","mode":"gateway","subnet":"10.1.0.0/24","listen_port":51821}`, true},
		{"POST", "/api/setup/step/4", `{"name":"X","role":"client"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.auth {
				req.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != http.StatusConflict {
				t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSetupGuard_BlocksAPIDuringSetup(t *testing.T) {
	srv := newSetupTestServer(t)

	// Generate a valid token to test the guard (not auth).
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Try to access a guarded endpoint during setup.
	req := httptest.NewRequest("GET", "/api/networks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 during setup, got %d: %s", w.Code, w.Body.String())
	}

	// Check that the error code is SETUP_REQUIRED.
	var resp errorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Code != "SETUP_REQUIRED" {
		t.Errorf("expected code=SETUP_REQUIRED, got %q", resp.Error.Code)
	}
}

func TestSetupGuard_AllowsAfterComplete(t *testing.T) {
	srv := newSetupTestServer(t)
	ctx := context.Background()

	// Mark setup as complete.
	if err := srv.db.SetSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	// Generate a valid token.
	token, err := srv.jwtService.Generate(1, "admin", "admin")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Access a guarded endpoint — should work (returns empty list, not 403).
	req := httptest.NewRequest("GET", "/api/networks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 after setup complete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDetectPublicIP_MockServer(t *testing.T) {
	// Create a mock HTTP server that returns a known IP.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "  198.51.100.42\n")
	}))
	defer mockServer.Close()

	// Override the service list and HTTP client.
	origServices := ipDetectServices
	origClient := newHTTPClient
	defer func() {
		ipDetectServices = origServices
		newHTTPClient = origClient
	}()

	ipDetectServices = []string{mockServer.URL}
	newHTTPClient = func(timeout time.Duration) httpDoer {
		return &http.Client{Timeout: timeout}
	}

	ip := detectPublicIP(context.Background())
	if ip != "198.51.100.42" {
		t.Errorf("expected 198.51.100.42, got %q", ip)
	}
}

func TestDetectPublicIP_AllFail(t *testing.T) {
	origServices := ipDetectServices
	origClient := newHTTPClient
	defer func() {
		ipDetectServices = origServices
		newHTTPClient = origClient
	}()

	// Point to a non-existent server.
	ipDetectServices = []string{"http://127.0.0.1:1"}
	newHTTPClient = func(timeout time.Duration) httpDoer {
		return &http.Client{Timeout: 100 * time.Millisecond}
	}

	ip := detectPublicIP(context.Background())
	if ip != "" {
		t.Errorf("expected empty string when all services fail, got %q", ip)
	}
}

func TestSetupStep2_InvalidIP(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)

	body := `{"public_ip":"not-an-ip","dns_servers":"1.1.1.1"}`
	req := httptest.NewRequest("POST", "/api/setup/step/2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid IP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep3_InvalidSubnet(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)

	body := `{"name":"Test","mode":"gateway","subnet":"999.999.999.0/24","listen_port":51820,"nat_enabled":true}`
	req := httptest.NewRequest("POST", "/api/setup/step/3", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid subnet, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep4_InvalidRole(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)
	doStep2(t, srv, cookie)
	doStep3(t, srv, cookie)

	body := `{"name":"Test Peer","role":"invalid-role"}`
	req := httptest.NewRequest("POST", "/api/setup/step/4", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetupStep2_Idempotent(t *testing.T) {
	srv := newSetupTestServer(t)
	otp := "test-otp"
	setupOTP(t, srv, otp)
	cookie := doStep1(t, srv, otp)

	// Execute step 2 twice — should be idempotent.
	doStep2(t, srv, cookie)
	doStep2(t, srv, cookie)

	// Should still be on step 3.
	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var status setupStatusResponse
	json.NewDecoder(w.Body).Decode(&status)
	if status.CurrentStep != 3 {
		t.Errorf("expected current_step=3 after idempotent step 2, got %d", status.CurrentStep)
	}
}

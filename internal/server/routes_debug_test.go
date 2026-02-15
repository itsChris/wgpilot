package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/itsChris/wgpilot/internal/logging"
)

func TestHandleDebugInfo_DevMode_ReturnsFields(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/api/debug/info", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check expected top-level keys.
	for _, key := range []string{"version", "go_version", "os", "arch", "uptime_seconds", "config", "system", "database"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}

	if resp["version"] != "test" {
		t.Errorf("expected version='test', got %v", resp["version"])
	}

	// Check system sub-fields.
	system, ok := resp["system"].(map[string]any)
	if !ok {
		t.Fatal("expected system to be a map")
	}
	for _, key := range []string{"memory_mb", "goroutines", "cpu_count"} {
		if _, ok := system[key]; !ok {
			t.Errorf("missing system key %q", key)
		}
	}

	// Check database sub-fields.
	database, ok := resp["database"].(map[string]any)
	if !ok {
		t.Fatal("expected database to be a map")
	}
	tables, ok := database["tables"].(map[string]any)
	if !ok {
		t.Fatal("expected database.tables to be a map")
	}
	for _, table := range []string{"networks", "peers", "peer_snapshots", "settings"} {
		if _, ok := tables[table]; !ok {
			t.Errorf("missing database table count for %q", table)
		}
	}
}

func TestHandleDebugInfo_DevMode_IncludesWireGuard(t *testing.T) {
	srv := newTestServerForMonitoring(t)
	ctx := req_ctx()

	// Create a network so WG section has data.
	_, err := srv.db.CreateNetwork(ctx, newTestNetwork())
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/debug/info", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wgData, ok := resp["wireguard"].(map[string]any)
	if !ok {
		t.Fatal("expected wireguard section in response")
	}
	ifaces, ok := wgData["interfaces"].([]any)
	if !ok {
		t.Fatal("expected wireguard.interfaces to be an array")
	}
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}
}

func TestHandleDebugInfo_ProductionMode_Returns404(t *testing.T) {
	// Create a server in production mode (devMode=false).
	srv := newTestServer(t)
	// newTestServer sets devMode=true, so we need a non-dev server.
	prodSrv := newProdModeServer(t)

	_ = srv // suppress unused

	req := httptest.NewRequest("GET", "/api/debug/info", nil)
	req.AddCookie(authCookie(t, prodSrv))
	w := httptest.NewRecorder()
	prodSrv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 in production mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDebugLogs_ReturnsRingBufferEntries(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	// Write some entries to the ring buffer.
	srv.ring.Write(logging.LogEntry{
		Timestamp: time.Now(),
		Level:     slog.LevelWarn,
		Message:   "test warning",
		Attrs:     map[string]any{"component": "test"},
	})
	srv.ring.Write(logging.LogEntry{
		Timestamp: time.Now(),
		Level:     slog.LevelError,
		Message:   "test error",
		Attrs:     map[string]any{"component": "test"},
	})

	req := httptest.NewRequest("GET", "/api/debug/logs", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entries []map[string]any `json:"entries"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Count != 2 {
		t.Fatalf("expected 2 entries, got %d", resp.Count)
	}

	// Verify first entry (warning).
	if resp.Entries[0]["message"] != "test warning" {
		t.Errorf("expected message 'test warning', got %v", resp.Entries[0]["message"])
	}
	if resp.Entries[0]["level"] != "WARN" {
		t.Errorf("expected level 'WARN', got %v", resp.Entries[0]["level"])
	}
}

func TestHandleDebugLogs_FilterByLevel(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	srv.ring.Write(logging.LogEntry{
		Timestamp: time.Now(),
		Level:     slog.LevelWarn,
		Message:   "a warning",
	})
	srv.ring.Write(logging.LogEntry{
		Timestamp: time.Now(),
		Level:     slog.LevelError,
		Message:   "an error",
	})

	req := httptest.NewRequest("GET", "/api/debug/logs?level=error", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Entries []map[string]any `json:"entries"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Count != 1 {
		t.Fatalf("expected 1 entry with level=error filter, got %d", resp.Count)
	}
	if resp.Entries[0]["message"] != "an error" {
		t.Errorf("expected 'an error', got %v", resp.Entries[0]["message"])
	}
}

func TestHandleDebugLogs_WithLimit(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	for i := 0; i < 10; i++ {
		srv.ring.Write(logging.LogEntry{
			Timestamp: time.Now(),
			Level:     slog.LevelWarn,
			Message:   "warning",
		})
	}

	req := httptest.NewRequest("GET", "/api/debug/logs?limit=3", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Entries []map[string]any `json:"entries"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Count != 3 {
		t.Fatalf("expected 3 entries with limit=3, got %d", resp.Count)
	}
}

func TestHandleDebugLogs_EmptyRingBuffer(t *testing.T) {
	srv := newTestServerForMonitoring(t)

	req := httptest.NewRequest("GET", "/api/debug/logs", nil)
	req.AddCookie(authCookie(t, srv))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Entries []map[string]any `json:"entries"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Count != 0 {
		t.Errorf("expected 0 entries, got %d", resp.Count)
	}
	if resp.Entries == nil {
		t.Error("expected non-nil empty entries array")
	}
}

package updater

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.0.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"v0.3.1", "0.3.1"},
		{"dev", "dev"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeVersion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestArchBinarySuffix(t *testing.T) {
	suffix := archBinarySuffix()
	if suffix == "" {
		t.Fatal("archBinarySuffix should return a non-empty string")
	}
	// It should start with "linux_"
	if len(suffix) < 7 || suffix[:6] != "linux_" {
		t.Errorf("expected suffix starting with 'linux_', got %q", suffix)
	}
}

func TestCheckLatest_UpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName:     "v1.2.0",
			PublishedAt: "2026-01-15T00:00:00Z",
			HTMLURL:     "https://github.com/test/repo/releases/tag/v1.2.0",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := &Updater{
		repo:       "test/repo",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}
	// Override the API base to point to test server.
	origBase := githubAPIBase
	defer func() { /* can't reassign const, see below */ }()
	_ = origBase

	// Instead, we'll construct a custom updater that uses the test server.
	u.httpClient = &http.Client{
		Transport: &rewriteTransport{
			base:    srv.URL,
			wrapped: http.DefaultTransport,
		},
	}

	result, err := u.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}

	if !result.UpdateAvailable {
		t.Error("expected update available")
	}
	if result.CurrentVersion != "1.0.0" {
		t.Errorf("expected current 1.0.0, got %s", result.CurrentVersion)
	}
	if result.LatestVersion != "1.2.0" {
		t.Errorf("expected latest 1.2.0, got %s", result.LatestVersion)
	}
}

func TestCheckLatest_AlreadyUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName:     "v1.0.0",
			PublishedAt: "2026-01-15T00:00:00Z",
			HTMLURL:     "https://github.com/test/repo/releases/tag/v1.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := &Updater{
		repo: "test/repo",
		httpClient: &http.Client{
			Transport: &rewriteTransport{
				base:    srv.URL,
				wrapped: http.DefaultTransport,
			},
		},
		logger: testLogger(),
	}

	result, err := u.CheckLatest(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("should not have update available when versions match")
	}
}

func TestCheckLatest_DevVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName: "v1.0.0",
			HTMLURL: "https://github.com/test/repo/releases/tag/v1.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer srv.Close()

	u := &Updater{
		repo: "test/repo",
		httpClient: &http.Client{
			Transport: &rewriteTransport{
				base:    srv.URL,
				wrapped: http.DefaultTransport,
			},
		},
		logger: testLogger(),
	}

	result, err := u.CheckLatest(context.Background(), "dev")
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("dev version should never report update available")
	}
}

func TestCheckLatest_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	u := &Updater{
		repo: "test/repo",
		httpClient: &http.Client{
			Transport: &rewriteTransport{
				base:    srv.URL,
				wrapped: http.DefaultTransport,
			},
		},
		logger: testLogger(),
	}

	_, err := u.CheckLatest(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("should fail on API error")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()

	// Create source file.
	srcPath := dir + "/src"
	if err := os.WriteFile(srcPath, []byte("new binary content"), 0755); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// Create destination file.
	dstPath := dir + "/dst"
	if err := os.WriteFile(dstPath, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("write dest: %v", err)
	}

	if err := replaceBinary(srcPath, dstPath); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read replaced: %v", err)
	}

	if string(content) != "new binary content" {
		t.Errorf("expected 'new binary content', got %q", string(content))
	}
}

func TestBinaryURL(t *testing.T) {
	u := &Updater{
		repo:   "itsChris/wgpilot",
		logger: testLogger(),
	}

	url := u.binaryURL("1.2.0")
	if url == "" {
		t.Fatal("binaryURL should not be empty")
	}

	// Should contain the version tag.
	if !contains(url, "v1.2.0") {
		t.Errorf("URL should contain version tag, got %q", url)
	}

	// Should contain the repo.
	if !contains(url, "itsChris/wgpilot") {
		t.Errorf("URL should contain repo, got %q", url)
	}
}

func TestBinaryURL_WithVPrefix(t *testing.T) {
	u := &Updater{
		repo:   "itsChris/wgpilot",
		logger: testLogger(),
	}

	url := u.binaryURL("v1.2.0")
	// Should not double-prefix with "v".
	if contains(url, "vv1.2.0") {
		t.Errorf("URL should not have double v prefix, got %q", url)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// rewriteTransport rewrites requests to point to a test server.
type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	// Parse base URL to get host.
	req.URL.Host = t.base[len("http://"):]
	return t.wrapped.RoundTrip(req)
}

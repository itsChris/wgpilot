package debug

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRun_TextOutput(t *testing.T) {
	var buf bytes.Buffer

	err := Run(Config{
		Version:    "1.0.0-test",
		DataDir:    t.TempDir(),
		DBPath:     "",
		JSONOutput: false,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "wgpilot diagnostic report") {
		t.Error("expected 'wgpilot diagnostic report' header")
	}
	if !strings.Contains(output, "Version:     1.0.0-test") {
		t.Error("expected version in output")
	}

	// Should contain at least some check markers.
	hasChecks := strings.Contains(output, "[PASS]") ||
		strings.Contains(output, "[WARN]") ||
		strings.Contains(output, "[FAIL]")
	if !hasChecks {
		t.Error("expected at least one check result marker")
	}
}

func TestRun_JSONOutput_ValidJSON(t *testing.T) {
	var buf bytes.Buffer

	err := Run(Config{
		Version:    "1.0.0-test",
		DataDir:    t.TempDir(),
		DBPath:     "",
		JSONOutput: true,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result DiagnoseResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput: %s", err, buf.String())
	}

	if result.Version != "1.0.0-test" {
		t.Errorf("expected version '1.0.0-test', got %q", result.Version)
	}
	if result.GoVersion == "" {
		t.Error("expected go_version to be set")
	}
	if result.OS == "" {
		t.Error("expected os to be set")
	}
	if result.Arch == "" {
		t.Error("expected arch to be set")
	}
	if len(result.Checks) == 0 {
		t.Error("expected at least one check result")
	}
}

func TestRun_JSONOutput_ChecksHaveStatus(t *testing.T) {
	var buf bytes.Buffer

	err := Run(Config{
		Version:    "test",
		DataDir:    t.TempDir(),
		DBPath:     "",
		JSONOutput: true,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result DiagnoseResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	validStatuses := map[CheckStatus]bool{
		StatusPass: true,
		StatusWarn: true,
		StatusFail: true,
	}

	for i, check := range result.Checks {
		if !validStatuses[check.Status] {
			t.Errorf("check[%d] has invalid status %q", i, check.Status)
		}
		if check.Message == "" {
			t.Errorf("check[%d] has empty message", i)
		}
	}
}

func TestRun_DataDirCheck_ExistingDir(t *testing.T) {
	var buf bytes.Buffer
	dataDir := t.TempDir()

	err := Run(Config{
		Version:    "test",
		DataDir:    dataDir,
		JSONOutput: true,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result DiagnoseResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	found := false
	for _, check := range result.Checks {
		if strings.Contains(check.Message, "exists and writable") {
			found = true
			if check.Status != StatusPass {
				t.Errorf("expected PASS for existing writable dir, got %s", check.Status)
			}
		}
	}
	if !found {
		t.Error("expected a data directory check result")
	}
}

func TestRun_DataDirCheck_NonexistentDir(t *testing.T) {
	var buf bytes.Buffer

	err := Run(Config{
		Version:    "test",
		DataDir:    "/nonexistent/path/wgpilot",
		JSONOutput: true,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result DiagnoseResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	found := false
	for _, check := range result.Checks {
		if strings.Contains(check.Message, "does not exist") {
			found = true
			if check.Status != StatusFail {
				t.Errorf("expected FAIL for nonexistent dir, got %s", check.Status)
			}
		}
	}
	if !found {
		t.Error("expected a data directory check failure")
	}
}

func TestRun_DBPathEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := Run(Config{
		Version:    "test",
		DataDir:    t.TempDir(),
		DBPath:     "",
		JSONOutput: true,
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result DiagnoseResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// DB stats should not be accessible if path is empty.
	if result.DBStats.Accessible {
		t.Error("expected db not accessible with empty path")
	}
}

func TestRun_NilWriter_DefaultsToStdout(t *testing.T) {
	// Should not panic with nil writer.
	err := Run(Config{
		Version:    "test",
		DataDir:    t.TempDir(),
		JSONOutput: false,
		Writer:     nil,
	})
	if err != nil {
		t.Fatalf("Run with nil writer: %v", err)
	}
}

func TestCheckDataDir_EmptyPath(t *testing.T) {
	result := checkDataDir("")
	if result.Status != StatusWarn {
		t.Errorf("expected WARN for empty data dir, got %s", result.Status)
	}
}

func TestCheckDBFile_EmptyPath(t *testing.T) {
	result := checkDBFile("")
	if result.Status != StatusWarn {
		t.Errorf("expected WARN for empty DB path, got %s", result.Status)
	}
}

func TestCheckDBFile_NonexistentFile(t *testing.T) {
	result := checkDBFile("/nonexistent/file.db")
	if result.Status != StatusFail {
		t.Errorf("expected FAIL for nonexistent DB file, got %s", result.Status)
	}
}

package debug

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// CheckStatus represents the result of a diagnostic check.
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult holds one diagnostic check outcome.
type CheckResult struct {
	Status  CheckStatus `json:"status"`
	Message string      `json:"message"`
}

// InterfaceInfo holds runtime WireGuard interface info for diagnostics.
type InterfaceInfo struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Address     string `json:"address"`
	ListenPort  int    `json:"listen_port"`
	PeerCount   int    `json:"peer_count"`
	PeersOnline int    `json:"peers_online"`
	TransferRx  int64  `json:"transfer_rx"`
	TransferTx  int64  `json:"transfer_tx"`
}

// DBStats holds database statistics for diagnostics.
type DBStats struct {
	Path          string         `json:"path"`
	Accessible    bool           `json:"accessible"`
	SchemaVersion int            `json:"schema_version"`
	Tables        map[string]int `json:"tables"`
}

// DiagnoseResult holds the complete diagnostic report.
type DiagnoseResult struct {
	Version    string          `json:"version"`
	GoVersion  string          `json:"go_version"`
	OS         string          `json:"os"`
	Arch       string          `json:"arch"`
	Kernel     string          `json:"kernel"`
	Checks     []CheckResult   `json:"checks"`
	Interfaces []InterfaceInfo `json:"interfaces"`
	DBStats    DBStats         `json:"database"`
}

// Config holds dependencies for the diagnose command.
type Config struct {
	Version    string
	DataDir    string
	DBPath     string
	JSONOutput bool
	Writer     io.Writer
}

// Run executes all diagnostic checks and writes output to the configured writer.
func Run(cfg Config) error {
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}

	result := DiagnoseResult{
		Version:   cfg.Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Kernel:    DetectKernelVersion(),
	}

	result.Checks = runChecks(cfg)
	result.DBStats = checkDatabase(cfg.DBPath)

	if cfg.JSONOutput {
		enc := json.NewEncoder(cfg.Writer)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	return writeTextReport(cfg.Writer, result)
}

func runChecks(cfg Config) []CheckResult {
	var checks []CheckResult

	checks = append(checks, checkWireGuardModule())
	checks = append(checks, checkIPForwardV4())
	checks = append(checks, checkIPForwardV6())
	checks = append(checks, checkNFTables())
	checks = append(checks, checkCapabilities()...)
	checks = append(checks, checkDataDir(cfg.DataDir))
	checks = append(checks, checkDBFile(cfg.DBPath))

	return checks
}

func checkWireGuardModule() CheckResult {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		// If we can't read /proc/modules, try to check if kernel >= 5.6 (built-in).
		kver := DetectKernelVersion()
		if kver != "unknown" {
			return CheckResult{StatusWarn, fmt.Sprintf("Cannot read /proc/modules, kernel: %s", kver)}
		}
		return CheckResult{StatusWarn, "Cannot determine WireGuard module status"}
	}
	if strings.Contains(string(data), "wireguard") {
		return CheckResult{StatusPass, "WireGuard kernel module loaded"}
	}
	// Check kernel version for built-in (5.6+).
	kver := DetectKernelVersion()
	parts := strings.SplitN(kver, ".", 3)
	if len(parts) >= 2 {
		major := 0
		minor := 0
		fmt.Sscanf(parts[0], "%d", &major)
		fmt.Sscanf(parts[1], "%d", &minor)
		if major > 5 || (major == 5 && minor >= 6) {
			return CheckResult{StatusPass, fmt.Sprintf("WireGuard built into kernel %s", kver)}
		}
	}
	return CheckResult{StatusFail, "WireGuard kernel module not loaded"}
}

func checkIPForwardV4() CheckResult {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return CheckResult{StatusWarn, "Cannot read IPv4 forwarding status"}
	}
	if strings.TrimSpace(string(data)) == "1" {
		return CheckResult{StatusPass, "IP forwarding enabled (v4)"}
	}
	return CheckResult{StatusFail, "IP forwarding disabled (v4)"}
}

func checkIPForwardV6() CheckResult {
	data, err := os.ReadFile("/proc/sys/net/ipv6/conf/all/forwarding")
	if err != nil {
		return CheckResult{StatusWarn, "Cannot read IPv6 forwarding status"}
	}
	if strings.TrimSpace(string(data)) == "1" {
		return CheckResult{StatusPass, "IP forwarding enabled (v6)"}
	}
	return CheckResult{StatusWarn, "IP forwarding disabled (v6)"}
}

func checkNFTables() CheckResult {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return CheckResult{StatusWarn, "Cannot determine nftables status"}
	}
	if strings.Contains(string(data), "nf_tables") {
		return CheckResult{StatusPass, "nftables available"}
	}
	return CheckResult{StatusWarn, "nftables module not detected"}
}

func checkCapabilities() []CheckResult {
	var results []CheckResult

	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		results = append(results, CheckResult{StatusWarn, "Cannot read process capabilities"})
		return results
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			hexVal := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
			var caps uint64
			fmt.Sscanf(hexVal, "%x", &caps)

			// CAP_NET_ADMIN = 12, CAP_NET_BIND_SERVICE = 10
			if caps&(1<<12) != 0 {
				results = append(results, CheckResult{StatusPass, "CAP_NET_ADMIN capability"})
			} else {
				results = append(results, CheckResult{StatusFail, "CAP_NET_ADMIN capability missing"})
			}
			if caps&(1<<10) != 0 {
				results = append(results, CheckResult{StatusPass, "CAP_NET_BIND_SERVICE capability"})
			} else {
				results = append(results, CheckResult{StatusWarn, "CAP_NET_BIND_SERVICE capability missing"})
			}
			return results
		}
	}

	results = append(results, CheckResult{StatusWarn, "Cannot parse process capabilities"})
	return results
}

func checkDataDir(dir string) CheckResult {
	if dir == "" {
		return CheckResult{StatusWarn, "Data directory not configured"}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return CheckResult{StatusFail, fmt.Sprintf("Data directory %s does not exist", dir)}
	}
	if !info.IsDir() {
		return CheckResult{StatusFail, fmt.Sprintf("%s is not a directory", dir)}
	}

	// Test write access.
	testFile := dir + "/.wgpilot_diag_test"
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return CheckResult{StatusFail, fmt.Sprintf("Data directory %s is not writable", dir)}
	}
	os.Remove(testFile)

	return CheckResult{StatusPass, fmt.Sprintf("Data directory %s exists and writable", dir)}
}

func checkDBFile(path string) CheckResult {
	if path == "" {
		return CheckResult{StatusWarn, "Database path not configured"}
	}
	if _, err := os.Stat(path); err != nil {
		return CheckResult{StatusFail, fmt.Sprintf("Database %s not accessible", path)}
	}
	return CheckResult{StatusPass, fmt.Sprintf("Database %s accessible", path)}
}

func checkDatabase(path string) DBStats {
	stats := DBStats{
		Path:   path,
		Tables: make(map[string]int),
	}

	if path == "" {
		return stats
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return stats
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats.Accessible = true

	// Schema version.
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM _migrations").Scan(&count); err == nil {
		stats.SchemaVersion = count
	}

	// Table row counts.
	tables := []string{"networks", "peers", "peer_snapshots", "settings", "users", "audit_log", "alerts"}
	for _, table := range tables {
		var n int
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err == nil {
			stats.Tables[table] = n
		}
	}

	return stats
}

// DetectKernelVersion returns the running kernel version string.
func DetectKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return fields[2]
	}
	return "unknown"
}

func writeTextReport(w io.Writer, r DiagnoseResult) error {
	fmt.Fprintf(w, "\nwgpilot diagnostic report\n")
	fmt.Fprintf(w, "========================\n")
	fmt.Fprintf(w, "Version:     %s\n", r.Version)
	fmt.Fprintf(w, "Go:          %s\n", r.GoVersion)
	fmt.Fprintf(w, "OS:          %s/%s\n", r.OS, r.Arch)
	fmt.Fprintf(w, "Kernel:      %s\n", r.Kernel)
	fmt.Fprintf(w, "\n")

	for _, c := range r.Checks {
		fmt.Fprintf(w, "[%s] %s\n", c.Status, c.Message)
	}
	fmt.Fprintf(w, "\n")

	for _, iface := range r.Interfaces {
		fmt.Fprintf(w, "Interface %s:\n", iface.Name)
		fmt.Fprintf(w, "  State:       %s\n", iface.State)
		if iface.Address != "" {
			fmt.Fprintf(w, "  Address:     %s\n", iface.Address)
		}
		fmt.Fprintf(w, "  Listen port: %d\n", iface.ListenPort)
		fmt.Fprintf(w, "  Peers:       %d configured, %d online\n", iface.PeerCount, iface.PeersOnline)
		fmt.Fprintf(w, "\n")
	}

	if r.DBStats.Accessible {
		fmt.Fprintf(w, "Database stats:\n")
		fmt.Fprintf(w, "  Path:            %s\n", r.DBStats.Path)
		fmt.Fprintf(w, "  Schema version:  %d\n", r.DBStats.SchemaVersion)
		for table, count := range r.DBStats.Tables {
			fmt.Fprintf(w, "  %-16s %d rows\n", table+":", count)
		}
	}

	return nil
}

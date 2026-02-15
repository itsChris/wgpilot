package sdnotify

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReady_NoSocket(t *testing.T) {
	os.Unsetenv("NOTIFY_SOCKET")
	if err := Ready(); err != nil {
		t.Errorf("Ready() without NOTIFY_SOCKET should succeed, got %v", err)
	}
}

func TestReady_WithSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "notify.sock")

	// Create a Unix datagram socket to receive the notification.
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: sockPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	t.Setenv("NOTIFY_SOCKET", sockPath)

	if err := Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	buf := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	got := string(buf[:n])
	if got != "READY=1" {
		t.Errorf("expected READY=1, got %q", got)
	}
}

func TestStopping_WithSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "notify.sock")

	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: sockPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	t.Setenv("NOTIFY_SOCKET", sockPath)

	if err := Stopping(); err != nil {
		t.Fatalf("Stopping: %v", err)
	}

	buf := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	got := string(buf[:n])
	if got != "STOPPING=1" {
		t.Errorf("expected STOPPING=1, got %q", got)
	}
}

func TestWatchdogInterval_NotSet(t *testing.T) {
	os.Unsetenv("WATCHDOG_USEC")
	if interval := WatchdogInterval(); interval != 0 {
		t.Errorf("expected 0, got %v", interval)
	}
}

func TestWatchdogInterval_Set(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "10000000") // 10 seconds
	interval := WatchdogInterval()
	expected := 5 * time.Second // half of 10s
	if interval != expected {
		t.Errorf("expected %v, got %v", expected, interval)
	}
}

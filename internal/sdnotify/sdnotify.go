package sdnotify

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Ready sends READY=1 to the systemd notification socket,
// indicating the service has finished starting. Returns nil
// if NOTIFY_SOCKET is not set (non-systemd environments).
func Ready() error {
	return send("READY=1")
}

// Stopping sends STOPPING=1 to indicate graceful shutdown has begun.
func Stopping() error {
	return send("STOPPING=1")
}

// Reloading sends RELOADING=1 to indicate config reload is in progress.
func Reloading() error {
	return send("RELOADING=1")
}

// WatchdogInterval returns the watchdog interval configured by systemd.
// Returns 0 if watchdog is not enabled.
func WatchdogInterval() time.Duration {
	usecStr := os.Getenv("WATCHDOG_USEC")
	if usecStr == "" {
		return 0
	}
	usec, err := strconv.ParseInt(usecStr, 10, 64)
	if err != nil {
		return 0
	}
	// Send heartbeat at half the watchdog interval per systemd recommendation.
	return time.Duration(usec) * time.Microsecond / 2
}

// Watchdog sends WATCHDOG=1 to reset the watchdog timer.
func Watchdog() error {
	return send("WATCHDOG=1")
}

func send(state string) error {
	socketPath := os.Getenv("NOTIFY_SOCKET")
	if socketPath == "" {
		return nil
	}

	conn, err := net.Dial("unixgram", socketPath)
	if err != nil {
		return fmt.Errorf("sdnotify: dial %s: %w", socketPath, err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(state)); err != nil {
		return fmt.Errorf("sdnotify: write %q: %w", state, err)
	}
	return nil
}

package server

import (
	"net/http"
	"runtime"
	"time"

	"github.com/itsChris/wgpilot/internal/debug"
)

// handleSystemInfo returns system information for the admin dashboard.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(s.startTime)

	resp := map[string]any{
		"version":    s.version,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"kernel":     debug.DetectKernelVersion(),
		"uptime":     formatDuration(uptime),
		"uptime_sec": int64(uptime.Seconds()),
		"memory": map[string]any{
			"alloc_mb":       memStats.Alloc / 1024 / 1024,
			"sys_mb":         memStats.Sys / 1024 / 1024,
			"num_goroutines": runtime.NumGoroutine(),
			"gc_cycles":      memStats.NumGC,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

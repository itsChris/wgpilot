package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"time"

	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// handleDebugInfo returns a full diagnostic JSON snapshot.
// Admin-only, dev mode only.
func (s *Server) handleDebugInfo(w http.ResponseWriter, r *http.Request) {
	if !s.devMode {
		writeError(w, r, fmt.Errorf("debug endpoints are disabled in production mode"),
			apperr.ErrInternal, http.StatusNotFound, s.devMode)
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	info := map[string]any{
		"version":        s.version,
		"go_version":     runtime.Version(),
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"uptime_seconds": int64(time.Since(s.startTime).Seconds()),
		"config": map[string]any{
			"dev_mode": s.devMode,
		},
		"system": map[string]any{
			"memory_mb":  mem.Alloc / 1024 / 1024,
			"goroutines": runtime.NumGoroutine(),
			"cpu_count":  runtime.NumCPU(),
		},
	}

	// WireGuard state if manager is available.
	if s.wgManager != nil {
		networks, err := s.db.ListNetworks(r.Context())
		if err == nil {
			var wgIfaces []map[string]any
			for _, net := range networks {
				ifaceInfo := map[string]any{
					"name":        net.Interface,
					"listen_port": net.ListenPort,
					"enabled":     net.Enabled,
				}
				if net.Enabled {
					statuses, err := s.wgManager.PeerStatus(net.Interface)
					if err == nil {
						online := 0
						for _, st := range statuses {
							if st.Online {
								online++
							}
						}
						ifaceInfo["peer_count"] = len(statuses)
						ifaceInfo["peers_online"] = online
						ifaceInfo["state"] = "up"
					} else {
						ifaceInfo["state"] = "error"
						ifaceInfo["error"] = err.Error()
					}
				} else {
					ifaceInfo["state"] = "down"
				}
				wgIfaces = append(wgIfaces, ifaceInfo)
			}
			info["wireguard"] = map[string]any{
				"interfaces": wgIfaces,
			}
		}
	}

	// Database stats.
	dbInfo := map[string]any{}
	ctx := r.Context()
	for _, table := range []string{"networks", "peers", "peer_snapshots", "settings"} {
		var count int64
		row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table)
		if err := row.Scan(&count); err == nil {
			dbInfo[table] = count
		}
	}
	info["database"] = map[string]any{
		"tables": dbInfo,
	}

	writeJSON(w, http.StatusOK, info)
}

// handleDebugLogs returns recent error/warning entries from the ring buffer.
// Admin-only.
func (s *Server) handleDebugLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	levelFilter := r.URL.Query().Get("level")

	var entries []map[string]any
	if s.ring != nil {
		recent := s.ring.Recent(limit)
		for _, e := range recent {
			if levelFilter != "" {
				switch levelFilter {
				case "error":
					if e.Level < slog.LevelError {
						continue
					}
				case "warn":
					if e.Level < slog.LevelWarn {
						continue
					}
				}
			}

			entry := map[string]any{
				"timestamp": e.Timestamp.Unix(),
				"level":     e.Level.String(),
				"message":   e.Message,
			}
			for k, v := range e.Attrs {
				entry[k] = v
			}
			entries = append(entries, entry)
		}
	}

	if entries == nil {
		entries = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

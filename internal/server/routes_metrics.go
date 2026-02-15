package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleMetrics returns Prometheus exposition format metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	networks, err := s.db.ListNetworks(r.Context())
	if err != nil {
		http.Error(w, "# error listing networks", http.StatusInternalServerError)
		return
	}

	b.WriteString("# HELP wg_peers_total Total number of configured peers per network.\n")
	b.WriteString("# TYPE wg_peers_total gauge\n")

	b.WriteString("# HELP wg_peers_online Number of online peers per network.\n")
	b.WriteString("# TYPE wg_peers_online gauge\n")

	b.WriteString("# HELP wg_transfer_bytes_total Total transfer bytes per network and direction.\n")
	b.WriteString("# TYPE wg_transfer_bytes_total counter\n")

	b.WriteString("# HELP wg_peer_last_handshake_seconds Seconds since last handshake per peer.\n")
	b.WriteString("# TYPE wg_peer_last_handshake_seconds gauge\n")

	b.WriteString("# HELP wg_interface_up Whether the WireGuard interface is up.\n")
	b.WriteString("# TYPE wg_interface_up gauge\n")

	now := time.Now()

	for _, net := range networks {
		iface := net.Interface

		if !net.Enabled || s.wgManager == nil {
			fmt.Fprintf(&b, "wg_interface_up{network=%q} 0\n", iface)
			continue
		}

		statuses, err := s.wgManager.PeerStatus(iface)
		if err != nil {
			fmt.Fprintf(&b, "wg_interface_up{network=%q} 0\n", iface)
			continue
		}

		// Look up peer names from DB.
		peers, _ := s.db.ListPeersByNetworkID(r.Context(), net.ID)
		nameByKey := make(map[string]string, len(peers))
		for _, p := range peers {
			nameByKey[p.PublicKey] = p.Name
		}

		fmt.Fprintf(&b, "wg_interface_up{network=%q} 1\n", iface)
		fmt.Fprintf(&b, "wg_peers_total{network=%q} %d\n", iface, len(statuses))

		var online int
		var totalRx, totalTx int64
		for _, st := range statuses {
			if st.Online {
				online++
			}
			totalRx += st.TransferRx
			totalTx += st.TransferTx

			peerLabel := nameByKey[st.PublicKey]
			if peerLabel == "" {
				peerLabel = st.PublicKey[:8] + "..."
			}

			if !st.LastHandshake.IsZero() {
				seconds := now.Sub(st.LastHandshake).Seconds()
				fmt.Fprintf(&b, "wg_peer_last_handshake_seconds{network=%q,peer=%q} %.0f\n",
					iface, peerLabel, seconds)
			}
		}

		fmt.Fprintf(&b, "wg_peers_online{network=%q} %d\n", iface, online)
		fmt.Fprintf(&b, "wg_transfer_bytes_total{network=%q,direction=\"rx\"} %d\n", iface, totalRx)
		fmt.Fprintf(&b, "wg_transfer_bytes_total{network=%q,direction=\"tx\"} %d\n", iface, totalTx)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, b.String())
}

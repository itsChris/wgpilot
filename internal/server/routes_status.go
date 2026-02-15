package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// statusResponse is the JSON shape for GET /api/status.
type statusResponse struct {
	Networks []networkStatus `json:"networks"`
}

type networkStatus struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Interface string       `json:"interface"`
	Enabled   bool         `json:"enabled"`
	Up        bool         `json:"up"`
	ListenPort int         `json:"listen_port"`
	Peers     []peerStatus `json:"peers"`
}

type peerStatus struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	PublicKey     string `json:"public_key"`
	Endpoint      string `json:"endpoint"`
	LastHandshake int64  `json:"last_handshake"`
	TransferRx    int64  `json:"transfer_rx"`
	TransferTx    int64  `json:"transfer_tx"`
	Online        bool   `json:"online"`
}

// handleStatus returns live interface stats from the kernel.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	networks, err := s.db.ListNetworks(r.Context())
	if err != nil {
		s.logger.Error("status_list_networks_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "status",
			"component", "http",
		)
		writeError(w, r, fmt.Errorf("list networks: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	resp := statusResponse{Networks: make([]networkStatus, 0, len(networks))}

	for _, net := range networks {
		ns := networkStatus{
			ID:         net.ID,
			Name:       net.Name,
			Interface:  net.Interface,
			Enabled:    net.Enabled,
			ListenPort: net.ListenPort,
		}

		if !net.Enabled || s.wgManager == nil {
			resp.Networks = append(resp.Networks, ns)
			continue
		}

		statuses, err := s.wgManager.PeerStatus(net.Interface)
		if err != nil {
			s.logger.Warn("status_peer_status_failed",
				"error", err,
				"interface", net.Interface,
				"operation", "status",
				"component", "http",
			)
			resp.Networks = append(resp.Networks, ns)
			continue
		}

		ns.Up = true

		// Map public keys to peer info for names and IDs.
		peers, err := s.db.ListPeersByNetworkID(r.Context(), net.ID)
		if err != nil {
			s.logger.Warn("status_list_peers_failed",
				"error", err,
				"network_id", net.ID,
				"operation", "status",
				"component", "http",
			)
			resp.Networks = append(resp.Networks, ns)
			continue
		}

		type peerInfo struct {
			id   int64
			name string
		}
		peerByKey := make(map[string]peerInfo, len(peers))
		for _, p := range peers {
			peerByKey[p.PublicKey] = peerInfo{id: p.ID, name: p.Name}
		}

		ns.Peers = make([]peerStatus, 0, len(statuses))
		for _, st := range statuses {
			ps := peerStatus{
				PublicKey:     st.PublicKey,
				Endpoint:      st.Endpoint,
				LastHandshake: st.LastHandshake.Unix(),
				TransferRx:    st.TransferRx,
				TransferTx:    st.TransferTx,
				Online:        st.Online,
			}
			if info, ok := peerByKey[st.PublicKey]; ok {
				ps.ID = info.id
				ps.Name = info.name
			}
			ns.Peers = append(ns.Peers, ps)
		}

		resp.Networks = append(resp.Networks, ns)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleSSEEvents streams peer status updates via Server-Sent Events.
func (s *Server) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	networkID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID: %w", err), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(r.Context(), networkID)
	if err != nil {
		writeError(w, r, fmt.Errorf("get network: %w", err), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send initial status immediately.
	s.sendSSEStatus(w, rc, network.Interface, networkID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.sendSSEStatus(w, rc, network.Interface, networkID)
		}
	}
}

func (s *Server) sendSSEStatus(w http.ResponseWriter, rc *http.ResponseController, iface string, networkID int64) {
	if s.wgManager == nil {
		return
	}

	statuses, err := s.wgManager.PeerStatus(iface)
	if err != nil {
		s.logger.Debug("sse_peer_status_failed",
			"error", err,
			"interface", iface,
			"component", "http",
		)
		return
	}

	events := make([]map[string]any, 0, len(statuses))
	for _, st := range statuses {
		events = append(events, map[string]any{
			"peer_id":        0, // will be enriched by frontend via cache
			"online":         st.Online,
			"last_handshake": st.LastHandshake.Unix(),
			"transfer_rx":    st.TransferRx,
			"transfer_tx":    st.TransferTx,
			"public_key":     st.PublicKey,
		})
	}

	data, err := json.Marshal(events)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
	rc.Flush()
}

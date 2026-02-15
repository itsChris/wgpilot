package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
	"github.com/itsChris/wgpilot/internal/wg"
)

// ── Request/Response types ───────────────────────────────────────────

type createPeerRequest struct {
	Name                string `json:"name"`
	Email               string `json:"email"`
	Role                string `json:"role"`
	PersistentKeepalive int    `json:"persistent_keepalive"`
	SiteNetworks        string `json:"site_networks"`
}

type updatePeerRequest struct {
	Name                *string `json:"name"`
	Email               *string `json:"email"`
	Enabled             *bool   `json:"enabled"`
	PersistentKeepalive *int    `json:"persistent_keepalive"`
	Endpoint            *string `json:"endpoint"`
}

type peerResponse struct {
	ID                  int64  `json:"id"`
	NetworkID           int64  `json:"network_id"`
	Name                string `json:"name"`
	Email               string `json:"email"`
	PublicKey           string `json:"public_key"`
	AllowedIPs          string `json:"allowed_ips"`
	Endpoint            string `json:"endpoint"`
	PersistentKeepalive int    `json:"persistent_keepalive"`
	Role                string `json:"role"`
	SiteNetworks        string `json:"site_networks"`
	Enabled             bool   `json:"enabled"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// ── Validation ───────────────────────────────────────────────────────

func isValidRole(role string) bool {
	return role == "client" || role == "site-gateway"
}

func isValidEndpoint(ep string) bool {
	if ep == "" {
		return true
	}
	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return false
	}
	return true
}

func isValidSiteNetworks(s string) bool {
	if s == "" {
		return true
	}
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		_, _, err := net.ParseCIDR(p)
		if err != nil {
			return false
		}
	}
	return true
}

func (s *Server) validateCreatePeer(req createPeerRequest) []fieldError {
	var errs []fieldError
	if !isValidName(req.Name) {
		errs = append(errs, fieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
	}
	if !isValidRole(req.Role) {
		errs = append(errs, fieldError{"role", "must be client or site-gateway"})
	}
	if req.PersistentKeepalive < 0 || req.PersistentKeepalive > 65535 {
		errs = append(errs, fieldError{"persistent_keepalive", "must be between 0 and 65535"})
	}
	if req.Role == "site-gateway" && !isValidSiteNetworks(req.SiteNetworks) {
		errs = append(errs, fieldError{"site_networks", "must be valid CIDRs, comma-separated"})
	}
	return errs
}

func (s *Server) validateUpdatePeer(req updatePeerRequest) []fieldError {
	var errs []fieldError
	if req.Name != nil && !isValidName(*req.Name) {
		errs = append(errs, fieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
	}
	if req.PersistentKeepalive != nil && (*req.PersistentKeepalive < 0 || *req.PersistentKeepalive > 65535) {
		errs = append(errs, fieldError{"persistent_keepalive", "must be between 0 and 65535"})
	}
	if req.Endpoint != nil && !isValidEndpoint(*req.Endpoint) {
		errs = append(errs, fieldError{"endpoint", "must be a valid host:port"})
	}
	return errs
}

// ── Handlers ─────────────────────────────────────────────────────────

// handleCreatePeer creates a new peer in a network.
func (s *Server) handleCreatePeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	var req createPeerRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if errs := s.validateCreatePeer(req); len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Verify network exists.
	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil {
		s.logger.Error("get_network_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_peer",
			"component", "handler",
			"network_id", networkID,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Allocate IP from subnet.
	_, subnet, _ := net.ParseCIDR(network.Subnet) // already validated on creation
	existingPeers, err := s.db.ListPeersByNetworkID(ctx, networkID)
	if err != nil {
		s.logger.Error("list_peers_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_peer",
			"component", "handler",
			"network_id", networkID,
		)
		writeError(w, r, fmt.Errorf("failed to list peers"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	var usedIPs []net.IP
	for _, p := range existingPeers {
		parts := strings.Split(p.AllowedIPs, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			ip, _, parseErr := net.ParseCIDR(part)
			if parseErr == nil {
				usedIPs = append(usedIPs, ip)
			}
		}
	}

	alloc, err := wg.NewIPAllocator(subnet, usedIPs)
	if err != nil {
		s.logger.Error("ip_allocator_failed",
			"error", err,
			"operation", "create_peer",
			"component", "handler",
			"network_id", networkID,
		)
		writeError(w, r, fmt.Errorf("failed to create IP allocator"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	allocatedIP, err := alloc.Allocate()
	if err != nil {
		s.logger.Warn("ip_exhausted",
			"error", err,
			"operation", "create_peer",
			"component", "handler",
			"network_id", networkID,
			"subnet", network.Subnet,
		)
		writeError(w, r, fmt.Errorf("no available IPs in subnet %s", network.Subnet), apperr.ErrIPExhausted, http.StatusConflict, s.devMode)
		return
	}

	// Generate peer keypair and preshared key.
	peerPrivKey, peerPubKey, err := wg.GenerateKeyPair()
	if err != nil {
		s.logger.Error("generate_peer_keypair_failed",
			"error", err,
			"operation", "create_peer",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to generate peer keypair"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	presharedKey, err := wg.GeneratePresharedKey()
	if err != nil {
		s.logger.Error("generate_preshared_key_failed",
			"error", err,
			"operation", "create_peer",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to generate preshared key"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Compute server-side AllowedIPs for this peer.
	serverAllowedIPs := allocatedIP.String() + "/32"
	if req.Role == "site-gateway" && req.SiteNetworks != "" {
		serverAllowedIPs = serverAllowedIPs + ", " + req.SiteNetworks
	}

	peer := &db.Peer{
		NetworkID:           networkID,
		Name:                req.Name,
		Email:               req.Email,
		PrivateKey:          peerPrivKey,
		PublicKey:           peerPubKey,
		PresharedKey:        presharedKey,
		AllowedIPs:          serverAllowedIPs,
		PersistentKeepalive: req.PersistentKeepalive,
		Role:                req.Role,
		SiteNetworks:        req.SiteNetworks,
		Enabled:             true,
	}

	// Add peer to WireGuard interface.
	if s.wgManager != nil {
		peerCfg := wg.PeerConfig{
			Name:                req.Name,
			PublicKey:           peerPubKey,
			PresharedKey:        presharedKey,
			AllowedIPs:          serverAllowedIPs,
			PersistentKeepalive: req.PersistentKeepalive,
		}
		if err := s.wgManager.AddPeer(ctx, network.Interface, peerCfg); err != nil {
			s.logger.Error("add_peer_wg_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "create_peer",
				"component", "handler",
				"network_id", networkID,
				"peer_name", req.Name,
			)
			writeError(w, r, fmt.Errorf("failed to add peer to WireGuard"), apperr.ErrPeerAddFailed, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Persist to database.
	peerID, err := s.db.CreatePeer(ctx, peer)
	if err != nil {
		s.logger.Error("create_peer_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_peer",
			"component", "handler",
			"network_id", networkID,
		)
		// Clean up WG peer on DB failure.
		if s.wgManager != nil {
			s.wgManager.RemovePeer(ctx, network.Interface, peerPubKey)
		}
		writeError(w, r, fmt.Errorf("failed to create peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Fetch created peer for timestamps.
	created, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil || created == nil {
		s.logger.Error("get_created_peer_failed",
			"error", err,
			"operation", "create_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to retrieve created peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("peer_created",
		"peer_id", peerID,
		"peer_name", created.Name,
		"network_id", networkID,
		"public_key", created.PublicKey,
		"allowed_ips", created.AllowedIPs,
		"component", "handler",
	)

	writeJSON(w, http.StatusCreated, peerToResponse(created))
}

// handleListPeers lists all peers for a network.
func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Verify network exists.
	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil {
		s.logger.Error("get_network_failed",
			"error", err,
			"operation", "list_peers",
			"component", "handler",
			"network_id", networkID,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	peers, err := s.db.ListPeersByNetworkID(ctx, networkID)
	if err != nil {
		s.logger.Error("list_peers_failed",
			"error", err,
			"operation", "list_peers",
			"component", "handler",
			"network_id", networkID,
		)
		writeError(w, r, fmt.Errorf("failed to list peers"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	result := make([]peerResponse, 0, len(peers))
	for _, p := range peers {
		result = append(result, peerToResponse(&p))
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetPeer returns a single peer.
func (s *Server) handleGetPeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peerID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid peer ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peer, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil {
		s.logger.Error("get_peer_failed",
			"error", err,
			"operation", "get_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to get peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if peer == nil || peer.NetworkID != networkID {
		writeError(w, r, fmt.Errorf("peer %d not found in network %d", peerID, networkID), apperr.ErrPeerNotFound, http.StatusNotFound, s.devMode)
		return
	}

	writeJSON(w, http.StatusOK, peerToResponse(peer))
}

// handleUpdatePeer updates a peer's mutable fields.
func (s *Server) handleUpdatePeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peerID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid peer ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	var req updatePeerRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if errs := s.validateUpdatePeer(req); len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	peer, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil {
		s.logger.Error("get_peer_failed",
			"error", err,
			"operation", "update_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to get peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if peer == nil || peer.NetworkID != networkID {
		writeError(w, r, fmt.Errorf("peer %d not found in network %d", peerID, networkID), apperr.ErrPeerNotFound, http.StatusNotFound, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil || network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Apply updates.
	if req.Name != nil {
		peer.Name = *req.Name
	}
	if req.Email != nil {
		peer.Email = *req.Email
	}
	if req.Enabled != nil {
		peer.Enabled = *req.Enabled
	}
	if req.PersistentKeepalive != nil {
		peer.PersistentKeepalive = *req.PersistentKeepalive
	}
	if req.Endpoint != nil {
		peer.Endpoint = *req.Endpoint
	}

	// Update WireGuard peer if manager is available.
	if s.wgManager != nil {
		peerCfg := wg.PeerConfig{
			Name:                peer.Name,
			PublicKey:           peer.PublicKey,
			PresharedKey:        peer.PresharedKey,
			AllowedIPs:          peer.AllowedIPs,
			Endpoint:            peer.Endpoint,
			PersistentKeepalive: peer.PersistentKeepalive,
			Enabled:             peer.Enabled,
		}
		if err := s.wgManager.UpdatePeer(ctx, network.Interface, peerCfg); err != nil {
			s.logger.Error("update_peer_wg_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "update_peer",
				"component", "handler",
				"peer_id", peerID,
				"network_id", networkID,
			)
			writeError(w, r, fmt.Errorf("failed to update peer in WireGuard"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	if err := s.db.UpdatePeer(ctx, peer); err != nil {
		s.logger.Error("update_peer_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "update_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to update peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Re-fetch for updated timestamps.
	updated, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil || updated == nil {
		writeError(w, r, fmt.Errorf("failed to retrieve updated peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("peer_updated",
		"peer_id", peerID,
		"peer_name", updated.Name,
		"network_id", networkID,
		"component", "handler",
	)

	writeJSON(w, http.StatusOK, peerToResponse(updated))
}

// handleDeletePeer removes a peer from a network.
func (s *Server) handleDeletePeer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peerID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid peer ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peer, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil {
		s.logger.Error("get_peer_failed",
			"error", err,
			"operation", "delete_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to get peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if peer == nil || peer.NetworkID != networkID {
		writeError(w, r, fmt.Errorf("peer %d not found in network %d", peerID, networkID), apperr.ErrPeerNotFound, http.StatusNotFound, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil || network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Remove from WireGuard.
	if s.wgManager != nil {
		if err := s.wgManager.RemovePeer(ctx, network.Interface, peer.PublicKey); err != nil {
			s.logger.Error("remove_peer_wg_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "delete_peer",
				"component", "handler",
				"peer_id", peerID,
				"network_id", networkID,
			)
			// Continue with DB deletion even if WG removal fails.
		}
	}

	if err := s.db.DeletePeer(ctx, peerID); err != nil {
		s.logger.Error("delete_peer_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_peer",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to delete peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("peer_deleted",
		"peer_id", peerID,
		"peer_name", peer.Name,
		"network_id", networkID,
		"public_key", peer.PublicKey,
		"component", "handler",
	)

	w.WriteHeader(http.StatusNoContent)
}

// handlePeerConfig returns the WireGuard .conf file for a peer.
func (s *Server) handlePeerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peerID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid peer ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peer, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil {
		s.logger.Error("get_peer_failed",
			"error", err,
			"operation", "peer_config",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to get peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if peer == nil || peer.NetworkID != networkID {
		writeError(w, r, fmt.Errorf("peer %d not found in network %d", peerID, networkID), apperr.ErrPeerNotFound, http.StatusNotFound, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil || network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Build server endpoint.
	publicIP, err := s.db.GetSetting(ctx, "public_ip")
	if err != nil {
		s.logger.Error("get_public_ip_failed",
			"error", err,
			"operation", "peer_config",
			"component", "handler",
		)
	}
	if publicIP == "" {
		publicIP = "YOUR_SERVER_IP"
	}
	serverEndpoint := fmt.Sprintf("%s:%d", publicIP, network.ListenPort)

	// Compute client-side AllowedIPs based on mode.
	clientAllowedIPs := wg.ComputeClientAllowedIPs(network.Mode, network.Subnet, peer.SiteNetworks)

	conf, err := wg.GenerateClientConfig(wg.ClientConfigParams{
		PeerName:            peer.Name,
		PeerPrivateKey:      peer.PrivateKey,
		PeerAddress:         peer.AllowedIPs,
		DNSServers:          network.DNSServers,
		ServerPublicKey:     network.PublicKey,
		PresharedKey:        peer.PresharedKey,
		ServerEndpoint:      serverEndpoint,
		AllowedIPs:          clientAllowedIPs,
		PersistentKeepalive: peer.PersistentKeepalive,
	})
	if err != nil {
		s.logger.Error("generate_config_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "peer_config",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to generate config"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Sanitize peer name for filename.
	safeName := strings.Map(func(c rune) rune {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			return c
		}
		return '-'
	}, peer.Name)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="wgpilot-%s.conf"`, safeName))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(conf))
}

// handlePeerQR returns a QR code PNG image for a peer's config.
func (s *Server) handlePeerQR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peerID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid peer ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	peer, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil {
		s.logger.Error("get_peer_failed",
			"error", err,
			"operation", "peer_qr",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to get peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if peer == nil || peer.NetworkID != networkID {
		writeError(w, r, fmt.Errorf("peer %d not found in network %d", peerID, networkID), apperr.ErrPeerNotFound, http.StatusNotFound, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil || network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", networkID), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	publicIP, err := s.db.GetSetting(ctx, "public_ip")
	if err != nil {
		s.logger.Error("get_public_ip_failed",
			"error", err,
			"operation", "peer_qr",
			"component", "handler",
		)
	}
	if publicIP == "" {
		publicIP = "YOUR_SERVER_IP"
	}
	serverEndpoint := fmt.Sprintf("%s:%d", publicIP, network.ListenPort)

	clientAllowedIPs := wg.ComputeClientAllowedIPs(network.Mode, network.Subnet, peer.SiteNetworks)

	conf, err := wg.GenerateClientConfig(wg.ClientConfigParams{
		PeerName:            peer.Name,
		PeerPrivateKey:      peer.PrivateKey,
		PeerAddress:         peer.AllowedIPs,
		DNSServers:          network.DNSServers,
		ServerPublicKey:     network.PublicKey,
		PresharedKey:        peer.PresharedKey,
		ServerEndpoint:      serverEndpoint,
		AllowedIPs:          clientAllowedIPs,
		PersistentKeepalive: peer.PersistentKeepalive,
	})
	if err != nil {
		s.logger.Error("generate_config_failed",
			"error", err,
			"operation", "peer_qr",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to generate config"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	png, err := wg.GenerateQRCode(conf, 256)
	if err != nil {
		s.logger.Error("generate_qr_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "peer_qr",
			"component", "handler",
			"peer_id", peerID,
		)
		writeError(w, r, fmt.Errorf("failed to generate QR code"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(png)
}

// ── Helpers ──────────────────────────────────────────────────────────

func peerToResponse(p *db.Peer) peerResponse {
	return peerResponse{
		ID:                  p.ID,
		NetworkID:           p.NetworkID,
		Name:                p.Name,
		Email:               p.Email,
		PublicKey:           p.PublicKey,
		AllowedIPs:          p.AllowedIPs,
		Endpoint:            p.Endpoint,
		PersistentKeepalive: p.PersistentKeepalive,
		Role:                p.Role,
		SiteNetworks:        p.SiteNetworks,
		Enabled:             p.Enabled,
		CreatedAt:           p.CreatedAt.Unix(),
		UpdatedAt:           p.UpdatedAt.Unix(),
	}
}

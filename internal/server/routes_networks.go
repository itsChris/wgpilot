package server

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
	"github.com/itsChris/wgpilot/internal/wg"
)

// ── Request/Response types ───────────────────────────────────────────

type createNetworkRequest struct {
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Subnet           string `json:"subnet"`
	ListenPort       int    `json:"listen_port"`
	DNSServers       string `json:"dns_servers"`
	NATEnabled       bool   `json:"nat_enabled"`
	InterPeerRouting bool   `json:"inter_peer_routing"`
}

type updateNetworkRequest struct {
	Name             *string `json:"name"`
	DNSServers       *string `json:"dns_servers"`
	NATEnabled       *bool   `json:"nat_enabled"`
	InterPeerRouting *bool   `json:"inter_peer_routing"`
}

type networkResponse struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Interface        string `json:"interface"`
	Mode             string `json:"mode"`
	Subnet           string `json:"subnet"`
	ListenPort       int    `json:"listen_port"`
	PublicKey        string `json:"public_key"`
	DNSServers       string `json:"dns_servers"`
	NATEnabled       bool   `json:"nat_enabled"`
	InterPeerRouting bool   `json:"inter_peer_routing"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

type networkListItem struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Interface        string `json:"interface"`
	Mode             string `json:"mode"`
	Subnet           string `json:"subnet"`
	ListenPort       int    `json:"listen_port"`
	PublicKey        string `json:"public_key"`
	DNSServers       string `json:"dns_servers"`
	NATEnabled       bool   `json:"nat_enabled"`
	InterPeerRouting bool   `json:"inter_peer_routing"`
	Enabled          bool   `json:"enabled"`
	PeerCount        int    `json:"peer_count"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

// ── Validation helpers ───────────────────────────────────────────────

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,64}$`)

func isValidName(name string) bool {
	return validNameRe.MatchString(name)
}

func isValidMode(mode string) bool {
	return mode == "gateway" || mode == "site-to-site" || mode == "hub-routed"
}

func isValidPrivateCIDR(cidr string) bool {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 || ones < 16 || ones > 30 {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// Check private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if ip4[0] == 10 {
		return true
	}
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	return false
}

var validHostnameRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func isValidHostname(h string) bool {
	if len(h) > 253 {
		return false
	}
	return validHostnameRe.MatchString(h)
}

func isValidDNSServers(s string) bool {
	if s == "" {
		return true
	}
	parts := strings.Split(s, ",")
	if len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		ip := net.ParseIP(strings.TrimSpace(p))
		if ip == nil {
			return false
		}
	}
	return true
}

func (s *Server) validateCreateNetwork(req createNetworkRequest) []fieldError {
	var errs []fieldError
	if !isValidName(req.Name) {
		errs = append(errs, fieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
	}
	if !isValidMode(req.Mode) {
		errs = append(errs, fieldError{"mode", "must be gateway, site-to-site, or hub-routed"})
	}
	if !isValidPrivateCIDR(req.Subnet) {
		errs = append(errs, fieldError{"subnet", "must be a valid private IPv4 CIDR (/16 to /30)"})
	}
	if req.ListenPort < 1024 || req.ListenPort > 65535 {
		errs = append(errs, fieldError{"listen_port", "must be between 1024 and 65535"})
	}
	if !isValidDNSServers(req.DNSServers) {
		errs = append(errs, fieldError{"dns_servers", "must be up to 3 valid IP addresses, comma-separated"})
	}
	return errs
}

func (s *Server) validateUpdateNetwork(req updateNetworkRequest) []fieldError {
	var errs []fieldError
	if req.Name != nil && !isValidName(*req.Name) {
		errs = append(errs, fieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
	}
	if req.DNSServers != nil && !isValidDNSServers(*req.DNSServers) {
		errs = append(errs, fieldError{"dns_servers", "must be up to 3 valid IP addresses, comma-separated"})
	}
	return errs
}

// nextInterfaceName determines the next available wgN interface name.
func (s *Server) nextInterfaceName(networks []db.Network) string {
	used := make(map[string]bool)
	for _, n := range networks {
		used[n.Interface] = true
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("wg%d", i)
		if !used[name] {
			return name
		}
	}
}

// ── Handlers ─────────────────────────────────────────────────────────

// handleCreateNetwork creates a new WireGuard network.
func (s *Server) handleCreateNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createNetworkRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	// Validate input.
	if errs := s.validateCreateNetwork(req); len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Check subnet conflict.
	networks, err := s.db.ListNetworks(ctx)
	if err != nil {
		s.logger.Error("list_networks_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_network",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to list networks"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	_, reqSubnet, _ := net.ParseCIDR(req.Subnet) // already validated
	for _, existing := range networks {
		_, existingSubnet, _ := net.ParseCIDR(existing.Subnet)
		if existingSubnet != nil && (reqSubnet.Contains(existingSubnet.IP) || existingSubnet.Contains(reqSubnet.IP)) {
			writeError(w, r,
				fmt.Errorf("subnet %s overlaps with network %q (%s)", req.Subnet, existing.Name, existing.Subnet),
				apperr.ErrSubnetConflict, http.StatusConflict, s.devMode)
			return
		}
	}

	// Check port conflict.
	for _, existing := range networks {
		if existing.ListenPort == req.ListenPort {
			writeError(w, r,
				fmt.Errorf("port %d already in use by network %q", req.ListenPort, existing.Name),
				apperr.ErrPortInUse, http.StatusConflict, s.devMode)
			return
		}
	}

	// Generate server keypair.
	privateKey, publicKey, err := wg.GenerateKeyPair()
	if err != nil {
		s.logger.Error("generate_keypair_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_network",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to generate server keypair"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	ifaceName := s.nextInterfaceName(networks)

	network := &db.Network{
		Name:             req.Name,
		Interface:        ifaceName,
		Mode:             req.Mode,
		Subnet:           req.Subnet,
		ListenPort:       req.ListenPort,
		PrivateKey:       privateKey,
		PublicKey:        publicKey,
		DNSServers:       req.DNSServers,
		NATEnabled:       req.NATEnabled,
		InterPeerRouting: req.InterPeerRouting,
		Enabled:          true,
	}

	// Create WireGuard interface.
	if s.wgManager != nil {
		netCfg := wg.NetworkConfig{
			Interface:  ifaceName,
			Subnet:     req.Subnet,
			ListenPort: req.ListenPort,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		}
		if err := s.wgManager.CreateInterface(ctx, netCfg); err != nil {
			s.logger.Error("create_interface_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "create_network",
				"component", "handler",
				"interface", ifaceName,
			)
			writeError(w, r, fmt.Errorf("failed to create WireGuard interface"), apperr.ErrInterfaceCreateFailed, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Apply nftables rules.
	if s.nftManager != nil {
		if req.NATEnabled {
			if err := s.nftManager.AddNATMasquerade(ifaceName, req.Subnet); err != nil {
				s.logger.Error("add_nat_failed",
					"error", err,
					"error_type", fmt.Sprintf("%T", err),
					"operation", "create_network",
					"component", "handler",
					"interface", ifaceName,
				)
				// Clean up WG interface on nftables failure.
				if s.wgManager != nil {
					s.wgManager.DeleteInterface(ctx, ifaceName)
				}
				writeError(w, r, fmt.Errorf("failed to add NAT rules"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
				return
			}
		}
		if req.InterPeerRouting {
			if err := s.nftManager.EnableInterPeerForwarding(ifaceName); err != nil {
				s.logger.Error("enable_forwarding_failed",
					"error", err,
					"error_type", fmt.Sprintf("%T", err),
					"operation", "create_network",
					"component", "handler",
					"interface", ifaceName,
				)
				if s.wgManager != nil {
					s.wgManager.DeleteInterface(ctx, ifaceName)
				}
				writeError(w, r, fmt.Errorf("failed to enable inter-peer forwarding"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
				return
			}
		}
	}

	// Persist to database.
	id, err := s.db.CreateNetwork(ctx, network)
	if err != nil {
		s.logger.Error("create_network_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "create_network",
			"component", "handler",
		)
		// Clean up interface on DB failure.
		if s.wgManager != nil {
			s.wgManager.DeleteInterface(ctx, ifaceName)
		}
		writeError(w, r, fmt.Errorf("failed to create network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Fetch the created network for response (to get timestamps).
	created, err := s.db.GetNetworkByID(ctx, id)
	if err != nil || created == nil {
		s.logger.Error("get_created_network_failed",
			"error", err,
			"operation", "create_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to retrieve created network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("network_created",
		"network_id", id,
		"network_name", created.Name,
		"interface", created.Interface,
		"mode", created.Mode,
		"component", "handler",
	)

	writeJSON(w, http.StatusCreated, networkToResponse(created))
}

// handleListNetworks lists all networks with peer counts.
func (s *Server) handleListNetworks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networks, err := s.db.ListNetworks(ctx)
	if err != nil {
		s.logger.Error("list_networks_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "list_networks",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to list networks"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	result := make([]networkListItem, 0, len(networks))
	for _, n := range networks {
		peers, err := s.db.ListPeersByNetworkID(ctx, n.ID)
		if err != nil {
			s.logger.Error("list_peers_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "list_networks",
				"component", "handler",
				"network_id", n.ID,
			)
			writeError(w, r, fmt.Errorf("failed to list peers"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
		result = append(result, networkListItem{
			ID:               n.ID,
			Name:             n.Name,
			Interface:        n.Interface,
			Mode:             n.Mode,
			Subnet:           n.Subnet,
			ListenPort:       n.ListenPort,
			PublicKey:        n.PublicKey,
			DNSServers:       n.DNSServers,
			NATEnabled:       n.NATEnabled,
			InterPeerRouting: n.InterPeerRouting,
			Enabled:          n.Enabled,
			PeerCount:        len(peers),
			CreatedAt:        n.CreatedAt.Unix(),
			UpdatedAt:        n.UpdatedAt.Unix(),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetNetwork returns a single network.
func (s *Server) handleGetNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, id)
	if err != nil {
		s.logger.Error("get_network_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "get_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", id), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	writeJSON(w, http.StatusOK, networkToResponse(network))
}

// handleUpdateNetwork updates mutable network settings.
func (s *Server) handleUpdateNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	var req updateNetworkRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if errs := s.validateUpdateNetwork(req); len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, id)
	if err != nil {
		s.logger.Error("get_network_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "update_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", id), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Apply updates.
	if req.Name != nil {
		network.Name = *req.Name
	}
	if req.DNSServers != nil {
		network.DNSServers = *req.DNSServers
	}

	// Handle NAT toggle.
	if req.NATEnabled != nil && *req.NATEnabled != network.NATEnabled {
		if s.nftManager != nil {
			if *req.NATEnabled {
				if err := s.nftManager.AddNATMasquerade(network.Interface, network.Subnet); err != nil {
					s.logger.Error("add_nat_failed",
						"error", err,
						"operation", "update_network",
						"component", "handler",
						"network_id", id,
					)
					writeError(w, r, fmt.Errorf("failed to enable NAT"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
					return
				}
			} else {
				if err := s.nftManager.RemoveNATMasquerade(network.Interface); err != nil {
					s.logger.Error("remove_nat_failed",
						"error", err,
						"operation", "update_network",
						"component", "handler",
						"network_id", id,
					)
					writeError(w, r, fmt.Errorf("failed to disable NAT"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
					return
				}
			}
		}
		network.NATEnabled = *req.NATEnabled
	}

	// Handle inter-peer routing toggle.
	if req.InterPeerRouting != nil && *req.InterPeerRouting != network.InterPeerRouting {
		if s.nftManager != nil {
			if *req.InterPeerRouting {
				if err := s.nftManager.EnableInterPeerForwarding(network.Interface); err != nil {
					s.logger.Error("enable_forwarding_failed",
						"error", err,
						"operation", "update_network",
						"component", "handler",
						"network_id", id,
					)
					writeError(w, r, fmt.Errorf("failed to enable inter-peer routing"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
					return
				}
			} else {
				if err := s.nftManager.DisableInterPeerForwarding(network.Interface); err != nil {
					s.logger.Error("disable_forwarding_failed",
						"error", err,
						"operation", "update_network",
						"component", "handler",
						"network_id", id,
					)
					writeError(w, r, fmt.Errorf("failed to disable inter-peer routing"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
					return
				}
			}
		}
		network.InterPeerRouting = *req.InterPeerRouting
	}

	if err := s.db.UpdateNetwork(ctx, network); err != nil {
		s.logger.Error("update_network_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "update_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to update network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Re-fetch for updated timestamps.
	updated, err := s.db.GetNetworkByID(ctx, id)
	if err != nil || updated == nil {
		s.logger.Error("get_updated_network_failed",
			"error", err,
			"operation", "update_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to retrieve updated network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("network_updated",
		"network_id", id,
		"network_name", updated.Name,
		"component", "handler",
	)

	writeJSON(w, http.StatusOK, networkToResponse(updated))
}

// handleDeleteNetwork deletes a network, its peers, interface, and rules.
func (s *Server) handleDeleteNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid network ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, id)
	if err != nil {
		s.logger.Error("get_network_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if network == nil {
		writeError(w, r, fmt.Errorf("network %d not found", id), apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Remove bridge nftables rules before DB cascade deletes them.
	if s.nftManager != nil {
		bridges, err := s.db.ListBridgesByNetworkID(ctx, id)
		if err != nil {
			s.logger.Error("list_bridges_for_delete_failed",
				"error", err,
				"operation", "delete_network",
				"component", "handler",
				"network_id", id,
			)
		} else {
			for _, bridge := range bridges {
				otherID := bridge.NetworkBID
				if otherID == id {
					otherID = bridge.NetworkAID
				}
				other, err := s.db.GetNetworkByID(ctx, otherID)
				if err != nil || other == nil {
					continue
				}
				if err := s.nftManager.RemoveNetworkBridge(network.Interface, other.Interface); err != nil {
					s.logger.Error("remove_bridge_nft_failed",
						"error", err,
						"operation", "delete_network",
						"component", "handler",
						"network_id", id,
						"bridge_id", bridge.ID,
					)
				}
			}
		}
	}

	// Remove nftables rules.
	if s.nftManager != nil {
		if network.NATEnabled {
			if err := s.nftManager.RemoveNATMasquerade(network.Interface); err != nil {
				s.logger.Error("remove_nat_failed",
					"error", err,
					"operation", "delete_network",
					"component", "handler",
					"network_id", id,
				)
			}
		}
		if network.InterPeerRouting {
			if err := s.nftManager.DisableInterPeerForwarding(network.Interface); err != nil {
				s.logger.Error("disable_forwarding_failed",
					"error", err,
					"operation", "delete_network",
					"component", "handler",
					"network_id", id,
				)
			}
		}
	}

	// Delete WireGuard interface.
	if s.wgManager != nil {
		if err := s.wgManager.DeleteInterface(ctx, network.Interface); err != nil {
			s.logger.Error("delete_interface_failed",
				"error", err,
				"error_type", fmt.Sprintf("%T", err),
				"operation", "delete_network",
				"component", "handler",
				"network_id", id,
				"interface", network.Interface,
			)
			// Continue with DB deletion even if interface removal fails.
		}
	}

	// Delete from database (cascade deletes peers).
	if err := s.db.DeleteNetwork(ctx, id); err != nil {
		s.logger.Error("delete_network_db_failed",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"operation", "delete_network",
			"component", "handler",
			"network_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to delete network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("network_deleted",
		"network_id", id,
		"network_name", network.Name,
		"interface", network.Interface,
		"component", "handler",
	)

	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ──────────────────────────────────────────────────────────

func networkToResponse(n *db.Network) networkResponse {
	return networkResponse{
		ID:               n.ID,
		Name:             n.Name,
		Interface:        n.Interface,
		Mode:             n.Mode,
		Subnet:           n.Subnet,
		ListenPort:       n.ListenPort,
		PublicKey:        n.PublicKey,
		DNSServers:       n.DNSServers,
		NATEnabled:       n.NATEnabled,
		InterPeerRouting: n.InterPeerRouting,
		Enabled:          n.Enabled,
		CreatedAt:        n.CreatedAt.Unix(),
		UpdatedAt:        n.UpdatedAt.Unix(),
	}
}

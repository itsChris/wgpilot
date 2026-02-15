package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/itsChris/wgpilot/internal/auth"
	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
	"github.com/itsChris/wgpilot/internal/wg"
)

// ── Request/Response types ───────────────────────────────────────────

type setupStatusResponse struct {
	Complete    bool     `json:"complete"`
	CurrentStep int      `json:"current_step"`
	WGInterfaces []string `json:"wg_interfaces,omitempty"`
}

type setupStep1Request struct {
	OTP      string `json:"otp"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupStep1Response struct {
	User userInfo `json:"user"`
}

type setupStep2Request struct {
	PublicIP   string `json:"public_ip"`
	Hostname   string `json:"hostname"`
	DNSServers string `json:"dns_servers"`
}

type setupStep3Request struct {
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Subnet           string `json:"subnet"`
	ListenPort       int    `json:"listen_port"`
	NATEnabled       bool   `json:"nat_enabled"`
	InterPeerRouting bool   `json:"inter_peer_routing"`
}

type setupStep3Response struct {
	Network networkResponse `json:"network"`
}

type setupStep4Request struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	TunnelType string `json:"tunnel_type"` // "full" or "split"
}

type setupStep4Response struct {
	Peer   peerResponse `json:"peer"`
	Config string       `json:"config"`
	QRData string       `json:"qr_data"` // base64-encoded PNG
}

// ── Setup status ─────────────────────────────────────────────────────

// handleSetupStatus returns the current setup state.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complete, step, err := s.getSetupState(ctx)
	if err != nil {
		s.logger.Error("setup_status_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to check setup status"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	resp := setupStatusResponse{
		Complete:    complete,
		CurrentStep: step,
	}

	// Detect existing WireGuard interfaces if setup is not complete.
	if !complete && s.wgManager != nil {
		ifaces, detectErr := s.wgManager.DetectInterfaces()
		if detectErr != nil {
			s.logger.Warn("setup_detect_interfaces_failed",
				"error", detectErr,
				"component", "setup",
			)
		} else {
			resp.WGInterfaces = ifaces
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// getSetupState returns whether setup is complete and which step is current.
func (s *Server) getSetupState(ctx context.Context) (complete bool, currentStep int, err error) {
	completeVal, err := s.db.GetSetting(ctx, "setup_complete")
	if err != nil {
		return false, 0, fmt.Errorf("check setup_complete: %w", err)
	}
	if completeVal == "true" {
		return true, 4, nil
	}

	// Check which steps have been completed.
	step1, err := s.db.GetSetting(ctx, "setup_step1_done")
	if err != nil {
		return false, 0, fmt.Errorf("check setup_step1: %w", err)
	}
	if step1 != "true" {
		return false, 1, nil
	}

	step2, err := s.db.GetSetting(ctx, "setup_step2_done")
	if err != nil {
		return false, 0, fmt.Errorf("check setup_step2: %w", err)
	}
	if step2 != "true" {
		return false, 2, nil
	}

	step3, err := s.db.GetSetting(ctx, "setup_step3_done")
	if err != nil {
		return false, 0, fmt.Errorf("check setup_step3: %w", err)
	}
	if step3 != "true" {
		return false, 3, nil
	}

	return false, 4, nil
}

// ── Step 1: Create admin account ─────────────────────────────────────

func (s *Server) handleSetupStep1(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if setup is already complete.
	complete, currentStep, err := s.getSetupState(ctx)
	if err != nil {
		s.logger.Error("setup_step1_state_check_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if complete {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}

	// Step 1 is idempotent — allow re-execution if already done (user refreshed).
	// But if step 1 is already done, return 409 since OTP is consumed.
	if currentStep > 1 {
		writeError(w, r, fmt.Errorf("step 1 already completed, OTP already used"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}

	var req setupStep1Request
	if code, status, decErr := decodeJSON(r, &req); decErr != nil {
		writeError(w, r, decErr, code, status, s.devMode)
		return
	}

	// Validate inputs.
	var errs []fieldError
	if req.OTP == "" {
		errs = append(errs, fieldError{"otp", "required"})
	}
	if req.Username == "" {
		errs = append(errs, fieldError{"username", "required"})
	}
	if !isValidName(req.Username) {
		errs = append(errs, fieldError{"username", "1-64 alphanumeric characters, hyphens, underscores"})
	}
	if len(req.Password) < auth.MinPasswordLength {
		errs = append(errs, fieldError{"password", fmt.Sprintf("must be at least %d characters", auth.MinPasswordLength)})
	}
	if len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Verify OTP.
	otpHash, err := s.db.GetSetting(ctx, "setup_otp")
	if err != nil {
		s.logger.Error("setup_step1_otp_read_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if otpHash == "" {
		writeError(w, r, fmt.Errorf("setup OTP already used"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}
	if verifyErr := auth.VerifyPassword(otpHash, req.OTP); verifyErr != nil {
		s.logger.Warn("setup_step1_invalid_otp",
			"remote_addr", r.RemoteAddr,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("invalid setup password"), apperr.ErrInvalidOTP, http.StatusUnauthorized, s.devMode)
		return
	}

	// Create admin user.
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("setup_step1_hash_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	userID, err := s.db.CreateUser(ctx, &db.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         "admin",
	})
	if err != nil {
		s.logger.Error("setup_step1_create_user_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Delete OTP and mark step 1 done.
	if delErr := s.db.DeleteSetting(ctx, "setup_otp"); delErr != nil {
		s.logger.Error("setup_step1_delete_otp_failed",
			"error", delErr,
			"component", "setup",
		)
	}
	if setErr := s.db.SetSetting(ctx, "setup_step1_done", "true"); setErr != nil {
		s.logger.Error("setup_step1_set_done_failed",
			"error", setErr,
			"component", "setup",
		)
	}

	// Issue JWT session.
	token, err := s.jwtService.Generate(userID, req.Username, "admin")
	if err != nil {
		s.logger.Error("setup_step1_token_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	s.sessions.SetCookie(w, token, int(s.jwtService.TTL().Seconds()))

	s.logger.Info("setup_step1_completed",
		"user", req.Username,
		"user_id", userID,
		"remote_addr", r.RemoteAddr,
		"component", "setup",
	)

	writeJSON(w, http.StatusCreated, setupStep1Response{
		User: userInfo{ID: userID, Username: req.Username},
	})
}

// ── Step 2: Server identity ──────────────────────────────────────────

func (s *Server) handleSetupStep2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complete, currentStep, err := s.getSetupState(ctx)
	if err != nil {
		s.logger.Error("setup_step2_state_check_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if complete {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}
	if currentStep < 2 {
		writeError(w, r, fmt.Errorf("step 1 must be completed first"), apperr.ErrStepOrderViolation, http.StatusBadRequest, s.devMode)
		return
	}

	var req setupStep2Request
	if code, status, decErr := decodeJSON(r, &req); decErr != nil {
		writeError(w, r, decErr, code, status, s.devMode)
		return
	}

	// Validate inputs.
	var errs []fieldError
	if req.PublicIP != "" && net.ParseIP(req.PublicIP) == nil {
		errs = append(errs, fieldError{"public_ip", "must be a valid IP address"})
	}
	if req.Hostname != "" && !isValidHostname(req.Hostname) {
		errs = append(errs, fieldError{"hostname", "must be a valid FQDN"})
	}
	if !isValidDNSServers(req.DNSServers) {
		errs = append(errs, fieldError{"dns_servers", "must be up to 3 valid IP addresses, comma-separated"})
	}
	if len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Save settings.
	if req.PublicIP != "" {
		if setErr := s.db.SetSetting(ctx, "public_ip", req.PublicIP); setErr != nil {
			s.logger.Error("setup_step2_save_public_ip_failed",
				"error", setErr,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("failed to save settings"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}
	if req.Hostname != "" {
		if setErr := s.db.SetSetting(ctx, "hostname", req.Hostname); setErr != nil {
			s.logger.Error("setup_step2_save_hostname_failed",
				"error", setErr,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("failed to save settings"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}
	if req.DNSServers != "" {
		if setErr := s.db.SetSetting(ctx, "dns_servers", req.DNSServers); setErr != nil {
			s.logger.Error("setup_step2_save_dns_failed",
				"error", setErr,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("failed to save settings"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Mark step 2 done.
	if setErr := s.db.SetSetting(ctx, "setup_step2_done", "true"); setErr != nil {
		s.logger.Error("setup_step2_set_done_failed",
			"error", setErr,
			"component", "setup",
		)
	}

	s.logger.Info("setup_step2_completed",
		"public_ip", req.PublicIP,
		"hostname", req.Hostname,
		"remote_addr", r.RemoteAddr,
		"component", "setup",
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Step 3: First network ────────────────────────────────────────────

func (s *Server) handleSetupStep3(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complete, currentStep, err := s.getSetupState(ctx)
	if err != nil {
		s.logger.Error("setup_step3_state_check_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if complete {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}
	if currentStep < 3 {
		writeError(w, r, fmt.Errorf("previous steps must be completed first"), apperr.ErrStepOrderViolation, http.StatusBadRequest, s.devMode)
		return
	}

	var req setupStep3Request
	if code, status, decErr := decodeJSON(r, &req); decErr != nil {
		writeError(w, r, decErr, code, status, s.devMode)
		return
	}

	// Validate using existing validators.
	createReq := createNetworkRequest{
		Name:             req.Name,
		Mode:             req.Mode,
		Subnet:           req.Subnet,
		ListenPort:       req.ListenPort,
		NATEnabled:       req.NATEnabled,
		InterPeerRouting: req.InterPeerRouting,
	}
	if errs := s.validateCreateNetwork(createReq); len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Get default DNS from step 2 settings.
	dnsServers, _ := s.db.GetSetting(ctx, "dns_servers")

	// Generate server keypair.
	privateKey, publicKey, err := wg.GenerateKeyPair()
	if err != nil {
		s.logger.Error("setup_step3_keygen_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to generate server keypair"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Determine interface name.
	networks, err := s.db.ListNetworks(ctx)
	if err != nil {
		s.logger.Error("setup_step3_list_networks_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
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
		DNSServers:       dnsServers,
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
		if createErr := s.wgManager.CreateInterface(ctx, netCfg); createErr != nil {
			s.logger.Error("setup_step3_create_interface_failed",
				"error", createErr,
				"interface", ifaceName,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("failed to create WireGuard interface"), apperr.ErrInterfaceCreateFailed, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Apply nftables rules.
	if s.nftManager != nil {
		// Open the UDP listen port in the firewall.
		if portErr := s.nftManager.OpenUDPPort(req.ListenPort); portErr != nil {
			s.logger.Error("setup_step3_open_port_failed",
				"error", portErr,
				"port", req.ListenPort,
				"component", "setup",
			)
			if s.wgManager != nil {
				s.wgManager.DeleteInterface(ctx, ifaceName)
			}
			writeError(w, r, fmt.Errorf("failed to open firewall port"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
		if req.NATEnabled {
			if natErr := s.nftManager.AddNATMasquerade(ifaceName, req.Subnet); natErr != nil {
				s.logger.Error("setup_step3_nat_failed",
					"error", natErr,
					"interface", ifaceName,
					"component", "setup",
				)
				if s.wgManager != nil {
					s.wgManager.DeleteInterface(ctx, ifaceName)
				}
				writeError(w, r, fmt.Errorf("failed to add NAT rules"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
				return
			}
		}
		if req.InterPeerRouting {
			if fwdErr := s.nftManager.EnableInterPeerForwarding(ifaceName); fwdErr != nil {
				s.logger.Error("setup_step3_forwarding_failed",
					"error", fwdErr,
					"interface", ifaceName,
					"component", "setup",
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
		s.logger.Error("setup_step3_create_network_db_failed",
			"error", err,
			"component", "setup",
		)
		if s.wgManager != nil {
			s.wgManager.DeleteInterface(ctx, ifaceName)
		}
		writeError(w, r, fmt.Errorf("failed to create network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Store network ID for step 4.
	if setErr := s.db.SetSetting(ctx, "setup_network_id", fmt.Sprintf("%d", id)); setErr != nil {
		s.logger.Error("setup_step3_save_network_id_failed",
			"error", setErr,
			"component", "setup",
		)
	}
	if setErr := s.db.SetSetting(ctx, "setup_step3_done", "true"); setErr != nil {
		s.logger.Error("setup_step3_set_done_failed",
			"error", setErr,
			"component", "setup",
		)
	}

	// Fetch for response.
	created, err := s.db.GetNetworkByID(ctx, id)
	if err != nil || created == nil {
		s.logger.Error("setup_step3_get_created_failed",
			"error", err,
			"network_id", id,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to retrieve created network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("setup_step3_completed",
		"network_id", id,
		"network_name", created.Name,
		"interface", created.Interface,
		"component", "setup",
	)

	writeJSON(w, http.StatusCreated, setupStep3Response{
		Network: networkToResponse(created),
	})
}

// ── Step 4: First peer ───────────────────────────────────────────────

func (s *Server) handleSetupStep4(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complete, currentStep, err := s.getSetupState(ctx)
	if err != nil {
		s.logger.Error("setup_step4_state_check_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if complete {
		writeError(w, r, fmt.Errorf("setup already completed"), apperr.ErrSetupComplete, http.StatusConflict, s.devMode)
		return
	}
	if currentStep < 4 {
		writeError(w, r, fmt.Errorf("previous steps must be completed first"), apperr.ErrStepOrderViolation, http.StatusBadRequest, s.devMode)
		return
	}

	var req setupStep4Request
	if code, status, decErr := decodeJSON(r, &req); decErr != nil {
		writeError(w, r, decErr, code, status, s.devMode)
		return
	}

	// Validate inputs.
	var errs []fieldError
	if !isValidName(req.Name) {
		errs = append(errs, fieldError{"name", "1-64 alphanumeric characters, spaces, hyphens, underscores"})
	}
	if !isValidRole(req.Role) {
		errs = append(errs, fieldError{"role", "must be client or site-gateway"})
	}
	if req.TunnelType != "" && req.TunnelType != "full" && req.TunnelType != "split" {
		errs = append(errs, fieldError{"tunnel_type", "must be full or split"})
	}
	if len(errs) > 0 {
		writeValidationError(w, r, errs)
		return
	}

	// Retrieve the network created in step 3.
	networkIDStr, err := s.db.GetSetting(ctx, "setup_network_id")
	if err != nil || networkIDStr == "" {
		s.logger.Error("setup_step4_no_network_id",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("no network found from step 3"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	var networkID int64
	if _, scanErr := fmt.Sscanf(networkIDStr, "%d", &networkID); scanErr != nil {
		writeError(w, r, fmt.Errorf("invalid network ID from step 3"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	network, err := s.db.GetNetworkByID(ctx, networkID)
	if err != nil || network == nil {
		s.logger.Error("setup_step4_get_network_failed",
			"error", err,
			"network_id", networkID,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("network from step 3 not found"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Allocate IP from subnet.
	_, subnet, _ := net.ParseCIDR(network.Subnet)
	existingPeers, err := s.db.ListPeersByNetworkID(ctx, networkID)
	if err != nil {
		s.logger.Error("setup_step4_list_peers_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
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
		s.logger.Error("setup_step4_ip_allocator_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to create IP allocator"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	allocatedIP, err := alloc.Allocate()
	if err != nil {
		writeError(w, r, fmt.Errorf("no available IPs in subnet"), apperr.ErrIPExhausted, http.StatusConflict, s.devMode)
		return
	}

	// Generate peer keypair and preshared key.
	peerPrivKey, peerPubKey, err := wg.GenerateKeyPair()
	if err != nil {
		s.logger.Error("setup_step4_keygen_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to generate peer keypair"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	presharedKey, err := wg.GeneratePresharedKey()
	if err != nil {
		s.logger.Error("setup_step4_psk_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to generate preshared key"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Compute server-side AllowedIPs.
	serverAllowedIPs := allocatedIP.String() + "/32"

	peer := &db.Peer{
		NetworkID:           networkID,
		Name:                req.Name,
		PrivateKey:          peerPrivKey,
		PublicKey:           peerPubKey,
		PresharedKey:        presharedKey,
		AllowedIPs:          serverAllowedIPs,
		PersistentKeepalive: 25,
		Role:                req.Role,
		Enabled:             true,
	}

	// Add peer to WireGuard interface.
	if s.wgManager != nil {
		peerCfg := wg.PeerConfig{
			Name:                req.Name,
			PublicKey:           peerPubKey,
			PresharedKey:        presharedKey,
			AllowedIPs:          serverAllowedIPs,
			PersistentKeepalive: 25,
		}
		if addErr := s.wgManager.AddPeer(ctx, network.Interface, peerCfg); addErr != nil {
			s.logger.Error("setup_step4_add_peer_failed",
				"error", addErr,
				"interface", network.Interface,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("failed to add peer to WireGuard"), apperr.ErrPeerAddFailed, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Persist to database.
	peerID, err := s.db.CreatePeer(ctx, peer)
	if err != nil {
		s.logger.Error("setup_step4_create_peer_db_failed",
			"error", err,
			"component", "setup",
		)
		if s.wgManager != nil {
			s.wgManager.RemovePeer(ctx, network.Interface, peerPubKey)
		}
		writeError(w, r, fmt.Errorf("failed to create peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Generate client config.
	publicIP, _ := s.db.GetSetting(ctx, "public_ip")
	if publicIP == "" {
		publicIP = "YOUR_SERVER_IP"
	}
	serverEndpoint := fmt.Sprintf("%s:%d", publicIP, network.ListenPort)

	// Determine client AllowedIPs based on tunnel type.
	tunnelType := req.TunnelType
	if tunnelType == "" {
		tunnelType = "full"
	}
	var clientAllowedIPs string
	if tunnelType == "full" {
		clientAllowedIPs = "0.0.0.0/0, ::/0"
	} else {
		clientAllowedIPs = network.Subnet
	}

	conf, err := wg.GenerateClientConfig(wg.ClientConfigParams{
		PeerName:            req.Name,
		PeerPrivateKey:      peerPrivKey,
		PeerAddress:         serverAllowedIPs,
		DNSServers:          network.DNSServers,
		ServerPublicKey:     network.PublicKey,
		PresharedKey:        presharedKey,
		ServerEndpoint:      serverEndpoint,
		AllowedIPs:          clientAllowedIPs,
		PersistentKeepalive: 25,
	})
	if err != nil {
		s.logger.Error("setup_step4_config_gen_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to generate client config"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Generate QR code as base64.
	qrPNG, err := wg.GenerateQRCode(conf, 256)
	if err != nil {
		s.logger.Error("setup_step4_qr_failed",
			"error", err,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to generate QR code"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	// Mark setup as complete.
	if setErr := s.db.SetSetting(ctx, "setup_complete", "true"); setErr != nil {
		s.logger.Error("setup_step4_set_complete_failed",
			"error", setErr,
			"component", "setup",
		)
	}

	// Fetch created peer for response.
	created, err := s.db.GetPeerByID(ctx, peerID)
	if err != nil || created == nil {
		s.logger.Error("setup_step4_get_created_failed",
			"error", err,
			"peer_id", peerID,
			"component", "setup",
		)
		writeError(w, r, fmt.Errorf("failed to retrieve created peer"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("setup_completed",
		"peer_id", peerID,
		"peer_name", created.Name,
		"network_id", networkID,
		"remote_addr", r.RemoteAddr,
		"component", "setup",
	)

	writeJSON(w, http.StatusCreated, setupStep4Response{
		Peer:   peerToResponse(created),
		Config: conf,
		QRData: qrBase64,
	})
}

// ── Setup guard middleware ────────────────────────────────────────────

// setupGuard wraps a handler and returns 403 with SETUP_REQUIRED if setup
// is not complete. Setup endpoints and public endpoints bypass this guard.
func (s *Server) setupGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		complete, err := s.db.GetSetting(ctx, "setup_complete")
		if err != nil {
			s.logger.Error("setup_guard_check_failed",
				"error", err,
				"component", "setup",
			)
			writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
		if complete != "true" {
			writeError(w, r, fmt.Errorf("setup not complete"), apperr.ErrSetupRequired, http.StatusForbidden, s.devMode)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Public IP detection ──────────────────────────────────────────────

// ipDetectServices are the external services used to detect the server's
// public IP address. Each is tried in order with a per-service timeout.
var ipDetectServices = []string{
	"https://api.ipify.org",
	"https://icanhazip.com",
	"https://ifconfig.me/ip",
}

// httpClient is overridable for testing.
var newHTTPClient = func(timeout time.Duration) httpDoer {
	return &http.Client{Timeout: timeout}
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// detectPublicIP tries multiple external services to determine the server's
// public IP address. Returns empty string if all fail.
func detectPublicIP(ctx context.Context) string {
	client := newHTTPClient(3 * time.Second)

	for _, svc := range ipDetectServices {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, svc, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	return ""
}

// handleDetectPublicIP is a convenience endpoint for the frontend to detect
// the server's public IP without the frontend needing to make external requests.
func (s *Server) handleDetectPublicIP(w http.ResponseWriter, r *http.Request) {
	ip := detectPublicIP(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"public_ip": ip})
}

// handleImportConfig imports a wg-quick configuration file.
func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Config string `json:"config"`
		Name   string `json:"name"`
	}
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	if req.Config == "" {
		writeError(w, r, fmt.Errorf("config content is required"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}
	if req.Name == "" {
		req.Name = "Imported Network"
	}

	parsed, err := wg.ParseWgQuickConfig(strings.NewReader(req.Config))
	if err != nil {
		writeError(w, r, fmt.Errorf("failed to parse config: %v", err), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Derive public key from private key.
	pubKey, err := wg.PublicKeyFromPrivate(parsed.PrivateKey)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid private key in config"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	listenPort := parsed.ListenPort
	if listenPort == 0 {
		listenPort = 51820
	}

	subnet := parsed.Address
	if subnet == "" {
		writeError(w, r, fmt.Errorf("no Address in [Interface] section"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Determine interface name.
	networks, err := s.db.ListNetworks(ctx)
	if err != nil {
		s.logger.Error("import_list_networks_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("internal error"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	ifaceName := s.nextInterfaceName(networks)

	network := &db.Network{
		Name:       req.Name,
		Interface:  ifaceName,
		Mode:       "gateway",
		Subnet:     subnet,
		ListenPort: listenPort,
		PrivateKey: parsed.PrivateKey,
		PublicKey:  pubKey,
		DNSServers: parsed.DNSServers,
		Enabled:    true,
	}

	netID, err := s.db.CreateNetwork(ctx, network)
	if err != nil {
		s.logger.Error("import_create_network_failed", "error", err, "component", "handler")
		writeError(w, r, fmt.Errorf("failed to create network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Import peers.
	var importedPeers int
	for i, p := range parsed.Peers {
		peerName := fmt.Sprintf("Imported Peer %d", i+1)
		peer := &db.Peer{
			NetworkID:           netID,
			Name:                peerName,
			PublicKey:           p.PublicKey,
			PresharedKey:        p.PresharedKey,
			AllowedIPs:          p.AllowedIPs,
			Endpoint:            p.Endpoint,
			PersistentKeepalive: p.PersistentKeepalive,
			Role:                "client",
			Enabled:             true,
		}
		if _, err := s.db.CreatePeer(ctx, peer); err != nil {
			s.logger.Error("import_create_peer_failed", "error", err, "component", "handler", "peer_index", i)
			continue
		}
		importedPeers++
	}

	s.logger.Info("config_imported",
		"network_id", netID,
		"network_name", req.Name,
		"interface", ifaceName,
		"peers_imported", importedPeers,
		"component", "handler",
	)
	s.auditf(r, "network.imported", "network", "imported network %q (id=%d) with %d peers", req.Name, netID, importedPeers)

	writeJSON(w, http.StatusCreated, map[string]any{
		"network_id":     netID,
		"interface":      ifaceName,
		"peers_imported": importedPeers,
	})
}

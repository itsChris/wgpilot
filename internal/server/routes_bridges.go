package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/itsChris/wgpilot/internal/db"
	apperr "github.com/itsChris/wgpilot/internal/errors"
)

// ── Request/Response types ───────────────────────────────────────────

type createBridgeRequest struct {
	NetworkAID  int64  `json:"network_a_id"`
	NetworkBID  int64  `json:"network_b_id"`
	Direction   string `json:"direction"`
	AllowedCIDRs string `json:"allowed_cidrs"`
}

type bridgeResponse struct {
	ID           int64  `json:"id"`
	NetworkAID   int64  `json:"network_a_id"`
	NetworkBID   int64  `json:"network_b_id"`
	NetworkAName string `json:"network_a_name"`
	NetworkBName string `json:"network_b_name"`
	InterfaceA   string `json:"interface_a"`
	InterfaceB   string `json:"interface_b"`
	Direction    string `json:"direction"`
	AllowedCIDRs string `json:"allowed_cidrs"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// ── Validation ───────────────────────────────────────────────────────

func isValidDirection(d string) bool {
	return d == "a_to_b" || d == "b_to_a" || d == "bidirectional"
}

// ── Handlers ─────────────────────────────────────────────────────────

// handleCreateBridge creates a bridge between two networks.
func (s *Server) handleCreateBridge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createBridgeRequest
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	// Validate direction.
	if !isValidDirection(req.Direction) {
		writeError(w, r,
			fmt.Errorf("direction must be a_to_b, b_to_a, or bidirectional"),
			apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	// Reject self-reference.
	if req.NetworkAID == req.NetworkBID {
		writeError(w, r,
			fmt.Errorf("cannot bridge a network to itself"),
			apperr.ErrBridgeSelfReference, http.StatusBadRequest, s.devMode)
		return
	}

	// Verify both networks exist.
	networkA, err := s.db.GetNetworkByID(ctx, req.NetworkAID)
	if err != nil {
		s.logger.Error("get_network_a_failed",
			"error", err,
			"operation", "create_bridge",
			"component", "handler",
			"network_a_id", req.NetworkAID,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if networkA == nil {
		writeError(w, r,
			fmt.Errorf("network %d not found", req.NetworkAID),
			apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	networkB, err := s.db.GetNetworkByID(ctx, req.NetworkBID)
	if err != nil {
		s.logger.Error("get_network_b_failed",
			"error", err,
			"operation", "create_bridge",
			"component", "handler",
			"network_b_id", req.NetworkBID,
		)
		writeError(w, r, fmt.Errorf("failed to get network"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if networkB == nil {
		writeError(w, r,
			fmt.Errorf("network %d not found", req.NetworkBID),
			apperr.ErrNetworkNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Check for duplicate bridge.
	exists, err := s.db.BridgeExistsBetween(ctx, req.NetworkAID, req.NetworkBID)
	if err != nil {
		s.logger.Error("check_bridge_exists_failed",
			"error", err,
			"operation", "create_bridge",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to check existing bridges"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if exists {
		writeError(w, r,
			fmt.Errorf("bridge already exists between network %d and %d", req.NetworkAID, req.NetworkBID),
			apperr.ErrBridgeAlreadyExists, http.StatusConflict, s.devMode)
		return
	}

	// Apply nftables rules.
	if s.nftManager != nil {
		if err := s.nftManager.AddNetworkBridge(networkA.Interface, networkB.Interface, req.Direction); err != nil {
			s.logger.Error("add_bridge_nft_failed",
				"error", err,
				"operation", "create_bridge",
				"component", "handler",
				"interface_a", networkA.Interface,
				"interface_b", networkB.Interface,
				"direction", req.Direction,
			)
			writeError(w, r, fmt.Errorf("failed to add bridge firewall rules"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
			return
		}
	}

	// Persist to database.
	bridge := &db.Bridge{
		NetworkAID:   req.NetworkAID,
		NetworkBID:   req.NetworkBID,
		Direction:    req.Direction,
		AllowedCIDRs: req.AllowedCIDRs,
		Enabled:      true,
	}
	id, err := s.db.CreateBridge(ctx, bridge)
	if err != nil {
		s.logger.Error("create_bridge_db_failed",
			"error", err,
			"operation", "create_bridge",
			"component", "handler",
		)
		// Clean up nftables rules on DB failure.
		if s.nftManager != nil {
			s.nftManager.RemoveNetworkBridge(networkA.Interface, networkB.Interface)
		}
		writeError(w, r, fmt.Errorf("failed to create bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Fetch the created bridge for response.
	created, err := s.db.GetBridgeByID(ctx, id)
	if err != nil || created == nil {
		s.logger.Error("get_created_bridge_failed",
			"error", err,
			"operation", "create_bridge",
			"component", "handler",
			"bridge_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to retrieve created bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("bridge_created",
		"bridge_id", id,
		"network_a_id", req.NetworkAID,
		"network_b_id", req.NetworkBID,
		"interface_a", networkA.Interface,
		"interface_b", networkB.Interface,
		"direction", req.Direction,
		"component", "handler",
	)
	s.auditf(r, "bridge.created", "bridge", "created bridge (id=%d) between networks %d and %d", id, req.NetworkAID, req.NetworkBID)

	writeJSON(w, http.StatusCreated, bridgeToResponse(created, networkA, networkB))
}

// handleListBridges lists all bridges with network names.
func (s *Server) handleListBridges(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	bridges, err := s.db.ListBridges(ctx)
	if err != nil {
		s.logger.Error("list_bridges_failed",
			"error", err,
			"operation", "list_bridges",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to list bridges"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	// Build network lookup for names/interfaces.
	networks, err := s.db.ListNetworks(ctx)
	if err != nil {
		s.logger.Error("list_networks_for_bridges_failed",
			"error", err,
			"operation", "list_bridges",
			"component", "handler",
		)
		writeError(w, r, fmt.Errorf("failed to list networks"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	networkMap := make(map[int64]*db.Network, len(networks))
	for i := range networks {
		networkMap[networks[i].ID] = &networks[i]
	}

	result := make([]bridgeResponse, 0, len(bridges))
	for _, b := range bridges {
		netA := networkMap[b.NetworkAID]
		netB := networkMap[b.NetworkBID]
		result = append(result, bridgeToResponse(&b, netA, netB))
	}

	writeJSON(w, http.StatusOK, result)
}

// handleGetBridge returns a single bridge.
func (s *Server) handleGetBridge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid bridge ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	bridge, err := s.db.GetBridgeByID(ctx, id)
	if err != nil {
		s.logger.Error("get_bridge_failed",
			"error", err,
			"operation", "get_bridge",
			"component", "handler",
			"bridge_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to get bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if bridge == nil {
		writeError(w, r, fmt.Errorf("bridge %d not found", id), apperr.ErrBridgeNotFound, http.StatusNotFound, s.devMode)
		return
	}

	networkA, _ := s.db.GetNetworkByID(ctx, bridge.NetworkAID)
	networkB, _ := s.db.GetNetworkByID(ctx, bridge.NetworkBID)

	writeJSON(w, http.StatusOK, bridgeToResponse(bridge, networkA, networkB))
}

// handleUpdateBridge updates a bridge's direction, CIDRs, or enabled status.
func (s *Server) handleUpdateBridge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid bridge ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	bridge, err := s.db.GetBridgeByID(ctx, id)
	if err != nil {
		s.logger.Error("get_bridge_failed", "error", err, "operation", "update_bridge", "component", "handler", "bridge_id", id)
		writeError(w, r, fmt.Errorf("failed to get bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if bridge == nil {
		writeError(w, r, fmt.Errorf("bridge %d not found", id), apperr.ErrBridgeNotFound, http.StatusNotFound, s.devMode)
		return
	}

	var req struct {
		Direction    *string `json:"direction"`
		AllowedCIDRs *string `json:"allowed_cidrs"`
		Enabled      *bool   `json:"enabled"`
	}
	if code, status, err := decodeJSON(r, &req); err != nil {
		writeError(w, r, err, code, status, s.devMode)
		return
	}

	oldDirection := bridge.Direction
	if req.Direction != nil {
		if !isValidDirection(*req.Direction) {
			writeError(w, r, fmt.Errorf("direction must be a_to_b, b_to_a, or bidirectional"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
			return
		}
		bridge.Direction = *req.Direction
	}
	if req.AllowedCIDRs != nil {
		bridge.AllowedCIDRs = *req.AllowedCIDRs
	}
	if req.Enabled != nil {
		bridge.Enabled = *req.Enabled
	}

	// Reconcile nftables if direction changed.
	if s.nftManager != nil && oldDirection != bridge.Direction {
		networkA, _ := s.db.GetNetworkByID(ctx, bridge.NetworkAID)
		networkB, _ := s.db.GetNetworkByID(ctx, bridge.NetworkBID)
		if networkA != nil && networkB != nil {
			// Remove old rules, add new ones.
			if err := s.nftManager.RemoveNetworkBridge(networkA.Interface, networkB.Interface); err != nil {
				s.logger.Error("remove_bridge_nft_failed", "error", err, "operation", "update_bridge", "component", "handler", "bridge_id", id)
			}
			if err := s.nftManager.AddNetworkBridge(networkA.Interface, networkB.Interface, bridge.Direction); err != nil {
				s.logger.Error("add_bridge_nft_failed", "error", err, "operation", "update_bridge", "component", "handler", "bridge_id", id)
				writeError(w, r, fmt.Errorf("failed to update bridge firewall rules"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
				return
			}
		}
	}

	if err := s.db.UpdateBridge(ctx, bridge); err != nil {
		s.logger.Error("update_bridge_db_failed", "error", err, "operation", "update_bridge", "component", "handler", "bridge_id", id)
		writeError(w, r, fmt.Errorf("failed to update bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	updated, _ := s.db.GetBridgeByID(ctx, id)
	if updated == nil {
		updated = bridge
	}

	networkA, _ := s.db.GetNetworkByID(ctx, updated.NetworkAID)
	networkB, _ := s.db.GetNetworkByID(ctx, updated.NetworkBID)

	s.logger.Info("bridge_updated", "bridge_id", id, "component", "handler")
	s.auditf(r, "bridge.updated", "bridge", "updated bridge (id=%d)", id)

	writeJSON(w, http.StatusOK, bridgeToResponse(updated, networkA, networkB))
}

// handleDeleteBridge deletes a bridge and removes its nftables rules.
func (s *Server) handleDeleteBridge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("invalid bridge ID"), apperr.ErrValidation, http.StatusBadRequest, s.devMode)
		return
	}

	bridge, err := s.db.GetBridgeByID(ctx, id)
	if err != nil {
		s.logger.Error("get_bridge_failed",
			"error", err,
			"operation", "delete_bridge",
			"component", "handler",
			"bridge_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to get bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}
	if bridge == nil {
		writeError(w, r, fmt.Errorf("bridge %d not found", id), apperr.ErrBridgeNotFound, http.StatusNotFound, s.devMode)
		return
	}

	// Remove nftables rules.
	if s.nftManager != nil {
		networkA, _ := s.db.GetNetworkByID(ctx, bridge.NetworkAID)
		networkB, _ := s.db.GetNetworkByID(ctx, bridge.NetworkBID)
		if networkA != nil && networkB != nil {
			if err := s.nftManager.RemoveNetworkBridge(networkA.Interface, networkB.Interface); err != nil {
				s.logger.Error("remove_bridge_nft_failed",
					"error", err,
					"operation", "delete_bridge",
					"component", "handler",
					"bridge_id", id,
				)
			}
		}
	}

	// Delete from database.
	if err := s.db.DeleteBridge(ctx, id); err != nil {
		s.logger.Error("delete_bridge_db_failed",
			"error", err,
			"operation", "delete_bridge",
			"component", "handler",
			"bridge_id", id,
		)
		writeError(w, r, fmt.Errorf("failed to delete bridge"), apperr.ErrInternal, http.StatusInternalServerError, s.devMode)
		return
	}

	s.logger.Info("bridge_deleted",
		"bridge_id", id,
		"network_a_id", bridge.NetworkAID,
		"network_b_id", bridge.NetworkBID,
		"component", "handler",
	)
	s.auditf(r, "bridge.deleted", "bridge", "deleted bridge (id=%d) between networks %d and %d", id, bridge.NetworkAID, bridge.NetworkBID)

	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ──────────────────────────────────────────────────────────

func bridgeToResponse(b *db.Bridge, netA, netB *db.Network) bridgeResponse {
	resp := bridgeResponse{
		ID:           b.ID,
		NetworkAID:   b.NetworkAID,
		NetworkBID:   b.NetworkBID,
		Direction:    b.Direction,
		AllowedCIDRs: b.AllowedCIDRs,
		Enabled:      b.Enabled,
		CreatedAt:    b.CreatedAt.Unix(),
		UpdatedAt:    b.UpdatedAt.Unix(),
	}
	if netA != nil {
		resp.NetworkAName = netA.Name
		resp.InterfaceA = netA.Interface
	}
	if netB != nil {
		resp.NetworkBName = netB.Name
		resp.InterfaceB = netB.Interface
	}
	return resp
}

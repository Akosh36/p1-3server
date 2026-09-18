package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/models"
	"github.com/Akosh36/p1-3server/internal/wgsync"
)

func (s *Server) handleListLANNetworks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT ln.id, ln.name, ln.type, ln.vpn_peer_id, ln.is_active, ln.is_reachable, ln.last_status_check_at,
		       vp.public_key, vp.allowed_subnet::text, vp.endpoint, vp.last_handshake_at
		FROM lan_networks ln
		LEFT JOIN vpn_peers vp ON vp.id = ln.vpn_peer_id
		ORDER BY ln.name
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list LAN networks")
		return
	}
	defer rows.Close()

	networks := []models.LANNetwork{}
	for rows.Next() {
		var n models.LANNetwork
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.VPNPeerID, &n.IsActive, &n.IsReachable, &n.LastStatusCheckAt,
			&n.VPNPublicKey, &n.VPNAllowedSubnet, &n.VPNEndpoint, &n.VPNLastHandshakeAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read LAN network row")
			return
		}
		networks = append(networks, n)
	}
	writeJSON(w, http.StatusOK, networks)
}

type updateLANNetworkRequest struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

func (s *Server) handleUpdateLANNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid LAN network id")
		return
	}

	var req updateLANNetworkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	if req.Name != nil {
		if _, err := s.pool.Exec(ctx, `UPDATE lan_networks SET name = $1 WHERE id = $2`, *req.Name, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to rename LAN network")
			return
		}
	}
	if req.IsActive != nil {
		if _, err := s.pool.Exec(ctx, `UPDATE lan_networks SET is_active = $1 WHERE id = $2`, *req.IsActive, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to toggle LAN network")
			return
		}
	}

	_ = s.recordAudit(ctx, claims, "lan_network.update", "lan_network", &id, map[string]interface{}{
		"name": req.Name, "is_active": req.IsActive,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

type createRemoteVPNRequest struct {
	Name          string `json:"name"`
	PublicKey     string `json:"public_key"`
	AllowedSubnet string `json:"allowed_subnet"`
	Endpoint      string `json:"endpoint,omitempty"`
}

// handleCreateRemoteVPNLAN is the LAN page's "+ Masofaviy LAN qo'shish"
// form: a remote site's WireGuard peer (vpn_peers) and the LAN entry
// representing it (lan_networks, type='wireless_remote_vpn') are always
// created together — this app never reuses one peer across multiple LAN
// entries, so keeping them as two separate admin actions would only add
// steps without adding flexibility anyone needs.
func (s *Server) handleCreateRemoteVPNLAN(w http.ResponseWriter, r *http.Request) {
	var req createRemoteVPNRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.PublicKey == "" || req.AllowedSubnet == "" {
		writeError(w, http.StatusBadRequest, "name, public_key and allowed_subnet are required")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var endpoint interface{}
	if req.Endpoint != "" {
		endpoint = req.Endpoint
	}

	var peerID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO vpn_peers (name, public_key, allowed_subnet, endpoint)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, req.Name, req.PublicKey, req.AllowedSubnet, endpoint).Scan(&peerID); err != nil {
		writeError(w, http.StatusConflict, "peer name/public key already in use, or allowed_subnet is not a valid CIDR (e.g. 192.168.50.0/24)")
		return
	}

	var lanID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO lan_networks (name, type, vpn_peer_id)
		VALUES ($1, 'wireless_remote_vpn', $2) RETURNING id
	`, req.Name, peerID).Scan(&lanID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create LAN network")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit changes")
		return
	}

	_ = s.recordAudit(ctx, claims, "lan_network.create_remote_vpn", "lan_network", &lanID, map[string]interface{}{
		"vpn_peer_id": peerID, "name": req.Name,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{"lan_network_id": lanID, "vpn_peer_id": peerID})
}

// handleDeleteLANNetwork removes a LAN entry and, if it's a remote VPN
// site, its underlying vpn_peers row too — the next wgsync tick then
// removes the peer from wgd's interface since it's no longer in Postgres
// at all.
func (s *Server) handleDeleteLANNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid LAN network id")
		return
	}

	ctx := r.Context()
	claims := claimsFromContext(r)

	var vpnPeerID *int64
	if err := s.pool.QueryRow(ctx, `SELECT vpn_peer_id FROM lan_networks WHERE id = $1`, id).Scan(&vpnPeerID); err != nil {
		writeError(w, http.StatusNotFound, "LAN network not found")
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM lan_networks WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete LAN network")
		return
	}
	if vpnPeerID != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM vpn_peers WHERE id = $1`, *vpnPeerID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete VPN peer")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit changes")
		return
	}

	_ = s.recordAudit(ctx, claims, "lan_network.delete", "lan_network", &id, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleWireGuardLocalInfo returns this gateway's own WireGuard public key
// and listen port, shown on the LAN page so the admin can hand it to the
// remote site's admin when setting up a new tunnel (site-to-site WireGuard
// needs both ends to know each other's public key).
func (s *Server) handleWireGuardLocalInfo(w http.ResponseWriter, r *http.Request) {
	publicKey, listenPort, err := wgsync.LocalInfo(r.Context(), s.cfg.WgdSocket)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "wgd is not reachable: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"public_key": publicKey, "listen_port": listenPort})
}

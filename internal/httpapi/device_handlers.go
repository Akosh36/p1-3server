package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Akosh36/p1-3server/internal/models"
)

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT d.id, d.mac_address::text, host(d.ip_address), d.hostname, d.nickname,
		       d.conn_type, d.switch_port_id, d.ssid, d.lan_network_id,
		       d.first_seen_at, d.last_seen_at, d.is_online, ag.role
		FROM devices d
		LEFT JOIN access_grants ag ON ag.device_id = d.id
		ORDER BY d.last_seen_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}
	defer rows.Close()

	devices := []models.Device{}
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.MACAddress, &d.IPAddress, &d.Hostname, &d.Nickname,
			&d.ConnType, &d.SwitchPortID, &d.SSID, &d.LANNetworkID,
			&d.FirstSeenAt, &d.LastSeenAt, &d.IsOnline, &d.AccessRole); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read device row")
			return
		}
		devices = append(devices, d)
	}
	writeJSON(w, http.StatusOK, devices)
}

type updateDeviceRequest struct {
	Nickname   *string             `json:"nickname,omitempty"`
	AccessRole *models.AccessRole  `json:"access_role,omitempty"` // "user", "admin", or null to revoke
}

// handleUpdateDevice is how an admin assigns/revokes User or Admin rights to
// a discovered device (identified by MAC), and sets its friendly nickname.
// This is the single write path that feeds fwctl's nftables allow-lists —
// a device with no access_grants row is blocked by default.
func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	var req updateDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	if req.Nickname != nil {
		if _, err := tx.Exec(ctx, `UPDATE devices SET nickname = $1 WHERE id = $2`, *req.Nickname, id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update nickname")
			return
		}
	}

	if req.AccessRole != nil {
		if *req.AccessRole == "" {
			if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE device_id = $1`, id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to revoke access")
				return
			}
		} else if *req.AccessRole == models.AccessUser || *req.AccessRole == models.AccessAdmin {
			if _, err := tx.Exec(ctx, `
				INSERT INTO access_grants (device_id, role, granted_by)
				VALUES ($1, $2, $3)
				ON CONFLICT (device_id) DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by, granted_at = now()
			`, id, *req.AccessRole, claims.AdminID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to grant access")
				return
			}
		} else {
			writeError(w, http.StatusBadRequest, "access_role must be 'user', 'admin', or empty")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit changes")
		return
	}

	_ = s.recordAudit(ctx, claims, "device.update", "device", &id, map[string]interface{}{
		"nickname": req.Nickname, "access_role": req.AccessRole,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleListSwitchPorts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, switch_name, port_number, label, vlan, link_status, last_change_at
		FROM switch_ports ORDER BY switch_name, port_number
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list switch ports")
		return
	}
	defer rows.Close()

	ports := []models.SwitchPort{}
	for rows.Next() {
		var p models.SwitchPort
		if err := rows.Scan(&p.ID, &p.SwitchName, &p.PortNumber, &p.Label, &p.VLAN, &p.LinkStatus, &p.LastChangeAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read switch port row")
			return
		}
		ports = append(ports, p)
	}
	writeJSON(w, http.StatusOK, ports)
}

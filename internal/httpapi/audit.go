package httpapi

import (
	"context"
	"encoding/json"

	"github.com/Akosh36/p1-3server/internal/auth"
)

// recordAudit writes one row to audit_logs. Failures are logged by the
// caller's request logger (via the returned error) but never block the
// action they describe — an audit trail gap must not take down the admin
// panel.
func (s *Server) recordAudit(ctx context.Context, claims *auth.Claims, action, targetType string, targetID *int64, details map[string]interface{}) error {
	var actorID *int64
	if claims != nil {
		actorID = &claims.AdminID
	}

	var detailsJSON []byte
	if details != nil {
		var err error
		detailsJSON, err = json.Marshal(details)
		if err != nil {
			return err
		}
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_admin_id, action, target_type, target_id, details)
		VALUES ($1, $2, $3, $4, $5)
	`, actorID, action, targetType, targetID, detailsJSON)
	return err
}

package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/mmcdole/kivo/internal/auth/dbq"
	"github.com/mmcdole/kivo/internal/id"
)

// Audit actions.
const (
	actionSetup                = "setup"
	actionSignIn               = "sign_in"
	actionSignInFailed         = "sign_in_failed"
	actionRevoke               = "revoke"
	actionRevokeOthers         = "revoke_others"
	actionUserCreated          = "user_created"
	actionPasswordSet          = "password_set"
	actionPasswordChanged      = "password_changed"
	actionPasswordChangeFailed = "password_change_failed"
	actionInviteCreated        = "invite_created"
	actionInviteRevoked        = "invite_revoked"
	actionInviteAccepted       = "invite_accepted"
	actionDeviceCode           = "device_code"
	actionAPIKeyCreated        = "api_key_created"
	actionAPIKeyRevoked        = "api_key_revoked"
	actionCastTokenCreated     = "cast_token_created"
)

var noAddr netip.Addr

// audit records an action on a user by a session or API key, in the
// caller's transaction. It never records a token, a password or an
// attempted username.
func audit(ctx context.Context, q *dbq.Queries, action string, user id.ID, by Caller, ip netip.Addr) error {
	addr := ""
	if ip.IsValid() {
		addr = ip.Unmap().String()
	}
	return q.InsertAudit(ctx, dbq.InsertAuditParams{
		ID:        id.New(),
		At:        time.Now().UnixMilli(),
		Action:    action,
		UserID:    user,
		SessionID: by.Session,
		ApiKeyID:  by.APIKey,
		RemoteIp:  addr,
	})
}

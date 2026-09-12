package mqtt

import "context"

// DynsecManager reserves the seam for managing mosquitto dynamic security
// from the backend (per-source and, later, per-device MQTT credentials over
// the $CONTROL/dynamic-security/v1 topic). v1 creates credentials out of band
// — deploy/mosquitto/dynsec-init.sh seeds the backend user and one user per
// declared source — so nothing implements this yet. The classroom controller
// runtime (per-device credentials issued at claim time, revoked at unbind)
// is the expected first implementation; keeping the interface here pins the
// shape the deploy script must eventually match.
type DynsecManager interface {
	// EnsureSourceUser creates or updates the broker credential for one
	// source (gateway/controller), scoped by ACL to its own subtree.
	EnsureSourceUser(ctx context.Context, sourceID, password string) error
	// RevokeSourceUser removes a source's broker credential.
	RevokeSourceUser(ctx context.Context, sourceID string) error
}

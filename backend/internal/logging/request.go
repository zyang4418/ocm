package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Request-id and user plumbing for the access-log chain. The access logger
// sits outside the auth middleware and Go contexts flow downward only, so the
// authenticated username travels back up through a pointer holder that the
// downstream auth middleware fills in (see WithUser).
type requestIDKey struct{}
type userKey struct{}
type userHolder struct{ name string }

// NewRequestContext derives a context carrying a fresh request id and a
// username holder for the downstream auth middleware.
func NewRequestContext(ctx context.Context) (context.Context, string) {
	id := newRequestID()
	return context.WithValue(context.WithValue(ctx, requestIDKey{}, id), userKey{}, &userHolder{}), id
}

// RequestIDFrom extracts the request id placed by NewRequestContext.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithUser records the authenticated username into the request's user holder
// so the access logger can include it in the completion line. A no-op when
// the request is not inside the access-log chain.
func WithUser(ctx context.Context, username string) {
	if h, ok := ctx.Value(userKey{}).(*userHolder); ok {
		h.name = username
	}
}

// UserFrom reads the username recorded by WithUser ("" when none).
func UserFrom(ctx context.Context) string {
	if h, ok := ctx.Value(userKey{}).(*userHolder); ok {
		return h.name
	}
	return ""
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

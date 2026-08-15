package mail

import (
	"context"
	"errors"
)

// Service is the future entry point for actually sending email. It reads the
// admin-configured settings from the store; nothing is wired to it yet — the
// current scope is configuration only. Implement Send when the first real
// consumer appears (e.g. booking reminders), using net/smtp with the
// configured host/port/encryption.
type Service struct {
	store *Store
}

func NewService(store *Store) *Service { return &Service{store: store} }

// Enabled reports whether mail sending is configured on. A Get failure counts
// as disabled — callers should treat "not enabled" as a skip, never a crash.
func (s *Service) Enabled(ctx context.Context) bool {
	cfg, err := s.store.Get(ctx)
	return err == nil && cfg.Enabled
}

// Send is a placeholder until the service goes live.
func (s *Service) Send(ctx context.Context, to, subject, html string) error {
	return errors.New("mail: send not implemented")
}

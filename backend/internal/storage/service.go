package storage

import (
	"context"
	"errors"
	"io"
)

// Service is the future entry point for actually uploading/downloading
// objects. It reads the admin-configured settings from the store; nothing is
// wired to it yet — the current scope is configuration only. Implement
// Upload/Download when the first real consumer appears (e.g. attachment or
// avatar storage), using an S3-compatible client with the configured
// endpoint/region/bucket.
type Service struct {
	store *Store
}

func NewService(store *Store) *Service { return &Service{store: store} }

// Enabled reports whether object storage is configured on. A Get failure
// counts as disabled — callers should treat "not enabled" as a skip, never a
// crash.
func (s *Service) Enabled(ctx context.Context) bool {
	cfg, err := s.store.Get(ctx)
	return err == nil && cfg.Enabled
}

// Upload is a placeholder until the service goes live.
func (s *Service) Upload(ctx context.Context, key string, body io.Reader) error {
	return errors.New("storage: upload not implemented")
}

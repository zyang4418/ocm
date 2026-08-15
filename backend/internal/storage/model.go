package storage

import "strings"

const (
	maxFieldLen  = 255
	maxRegionLen = 64
)

// Settings is the admin-editable S3-compatible object storage configuration
// (single row, id=1). secretKey is stored as configured and never leaves the
// backend: the GET response masks it and reports SecretKeySet instead.
//
// Defaults favor self-hosted/MinIO deployments (ssl + path style on); an
// admin pointed at real AWS S3 turns usePathStyle off.
type Settings struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	UseSSL        bool   `json:"useSsl"`
	UsePathStyle  bool   `json:"usePathStyle"`
	PublicBaseURL string `json:"publicBaseUrl"`
}

// Normalize trims string fields. It does not validate; callers combine it
// with validate when the service is being enabled.
func (s *Settings) Normalize() {
	s.Endpoint = strings.TrimSpace(s.Endpoint)
	s.Region = strings.TrimSpace(s.Region)
	s.Bucket = strings.TrimSpace(s.Bucket)
	s.AccessKey = strings.TrimSpace(s.AccessKey)
	s.SecretKey = strings.TrimSpace(s.SecretKey)
	s.PublicBaseURL = strings.TrimSpace(s.PublicBaseURL)
}

// validate returns an error message when the settings cannot be activated
// (enabled) or are outright malformed (always checked). A disabled service
// may keep partial configuration.
func validate(in *Settings) (string, bool) {
	if len(in.Endpoint) > maxFieldLen || len(in.Bucket) > maxFieldLen ||
		len(in.AccessKey) > maxFieldLen || len(in.SecretKey) > maxFieldLen ||
		len(in.PublicBaseURL) > maxFieldLen {
		return "field too long", false
	}
	if len(in.Region) > maxRegionLen {
		return "region too long", false
	}
	// The scheme is the useSsl flag's job; rejecting it here avoids
	// double-scheme mistakes like "https://bucket.example.com" + useSsl.
	if strings.Contains(in.Endpoint, "://") {
		return "endpoint must not contain a scheme (use the useSsl flag)", false
	}
	if !in.Enabled {
		return "", true
	}
	if in.Endpoint == "" {
		return "endpoint is required when enabled", false
	}
	if in.Bucket == "" {
		return "bucket is required when enabled", false
	}
	if in.AccessKey == "" {
		return "accessKey is required when enabled", false
	}
	return "", true
}

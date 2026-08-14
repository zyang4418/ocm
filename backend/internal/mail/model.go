package mail

import "strings"

// Encryption schemes for the SMTP connection.
const (
	EncryptionSSL      = "ssl"
	EncryptionStartTLS = "starttls"
	EncryptionNone     = "none"

	// DefaultPort is the implicit SMTP port when the admin leaves it unset
	// (ssl on 465).
	DefaultPort = 465

	maxFieldLen = 255
)

var validEncryptions = map[string]bool{
	EncryptionSSL:      true,
	EncryptionStartTLS: true,
	EncryptionNone:     true,
}

// Settings is the admin-editable SMTP configuration (single row, id=1).
// Password is stored as configured and never leaves the backend: the GET
// response masks it and reports PasswordSet instead.
type Settings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromName    string `json:"fromName"`
	FromAddress string `json:"fromAddress"`
	Encryption  string `json:"encryption"`
}

// Normalize trims string fields and applies defaults for port and encryption.
// It does not validate; callers combine it with validate when the service is
// being enabled.
func (s *Settings) Normalize() {
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	s.Password = strings.TrimSpace(s.Password)
	s.FromName = strings.TrimSpace(s.FromName)
	s.FromAddress = strings.TrimSpace(s.FromAddress)
	s.Encryption = strings.TrimSpace(s.Encryption)
	if s.Port == 0 {
		s.Port = DefaultPort
	}
	if s.Encryption == "" {
		s.Encryption = EncryptionSSL
	}
}

// validate returns an error message when the settings cannot be activated
// (enabled) or are outright malformed (always checked). A disabled service
// may keep partial configuration.
func validate(in *Settings) (string, bool) {
	if in.Port < 1 || in.Port > 65535 {
		return "port must be between 1 and 65535", false
	}
	if !validEncryptions[in.Encryption] {
		return "invalid encryption, expected ssl, starttls or none", false
	}
	if len(in.Host) > maxFieldLen || len(in.Username) > maxFieldLen ||
		len(in.Password) > maxFieldLen || len(in.FromName) > maxFieldLen ||
		len(in.FromAddress) > maxFieldLen {
		return "field too long", false
	}
	if !in.Enabled {
		return "", true
	}
	if in.Host == "" {
		return "host is required when enabled", false
	}
	if in.Username == "" {
		return "username is required when enabled", false
	}
	if in.FromAddress == "" || !strings.Contains(in.FromAddress, "@") {
		return "fromAddress must be a valid email address when enabled", false
	}
	return "", true
}

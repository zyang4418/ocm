package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/tjfoc/gmsm/sm2"
)

const (
	maxB64CipherLen = 2048
	maxRawCipherLen = 1024
	maxPlaintextLen = 256
	// Minimum length for normalized C1C3C2 ciphertext:
	// 0x04 (1B) + X(32B) + Y(32B) + C3(32B) + C2(min 1B) = 98 bytes
	minNormalizedCipherLen = 98
	// sm-crypto emits C1 as X || Y, without the uncompressed-point marker.
	minUnprefixedCipherLen = minNormalizedCipherLen - 1
)

// SM2Service provides application-layer SM2 encryption and decryption for
// sensitive transmission data such as passwords. It uses the C1C3C2 mode
// with Base64 encoding, adhering to Chinese national cryptographic standards.
type SM2Service struct {
	privKey      *sm2.PrivateKey
	canonicalPub string // 64-byte hex string (128 hex chars, X || Y)
	fingerprint  string // SHA-256(canonicalPubBytes)[:16] in uppercase (legacy prefix)
	fullDigest   string // Full 64-char SHA-256 hex digest in uppercase
}

// NewSM2Service constructs an SM2Service from hexadecimal private and public keys.
// Both keys are mandatory. It validates the key ranges, derives the public point from
// the private key, and verifies that the keypair matches.
func NewSM2Service(privHex, pubHex string) (*SM2Service, error) {
	privHex = strings.TrimSpace(privHex)
	pubHex = strings.TrimSpace(pubHex)

	if privHex == "" || pubHex == "" {
		return nil, errors.New("auth: SM2_PRIVATE_KEY and SM2_PUBLIC_KEY must both be configured")
	}

	privBytes, err := hex.DecodeString(privHex)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid SM2_PRIVATE_KEY (must be 32-byte hex string): %w", err)
	}
	if len(privBytes) != 32 {
		return nil, errors.New("auth: invalid SM2_PRIVATE_KEY (must be 32-byte hex string)")
	}

	cleanPubHex := strings.ToLower(pubHex)
	switch len(cleanPubHex) {
	case 128:
		// Canonical X || Y form. X itself may validly start with 04.
	case 130:
		if !strings.HasPrefix(cleanPubHex, "04") {
			return nil, errors.New("auth: invalid SM2_PUBLIC_KEY (65-byte key must have 04 prefix)")
		}
		cleanPubHex = cleanPubHex[2:]
	default:
		return nil, errors.New("auth: invalid SM2_PUBLIC_KEY (must be 64-byte or 65-byte hex string)")
	}
	pubBytes, err := hex.DecodeString(cleanPubHex)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid SM2_PUBLIC_KEY (must be hexadecimal): %w", err)
	}

	curve := sm2.P256Sm2()
	d := new(big.Int).SetBytes(privBytes)
	n := curve.Params().N
	if d.Sign() <= 0 || d.Cmp(n) >= 0 {
		return nil, errors.New("auth: SM2 private key D is out of valid range (0, N)")
	}

	derivedX, derivedY := curve.ScalarBaseMult(privBytes)
	expectedX := new(big.Int).SetBytes(pubBytes[:32])
	expectedY := new(big.Int).SetBytes(pubBytes[32:64])

	if derivedX.Cmp(expectedX) != 0 || derivedY.Cmp(expectedY) != 0 {
		return nil, errors.New("auth: SM2_PRIVATE_KEY and SM2_PUBLIC_KEY do not match")
	}
	if !curve.IsOnCurve(expectedX, expectedY) {
		return nil, errors.New("auth: SM2 public key is not on curve")
	}

	sum := sha256.Sum256(pubBytes)
	fullDigest := strings.ToUpper(hex.EncodeToString(sum[:]))
	fingerprint := fullDigest[:16]

	privKey := &sm2.PrivateKey{
		PublicKey: sm2.PublicKey{
			Curve: curve,
			X:     derivedX,
			Y:     derivedY,
		},
		D: d,
	}

	return &SM2Service{
		privKey:      privKey,
		canonicalPub: cleanPubHex,
		fingerprint:  fingerprint,
		fullDigest:   fullDigest,
	}, nil
}

// PublicKey returns the canonical 64-byte hex public key (128 characters) and
// the full 64-character SHA-256 fingerprint. Older clients may continue to
// compare the legacy 16-character prefix returned by earlier servers.
func (s *SM2Service) PublicKey() (string, string) {
	return s.canonicalPub, s.fullDigest
}

// LegacyFingerprint returns the historical 16-character SHA-256 prefix for
// compatibility with clients that still pin the shorter value.
func (s *SM2Service) LegacyFingerprint() string {
	return s.fingerprint
}

// FullDigest returns the full 64-character SHA-256 hex digest of the canonical public key.
func (s *SM2Service) FullDigest() string {
	return s.fullDigest
}

// Decrypt decrypts a Base64-encoded SM2 C1C3C2 ciphertext and returns the plaintext UTF-8 string.
// sm-crypto sends C1 as X||Y while older clients may include the 04 uncompressed-point
// marker, so both fully validated layouts are considered when their byte lengths permit it.
func (s *SM2Service) Decrypt(b64Cipher string) (string, error) {
	b64Cipher = strings.TrimSpace(b64Cipher)
	if len(b64Cipher) == 0 {
		return "", errors.New("empty ciphertext")
	}
	if len(b64Cipher) > maxB64CipherLen {
		return "", errors.New("ciphertext too long")
	}

	rawBytes, err := base64.StdEncoding.Strict().DecodeString(b64Cipher)
	if err != nil {
		return "", fmt.Errorf("invalid base64 ciphertext: %w", err)
	}
	if len(rawBytes) == 0 {
		return "", errors.New("decoded ciphertext is empty")
	}
	if len(rawBytes) > maxRawCipherLen {
		return "", errors.New("decoded ciphertext too long")
	}
	if len(rawBytes) < minUnprefixedCipherLen {
		return "", fmt.Errorf("ciphertext too short (%d bytes, minimum %d bytes)", len(rawBytes), minUnprefixedCipherLen)
	}

	// Web clients use the unprefixed form. It is attempted first because a valid
	// X coordinate may itself start with 04 and cannot be distinguished by that
	// byte alone from a legacy marker-prefixed ciphertext.
	unprefixed := make([]byte, len(rawBytes)+1)
	unprefixed[0] = 0x04
	copy(unprefixed[1:], rawBytes)
	candidates := [][]byte{unprefixed}
	if len(rawBytes) >= minNormalizedCipherLen && rawBytes[0] == 0x04 {
		candidates = append(candidates, rawBytes)
	}

	var plaintext string
	successes := 0
	for _, candidate := range candidates {
		decrypted, err := s.decryptNormalized(candidate)
		if err != nil {
			continue
		}
		plaintext = decrypted
		successes++
	}

	switch successes {
	case 1:
		return plaintext, nil
	case 0:
		return "", errors.New("SM2 decryption failed")
	default:
		return "", errors.New("ambiguous SM2 ciphertext format")
	}
}

func (s *SM2Service) decryptNormalized(normalized []byte) (plaintext string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sm2 decrypt panic recovered: %v", r)
		}
	}()

	if len(normalized) < minNormalizedCipherLen || normalized[0] != 0x04 {
		return "", errors.New("invalid normalized SM2 ciphertext")
	}

	// Validate C1 before the tjfoc/gmsm parser, which assumes well-formed input.
	c1x := new(big.Int).SetBytes(normalized[1:33])
	c1y := new(big.Int).SetBytes(normalized[33:65])
	if !s.privKey.Curve.IsOnCurve(c1x, c1y) || (c1x.Sign() == 0 && c1y.Sign() == 0) {
		return "", errors.New("invalid C1 elliptic curve point")
	}

	// tjfoc/gmsm uses C1C3C2 == 0, while sm-crypto calls its matching mode 1.
	decrypted, err := sm2.Decrypt(s.privKey, normalized, sm2.C1C3C2)
	if err != nil {
		return "", fmt.Errorf("sm2 decryption failed: %w", err)
	}
	if len(decrypted) == 0 {
		return "", errors.New("decrypted plaintext is empty")
	}
	if len(decrypted) > maxPlaintextLen {
		return "", errors.New("decrypted plaintext exceeds max length")
	}
	if !utf8.Valid(decrypted) {
		return "", errors.New("decrypted plaintext is not valid UTF-8")
	}
	plaintext = string(decrypted)
	if len(strings.TrimSpace(plaintext)) == 0 {
		return "", errors.New("decrypted plaintext is empty or whitespace")
	}
	return plaintext, nil
}

// Encrypt encrypts a plaintext string using the SM2 public key in C1C3C2 mode,
// returning a Base64-encoded ciphertext string.
func (s *SM2Service) Encrypt(plaintext string) (string, error) {
	if len(plaintext) == 0 {
		return "", errors.New("empty plaintext")
	}
	cipherBytes, err := sm2.Encrypt(&s.privKey.PublicKey, []byte(plaintext), rand.Reader, sm2.C1C3C2)
	if err != nil {
		return "", fmt.Errorf("sm2 encryption failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(cipherBytes), nil
}

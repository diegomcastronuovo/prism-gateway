package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// FieldEncryptionService encrypts/decrypts sensitive fields with key versioning.
type FieldEncryptionService interface {
	EncryptString(plain string) (cipher []byte, keyVersion string, err error)
	DecryptString(cipher []byte, keyVersion string) (plain string, err error)
	ActiveKeyVersion() string
}

// ErrEncryptionNotConfigured indicates no keys are configured.
var ErrEncryptionNotConfigured = errors.New("encryption not configured")

// ErrUnknownKeyVersion indicates the key version is unknown.
var ErrUnknownKeyVersion = errors.New("unknown key version")

type aesGCMService struct {
	activeVersion string
	keys          map[string][]byte
}

// NewFieldEncryptionService creates a field encryption service using raw keys.
func NewFieldEncryptionService(activeVersion string, keys map[string][]byte) (FieldEncryptionService, error) {
	if activeVersion == "" {
		return nil, fmt.Errorf("active version required")
	}
	if len(keys) == 0 {
		return nil, ErrEncryptionNotConfigured
	}
	key, ok := keys[activeVersion]
	if !ok {
		return nil, fmt.Errorf("active key version %q not configured", activeVersion)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key for %q must be 32 bytes", activeVersion)
	}
	return &aesGCMService{activeVersion: activeVersion, keys: keys}, nil
}

// NewFieldEncryptionServiceFromEnv loads keys from environment variables.
// Active version uses LOG_ENC_ACTIVE_VERSION (defaults to v1).
// Keys are loaded from LOG_ENC_KEY_V1, LOG_ENC_KEY_V2, ... (base64-encoded 32 bytes).
func NewFieldEncryptionServiceFromEnv() (FieldEncryptionService, error) {
	activeVersion := strings.TrimSpace(os.Getenv("LOG_ENC_ACTIVE_VERSION"))
	if activeVersion == "" {
		activeVersion = "v1"
	}

	keys := make(map[string][]byte)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(name, "LOG_ENC_KEY_") {
			continue
		}
		versionSuffix := strings.TrimPrefix(name, "LOG_ENC_KEY_")
		if versionSuffix == "" {
			continue
		}
		version := strings.ToLower(versionSuffix)
		keyBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		if len(keyBytes) != 32 {
			return nil, fmt.Errorf("%s must decode to 32 bytes", name)
		}
		keys[version] = keyBytes
	}

	return NewFieldEncryptionService(activeVersion, keys)
}

func (s *aesGCMService) ActiveKeyVersion() string {
	return s.activeVersion
}

func (s *aesGCMService) EncryptString(plain string) ([]byte, string, error) {
	key, ok := s.keys[s.activeVersion]
	if !ok {
		return nil, "", ErrUnknownKeyVersion
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return append(nonce, ciphertext...), s.activeVersion, nil
}

func (s *aesGCMService) DecryptString(ciphertext []byte, keyVersion string) (string, error) {
	key, ok := s.keys[keyVersion]
	if !ok {
		return "", ErrUnknownKeyVersion
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) <= nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	enc := ciphertext[nonceSize:]
	plain, err := gcm.Open(nil, nonce, enc, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

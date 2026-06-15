package encryption

import (
	"encoding/base64"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	service, err := NewFieldEncryptionService("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ct, version, err := service.EncryptString("hello")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if version != "v1" {
		t.Fatalf("version=%q, want v1", version)
	}
	plain, err := service.DecryptString(ct, version)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "hello" {
		t.Fatalf("plain=%q, want hello", plain)
	}
}

func TestEncryptNonceDiffers(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 2)
	}
	service, err := NewFieldEncryptionService("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ct1, _, err := service.EncryptString("same")
	if err != nil {
		t.Fatalf("encrypt1: %v", err)
	}
	ct2, _, err := service.EncryptString("same")
	if err != nil {
		t.Fatalf("encrypt2: %v", err)
	}
	if string(ct1) == string(ct2) {
		t.Fatal("expected different ciphertexts for same plaintext")
	}
}

func TestDecryptUnknownKeyVersionFails(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	service, err := NewFieldEncryptionService("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ct, _, err := service.EncryptString("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := service.DecryptString(ct, "v2"); err == nil {
		t.Fatal("expected error for unknown key version")
	}
}

func TestNewFieldEncryptionServiceFromEnv_Valid(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(10 + i)
	}
	b64 := base64.StdEncoding.EncodeToString(key)
	t.Setenv("LOG_ENC_ACTIVE_VERSION", "v1")
	t.Setenv("LOG_ENC_KEY_V1", b64)
	service, err := NewFieldEncryptionServiceFromEnv()
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	ct, version, err := service.EncryptString("ok")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if version != "v1" {
		t.Fatalf("version=%q, want v1", version)
	}
	plain, err := service.DecryptString(ct, "v1")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "ok" {
		t.Fatalf("plain=%q, want ok", plain)
	}
}

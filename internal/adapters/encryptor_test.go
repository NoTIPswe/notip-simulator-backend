package adapters_test

import (
	"encoding/base64"
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

func makeKey(t *testing.T) domain.EncryptionKey {
	t.Helper()
	key, err := domain.NewEncryptionKey(make([]byte, 32))
	if err != nil {
		t.Fatalf("failed to create EncryptionKey: %v", err)
	}
	return key
}

func TestAESGCMEncryptor_Encrypt_OutputIsBase64(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := makeKey(t)

	payload, err := enc.Encrypt(key, []byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for field, val := range map[string]string{
		"EncryptedData": payload.EncryptedData,
		"IV":            payload.IV,
		"AuthTag":       payload.AuthTag,
	} {
		if _, decErr := base64.StdEncoding.DecodeString(val); decErr != nil {
			t.Errorf("%s is not valid base64: %v", field, decErr)
		}
	}
}

func TestAESGCMEncryptor_Encrypt_NonEmptyFields(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := makeKey(t)

	payload, err := enc.Encrypt(key, []byte("sensor data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.EncryptedData == "" {
		t.Error("EncryptedData must not be empty")
	}
	if payload.IV == "" {
		t.Error("IV must not be empty")
	}
	if payload.AuthTag == "" {
		t.Error("AuthTag must not be empty")
	}
}

func TestAESGCMEncryptor_Encrypt_RandomIVEachCall(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := makeKey(t)

	p1, _ := enc.Encrypt(key, []byte("data"))
	p2, _ := enc.Encrypt(key, []byte("data"))

	if p1.IV == p2.IV {
		t.Error("IV should be random on each call, got identical IVs")
	}
}

func TestAESGCMEncryptor_Encrypt_EmptyPlaintext(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := makeKey(t)

	_, err := enc.Encrypt(key, []byte{})
	if err != nil {
		t.Fatalf("encrypting empty plaintext should not error: %v", err)
	}
}

func TestAESGCMEncryptor_Encrypt_LargePayload(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := makeKey(t)
	large := make([]byte, 4096)
	for i := range large {
		large[i] = byte(i % 256)
	}

	_, err := enc.Encrypt(key, large)
	if err != nil {
		t.Fatalf("unexpected error encrypting large payload: %v", err)
	}
}

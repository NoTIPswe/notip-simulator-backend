package adapters_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type innerSensorData struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func validAESKey(t *testing.T) domain.EncryptionKey {
	t.Helper()
	key, err := domain.NewEncryptionKey(make([]byte, 32))
	if err != nil {
		t.Fatalf("failed to create EncryptionKey: %v", err)
	}
	return key
}

func TestAESGCMEncryptorEncryptProducesThreeParts(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := validAESKey(t)

	payload, _ := json.Marshal(innerSensorData{Value: 42.5, Unit: "°C"})
	result, err := enc.Encrypt(key, payload)

	require.NoError(t, err)
	assert.NotEmpty(t, result.EncryptedData, "EncryptedData must not be empty")
	assert.NotEmpty(t, result.IV, "IV must not be empty")
	assert.NotEmpty(t, result.AuthTag, "AuthTag must not be empty")
}

func TestAESGCMEncryptorEncryptOutputIsBase64(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := validAESKey(t)

	payload, _ := json.Marshal(innerSensorData{Value: 10.0, Unit: "%"})
	result, err := enc.Encrypt(key, payload)
	require.NoError(t, err)

	_, err = base64.StdEncoding.DecodeString(result.EncryptedData)
	assert.NoError(t, err, "EncryptedData must be valid base64")

	_, err = base64.StdEncoding.DecodeString(result.IV)
	assert.NoError(t, err, "IV must be valid base64")

	_, err = base64.StdEncoding.DecodeString(result.AuthTag)
	assert.NoError(t, err, "AuthTag must be valid base64")
}

func TestAESGCMEncryptorEncryptIVUniquePerCall(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := validAESKey(t)
	payload, _ := json.Marshal(innerSensorData{Value: 1.0, Unit: "hPa"})

	results := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		r, err := enc.Encrypt(key, payload)
		require.NoError(t, err)
		results[r.IV] = struct{}{}
	}

	// All 50 IVs must be distinct (birthday collision probability is negligible for 12-byte IVs).
	assert.Len(t, results, 50, "each Encrypt call must produce a unique IV")
}

func TestAESGCMEncryptorEncryptDifferentKeysProduceDifferentCiphertext(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	payload, _ := json.Marshal(innerSensorData{Value: 99.9, Unit: "bpm"})

	key1, err := domain.NewEncryptionKey(make([]byte, 32))
	require.NoError(t, err)

	key2Bytes := make([]byte, 32)
	key2Bytes[0] = 0xFF
	key2, err := domain.NewEncryptionKey(key2Bytes)
	require.NoError(t, err)

	r1, err := enc.Encrypt(key1, payload)
	require.NoError(t, err)

	r2, err := enc.Encrypt(key2, payload)
	require.NoError(t, err)

	assert.NotEqual(t, r1.EncryptedData, r2.EncryptedData,
		"different keys must produce different ciphertext")
}

func TestAESGCMEncryptorInvalidKeyLength(t *testing.T) {
	// NewEncryptionKey already validates length — this tests the domain guard.
	_, err := domain.NewEncryptionKey([]byte("too-short"))
	assert.Error(t, err, "NewEncryptionKey must reject keys shorter than 32 bytes")
}

func TestAESGCMEncryptorEncryptEmptyPayload(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := validAESKey(t)

	// Encrypting an empty byte slice must not panic and must produce valid output.
	result, err := enc.Encrypt(key, []byte{})
	require.NoError(t, err)
	assert.NotEmpty(t, result.IV)
}

func TestAESGCMEncryptorEncryptLargePayload(t *testing.T) {
	enc := adapters.AESGCMEncryptor{}
	key := validAESKey(t)
	large := make([]byte, 4096)
	for i := range large {
		large[i] = byte(i % 256)
	}

	_, err := enc.Encrypt(key, large)
	if err != nil {
		t.Fatalf("unexpected error encrypting large payload: %v", err)
	}
}

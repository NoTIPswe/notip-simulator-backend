package adapters

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

type AESGCMEncryptor struct{}

func (e AESGCMEncryptor) Encrypt(key domain.EncryptionKey, value float64) (domain.EncryptedPayload, error) {
	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return domain.EncryptedPayload{}, fmt.Errorf("create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return domain.EncryptedPayload{}, fmt.Errorf("create GCM: %w", err)
	}

	iv := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return domain.EncryptedPayload{}, fmt.Errorf("generate IV: %w", err)
	}

	plaintext := math.Float64bits(value)
	plaintextBytes := []byte{
		byte(plaintext >> 56),
		byte(plaintext >> 48),
		byte(plaintext >> 40),
		byte(plaintext >> 32),
		byte(plaintext >> 24),
		byte(plaintext >> 16),
		byte(plaintext >> 8),
		byte(plaintext),
	}

	ciphertext := aesgcm.Seal(nil, iv, plaintextBytes, nil)

	// GCM appends the auth tag at the end of the ciphertext
	tagSize := aesgcm.Overhead()
	encryptedData := ciphertext[:len(ciphertext)-tagSize]
	authTag := ciphertext[len(ciphertext)-tagSize:]

	return domain.EncryptedPayload{
		EncryptedData: base64.StdEncoding.EncodeToString(encryptedData),
		IV:            base64.StdEncoding.EncodeToString(iv),
		AuthTag:       base64.StdEncoding.EncodeToString(authTag),
	}, nil
}

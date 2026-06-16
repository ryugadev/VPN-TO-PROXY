package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

// GetSecretKey retrieves key from VPN_SECRET_KEY env var.
// Returns error if missing and GIN_MODE is release.
func GetSecretKey() ([]byte, error) {
	keyStr := os.Getenv("VPN_SECRET_KEY")
	if keyStr == "" {
		if os.Getenv("GIN_MODE") == "release" {
			return nil, errors.New("VPN_SECRET_KEY environment variable is empty but GIN_MODE=release")
		}
		// Fallback for development
		keyStr = "dev-secret-key-32-bytes-long!!!"
	}

	// Pad or truncate key to 32 bytes for AES-256
	key := []byte(keyStr)
	if len(key) < 32 {
		padded := make([]byte, 32)
		copy(padded, key)
		return padded, nil
	}
	return key[:32], nil
}

// EncryptSecret encrypts plaintext using AES-256-GCM
func EncryptSecret(plainText string) (string, error) {
	key, err := GetSecretKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts hex-encoded AES-256-GCM ciphertext
func DecryptSecret(cipherText string) (string, error) {
	key, err := GetSecretKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := hex.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// MaskSecret masks activation codes in API responses (e.g. XXXX-XXXX-XXXX-ABCD)
func MaskSecret(secret string) string {
	if len(secret) <= 4 {
		return "XXXX"
	}
	// Extract last 4 chars
	last4 := secret[len(secret)-4:]
	return "XXXX-XXXX-XXXX-" + last4
}

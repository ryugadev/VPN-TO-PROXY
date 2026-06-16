package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

type ProxyCredentialManager struct{}

type Credential struct {
	Username   string
	Password   string // Plaintext password (returned only on generation/rotation)
	HashedPass string // Bcrypt hashed password (stored in DB)
}

func NewProxyCredentialManager() *ProxyCredentialManager {
	return &ProxyCredentialManager{}
}

// GenerateCredential generates a random username and password, returning both plain and hashed versions.
func (m *ProxyCredentialManager) GenerateCredential() (*Credential, error) {
	username, err := generateRandomString(8)
	if err != nil {
		return nil, err
	}
	password, err := generateRandomString(16)
	if err != nil {
		return nil, err
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %v", err)
	}

	return &Credential{
		Username:   "proxy_" + username,
		Password:   password,
		HashedPass: string(hashedBytes),
	}, nil
}

// ValidateCredential compares a plaintext password against a bcrypt hash.
func (m *ProxyCredentialManager) ValidateCredential(plainPassword, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	return err == nil
}

// RotateCredential rotates credentials by generating a new set of credentials.
func (m *ProxyCredentialManager) RotateCredential() (*Credential, error) {
	return m.GenerateCredential()
}

func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func generateRandomHex(n int) (string, error) {
	bytes, err := generateRandomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

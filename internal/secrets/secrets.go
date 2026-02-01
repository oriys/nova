package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// Cipher handles AES-256-GCM encryption/decryption
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher creates a new cipher from a hex-encoded 256-bit key
func NewCipher(hexKey string) (*Cipher, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes (256 bits), got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &Cipher{gcm: gcm}, nil
}

// NewCipherFromFile loads the master key from a file
func NewCipherFromFile(path string) (*Cipher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Trim whitespace
	key := strings.TrimSpace(string(data))
	return NewCipher(key)
}

// Encrypt encrypts plaintext using AES-256-GCM
// Returns: nonce (12 bytes) || ciphertext || tag (16 bytes)
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce
	ciphertext := c.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
// Input: nonce (12 bytes) || ciphertext || tag (16 bytes)
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	encryptedData := ciphertext[nonceSize:]

	plaintext, err := c.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// GenerateKey generates a random 256-bit key and returns it hex-encoded
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}

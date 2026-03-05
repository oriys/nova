package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Cipher handles AES-256-GCM encryption/decryption with key versioning.
// New ciphertexts are prefixed with a 2-byte key version allowing transparent
// rotation: encrypt always uses the latest key while decrypt tries all known
// versions.
type Cipher struct {
	mu       sync.RWMutex
	keys     []versionedKey // ordered by version ascending
	latest   int            // index of the key used for encryption
}

type versionedKey struct {
	version uint16
	gcm     cipher.AEAD
}

// NewCipher creates a new cipher from a hex-encoded 256-bit key (version 1).
func NewCipher(hexKey string) (*Cipher, error) {
	gcm, err := buildGCM(hexKey)
	if err != nil {
		return nil, err
	}
	c := &Cipher{
		keys:   []versionedKey{{version: 1, gcm: gcm}},
		latest: 0,
	}
	return c, nil
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

// AddKey registers an additional key version for decryption and makes it the
// active encryption key. This enables zero-downtime key rotation: first add
// the new key, re-encrypt data, then remove the old one.
func (c *Cipher) AddKey(version uint16, hexKey string) error {
	gcm, err := buildGCM(hexKey)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.keys {
		if k.version == version {
			return fmt.Errorf("key version %d already registered", version)
		}
	}
	c.keys = append(c.keys, versionedKey{version: version, gcm: gcm})
	c.latest = len(c.keys) - 1
	return nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a 2-byte version prefix.
// Output format: version (2 bytes big-endian) || nonce (12 bytes) || ciphertext || tag (16 bytes)
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	c.mu.RLock()
	k := c.keys[c.latest]
	c.mu.RUnlock()

	nonce := make([]byte, k.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// version prefix + nonce + ciphertext
	versionBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(versionBuf, k.version)
	sealed := k.gcm.Seal(nonce, nonce, plaintext, nil) // nonce || ciphertext+tag
	return append(versionBuf, sealed...), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM. It reads the 2-byte version
// prefix to select the correct key. For backward compatibility with pre-rotation
// data (no version prefix), it falls back to trying the oldest key.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(ciphertext) < 2 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	version := binary.BigEndian.Uint16(ciphertext[:2])
	for _, k := range c.keys {
		if k.version == version {
			payload := ciphertext[2:]
			nonceSize := k.gcm.NonceSize()
			if len(payload) < nonceSize {
				return nil, fmt.Errorf("ciphertext too short for nonce")
			}
			return k.gcm.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
		}
	}

	// Backward compatibility: pre-rotation ciphertexts have no version prefix.
	// Try decrypting the entire blob with the oldest (v1) key.
	if len(c.keys) > 0 {
		k := c.keys[0]
		nonceSize := k.gcm.NonceSize()
		if len(ciphertext) >= nonceSize {
			if pt, err := k.gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil); err == nil {
				return pt, nil
			}
		}
	}

	return nil, fmt.Errorf("decrypt: no matching key for version %d", version)
}

// GenerateKey generates a random 256-bit key and returns it hex-encoded
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(key), nil
}

func buildGCM(hexKey string) (cipher.AEAD, error) {
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
	return gcm, nil
}

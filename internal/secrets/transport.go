package secrets

import (
	"encoding/base64"
	"fmt"
)

const encryptedPrefix = "$ENC:"

// TransportCipher wraps a Cipher to encrypt/decrypt secret values
// for safe transport over vsock Init messages.
//
// Secret values that were resolved from $SECRET: references are encrypted
// with AES-256-GCM before being placed into the InitPayload env vars map.
// The agent side uses the same key to decrypt values prefixed with $ENC:.
type TransportCipher struct {
	cipher *Cipher
}

// NewTransportCipher creates a TransportCipher from an existing Cipher.
func NewTransportCipher(c *Cipher) *TransportCipher {
	if c == nil {
		return nil
	}
	return &TransportCipher{cipher: c}
}

// EncryptValue encrypts a single value and returns it with the $ENC: prefix.
// The encrypted data is base64-encoded for safe JSON transport.
func (tc *TransportCipher) EncryptValue(plaintext string) (string, error) {
	if tc == nil || tc.cipher == nil {
		return plaintext, nil
	}
	encrypted, err := tc.cipher.Encrypt([]byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("encrypt value: %w", err)
	}
	return encryptedPrefix + base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptValue decrypts a value that starts with $ENC: prefix.
// Non-encrypted values are returned as-is.
func (tc *TransportCipher) DecryptValue(value string) (string, error) {
	if tc == nil || tc.cipher == nil {
		return value, nil
	}
	if len(value) <= len(encryptedPrefix) || value[:len(encryptedPrefix)] != encryptedPrefix {
		return value, nil
	}
	encoded := value[len(encryptedPrefix):]
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}
	plaintext, err := tc.cipher.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt value: %w", err)
	}
	return string(plaintext), nil
}

// EncryptEnvVars encrypts the values of specified keys in the env vars map.
// secretKeys is the set of env var keys whose values were resolved from secrets.
// Returns a new map with encrypted values for the secret keys.
func (tc *TransportCipher) EncryptEnvVars(envVars map[string]string, secretKeys map[string]bool) (map[string]string, error) {
	if tc == nil || tc.cipher == nil || len(secretKeys) == 0 {
		return envVars, nil
	}
	result := make(map[string]string, len(envVars))
	for k, v := range envVars {
		if secretKeys[k] {
			encrypted, err := tc.EncryptValue(v)
			if err != nil {
				return nil, fmt.Errorf("encrypt env var %s: %w", k, err)
			}
			result[k] = encrypted
		} else {
			result[k] = v
		}
	}
	return result, nil
}

// DecryptEnvVars decrypts any $ENC:-prefixed values in the env vars map.
// Returns a new map with decrypted values.
func (tc *TransportCipher) DecryptEnvVars(envVars map[string]string) (map[string]string, error) {
	if tc == nil || tc.cipher == nil {
		return envVars, nil
	}
	result := make(map[string]string, len(envVars))
	for k, v := range envVars {
		decrypted, err := tc.DecryptValue(v)
		if err != nil {
			return nil, fmt.Errorf("decrypt env var %s: %w", k, err)
		}
		result[k] = decrypted
	}
	return result, nil
}

// IsEncryptedValue checks if a value is encrypted (has $ENC: prefix).
func IsEncryptedValue(value string) bool {
	return len(value) > len(encryptedPrefix) && value[:len(encryptedPrefix)] == encryptedPrefix
}

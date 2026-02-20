package secrets

import (
	"testing"
)

func TestTransportCipher_EncryptDecryptValue(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	tc := NewTransportCipher(cipher)

	original := "super-secret-db-password"
	encrypted, err := tc.EncryptValue(original)
	if err != nil {
		t.Fatalf("EncryptValue: %v", err)
	}

	if !IsEncryptedValue(encrypted) {
		t.Fatalf("expected encrypted value to have $ENC: prefix, got %q", encrypted)
	}

	if encrypted == original {
		t.Fatal("encrypted value should differ from original")
	}

	decrypted, err := tc.DecryptValue(encrypted)
	if err != nil {
		t.Fatalf("DecryptValue: %v", err)
	}

	if decrypted != original {
		t.Fatalf("decrypted value mismatch: got %q, want %q", decrypted, original)
	}
}

func TestTransportCipher_DecryptPlainValue(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	tc := NewTransportCipher(cipher)

	plain := "not-a-secret"
	result, err := tc.DecryptValue(plain)
	if err != nil {
		t.Fatalf("DecryptValue: %v", err)
	}
	if result != plain {
		t.Fatalf("plain value should pass through unchanged: got %q, want %q", result, plain)
	}
}

func TestTransportCipher_EncryptDecryptEnvVars(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	tc := NewTransportCipher(cipher)

	envVars := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PASSWORD": "secret123",
		"API_KEY":     "key-abc-def",
	}
	secretKeys := map[string]bool{
		"DB_PASSWORD": true,
		"API_KEY":     true,
	}

	encrypted, err := tc.EncryptEnvVars(envVars, secretKeys)
	if err != nil {
		t.Fatalf("EncryptEnvVars: %v", err)
	}

	// Non-secret should be unchanged
	if encrypted["DB_HOST"] != "localhost" {
		t.Fatalf("DB_HOST should be unchanged, got %q", encrypted["DB_HOST"])
	}

	// Secret values should be encrypted
	if !IsEncryptedValue(encrypted["DB_PASSWORD"]) {
		t.Fatal("DB_PASSWORD should be encrypted")
	}
	if !IsEncryptedValue(encrypted["API_KEY"]) {
		t.Fatal("API_KEY should be encrypted")
	}

	// Decrypt all
	decrypted, err := tc.DecryptEnvVars(encrypted)
	if err != nil {
		t.Fatalf("DecryptEnvVars: %v", err)
	}

	for k, v := range envVars {
		if decrypted[k] != v {
			t.Fatalf("key %s: got %q, want %q", k, decrypted[k], v)
		}
	}
}

func TestTransportCipher_NilCipher(t *testing.T) {
	var tc *TransportCipher

	// Nil cipher should pass through values unchanged
	val, err := tc.EncryptValue("test")
	if err != nil || val != "test" {
		t.Fatalf("nil cipher EncryptValue: val=%q, err=%v", val, err)
	}

	val, err = tc.DecryptValue("test")
	if err != nil || val != "test" {
		t.Fatalf("nil cipher DecryptValue: val=%q, err=%v", val, err)
	}

	envVars := map[string]string{"KEY": "value"}
	result, err := tc.EncryptEnvVars(envVars, map[string]bool{"KEY": true})
	if err != nil || result["KEY"] != "value" {
		t.Fatalf("nil cipher EncryptEnvVars: result=%v, err=%v", result, err)
	}
}

func TestIsEncryptedValue(t *testing.T) {
	if IsEncryptedValue("plaintext") {
		t.Fatal("plaintext should not be detected as encrypted")
	}
	if IsEncryptedValue("$ENC:") {
		t.Fatal("empty $ENC: should not be detected as encrypted")
	}
	if !IsEncryptedValue("$ENC:abc123") {
		t.Fatal("$ENC:abc123 should be detected as encrypted")
	}
}

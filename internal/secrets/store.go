package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
)

// Backend defines the storage interface for secrets
type Backend interface {
	SaveSecret(ctx context.Context, name, encryptedValue string) error
	GetSecret(ctx context.Context, name string) (string, error)
	DeleteSecret(ctx context.Context, name string) error
	ListSecrets(ctx context.Context) (map[string]string, error)
	SecretExists(ctx context.Context, name string) (bool, error)
}

// Store manages encrypted secrets in the database
type Store struct {
	backend Backend
	cipher  *Cipher
}

// NewStore creates a new secrets store
func NewStore(backend Backend, cipher *Cipher) *Store {
	return &Store{
		backend: backend,
		cipher:  cipher,
	}
}

// Set encrypts and stores a secret
func (s *Store) Set(ctx context.Context, name string, value []byte) error {
	encrypted, err := s.cipher.Encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	// Store as base64 to ensure safe storage
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	return s.backend.SaveSecret(ctx, name, encoded)
}

// Get retrieves and decrypts a secret
func (s *Store) Get(ctx context.Context, name string) ([]byte, error) {
	encoded, err := s.backend.GetSecret(ctx, name)
	if err != nil {
		return nil, err
	}

	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode secret: %w", err)
	}

	plaintext, err := s.cipher.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}

	return plaintext, nil
}

// Delete removes a secret
func (s *Store) Delete(ctx context.Context, name string) error {
	return s.backend.DeleteSecret(ctx, name)
}

// List returns all secret names with their creation times
func (s *Store) List(ctx context.Context) (map[string]string, error) {
	return s.backend.ListSecrets(ctx)
}

// Exists checks if a secret exists
func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	return s.backend.SecretExists(ctx, name)
}

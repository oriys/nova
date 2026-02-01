package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	secretPrefix = "nova:secret:"
	secretIndex  = "nova:secrets"
)

// Store manages encrypted secrets in Redis
type Store struct {
	redis  *redis.Client
	cipher *Cipher
}

// NewStore creates a new secrets store
func NewStore(redis *redis.Client, cipher *Cipher) *Store {
	return &Store{
		redis:  redis,
		cipher: cipher,
	}
}

// Set encrypts and stores a secret
func (s *Store) Set(ctx context.Context, name string, value []byte) error {
	encrypted, err := s.cipher.Encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	// Store as base64 to ensure safe Redis storage
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	pipe := s.redis.Pipeline()
	pipe.Set(ctx, secretPrefix+name, encoded, 0)
	pipe.HSet(ctx, secretIndex, name, time.Now().Format(time.RFC3339))
	_, err = pipe.Exec(ctx)
	return err
}

// Get retrieves and decrypts a secret
func (s *Store) Get(ctx context.Context, name string) ([]byte, error) {
	encoded, err := s.redis.Get(ctx, secretPrefix+name).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("secret not found: %s", name)
	}
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
	pipe := s.redis.Pipeline()
	pipe.Del(ctx, secretPrefix+name)
	pipe.HDel(ctx, secretIndex, name)
	_, err := pipe.Exec(ctx)
	return err
}

// List returns all secret names with their creation times
func (s *Store) List(ctx context.Context) (map[string]string, error) {
	return s.redis.HGetAll(ctx, secretIndex).Result()
}

// Exists checks if a secret exists
func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	n, err := s.redis.Exists(ctx, secretPrefix+name).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

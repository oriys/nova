package secrets

import (
	"context"
	"fmt"
	"strings"
)

const secretRefPrefix = "$SECRET:"

// Resolver resolves $SECRET:name references to actual values
type Resolver struct {
	store *Store
}

// NewResolver creates a new secret resolver
func NewResolver(store *Store) *Resolver {
	return &Resolver{store: store}
}

// ResolveEnvVars resolves all $SECRET: references in environment variables
// Returns a new map with secrets resolved
func (r *Resolver) ResolveEnvVars(ctx context.Context, envVars map[string]string) (map[string]string, error) {
	if len(envVars) == 0 {
		return envVars, nil
	}

	resolved := make(map[string]string, len(envVars))
	for k, v := range envVars {
		resolvedValue, err := r.ResolveValue(ctx, v)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", k, err)
		}
		resolved[k] = resolvedValue
	}

	return resolved, nil
}

// ResolveValue resolves a single value that may contain $SECRET:name reference
func (r *Resolver) ResolveValue(ctx context.Context, value string) (string, error) {
	if !strings.HasPrefix(value, secretRefPrefix) {
		return value, nil
	}

	secretName := strings.TrimPrefix(value, secretRefPrefix)
	if secretName == "" {
		return "", fmt.Errorf("empty secret name in reference")
	}

	secretValue, err := r.store.Get(ctx, secretName)
	if err != nil {
		return "", fmt.Errorf("get secret '%s': %w", secretName, err)
	}

	return string(secretValue), nil
}

// IsSecretRef checks if a value is a secret reference
func IsSecretRef(value string) bool {
	return strings.HasPrefix(value, secretRefPrefix)
}

// ExtractSecretName extracts the secret name from a reference
func ExtractSecretName(value string) string {
	if !strings.HasPrefix(value, secretRefPrefix) {
		return ""
	}
	return strings.TrimPrefix(value, secretRefPrefix)
}

// ListSecretRefs returns all secret names referenced in the environment variables
func ListSecretRefs(envVars map[string]string) []string {
	var refs []string
	for _, v := range envVars {
		if name := ExtractSecretName(v); name != "" {
			refs = append(refs, name)
		}
	}
	return refs
}

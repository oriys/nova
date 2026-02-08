package auth

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// JWTAuthenticator validates JWT tokens
type JWTAuthenticator struct {
	algorithm string
	hmacKey   []byte
	rsaPubKey *rsa.PublicKey
	issuer    string
}

// JWTConfig holds JWT authenticator configuration
type JWTAuthConfig struct {
	Algorithm     string // HS256, RS256
	Secret        string // HMAC secret
	PublicKeyFile string // RSA public key file
	Issuer        string // Optional issuer validation
}

// NewJWTAuthenticator creates a new JWT authenticator
func NewJWTAuthenticator(cfg JWTAuthConfig) (*JWTAuthenticator, error) {
	auth := &JWTAuthenticator{
		algorithm: cfg.Algorithm,
		issuer:    cfg.Issuer,
	}

	switch cfg.Algorithm {
	case "HS256":
		if cfg.Secret == "" {
			return nil, fmt.Errorf("JWT secret required for HS256")
		}
		auth.hmacKey = []byte(cfg.Secret)

	case "RS256":
		if cfg.PublicKeyFile == "" {
			return nil, fmt.Errorf("public key file required for RS256")
		}
		pubKey, err := loadRSAPublicKey(cfg.PublicKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load public key: %w", err)
		}
		auth.rsaPubKey = pubKey

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", cfg.Algorithm)
	}

	return auth, nil
}

// Authenticate implements Authenticator
func (a *JWTAuthenticator) Authenticate(r *http.Request) *Identity {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}

	// Must be Bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Parse and validate token
	claims, err := a.validateToken(token)
	if err != nil {
		return nil
	}

	// Build identity
	subject := "unknown"
	if sub, ok := claims["sub"].(string); ok {
		subject = sub
	}

	tier := "default"
	if t, ok := claims["tier"].(string); ok {
		tier = t
	}

	return &Identity{
		Subject:       "user:" + subject,
		Tier:          tier,
		Claims:        claims,
		Policies:      extractPoliciesFromClaims(claims),
		AllowedScopes: extractTenantScopesFromClaims(claims),
	}
}

// validateToken parses and validates a JWT token
func (a *JWTAuthenticator) validateToken(tokenStr string) (map[string]any, error) {
	// Split token into parts
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerB64, payloadB64, signatureB64 := parts[0], parts[1], parts[2]

	// Decode header
	headerBytes, err := base64URLDecode(headerB64)
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	// Verify algorithm matches
	if header.Alg != a.algorithm {
		return nil, fmt.Errorf("algorithm mismatch: expected %s, got %s", a.algorithm, header.Alg)
	}

	// Decode signature
	signature, err := base64URLDecode(signatureB64)
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	// Verify signature
	signingInput := headerB64 + "." + payloadB64
	if err := a.verifySignature(signingInput, signature); err != nil {
		return nil, fmt.Errorf("verify signature: %w", err)
	}

	// Decode payload
	payloadBytes, err := base64URLDecode(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}

	// Validate standard claims
	now := time.Now().Unix()

	// Check expiration
	if exp, ok := claims["exp"].(float64); ok {
		if int64(exp) < now {
			return nil, fmt.Errorf("token expired")
		}
	}

	// Check not-before
	if nbf, ok := claims["nbf"].(float64); ok {
		if int64(nbf) > now {
			return nil, fmt.Errorf("token not yet valid")
		}
	}

	// Check issuer if configured
	if a.issuer != "" {
		if iss, ok := claims["iss"].(string); ok {
			if iss != a.issuer {
				return nil, fmt.Errorf("issuer mismatch")
			}
		} else {
			return nil, fmt.Errorf("missing issuer claim")
		}
	}

	return claims, nil
}

// verifySignature verifies the token signature
func (a *JWTAuthenticator) verifySignature(input string, signature []byte) error {
	switch a.algorithm {
	case "HS256":
		return a.verifyHS256(input, signature)
	case "RS256":
		return a.verifyRS256(input, signature)
	default:
		return fmt.Errorf("unsupported algorithm")
	}
}

// verifyHS256 verifies HMAC-SHA256 signature
func (a *JWTAuthenticator) verifyHS256(input string, signature []byte) error {
	mac := hmac.New(sha256.New, a.hmacKey)
	mac.Write([]byte(input))
	expected := mac.Sum(nil)

	if !hmac.Equal(signature, expected) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// verifyRS256 verifies RSA-SHA256 signature
func (a *JWTAuthenticator) verifyRS256(input string, signature []byte) error {
	hashed := sha256.Sum256([]byte(input))
	return rsa.VerifyPKCS1v15(a.rsaPubKey, crypto.SHA256, hashed[:], signature)
}

// base64URLDecode decodes base64url encoded string
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// loadRSAPublicKey loads an RSA public key from a PEM file
func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPub, nil
}

// extractPoliciesFromClaims builds PolicyBindings from JWT claims.
// Supports "role" claim (string) and "policies" claim (array of PolicyBinding).
func extractPoliciesFromClaims(claims map[string]any) []domain.PolicyBinding {
	var policies []domain.PolicyBinding

	// Check for "role" claim (simple role assignment)
	if role, ok := claims["role"].(string); ok {
		r := domain.Role(role)
		if domain.ValidRole(r) {
			policies = append(policies, domain.PolicyBinding{Role: r})
		}
	}

	// Check for "policies" claim (detailed policies)
	if rawPolicies, ok := claims["policies"]; ok {
		if data, err := json.Marshal(rawPolicies); err == nil {
			var claimPolicies []domain.PolicyBinding
			if err := json.Unmarshal(data, &claimPolicies); err == nil {
				policies = append(policies, claimPolicies...)
			}
		}
	}

	return policies
}

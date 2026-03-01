package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/httpjson"
	"github.com/oriys/nova/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints (register, login, change-password).
type AuthHandler struct {
	Store     *store.Store
	JWTSecret string // HS256 signing key (same as auth config jwt.secret)

	// revokedTokens holds JTIs of revoked tokens with their expiry times.
	revokedMu     sync.RWMutex
	revokedTokens map[string]time.Time
}

// IsTokenRevoked checks if a token has been revoked by JTI or raw token.
func (h *AuthHandler) IsTokenRevoked(tokenID string) bool {
	h.revokedMu.RLock()
	defer h.revokedMu.RUnlock()
	if h.revokedTokens == nil {
		return false
	}
	exp, ok := h.revokedTokens[tokenID]
	if !ok {
		return false
	}
	// Clean up expired entries lazily
	if time.Now().After(exp) {
		return false
	}
	return true
}

func (h *AuthHandler) revokeToken(tokenID string, expiry time.Time) {
	h.revokedMu.Lock()
	defer h.revokedMu.Unlock()
	if h.revokedTokens == nil {
		h.revokedTokens = make(map[string]time.Time)
	}
	h.revokedTokens[tokenID] = expiry
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.Register)
	mux.HandleFunc("POST /auth/login", h.Login)
	mux.HandleFunc("POST /auth/logout", h.Logout)
	mux.HandleFunc("POST /auth/change-password", h.ChangePassword)
}

// Register handles POST /auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID    string `json:"tenant_id"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		httpjson.Error(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	// Prevent registration with the default tenant ID to avoid
	// auto-granting admin role via issueToken.
	if tenantID == store.DefaultTenantID {
		httpjson.Error(w, http.StatusForbidden, "cannot register with the default tenant ID")
		return
	}
	if req.Password == "" {
		httpjson.Error(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < 8 {
		httpjson.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Check if tenant already exists
	existing, _ := h.Store.GetTenant(r.Context(), tenantID)
	if existing != nil {
		httpjson.Error(w, http.StatusConflict, "tenant already exists")
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Create tenant with password hash
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = tenantID
	}
	_, err = h.Store.CreateTenant(r.Context(), &store.TenantRecord{
		ID:           tenantID,
		Name:         name,
		PasswordHash: string(hash),
	})
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Generate JWT
	token, err := h.issueToken(tenantID)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"token":     token,
		"tenant_id": tenantID,
	})
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string `json:"tenant_id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		httpjson.Error(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	if req.Password == "" {
		httpjson.Error(w, http.StatusBadRequest, "password is required")
		return
	}

	tenant, err := h.Store.GetTenant(r.Context(), tenantID)
	if err != nil {
		logging.Op().Warn("login failed: tenant not found", "tenant", tenantID, "ip", r.RemoteAddr)
		httpjson.Error(w, http.StatusUnauthorized, "invalid tenant or password")
		return
	}

	// Verify password. For bootstrap tenants with empty password hash, reject
	// login and require explicit password setup via registration.
	if tenant.PasswordHash == "" {
		logging.Op().Warn("login failed: bootstrap tenant without password", "tenant", tenantID, "ip", r.RemoteAddr)
		httpjson.Error(w, http.StatusUnauthorized, "tenant requires password setup, please register first")
		return
	} else if err := bcrypt.CompareHashAndPassword([]byte(tenant.PasswordHash), []byte(req.Password)); err != nil {
		logging.Op().Warn("login failed: invalid password", "tenant", tenantID, "ip", r.RemoteAddr)
		httpjson.Error(w, http.StatusUnauthorized, "invalid tenant or password")
		return
	}

	// Generate JWT
	token, err := h.issueToken(tenantID)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	logging.Op().Info("login successful", "tenant", tenantID, "ip", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":     token,
		"tenant_id": tenant.ID,
	})
}

// Logout handles POST /auth/logout — revokes the current token.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	if identity != nil && identity.Subject != "" {
		// Revoke the token for 24 hours (matching token TTL)
		h.revokeToken(identity.Subject+":"+r.Header.Get("Authorization"), time.Now().Add(24*time.Hour))
		logging.Op().Info("user logged out", "tenant", identity.Subject)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ChangePassword handles POST /auth/change-password (requires valid JWT)
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	if identity == nil {
		httpjson.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		httpjson.Error(w, http.StatusBadRequest, "old_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		httpjson.Error(w, http.StatusBadRequest, "new_password must be at least 8 characters")
		return
	}

	// Extract tenant ID from identity subject (format: "user:tenantID")
	tenantID := strings.TrimPrefix(identity.Subject, "user:")
	if tenantID == "" || tenantID == identity.Subject {
		httpjson.Error(w, http.StatusBadRequest, "cannot determine tenant from identity")
		return
	}

	tenant, err := h.Store.GetTenant(r.Context(), tenantID)
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, "tenant not found")
		return
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(tenant.PasswordHash), []byte(req.OldPassword)); err != nil {
		httpjson.Error(w, http.StatusUnauthorized, "old password is incorrect")
		return
	}

	// Hash and set new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.Store.SetTenantPasswordHash(r.Context(), tenantID, string(hash)); err != nil {
		httpjson.Error(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *AuthHandler) issueToken(tenantID string) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"sub":       tenantID,
		"tenant_id": tenantID,
		"role":      "operator",
		"iss":       "nova",
		"iat":       now.Unix(),
		"exp":       now.Add(24 * time.Hour).Unix(),
		"allowed_tenants": []string{tenantID},
	}
	if tenantID == store.DefaultTenantID {
		claims["role"] = "admin"
	}
	return auth.SignToken([]byte(h.JWTSecret), claims)
}

package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints (register, login, change-password).
type AuthHandler struct {
	Store     *store.Store
	JWTSecret string // HS256 signing key (same as auth config jwt.secret)
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
		writeAuthError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		writeAuthError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	if req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < 4 {
		writeAuthError(w, http.StatusBadRequest, "password must be at least 4 characters")
		return
	}

	// Check if tenant already exists
	existing, _ := h.Store.GetTenant(r.Context(), tenantID)
	if existing != nil {
		writeAuthError(w, http.StatusConflict, "tenant already exists")
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to hash password")
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
		writeAuthError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Generate JWT
	token, err := h.issueToken(tenantID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to generate token")
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
		writeAuthError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		writeAuthError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	if req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "password is required")
		return
	}

	tenant, err := h.Store.GetTenant(r.Context(), tenantID)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "invalid tenant or password")
		return
	}

	// Verify password. For bootstrap tenants with empty password hash, allow
	// tenant-id-as-password once and persist the hashed password.
	if tenant.PasswordHash == "" {
		if req.Password != tenant.ID {
			writeAuthError(w, http.StatusUnauthorized, "invalid tenant or password")
			return
		}
		if hash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost); hashErr == nil {
			_ = h.Store.SetTenantPasswordHash(r.Context(), tenant.ID, string(hash))
		}
	} else if err := bcrypt.CompareHashAndPassword([]byte(tenant.PasswordHash), []byte(req.Password)); err != nil {
		writeAuthError(w, http.StatusUnauthorized, "invalid tenant or password")
		return
	}

	// Generate JWT
	token, err := h.issueToken(tenantID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":     token,
		"tenant_id": tenant.ID,
	})
}

// Logout handles POST /auth/logout (stateless JWT — client discards token)
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ChangePassword handles POST /auth/change-password (requires valid JWT)
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	if identity == nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		writeAuthError(w, http.StatusBadRequest, "old_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 4 {
		writeAuthError(w, http.StatusBadRequest, "new_password must be at least 4 characters")
		return
	}

	// Extract tenant ID from identity subject (format: "user:tenantID")
	tenantID := strings.TrimPrefix(identity.Subject, "user:")
	if tenantID == "" || tenantID == identity.Subject {
		writeAuthError(w, http.StatusBadRequest, "cannot determine tenant from identity")
		return
	}

	tenant, err := h.Store.GetTenant(r.Context(), tenantID)
	if err != nil {
		writeAuthError(w, http.StatusNotFound, "tenant not found")
		return
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(tenant.PasswordHash), []byte(req.OldPassword)); err != nil {
		writeAuthError(w, http.StatusUnauthorized, "old password is incorrect")
		return
	}

	// Hash and set new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.Store.SetTenantPasswordHash(r.Context(), tenantID, string(hash)); err != nil {
		writeAuthError(w, http.StatusInternalServerError, err.Error())
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

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

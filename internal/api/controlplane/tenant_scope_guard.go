package controlplane

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/store"
)

func enforceTenantAccess(w http.ResponseWriter, r *http.Request, tenantID string) bool {
	identity := auth.GetIdentity(r.Context())
	if identity == nil || !identity.ScopeRestricted() {
		return true
	}

	tenantID = strings.TrimSpace(tenantID)
	for _, scope := range identity.AllowedScopes {
		if scope.TenantID == "*" || scope.TenantID == tenantID {
			return true
		}
	}

	writeTenantForbidden(w)
	return false
}

func enforceNamespaceAccess(w http.ResponseWriter, r *http.Request, tenantID, namespace string) bool {
	identity := auth.GetIdentity(r.Context())
	if identity == nil || !identity.ScopeRestricted() {
		return true
	}

	if identity.AllowsScope(tenantID, namespace) {
		return true
	}

	writeTenantForbidden(w)
	return false
}

func visibleTenantIDs(identity *auth.Identity) ([]string, bool) {
	if identity == nil || !identity.ScopeRestricted() {
		return nil, true
	}

	ids := make(map[string]struct{}, len(identity.AllowedScopes))
	for _, scope := range identity.AllowedScopes {
		if scope.TenantID == "*" {
			return nil, true
		}
		ids[scope.TenantID] = struct{}{}
	}

	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, false
}

func filterVisibleNamespaces(identity *auth.Identity, tenantID string, namespaces []*store.NamespaceRecord) []*store.NamespaceRecord {
	if identity == nil || !identity.ScopeRestricted() {
		return namespaces
	}
	filtered := make([]*store.NamespaceRecord, 0, len(namespaces))
	for _, ns := range namespaces {
		if identity.AllowsScope(tenantID, ns.Name) {
			filtered = append(filtered, ns)
		}
	}
	return filtered
}

func writeTenantForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "forbidden",
		"message": "tenant scope is not allowed for this identity",
	})
}

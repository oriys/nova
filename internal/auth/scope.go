package auth

import (
	"encoding/json"
	"slices"
	"strings"
)

const (
	scopeWildcard          = "*"
	defaultTenantID        = "default"
	defaultTenantNamespace = "default"
)

// TenantScope defines a tenant/namespace authorization boundary.
// A value of "*" for either field means wildcard.
type TenantScope struct {
	TenantID  string `json:"tenant_id"`
	Namespace string `json:"namespace"`
}

func normalizeTenantScope(scope TenantScope) TenantScope {
	tenantID := strings.TrimSpace(scope.TenantID)
	namespace := strings.TrimSpace(scope.Namespace)

	if tenantID == "" {
		tenantID = defaultTenantID
	}
	if namespace == "" {
		namespace = defaultTenantNamespace
	}

	return TenantScope{
		TenantID:  tenantID,
		Namespace: namespace,
	}
}

func normalizeTenantScopeWithWildcard(scope TenantScope) TenantScope {
	tenantID := strings.TrimSpace(scope.TenantID)
	namespace := strings.TrimSpace(scope.Namespace)

	if tenantID == "" {
		tenantID = defaultTenantID
	}
	if namespace == "" {
		namespace = defaultTenantNamespace
	}
	if tenantID == scopeWildcard {
		tenantID = scopeWildcard
	}
	if namespace == scopeWildcard {
		namespace = scopeWildcard
	}

	return TenantScope{
		TenantID:  tenantID,
		Namespace: namespace,
	}
}

func (s TenantScope) Allows(tenantID, namespace string) bool {
	requested := normalizeTenantScope(TenantScope{TenantID: tenantID, Namespace: namespace})
	allowed := normalizeTenantScopeWithWildcard(s)

	tenantMatch := allowed.TenantID == scopeWildcard || allowed.TenantID == requested.TenantID
	namespaceMatch := allowed.Namespace == scopeWildcard || allowed.Namespace == requested.Namespace
	return tenantMatch && namespaceMatch
}

func dedupeScopes(scopes []TenantScope) []TenantScope {
	if len(scopes) == 0 {
		return nil
	}
	uniq := make([]TenantScope, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		norm := normalizeTenantScopeWithWildcard(scope)
		key := norm.TenantID + ":" + norm.Namespace
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, norm)
	}
	slices.SortFunc(uniq, func(a, b TenantScope) int {
		if a.TenantID != b.TenantID {
			return strings.Compare(a.TenantID, b.TenantID)
		}
		return strings.Compare(a.Namespace, b.Namespace)
	})
	return uniq
}

func parseClaimStringArray(claim any) []string {
	switch v := claim.(type) {
	case []string:
		result := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					result = append(result, s)
				}
			}
		}
		return result
	default:
		return nil
	}
}

func parseAllowedScopesClaim(claim any) []TenantScope {
	if claim == nil {
		return nil
	}

	var scopes []TenantScope
	switch v := claim.(type) {
	case []TenantScope:
		return dedupeScopes(v)
	case []any:
		for _, item := range v {
			scopes = append(scopes, parseScopeItem(item)...)
		}
	default:
		scopes = append(scopes, parseScopeItem(v)...)
	}

	return dedupeScopes(scopes)
}

func parseScopeItem(item any) []TenantScope {
	switch v := item.(type) {
	case string:
		scope, ok := parseScopeString(v)
		if !ok {
			return nil
		}
		return []TenantScope{scope}
	case map[string]any:
		tenantID, _ := v["tenant_id"].(string)
		namespace, _ := v["namespace"].(string)
		if tenantID == "" {
			if value, ok := v["tenant"].(string); ok {
				tenantID = value
			}
		}
		if namespace == "" {
			if value, ok := v["ns"].(string); ok {
				namespace = value
			}
		}
		if strings.TrimSpace(tenantID) == "" && strings.TrimSpace(namespace) == "" {
			return nil
		}
		return []TenantScope{normalizeTenantScopeWithWildcard(TenantScope{TenantID: tenantID, Namespace: namespace})}
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var mapped map[string]any
		if err := json.Unmarshal(data, &mapped); err != nil {
			return nil
		}
		return parseScopeItem(mapped)
	}
}

func parseScopeString(raw string) (TenantScope, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return TenantScope{}, false
	}

	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		return normalizeTenantScopeWithWildcard(TenantScope{TenantID: parts[0], Namespace: parts[1]}), true
	}
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		return normalizeTenantScopeWithWildcard(TenantScope{TenantID: parts[0], Namespace: parts[1]}), true
	}
	return normalizeTenantScopeWithWildcard(TenantScope{TenantID: s, Namespace: scopeWildcard}), true
}

func extractTenantScopesFromClaims(claims map[string]any) []TenantScope {
	if len(claims) == 0 {
		return nil
	}

	if scopes := parseAllowedScopesClaim(claims["allowed_scopes"]); len(scopes) > 0 {
		return scopes
	}

	allowedTenants := parseClaimStringArray(claims["allowed_tenants"])
	allowedNamespaces := parseClaimStringArray(claims["allowed_namespaces"])
	if len(allowedTenants) > 0 {
		scopes := make([]TenantScope, 0, len(allowedTenants)*max(1, len(allowedNamespaces)))
		if len(allowedNamespaces) == 0 {
			for _, tenantID := range allowedTenants {
				scopes = append(scopes, TenantScope{TenantID: tenantID, Namespace: scopeWildcard})
			}
		} else {
			for _, tenantID := range allowedTenants {
				for _, namespace := range allowedNamespaces {
					scopes = append(scopes, TenantScope{TenantID: tenantID, Namespace: namespace})
				}
			}
		}
		return dedupeScopes(scopes)
	}

	tenantID, _ := claims["tenant_id"].(string)
	namespace, _ := claims["namespace"].(string)

	tenantID = strings.TrimSpace(tenantID)
	namespace = strings.TrimSpace(namespace)

	if len(allowedNamespaces) > 0 && tenantID != "" {
		scopes := make([]TenantScope, 0, len(allowedNamespaces))
		for _, ns := range allowedNamespaces {
			scopes = append(scopes, TenantScope{TenantID: tenantID, Namespace: ns})
		}
		return dedupeScopes(scopes)
	}

	if tenantID != "" && namespace != "" {
		return []TenantScope{normalizeTenantScopeWithWildcard(TenantScope{TenantID: tenantID, Namespace: namespace})}
	}
	if tenantID != "" {
		return []TenantScope{normalizeTenantScopeWithWildcard(TenantScope{TenantID: tenantID, Namespace: scopeWildcard})}
	}
	if namespace != "" {
		return []TenantScope{normalizeTenantScopeWithWildcard(TenantScope{TenantID: scopeWildcard, Namespace: namespace})}
	}
	return nil
}

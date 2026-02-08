export const DEFAULT_TENANT_ID = "default";
export const DEFAULT_NAMESPACE = "default";

const TENANT_STORAGE_KEY = "nova.tenant_id";
const NAMESPACE_STORAGE_KEY = "nova.namespace";
const TENANT_PART_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;

export interface TenantScope {
  tenantId: string;
  namespace: string;
}

function normalizeScopePart(value: string | undefined, fallback: string): string {
  const trimmed = (value || "").trim();
  if (!trimmed || !TENANT_PART_PATTERN.test(trimmed)) {
    return fallback;
  }
  return trimmed;
}

export function normalizeTenantScope(scope?: Partial<TenantScope>): TenantScope {
  return {
    tenantId: normalizeScopePart(scope?.tenantId, DEFAULT_TENANT_ID),
    namespace: normalizeScopePart(scope?.namespace, DEFAULT_NAMESPACE),
  };
}

export function getTenantScope(): TenantScope {
  if (typeof window === "undefined") {
    return { tenantId: DEFAULT_TENANT_ID, namespace: DEFAULT_NAMESPACE };
  }
  try {
    return normalizeTenantScope({
      tenantId: window.localStorage.getItem(TENANT_STORAGE_KEY) || undefined,
      namespace: window.localStorage.getItem(NAMESPACE_STORAGE_KEY) || undefined,
    });
  } catch {
    return { tenantId: DEFAULT_TENANT_ID, namespace: DEFAULT_NAMESPACE };
  }
}

export function setTenantScope(scope: Partial<TenantScope>): TenantScope {
  const normalized = normalizeTenantScope(scope);
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(TENANT_STORAGE_KEY, normalized.tenantId);
      window.localStorage.setItem(NAMESPACE_STORAGE_KEY, normalized.namespace);
      window.dispatchEvent(
        new CustomEvent("nova:tenant-scope-changed", { detail: normalized })
      );
    } catch {
      // Ignore storage failures and continue with normalized fallback scope.
    }
  }
  return normalized;
}

export function getTenantScopeHeaders(): Record<string, string> {
  const scope = getTenantScope();
  return {
    "X-Nova-Tenant": scope.tenantId,
    "X-Nova-Namespace": scope.namespace,
  };
}

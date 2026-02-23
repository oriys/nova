import {
  DEFAULT_NAMESPACE,
  DEFAULT_TENANT_ID,
  getTenantScope,
  setTenantScope,
} from "@/lib/tenant-scope";

const AUTH_STORAGE_KEY = "nova.auth.session";
const AUTH_TOKEN_KEY = "nova.auth.token";
export const AUTH_CHANGED_EVENT = "nova:auth-changed";
const AUTH_DISABLED = false;
const TENANT_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const API_BASE = "/api";

type UserRole = "super-admin" | "operator" | "viewer";

export interface AuthSession {
  username: string;
  displayName: string;
  role: UserRole;
  canAccessAllTenants: boolean;
  tenantIds: string[];
  loggedInAt: string;
}

export interface LoginAccountHint {
  username: string;
  password: string;
  note: string;
}

function normalizeTenantID(tenantID: string): string {
  return tenantID.trim();
}

function normalizeTenantIDs(tenantIDs: string[] | undefined): string[] {
  if (!tenantIDs || tenantIDs.length === 0) {
    return [];
  }
  const unique = new Set<string>();
  tenantIDs.forEach((id) => {
    const normalized = id.trim();
    if (normalized) {
      unique.add(normalized);
    }
  });
  return Array.from(unique);
}

async function requestAuthAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers);
  if (!(options?.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  headers.set("X-Nova-Tenant", DEFAULT_TENANT_ID);
  headers.set("X-Nova-Namespace", DEFAULT_NAMESPACE);

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const text = await response.text();
    if (text) {
      let parsed: Record<string, unknown> | null = null;
      try {
        parsed = JSON.parse(text) as Record<string, unknown>;
      } catch {
        parsed = null;
      }
      if (parsed && typeof parsed.error === "string" && parsed.error.trim()) {
        throw new Error(parsed.error.trim());
      }
      if (parsed && typeof parsed.message === "string" && parsed.message.trim()) {
        throw new Error(parsed.message.trim());
      }
      if (!parsed) {
        throw new Error(text.trim() || `Request failed: ${response.status}`);
      }
      throw new Error(`Request failed: ${response.status}`);
    }
    throw new Error(`Request failed: ${response.status}`);
  }

  if (response.status === 204 || response.status === 205) {
    return undefined as T;
  }
  const raw = await response.text();
  if (!raw.trim()) {
    return undefined as T;
  }
  return JSON.parse(raw) as T;
}

function emitAuthChanged(session: AuthSession | null): void {
  if (typeof window === "undefined") {
    return;
  }
  window.dispatchEvent(new CustomEvent(AUTH_CHANGED_EVENT, { detail: session }));
}

function parseStoredSession(raw: string | null): AuthSession | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as Partial<AuthSession>;
    if (
      typeof parsed.username !== "string" ||
      typeof parsed.displayName !== "string" ||
      typeof parsed.role !== "string" ||
      typeof parsed.canAccessAllTenants !== "boolean" ||
      !Array.isArray(parsed.tenantIds) ||
      typeof parsed.loggedInAt !== "string"
    ) {
      return null;
    }
    return {
      username: parsed.username,
      displayName: parsed.displayName,
      role: parsed.role as UserRole,
      canAccessAllTenants: parsed.canAccessAllTenants,
      tenantIds: normalizeTenantIDs(parsed.tenantIds),
      loggedInAt: parsed.loggedInAt,
    };
  } catch {
    return null;
  }
}

export function getAuthSession(): AuthSession | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    return parseStoredSession(window.localStorage.getItem(AUTH_STORAGE_KEY));
  } catch {
    return null;
  }
}

export function isAuthenticated(): boolean {
  return AUTH_DISABLED || Boolean(getAuthSession());
}

export function isSuperUser(session: AuthSession | null = getAuthSession()): boolean {
  return Boolean(session?.canAccessAllTenants);
}

export function canAccessTenant(
  tenantID: string,
  session: AuthSession | null = getAuthSession(),
): boolean {
  if (!session) {
    return false;
  }
  if (session.canAccessAllTenants) {
    return true;
  }
  const normalizedTenantID = tenantID.trim();
  if (!normalizedTenantID) {
    return false;
  }
  return session.tenantIds.includes(normalizedTenantID);
}

export function filterTenantsForSession<T extends { id: string }>(
  tenants: T[],
  session: AuthSession | null = getAuthSession(),
): T[] {
  if (AUTH_DISABLED) {
    return tenants;
  }
  if (!session) {
    return [];
  }
  if (session.canAccessAllTenants) {
    return tenants;
  }
  const allowed = new Set(normalizeTenantIDs(session.tenantIds));
  return tenants.filter((tenant) => allowed.has(tenant.id));
}

export function syncTenantScopeWithSession(session: AuthSession | null = getAuthSession()): void {
  if (typeof window === "undefined") {
    return;
  }

  if (!session) {
    setTenantScope({ tenantId: DEFAULT_TENANT_ID, namespace: DEFAULT_NAMESPACE });
    return;
  }

  if (session.canAccessAllTenants) {
    return;
  }

  const allowedTenantIDs = normalizeTenantIDs(session.tenantIds);
  if (allowedTenantIDs.length === 0) {
    return;
  }

  const currentScope = getTenantScope();
  if (allowedTenantIDs.includes(currentScope.tenantId)) {
    return;
  }

  setTenantScope({
    tenantId: allowedTenantIDs[0],
    namespace: DEFAULT_NAMESPACE,
  });
}

export async function login(username: string, password: string): Promise<AuthSession> {
  if (typeof window === "undefined") {
    throw new Error("Login is only available in browser.");
  }

  const requestedTenantID = normalizeTenantID(username);
  if (!requestedTenantID) {
    throw new Error("Tenant is required.");
  }
  if (!password) {
    throw new Error("Password is required.");
  }

  const result = await requestAuthAPI<{ token: string; tenant_id: string }>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ tenant_id: requestedTenantID, password }),
  });

  const isDefaultTenant = result.tenant_id === DEFAULT_TENANT_ID;
  const session: AuthSession = {
    username: result.tenant_id,
    displayName: result.tenant_id,
    role: isDefaultTenant ? "super-admin" : "operator",
    canAccessAllTenants: isDefaultTenant,
    tenantIds: normalizeTenantIDs([result.tenant_id]),
    loggedInAt: new Date().toISOString(),
  };

  window.localStorage.setItem(AUTH_TOKEN_KEY, result.token);
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session));
  setTenantScope({ tenantId: result.tenant_id, namespace: DEFAULT_NAMESPACE });
  emitAuthChanged(session);
  return session;
}

export async function registerTenant(
  tenantID: string,
  password: string,
  displayName?: string,
): Promise<AuthSession> {
  if (typeof window === "undefined") {
    throw new Error("Registration is only available in browser.");
  }
  const normalizedTenantID = normalizeTenantID(tenantID);
  if (!normalizedTenantID) {
    throw new Error("Tenant is required.");
  }
  if (!TENANT_ID_PATTERN.test(normalizedTenantID)) {
    throw new Error("Tenant ID format is invalid.");
  }
  if (!password) {
    throw new Error("Password is required.");
  }

  const result = await requestAuthAPI<{ token: string; tenant_id: string }>("/auth/register", {
    method: "POST",
    body: JSON.stringify({
      tenant_id: normalizedTenantID,
      password,
      ...(displayName?.trim() ? { display_name: displayName.trim() } : {}),
    }),
  });

  const isDefaultTenant = result.tenant_id === DEFAULT_TENANT_ID;
  const session: AuthSession = {
    username: result.tenant_id,
    displayName: displayName?.trim() || result.tenant_id,
    role: isDefaultTenant ? "super-admin" : "operator",
    canAccessAllTenants: isDefaultTenant,
    tenantIds: normalizeTenantIDs([result.tenant_id]),
    loggedInAt: new Date().toISOString(),
  };

  window.localStorage.setItem(AUTH_TOKEN_KEY, result.token);
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session));
  setTenantScope({ tenantId: result.tenant_id, namespace: DEFAULT_NAMESPACE });
  emitAuthChanged(session);
  return session;
}

export function logout(): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(AUTH_STORAGE_KEY);
  window.localStorage.removeItem(AUTH_TOKEN_KEY);
  syncTenantScopeWithSession(null);
  emitAuthChanged(null);
}

export async function getLoginAccountHints(): Promise<LoginAccountHint[]> {
  return [
    {
      username: DEFAULT_TENANT_ID,
      password: "",
      note: "Default Tenant",
    },
  ];
}

export function getAuthToken(): string | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    return window.localStorage.getItem(AUTH_TOKEN_KEY);
  } catch {
    return null;
  }
}

export function isAuthDisabled(): boolean {
  return AUTH_DISABLED;
}

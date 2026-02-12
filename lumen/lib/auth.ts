import {
  DEFAULT_NAMESPACE,
  DEFAULT_TENANT_ID,
  getTenantScope,
  setTenantScope,
} from "@/lib/tenant-scope";

const AUTH_STORAGE_KEY = "nova.auth.session";
export const AUTH_CHANGED_EVENT = "nova:auth-changed";

type UserRole = "super-admin" | "operator" | "viewer";

interface UserRecord {
  username: string;
  password: string;
  displayName: string;
  role: UserRole;
  canAccessAllTenants?: boolean;
  tenantIds?: string[];
}

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

const USER_DIRECTORY: UserRecord[] = [
  {
    username: "admin",
    password: "admin",
    displayName: "Platform Admin",
    role: "super-admin",
    canAccessAllTenants: true,
  },
  {
    username: "ops",
    password: "ops",
    displayName: "Operations",
    role: "operator",
    tenantIds: ["default", "team-a", "team-b"],
  },
  {
    username: "dev",
    password: "dev",
    displayName: "Tenant Developer",
    role: "viewer",
    tenantIds: ["team-a", "team-b"],
  },
];

const LOGIN_ACCOUNT_HINTS: LoginAccountHint[] = [
  { username: "admin", password: "admin", note: "Super admin, can access all tenants" },
  { username: "ops", password: "ops", note: "Member of default/team-a/team-b" },
  { username: "dev", password: "dev", note: "Member of team-a/team-b" },
];

function normalizeUsername(username: string): string {
  return username.trim().toLowerCase();
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

function toSession(user: UserRecord): AuthSession {
  return {
    username: user.username,
    displayName: user.displayName,
    role: user.role,
    canAccessAllTenants: Boolean(user.canAccessAllTenants),
    tenantIds: normalizeTenantIDs(user.tenantIds),
    loggedInAt: new Date().toISOString(),
  };
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
  return Boolean(getAuthSession());
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

export function login(username: string, password: string): AuthSession {
  if (typeof window === "undefined") {
    throw new Error("Login is only available in browser.");
  }

  const normalizedUsername = normalizeUsername(username);
  const user = USER_DIRECTORY.find(
    (item) =>
      normalizeUsername(item.username) === normalizedUsername && item.password === password,
  );

  if (!user) {
    throw new Error("Invalid username or password.");
  }

  const session = toSession(user);
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session));
  emitAuthChanged(session);
  syncTenantScopeWithSession(session);
  return session;
}

export function logout(): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.removeItem(AUTH_STORAGE_KEY);
  } catch {
    // Ignore storage cleanup errors and continue logout.
  }
  syncTenantScopeWithSession(null);
  emitAuthChanged(null);
}

export function getLoginAccountHints(): LoginAccountHint[] {
  return LOGIN_ACCOUNT_HINTS.slice();
}

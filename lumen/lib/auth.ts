import {
  DEFAULT_NAMESPACE,
  DEFAULT_TENANT_ID,
  getTenantScope,
  setTenantScope,
} from "@/lib/tenant-scope";

const AUTH_STORAGE_KEY = "nova.auth.session";
const AUTH_CREDENTIALS_STORAGE_KEY = "nova.auth.credentials";
export const AUTH_CHANGED_EVENT = "nova:auth-changed";
const AUTH_DISABLED = false;
const TENANT_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const API_BASE = "/api";

type UserRole = "super-admin" | "operator" | "viewer";

interface TenantRecord {
  id: string;
  name?: string;
}

interface StoredCredential {
  tenant_id: string;
  password: string;
  display_name?: string;
  updated_at: string;
}

interface CredentialStore {
  version: 1;
  records: Record<string, StoredCredential>;
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

const EMPTY_CREDENTIAL_STORE: CredentialStore = {
  version: 1,
  records: {},
};

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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function parseCredentialStore(raw: string | null): CredentialStore {
  if (!raw) {
    return { ...EMPTY_CREDENTIAL_STORE, records: {} };
  }
  try {
    const parsed = JSON.parse(raw) as Partial<CredentialStore>;
    if (!parsed || parsed.version !== 1 || !isRecord(parsed.records)) {
      return { ...EMPTY_CREDENTIAL_STORE, records: {} };
    }
    const records: Record<string, StoredCredential> = {};
    Object.entries(parsed.records).forEach(([key, value]) => {
      if (!isRecord(value)) {
        return;
      }
      const tenantID = typeof value.tenant_id === "string" ? normalizeTenantID(value.tenant_id) : "";
      const password = typeof value.password === "string" ? value.password : "";
      const updatedAt = typeof value.updated_at === "string" ? value.updated_at : "";
      if (!tenantID || !password || !updatedAt) {
        return;
      }
      records[key] = {
        tenant_id: tenantID,
        password,
        display_name: typeof value.display_name === "string" ? value.display_name : undefined,
        updated_at: updatedAt,
      };
    });
    return { version: 1, records };
  } catch {
    return { ...EMPTY_CREDENTIAL_STORE, records: {} };
  }
}

function credentialStoreKey(tenantID: string): string {
  return normalizeTenantID(tenantID).toLowerCase();
}

function readCredentialStore(): CredentialStore {
  if (typeof window === "undefined") {
    return { ...EMPTY_CREDENTIAL_STORE, records: {} };
  }
  try {
    return parseCredentialStore(window.localStorage.getItem(AUTH_CREDENTIALS_STORAGE_KEY));
  } catch {
    return { ...EMPTY_CREDENTIAL_STORE, records: {} };
  }
}

function writeCredentialStore(store: CredentialStore): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(AUTH_CREDENTIALS_STORAGE_KEY, JSON.stringify(store));
}

function readTenantCredential(tenantID: string): StoredCredential | null {
  const normalized = normalizeTenantID(tenantID);
  if (!normalized) {
    return null;
  }
  const store = readCredentialStore();
  return store.records[credentialStoreKey(normalized)] || null;
}

function upsertTenantCredential(tenantID: string, password: string, displayName?: string): void {
  const normalizedTenantID = normalizeTenantID(tenantID);
  if (!normalizedTenantID || !password) {
    return;
  }
  const store = readCredentialStore();
  store.records[credentialStoreKey(normalizedTenantID)] = {
    tenant_id: normalizedTenantID,
    password,
    display_name: displayName?.trim() || undefined,
    updated_at: new Date().toISOString(),
  };
  writeCredentialStore(store);
}

function defaultPasswordForTenant(tenantID: string): string {
  return tenantID;
}

function toSession(tenant: TenantRecord): AuthSession {
  const isDefaultTenant = tenant.id === DEFAULT_TENANT_ID;
  return {
    username: tenant.id,
    displayName: tenant.name?.trim() || tenant.id,
    role: isDefaultTenant ? "super-admin" : "operator",
    canAccessAllTenants: isDefaultTenant,
    tenantIds: normalizeTenantIDs([tenant.id]),
    loggedInAt: new Date().toISOString(),
  };
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

function parseTenantsPayload(payload: unknown): TenantRecord[] {
  const rows: unknown[] = Array.isArray(payload)
    ? payload
    : isRecord(payload) && Array.isArray(payload.items)
      ? payload.items
      : [];

  const tenants: TenantRecord[] = [];
  rows.forEach((row) => {
    if (!isRecord(row) || typeof row.id !== "string") {
      return;
    }
    const id = normalizeTenantID(row.id);
    if (!id) {
      return;
    }
    tenants.push({
      id,
      name: typeof row.name === "string" ? row.name : undefined,
    });
  });
  return tenants;
}

async function listTenantsRaw(): Promise<TenantRecord[]> {
  const payload = await requestAuthAPI<unknown>("/tenants?limit=500");
  return parseTenantsPayload(payload);
}

async function resolveTenant(tenantID: string): Promise<TenantRecord> {
  const normalizedTenantID = normalizeTenantID(tenantID);
  if (!normalizedTenantID || !TENANT_ID_PATTERN.test(normalizedTenantID)) {
    throw new Error("Tenant ID format is invalid.");
  }
  const tenants = await listTenantsRaw();
  const tenant =
    tenants.find((item) => item.id === normalizedTenantID) ||
    tenants.find((item) => item.id.toLowerCase() === normalizedTenantID.toLowerCase());
  if (!tenant) {
    throw new Error(`Tenant '${normalizedTenantID}' not found.`);
  }
  return tenant;
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

  const tenant = await resolveTenant(requestedTenantID);
  const storedCredential = readTenantCredential(tenant.id);
  const expectedPassword = storedCredential?.password ?? defaultPasswordForTenant(tenant.id);
  if (password !== expectedPassword) {
    throw new Error("Invalid tenant password.");
  }

  const session = toSession({
    id: tenant.id,
    name: storedCredential?.display_name || tenant.name,
  });

  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session));
  setTenantScope({ tenantId: tenant.id, namespace: DEFAULT_NAMESPACE });
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

  const existingTenants = await listTenantsRaw();
  const exists = existingTenants.some(
    (item) => item.id === normalizedTenantID || item.id.toLowerCase() === normalizedTenantID.toLowerCase(),
  );
  if (exists) {
    throw new Error(`Tenant '${normalizedTenantID}' already exists.`);
  }

  await requestAuthAPI<TenantRecord>("/tenants", {
    method: "POST",
    body: JSON.stringify({
      id: normalizedTenantID,
      ...(displayName?.trim() ? { name: displayName.trim() } : {}),
    }),
  });

  upsertTenantCredential(normalizedTenantID, password, displayName);
  return login(normalizedTenantID, password);
}

export function logout(): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(AUTH_STORAGE_KEY);
  syncTenantScopeWithSession(null);
  emitAuthChanged(null);
}

export async function getLoginAccountHints(): Promise<LoginAccountHint[]> {
  let tenants: TenantRecord[] = [];
  try {
    tenants = await listTenantsRaw();
  } catch {
    const store = readCredentialStore();
    tenants = Object.values(store.records).map((record) => ({
      id: record.tenant_id,
      name: record.display_name,
    }));
  }

  const deduped = new Map<string, TenantRecord>();
  tenants.forEach((tenant) => {
    const id = normalizeTenantID(tenant.id);
    if (!id || deduped.has(id.toLowerCase())) {
      return;
    }
    deduped.set(id.toLowerCase(), { id, name: tenant.name });
  });

  if (!deduped.has(DEFAULT_TENANT_ID.toLowerCase())) {
    deduped.set(DEFAULT_TENANT_ID.toLowerCase(), {
      id: DEFAULT_TENANT_ID,
      name: "Default Tenant",
    });
  }

  return Array.from(deduped.values())
    .sort((a, b) => a.id.localeCompare(b.id))
    .slice(0, 12)
    .map((tenant) => {
      const storedCredential = readTenantCredential(tenant.id);
      const password = storedCredential?.password ?? defaultPasswordForTenant(tenant.id);
      const noteParts = [
        tenant.name && tenant.name !== tenant.id ? tenant.name : "",
        `default password: ${tenant.id}`,
      ].filter(Boolean);
      return {
        username: tenant.id,
        password,
        note: noteParts.join(" · "),
      };
    });
}

export function isAuthDisabled(): boolean {
  return AUTH_DISABLED;
}

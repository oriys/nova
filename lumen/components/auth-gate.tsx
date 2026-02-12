"use client";

import { PropsWithChildren, useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  AUTH_CHANGED_EVENT,
  getAuthSession,
  syncTenantScopeWithSession,
} from "@/lib/auth";

const PUBLIC_PATHS = new Set(["/", "/login"]);
const PUBLIC_PREFIXES = ["/api-docs/shared/"];

function isPublicPath(pathname: string): boolean {
  if (PUBLIC_PATHS.has(pathname)) {
    return true;
  }
  return PUBLIC_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

function isSafeRedirectTarget(target: string | null): target is string {
  return Boolean(target && target.startsWith("/") && !target.startsWith("//"));
}

export function AuthGate({ children }: PropsWithChildren) {
  const router = useRouter();
  const pathname = usePathname() || "/";
  const searchParams = useSearchParams();
  const queryString = searchParams?.toString() || "";
  const nextParam = searchParams?.get("next") || null;
  const [authVersion, setAuthVersion] = useState(0);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const sync = () => setAuthVersion((prev) => prev + 1);
    window.addEventListener("storage", sync);
    window.addEventListener(AUTH_CHANGED_EVENT, sync as EventListener);
    return () => {
      window.removeEventListener("storage", sync);
      window.removeEventListener(AUTH_CHANGED_EVENT, sync as EventListener);
    };
  }, []);

  useEffect(() => {
    const session = getAuthSession();
    const fullPath = queryString ? `${pathname}?${queryString}` : pathname;

    if (!session) {
      if (isPublicPath(pathname)) {
        setReady(true);
        return;
      }
      setReady(false);
      router.replace(`/login?next=${encodeURIComponent(fullPath)}`);
      return;
    }

    syncTenantScopeWithSession(session);

    if (pathname === "/login") {
      setReady(false);
      router.replace(isSafeRedirectTarget(nextParam) ? nextParam : "/dashboard");
      return;
    }

    setReady(true);
  }, [authVersion, nextParam, pathname, queryString, router]);

  if (!ready) {
    return null;
  }

  return <>{children}</>;
}

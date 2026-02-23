"use client"

import { useEffect, useState, useCallback } from "react"
import { rbacApi, type EffectivePermissions } from "@/lib/api"

/**
 * useEffectivePermissions fetches the current user's effective permissions
 * from GET /rbac/my-permissions. Returns null while loading.
 *
 * Refetches when `nova:tenant-scope-changed` fires.
 */
export function useEffectivePermissions(): EffectivePermissions | null {
  const [data, setData] = useState<EffectivePermissions | null>(null)

  const fetch = useCallback(() => {
    rbacApi
      .getMyPermissions()
      .then(setData)
      .catch(() => setData(null))
  }, [])

  useEffect(() => {
    fetch()
    const handler = () => fetch()
    window.addEventListener("nova:tenant-scope-changed", handler)
    return () => window.removeEventListener("nova:tenant-scope-changed", handler)
  }, [fetch])

  return data
}

/**
 * hasPermission checks whether the effective permissions include a given code.
 */
export function hasPermission(
  ep: EffectivePermissions | null,
  code: string
): boolean {
  if (!ep) return false
  return ep.permissions.includes(code)
}

/**
 * isButtonEnabled checks both the tenant-level button cap and the user's
 * effective permission for a given button key.
 */
export function isButtonEnabled(
  ep: EffectivePermissions | null,
  buttonKey: string
): boolean {
  if (!ep) return false
  return ep.button_permissions[buttonKey] === true
}

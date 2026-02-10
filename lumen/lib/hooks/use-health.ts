"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { healthApi, type HealthStatus } from "@/lib/api"

export type HealthLevel = "healthy" | "degraded" | "down" | "unknown"

export interface SystemHealthSnapshot {
  status: HealthLevel
  components: Record<string, HealthLevel>
  raw: HealthStatus | null
  loading: boolean
  updatedAt: Date | null
  error: string | null
}

function normalizeHealthValue(value: unknown): HealthLevel {
  if (typeof value === "boolean") {
    return value ? "healthy" : "down"
  }
  if (typeof value === "string") {
    const next = value.trim().toLowerCase()
    if (next === "ok" || next === "healthy" || next.startsWith("healthy")) {
      return "healthy"
    }
    if (next === "degraded" || next.includes("degraded")) {
      return "degraded"
    }
    if (next === "down" || next.startsWith("unhealthy") || next.includes("unavailable")) {
      return "down"
    }
  }
  return "unknown"
}

function normalizeSystemStatus(status: string | undefined): HealthLevel {
  if (!status) return "unknown"
  const next = status.trim().toLowerCase()
  if (next === "ok" || next === "healthy") return "healthy"
  if (next === "degraded") return "degraded"
  if (next === "down" || next === "error") return "down"
  return "unknown"
}

export function useHealth(pollIntervalMs: number = 15000) {
  const [snapshot, setSnapshot] = useState<SystemHealthSnapshot>({
    status: "unknown",
    components: {},
    raw: null,
    loading: true,
    updatedAt: null,
    error: null,
  })

  const refresh = useCallback(async () => {
    try {
      const data = await healthApi.check()
      const normalizedComponents: Record<string, HealthLevel> = {}

      if (data.components && typeof data.components === "object") {
        Object.entries(data.components).forEach(([key, value]) => {
          normalizedComponents[key] = normalizeHealthValue(value)
        })
      }

      const componentLevels = Object.values(normalizedComponents)
      let systemStatus = normalizeSystemStatus(data.status)
      if (componentLevels.some((level) => level === "down")) {
        systemStatus = "down"
      } else if (
        componentLevels.some((level) => level === "degraded") &&
        systemStatus === "healthy"
      ) {
        systemStatus = "degraded"
      }

      setSnapshot({
        status: systemStatus,
        components: normalizedComponents,
        raw: data,
        loading: false,
        updatedAt: new Date(),
        error: null,
      })
    } catch (err) {
      setSnapshot((prev) => ({
        ...prev,
        status: "down",
        loading: false,
        updatedAt: new Date(),
        error: err instanceof Error ? err.message : "Health check failed",
      }))
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    const run = async () => {
      if (cancelled) return
      await refresh()
    }

    run()
    const timer = window.setInterval(run, Math.max(3000, pollIntervalMs))
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [pollIntervalMs, refresh])

  const result = useMemo(
    () => ({
      ...snapshot,
      refresh,
    }),
    [snapshot, refresh]
  )

  return result
}

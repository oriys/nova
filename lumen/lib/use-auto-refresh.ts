"use client"

import { useEffect, useState, useCallback, useRef } from "react"

const STORAGE_KEY = "lumen-auto-refresh"
const LEGACY_STORAGE_KEY = "nova-auto-refresh"

export interface AutoRefreshConfig {
  dashboard: boolean
  configurations: boolean
  logs: boolean
}

const defaultConfig: AutoRefreshConfig = {
  dashboard: false,
  configurations: false,
  logs: false,
}

function loadConfig(): AutoRefreshConfig {
  if (typeof window === "undefined") return defaultConfig
  try {
    let raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      raw = localStorage.getItem(LEGACY_STORAGE_KEY)
      if (raw) {
        localStorage.setItem(STORAGE_KEY, raw)
        localStorage.removeItem(LEGACY_STORAGE_KEY)
      }
    }
    if (!raw) return defaultConfig
    return { ...defaultConfig, ...JSON.parse(raw) }
  } catch {
    return defaultConfig
  }
}

function saveConfig(config: AutoRefreshConfig) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
}

export function useAutoRefreshConfig() {
  const [config, setConfig] = useState<AutoRefreshConfig>(defaultConfig)

  useEffect(() => {
    setConfig(loadConfig())
  }, [])

  const update = useCallback((key: keyof AutoRefreshConfig, value: boolean) => {
    setConfig(prev => {
      const next = { ...prev, [key]: value }
      saveConfig(next)
      return next
    })
  }, [])

  return { config, update }
}

/**
 * Hook that runs a callback on an interval only when enabled.
 * Returns the enabled state and a toggle function.
 */
export function useAutoRefresh(
  key: keyof AutoRefreshConfig,
  callback: () => void,
  intervalMs: number = 30000,
) {
  const [enabled, setEnabled] = useState(false)
  const callbackRef = useRef(callback)
  callbackRef.current = callback

  // Load initial state from localStorage
  useEffect(() => {
    const config = loadConfig()
    setEnabled(config[key])
  }, [key])

  // Run interval when enabled
  useEffect(() => {
    if (!enabled) return
    const interval = setInterval(() => callbackRef.current(), intervalMs)
    return () => clearInterval(interval)
  }, [enabled, intervalMs, key])

  const toggle = useCallback(() => {
    setEnabled(prev => {
      const next = !prev
      const config = loadConfig()
      config[key] = next
      saveConfig(config)
      return next
    })
  }, [key])

  return { enabled, toggle }
}

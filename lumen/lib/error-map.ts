"use client"

import { ApiError } from "@/lib/api"

export interface UserError {
  title: string
  message: string
  hint?: string
  status?: number
  code?: string
}

function inferHint(status: number | undefined, message: string, code?: string): string | undefined {
  const normalized = message.toLowerCase()
  const normalizedCode = (code || "").toLowerCase()

  if (normalizedCode === "invalid_json" || normalized.includes("invalid json") || normalized.includes("valid json")) {
    return "Check that the request body is valid JSON and that field types match the API examples."
  }
  if (status === 403 || normalized.includes("ingress") || normalized.includes("network policy")) {
    return "Check whether the function network policy (ingress/egress rules) allows the current source."
  }
  if (status === 404 && normalized.includes("function")) {
    return "Confirm the function exists and that your tenant/namespace scope is correct."
  }
  if (status === 429 || normalized.includes("quota exceeded") || normalized.includes("rate limit")) {
    return "Adjust rate limits/quotas or retry later."
  }
  if (status === 502 || status === 503 || normalized.includes("unavailable")) {
    return "Check Zenith / Nova / Comet container status and their /health endpoints."
  }
  if (status === 504 || normalized.includes("timeout")) {
    return "The request timed out. Increase timeout or reduce function execution time."
  }
  return undefined
}

export function resolveUserError(error: unknown, fallbackTitle: string = "Request failed"): UserError {
  if (error instanceof ApiError) {
    const title = fallbackTitle
    const message = error.message || "Request failed"
    const hint = error.hint || inferHint(error.status, message, error.code)
    return {
      title,
      message,
      hint,
      status: error.status,
      code: error.code,
    }
  }

  if (error instanceof Error) {
    return {
      title: fallbackTitle,
      message: error.message || "Unexpected error",
      hint: inferHint(undefined, error.message || ""),
    }
  }

  const text = typeof error === "string" ? error : "Unexpected error"
  return {
    title: fallbackTitle,
    message: text,
    hint: inferHint(undefined, text),
  }
}

export function toUserErrorMessage(error: unknown, fallbackTitle?: string): string {
  const resolved = resolveUserError(error, fallbackTitle)
  if (resolved.hint) {
    return `${resolved.message} Hint: ${resolved.hint}`
  }
  return resolved.message
}

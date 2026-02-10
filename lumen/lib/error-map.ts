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
    return "请检查请求体是否为合法 JSON，并确认字段类型与示例一致。"
  }
  if (status === 403 || normalized.includes("ingress") || normalized.includes("network policy")) {
    return "请检查函数的 Network Policy（ingress/egress 规则）是否允许当前来源。"
  }
  if (status === 404 && normalized.includes("function")) {
    return "请先在 Functions 页面确认函数名存在，且租户/namespace 作用域正确。"
  }
  if (status === 429 || normalized.includes("quota exceeded") || normalized.includes("rate limit")) {
    return "请调整限流/配额，或稍后重试。"
  }
  if (status === 502 || status === 503 || normalized.includes("unavailable")) {
    return "请先检查 Zenith / Nova / Comet 容器状态与 /health 接口。"
  }
  if (status === 504 || normalized.includes("timeout")) {
    return "请求超时，可尝试增大超时或降低函数执行耗时。"
  }
  return undefined
}

export function resolveUserError(error: unknown, fallbackTitle: string = "请求失败"): UserError {
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
    return `${resolved.message} 建议：${resolved.hint}`
  }
  return resolved.message
}

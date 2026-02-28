"use client"

import { useState, useEffect, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { FunctionData } from "@/lib/types"
import { functionsApi } from "@/lib/api"
import {
  Save,
  Loader2,
  AlertCircle,
  RotateCcw,
  Package,
  Plus,
  Trash2,
} from "lucide-react"

interface FunctionDependenciesProps {
  func: FunctionData
  onDependenciesSaved?: () => void
}

const DEP_FILES: Record<string, string> = {
  go: "go.mod",
  rust: "Cargo.toml",
  node: "package.json",
  python: "requirements.txt",
  ruby: "Gemfile",
  php: "composer.json",
  bun: "package.json",
  deno: "package.json",
}

const DEP_PLACEHOLDERS: Record<string, string> = {
  go: `module handler

go 1.23

require (
    github.com/example/pkg v1.0.0
)`,
  rust: `[package]
name = "handler"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"`,
  node: `{
  "name": "handler",
  "version": "1.0.0",
  "dependencies": {
    "axios": "^1.6.0"
  }
}`,
  python: `requests>=2.31.0
boto3>=1.28.0`,
  ruby: `source 'https://rubygems.org'

gem 'json'
gem 'httparty'`,
  php: `{
  "name": "handler",
  "require": {
    "guzzlehttp/guzzle": "^7.0"
  }
}`,
}

function getBaseRuntime(runtimeId: string): string {
  const prefixes = [
    "python",
    "node",
    "go",
    "rust",
    "java",
    "ruby",
    "php",
    "deno",
    "bun",
  ]
  for (const prefix of prefixes) {
    if (runtimeId.startsWith(prefix)) return prefix
  }
  return runtimeId
}

function getDepFileName(runtimeId: string): string {
  return DEP_FILES[getBaseRuntime(runtimeId)] || ""
}

function getDepPlaceholder(runtimeId: string): string {
  return DEP_PLACEHOLDERS[getBaseRuntime(runtimeId)] || ""
}

function hasDepFileSupport(runtimeId: string): boolean {
  return getDepFileName(runtimeId) !== ""
}

// Map dep file names to Monaco language IDs
function getDepFileLanguage(depFileName: string): string {
  if (depFileName === "package.json" || depFileName === "composer.json")
    return "json"
  if (depFileName === "Cargo.toml") return "toml"
  if (depFileName === "go.mod") return "go"
  if (depFileName === "Gemfile") return "ruby"
  return "plaintext"
}

export function FunctionDependencies({
  func,
  onDependenciesSaved,
}: FunctionDependenciesProps) {
  const t = useTranslations("functionDetailPage.dependencies")
  const runtimeId = func.runtimeId || func.runtime
  const depFileName = getDepFileName(runtimeId)
  const supported = hasDepFileSupport(runtimeId)

  const [content, setContent] = useState("")
  const [originalContent, setOriginalContent] = useState("")
  const [enabled, setEnabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const hasChanges = content !== originalContent

  // Load existing dependency file from backend
  const loadDeps = useCallback(async () => {
    if (!supported) {
      setLoading(false)
      return
    }
    try {
      setLoading(true)
      setError(null)
      const response = await functionsApi.getFileContents(func.name)
      const files = response.files || {}
      const depContent = files[depFileName] || ""
      if (depContent) {
        setContent(depContent)
        setOriginalContent(depContent)
        setEnabled(true)
      } else {
        setContent("")
        setOriginalContent("")
        setEnabled(false)
      }
    } catch {
      // No files found is not an error - just means no deps configured
      setContent("")
      setOriginalContent("")
      setEnabled(false)
    } finally {
      setLoading(false)
    }
  }, [func.name, depFileName, supported])

  useEffect(() => {
    loadDeps()
  }, [loadDeps])

  const handleSave = async () => {
    try {
      setSaving(true)
      setError(null)
      // Get current code first
      const codeResponse = await functionsApi.getCode(func.name)
      const currentCode = codeResponse.source_code || ""
      // Update code with dependency files
      const depFiles: Record<string, string> = {}
      if (enabled && content.trim()) {
        depFiles[depFileName] = content
      }
      await functionsApi.updateCodeWithFiles(func.name, currentCode, depFiles)
      setOriginalContent(content)
      onDependenciesSaved?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : t("saveFailed"))
    } finally {
      setSaving(false)
    }
  }

  const handleDiscard = () => {
    setContent(originalContent)
    if (!originalContent) {
      setEnabled(false)
    }
  }

  const handleToggle = () => {
    if (enabled) {
      setContent("")
      setEnabled(false)
    } else {
      if (!content) {
        setContent(getDepPlaceholder(runtimeId))
      }
      setEnabled(true)
    }
  }

  if (!supported) {
    return (
      <div className="rounded-lg border bg-card p-8 text-center">
        <Package className="mx-auto h-12 w-12 text-muted-foreground/30" />
        <p className="mt-4 text-sm text-muted-foreground">
          {t("unsupported")}
        </p>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="rounded-lg border bg-card p-8 text-center">
        <Loader2 className="mx-auto h-8 w-8 animate-spin text-muted-foreground" />
        <p className="mt-4 text-sm text-muted-foreground">{t("loading")}</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">{t("title")}</h3>
          <p className="text-sm text-muted-foreground">{t("description")}</p>
        </div>
        <div className="flex items-center gap-2">
          {hasChanges && (
            <Button
              variant="ghost"
              size="sm"
              onClick={handleDiscard}
              disabled={saving}
            >
              <RotateCcw className="mr-1 h-4 w-4" />
              {t("discard")}
            </Button>
          )}
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving || !hasChanges}
          >
            {saving ? (
              <Loader2 className="mr-1 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-1 h-4 w-4" />
            )}
            {saving ? t("saving") : t("save")}
          </Button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {error}
        </div>
      )}

      {/* Dependency file editor */}
      <div className="rounded-lg border bg-card">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <div className="flex items-center gap-2">
            <Package className="h-4 w-4 text-muted-foreground" />
            <Badge variant="outline">{depFileName}</Badge>
          </div>
          <Button
            variant={enabled ? "secondary" : "outline"}
            size="sm"
            onClick={handleToggle}
            disabled={saving}
          >
            {enabled ? (
              <>
                <Trash2 className="mr-1 h-3 w-3" />
                {t("removeDeps")}
              </>
            ) : (
              <>
                <Plus className="mr-1 h-3 w-3" />
                {t("addDeps")}
              </>
            )}
          </Button>
        </div>

        {enabled ? (
          <div className="p-4">
            <Textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder={getDepPlaceholder(runtimeId)}
              className="min-h-[240px] font-mono text-sm"
              disabled={saving}
            />
            <p className="mt-2 text-xs text-muted-foreground">
              {t("help", { file: depFileName })}
            </p>
          </div>
        ) : (
          <div className="p-8 text-center text-muted-foreground">
            <Package className="mx-auto h-10 w-10 opacity-30" />
            <p className="mt-3 text-sm">{t("empty")}</p>
            <p className="mt-1 text-xs">
              {t("emptyHint", { file: depFileName })}
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-4"
              onClick={handleToggle}
            >
              <Plus className="mr-1 h-3 w-3" />
              {t("addDeps")}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

"use client"

import { useState, useEffect, useCallback } from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { CodeEditor, CodeDisplay } from "@/components/code-editor"
import { FunctionData } from "@/lib/types"
import { functionsApi, CompileStatus } from "@/lib/api"
import { Copy, Check, Download, Save, Loader2, AlertCircle } from "lucide-react"

interface FunctionCodeProps {
  func: FunctionData
  onCodeSaved?: () => void
}

// Map display runtime names back to runtime IDs for highlighting
function getRuntimeId(displayName: string): string {
  const lower = displayName.toLowerCase()
  if (lower.includes("python")) return "python"
  if (lower.includes("node")) return "node"
  if (lower.includes("go ") || lower === "go") return "go"
  if (lower.includes("rust")) return "rust"
  if (lower.includes("java") && !lower.includes("javascript")) return "java"
  if (lower.includes("ruby")) return "ruby"
  if (lower.includes("php")) return "php"
  if (lower.includes(".net") || lower.includes("dotnet")) return "dotnet"
  if (lower.includes("deno")) return "deno"
  if (lower.includes("bun")) return "bun"
  return "javascript"
}

function getCompileStatusBadge(status: CompileStatus | undefined) {
  switch (status) {
    case 'compiling':
      return <Badge variant="outline" className="text-yellow-600 border-yellow-600">
        <Loader2 className="mr-1 h-3 w-3 animate-spin" />
        Compiling
      </Badge>
    case 'success':
      return <Badge variant="outline" className="text-green-600 border-green-600">
        <Check className="mr-1 h-3 w-3" />
        Compiled
      </Badge>
    case 'failed':
      return <Badge variant="destructive">
        <AlertCircle className="mr-1 h-3 w-3" />
        Failed
      </Badge>
    case 'not_required':
      return <Badge variant="secondary">Interpreted</Badge>
    case 'pending':
      return <Badge variant="outline">Pending</Badge>
    default:
      return null
  }
}

export function FunctionCode({ func, onCodeSaved }: FunctionCodeProps) {
  const [code, setCode] = useState("")
  const [originalCode, setOriginalCode] = useState("")
  const [compileStatus, setCompileStatus] = useState<CompileStatus | undefined>(func.compileStatus)
  const [compileError, setCompileError] = useState<string | undefined>(func.compileError)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const runtimeId = func.runtimeId || getRuntimeId(func.runtime)
  const hasChanges = code !== originalCode

  // Load code from backend
  const loadCode = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await functionsApi.getCode(func.name)
      const sourceCode = response.source_code || ""
      setCode(sourceCode)
      setOriginalCode(sourceCode)
      setCompileStatus(response.compile_status)
      setCompileError(response.compile_error)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load code")
    } finally {
      setLoading(false)
    }
  }, [func.name])

  useEffect(() => {
    loadCode()
  }, [loadCode])

  // Poll for compile status when compiling
  useEffect(() => {
    if (compileStatus !== 'compiling') return

    const interval = setInterval(async () => {
      try {
        const response = await functionsApi.getCode(func.name)
        setCompileStatus(response.compile_status)
        setCompileError(response.compile_error)
        if (response.compile_status !== 'compiling') {
          // Update original code on compile complete
          if (response.source_code) {
            setOriginalCode(response.source_code)
          }
        }
      } catch {
        // Ignore polling errors
      }
    }, 2000)

    return () => clearInterval(interval)
  }, [compileStatus, func.name])

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleDownload = () => {
    const ext = {
      python: ".py",
      go: ".go",
      rust: ".rs",
      node: ".js",
      ruby: ".rb",
      java: ".java",
      deno: ".ts",
      bun: ".ts",
      php: ".php",
      dotnet: ".cs",
    }[runtimeId] || ".txt"

    const blob = new Blob([code], { type: "text/plain" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `${func.name}${ext}`
    a.click()
    URL.revokeObjectURL(url)
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      setError(null)
      const response = await functionsApi.updateCode(func.name, code)
      setCompileStatus(response.compile_status)
      setCompileError(undefined)
      setOriginalCode(code)
      onCodeSaved?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save code")
    } finally {
      setSaving(false)
    }
  }

  const handleDiscard = () => {
    setCode(originalCode)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-foreground">
            {func.handler}
          </span>
          <span className="text-xs text-muted-foreground">
            {func.runtime}
          </span>
          {getCompileStatusBadge(compileStatus)}
        </div>
        <div className="flex items-center gap-2">
          {hasChanges && (
            <>
              <Button variant="outline" size="sm" onClick={handleDiscard}>
                Discard
              </Button>
              <Button size="sm" onClick={handleSave} disabled={saving}>
                {saving ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Save className="mr-2 h-4 w-4" />
                )}
                Save
              </Button>
            </>
          )}
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <Check className="mr-2 h-4 w-4" />
            ) : (
              <Copy className="mr-2 h-4 w-4" />
            )}
            {copied ? "Copied" : "Copy"}
          </Button>
          <Button variant="outline" size="sm" onClick={handleDownload}>
            <Download className="mr-2 h-4 w-4" />
            Download
          </Button>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Compile error display */}
      {compileStatus === 'failed' && compileError && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          <div className="font-medium mb-1">Compilation Failed</div>
          <pre className="whitespace-pre-wrap text-xs font-mono">{compileError}</pre>
        </div>
      )}

      {/* Code Editor */}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-2">
          <div className="flex items-center gap-2">
            <div className="h-3 w-3 rounded-full bg-destructive/50" />
            <div className="h-3 w-3 rounded-full bg-warning/50" />
            <div className="h-3 w-3 rounded-full bg-success/50" />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{func.handler}</span>
            {hasChanges && (
              <Badge variant="outline" className="text-xs">Modified</Badge>
            )}
          </div>
        </div>
        <CodeEditor
          code={code}
          onChange={setCode}
          runtime={runtimeId}
          minHeight="500px"
          minimap
        />
      </div>

      {/* Info */}
      <div className="rounded-lg border border-border bg-muted/30 p-4">
        <p className="text-sm text-muted-foreground">
          Edit your function code directly in the browser. Changes are saved to the database
          {compileStatus !== 'not_required' && ' and automatically compiled'}.
          The function will be updated with the new code on save.
        </p>
      </div>
    </div>
  )
}

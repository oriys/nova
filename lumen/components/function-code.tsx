"use client"

import { useState, useEffect, useCallback } from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { CodeEditor } from "@/components/code-editor"
import { FunctionData } from "@/lib/types"
import { functionsApi, aiApi, CompileStatus, AsyncInvocationJob, AsyncInvocationStatus } from "@/lib/api"
import { Textarea } from "@/components/ui/textarea"
import { Copy, Check, Download, Save, Loader2, AlertCircle, Play, RefreshCw, RotateCcw, Sparkles, MessageSquare, Wand2 } from "lucide-react"

interface FunctionCodeProps {
  func: FunctionData
  onCodeSaved?: () => void
  invokeInput: string
  onInvokeInputChange: (value: string) => void
  invokeOutput: string | null
  invokeError: string | null
  invokeMeta: string | null
  invoking: boolean
  invokeMode: "sync" | "async"
  onInvokeModeChange: (mode: "sync" | "async") => void
  asyncJobs: AsyncInvocationJob[]
  loadingAsyncJobs: boolean
  retryingJobId: string | null
  onRefreshAsyncJobs: () => void
  onRetryAsyncJob: (jobId: string) => void
  onInvoke: () => void
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

function getAsyncStatusBadge(status: AsyncInvocationStatus) {
  switch (status) {
    case "queued":
      return <Badge variant="outline">Queued</Badge>
    case "running":
      return <Badge variant="outline" className="text-blue-600 border-blue-600">Running</Badge>
    case "succeeded":
      return <Badge variant="outline" className="text-green-600 border-green-600">Succeeded</Badge>
    case "dlq":
      return <Badge variant="destructive">DLQ</Badge>
    default:
      return <Badge variant="secondary">{status}</Badge>
  }
}

export function FunctionCode({
  func,
  onCodeSaved,
  invokeInput,
  onInvokeInputChange,
  invokeOutput,
  invokeError,
  invokeMeta,
  invoking,
  invokeMode,
  onInvokeModeChange,
  asyncJobs,
  loadingAsyncJobs,
  retryingJobId,
  onRefreshAsyncJobs,
  onRetryAsyncJob,
  onInvoke,
}: FunctionCodeProps) {
  const [code, setCode] = useState("")
  const [originalCode, setOriginalCode] = useState("")
  const [compileStatus, setCompileStatus] = useState<CompileStatus | undefined>(func.compileStatus)
  const [compileError, setCompileError] = useState<string | undefined>(func.compileError)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [aiEnabled, setAiEnabled] = useState(false)
  const [aiReviewing, setAiReviewing] = useState(false)
  const [aiRewriting, setAiRewriting] = useState(false)
  const [aiReview, setAiReview] = useState<string | null>(null)
  const [aiReviewScore, setAiReviewScore] = useState<number | undefined>()
  const [aiReviewSuggestions, setAiReviewSuggestions] = useState<string[]>([])

  const runtimeId = func.runtimeId || getRuntimeId(func.runtime)
  const hasChanges = code !== originalCode

  // Check AI status on mount
  useEffect(() => {
    aiApi.status().then((res) => setAiEnabled(res.enabled)).catch(() => {})
  }, [])

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

  const handleAiReview = async () => {
    if (!code.trim()) return
    try {
      setAiReviewing(true)
      setError(null)
      setAiReview(null)
      setAiReviewSuggestions([])
      setAiReviewScore(undefined)
      const response = await aiApi.review({ code, runtime: runtimeId })
      setAiReview(response.feedback)
      setAiReviewSuggestions(response.suggestions || [])
      setAiReviewScore(response.score)
    } catch (err) {
      setError(err instanceof Error ? err.message : "AI review failed")
    } finally {
      setAiReviewing(false)
    }
  }

  const handleAiRewrite = async () => {
    if (!code.trim()) return
    try {
      setAiRewriting(true)
      setError(null)
      const response = await aiApi.rewrite({ code, runtime: runtimeId })
      setCode(response.code)
    } catch (err) {
      setError(err instanceof Error ? err.message : "AI rewrite failed")
    } finally {
      setAiRewriting(false)
    }
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
          {aiEnabled && (
            <>
              <Button variant="outline" size="sm" onClick={handleAiReview} disabled={aiReviewing || !code.trim()}>
                {aiReviewing ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <MessageSquare className="mr-2 h-4 w-4" />
                )}
                AI Review
              </Button>
              <Button variant="outline" size="sm" onClick={handleAiRewrite} disabled={aiRewriting || !code.trim()}>
                {aiRewriting ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Wand2 className="mr-2 h-4 w-4" />
                )}
                AI Rewrite
              </Button>
            </>
          )}
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

      {/* AI Review display */}
      {aiReview && (
        <div className="rounded-md border border-purple-200 bg-purple-50 dark:border-purple-800 dark:bg-purple-950/30 p-4">
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-purple-600 dark:text-purple-400" />
              <span className="text-sm font-medium text-purple-900 dark:text-purple-200">AI Review</span>
            </div>
            <div className="flex items-center gap-2">
              {aiReviewScore !== undefined && (
                <Badge variant="outline" className="text-purple-700 border-purple-300 dark:text-purple-300 dark:border-purple-700">
                  Score: {aiReviewScore}/10
                </Badge>
              )}
              <Button variant="ghost" size="sm" onClick={() => setAiReview(null)} className="h-6 w-6 p-0 text-muted-foreground" aria-label="Dismiss AI review">
                ×
              </Button>
            </div>
          </div>
          <div className="text-sm text-purple-800 dark:text-purple-200 whitespace-pre-wrap">{aiReview}</div>
          {aiReviewSuggestions.length > 0 && (
            <div className="mt-3 space-y-1">
              <div className="text-xs font-medium text-purple-700 dark:text-purple-300">Suggestions:</div>
              <ul className="list-disc list-inside space-y-1">
                {aiReviewSuggestions.map((s, i) => (
                  <li key={i} className="text-xs text-purple-700 dark:text-purple-300">{s}</li>
                ))}
              </ul>
            </div>
          )}
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

      {/* Invoke */}
      <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-foreground">
              Invoke function
            </h2>
            <p className="text-sm text-muted-foreground">
              Choose sync invoke for immediate output, or async invoke with retry and DLQ fallback.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant={invokeMode === "sync" ? "default" : "outline"}
              size="sm"
              onClick={() => onInvokeModeChange("sync")}
            >
              Sync
            </Button>
            <Button
              variant={invokeMode === "async" ? "default" : "outline"}
              size="sm"
              onClick={() => onInvokeModeChange("async")}
            >
              Async
            </Button>
            <Button onClick={onInvoke} disabled={invoking} size="sm">
              {invoking ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Play className="mr-2 h-4 w-4" />
              )}
              {invokeMode === "async" ? "Enqueue" : "Invoke"}
            </Button>
          </div>
        </div>

        <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <div className="space-y-2">
            <div className="text-sm font-medium text-foreground">Input</div>
            <Textarea
              value={invokeInput}
              onChange={(event) => onInvokeInputChange(event.target.value)}
              className="min-h-[160px] font-mono text-xs"
              placeholder='{\n  "key": "value"\n}'
            />
            <p className="text-xs text-muted-foreground">
              Payload must be valid JSON. Leave empty to send an empty object.
            </p>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-foreground">Output</span>
              {invokeMeta && (
                <span className="text-xs text-muted-foreground">{invokeMeta}</span>
              )}
            </div>
            <div className="min-h-[160px] rounded-md border border-border bg-muted/30 p-3">
              {invokeOutput ? (
                <pre className="whitespace-pre-wrap text-xs text-foreground">
                  {invokeOutput}
                </pre>
              ) : (
                <p className="text-xs text-muted-foreground">
                  {invokeMode === "async"
                    ? "No result yet. Async mode returns a job ticket and executes in background."
                    : "No output yet. Invoke the function to see results."}
                </p>
              )}
            </div>
            {invokeError && (
              <p className="text-xs text-destructive">{invokeError}</p>
            )}
          </div>
        </div>

        <div className="mt-4 rounded-lg border border-border bg-muted/20 p-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-foreground">Async Queue / DLQ</p>
              <p className="text-xs text-muted-foreground">Failed jobs are retried with backoff, then moved to DLQ.</p>
            </div>
            <Button variant="outline" size="sm" onClick={onRefreshAsyncJobs} disabled={loadingAsyncJobs}>
              {loadingAsyncJobs ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="mr-2 h-4 w-4" />
              )}
              Refresh
            </Button>
          </div>

          <div className="mt-3 overflow-x-auto">
            <table className="w-full min-w-[720px] text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-2 py-2 text-left font-medium text-muted-foreground">Job ID</th>
                  <th className="px-2 py-2 text-left font-medium text-muted-foreground">Status</th>
                  <th className="px-2 py-2 text-left font-medium text-muted-foreground">Attempts</th>
                  <th className="px-2 py-2 text-left font-medium text-muted-foreground">Next Run</th>
                  <th className="px-2 py-2 text-left font-medium text-muted-foreground">Last Error</th>
                  <th className="px-2 py-2 text-right font-medium text-muted-foreground">Action</th>
                </tr>
              </thead>
              <tbody>
                {asyncJobs.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-2 py-4 text-center text-xs text-muted-foreground">
                      No async jobs yet.
                    </td>
                  </tr>
                ) : (
                  asyncJobs.map((job) => (
                    <tr key={job.id} className="border-b border-border/60">
                      <td className="px-2 py-2">
                        <code className="text-xs">{job.id.slice(0, 12)}</code>
                      </td>
                      <td className="px-2 py-2">{getAsyncStatusBadge(job.status)}</td>
                      <td className="px-2 py-2 text-xs text-muted-foreground">
                        {job.attempt}/{job.max_attempts}
                      </td>
                      <td className="px-2 py-2 text-xs text-muted-foreground">
                        {job.next_run_at ? new Date(job.next_run_at).toLocaleString() : "-"}
                      </td>
                      <td className="px-2 py-2 text-xs text-muted-foreground">
                        {job.last_error ? job.last_error.slice(0, 80) : "-"}
                      </td>
                      <td className="px-2 py-2 text-right">
                        {job.status === "dlq" ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => onRetryAsyncJob(job.id)}
                            disabled={retryingJobId === job.id}
                          >
                            {retryingJobId === job.id ? (
                              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            ) : (
                              <RotateCcw className="mr-2 h-4 w-4" />
                            )}
                            Retry
                          </Button>
                        ) : (
                          <span className="text-xs text-muted-foreground">-</span>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}

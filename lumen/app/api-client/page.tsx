"use client"

import { useState, useCallback, useMemo } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import {
  Send,
  Plus,
  Trash2,
  Copy,
  Check,
  Clock,
  ArrowDownUp,
  FileText,
  Code2,
  Variable,
} from "lucide-react"

// ─── Types ───────────────────────────────────────────────────────────────────

type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH" | "HEAD" | "OPTIONS"

interface KeyValuePair {
  id: string
  key: string
  value: string
  enabled: boolean
}

interface EnvVariable {
  id: string
  key: string
  value: string
}

interface ResponseData {
  status: number
  statusText: string
  headers: Record<string, string>
  body: string
  elapsed: number
  size: number
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

const HTTP_METHODS: HttpMethod[] = ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"]

const METHOD_COLORS: Record<HttpMethod, string> = {
  GET: "text-green-500",
  POST: "text-blue-500",
  PUT: "text-amber-500",
  DELETE: "text-red-500",
  PATCH: "text-purple-500",
  HEAD: "text-cyan-500",
  OPTIONS: "text-gray-500",
}

let nextId = 1
function newId(): string {
  return `kv-${nextId++}-${Date.now()}`
}

function newKeyValue(key = "", value = ""): KeyValuePair {
  return { id: newId(), key, value, enabled: true }
}

function newEnvVar(key = "", value = ""): EnvVariable {
  return { id: newId(), key, value }
}

function parseQueryParams(url: string): KeyValuePair[] {
  try {
    const u = new URL(url)
    const pairs: KeyValuePair[] = []
    u.searchParams.forEach((value, key) => {
      pairs.push(newKeyValue(key, value))
    })
    return pairs.length > 0 ? pairs : [newKeyValue()]
  } catch {
    return [newKeyValue()]
  }
}

function buildUrlWithParams(baseUrl: string, params: KeyValuePair[]): string {
  try {
    const u = new URL(baseUrl.split("?")[0])
    for (const p of params) {
      if (p.enabled && p.key) {
        u.searchParams.set(p.key, p.value)
      }
    }
    return u.toString()
  } catch {
    return baseUrl
  }
}

function substituteEnvVars(text: string, vars: EnvVariable[]): string {
  let result = text
  for (const v of vars) {
    if (v.key) {
      result = result.replaceAll(`{{${v.key}}}`, v.value)
    }
  }
  return result
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function tryFormatJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

function isJsonResponse(headers: Record<string, string>): boolean {
  const ct = headers["content-type"] || ""
  return ct.includes("application/json") || ct.includes("+json")
}

function statusColorClass(status: number): string {
  if (status >= 200 && status < 300) return "bg-green-500/10 text-green-500 border-green-500/20"
  if (status >= 300 && status < 400) return "bg-blue-500/10 text-blue-500 border-blue-500/20"
  if (status >= 400 && status < 500) return "bg-amber-500/10 text-amber-500 border-amber-500/20"
  return "bg-red-500/10 text-red-500 border-red-500/20"
}

// ─── Sub-Components ──────────────────────────────────────────────────────────

function KeyValueEditor({
  pairs,
  onChange,
  keyPlaceholder = "Key",
  valuePlaceholder = "Value",
  addLabel = "Add",
}: {
  pairs: KeyValuePair[]
  onChange: (pairs: KeyValuePair[]) => void
  keyPlaceholder?: string
  valuePlaceholder?: string
  addLabel?: string
}) {
  const update = (id: string, field: keyof KeyValuePair, val: string | boolean) => {
    onChange(pairs.map((p) => (p.id === id ? { ...p, [field]: val } : p)))
  }
  const remove = (id: string) => {
    const next = pairs.filter((p) => p.id !== id)
    onChange(next.length > 0 ? next : [newKeyValue()])
  }
  const add = () => onChange([...pairs, newKeyValue()])

  return (
    <div className="space-y-2">
      {pairs.map((pair) => (
        <div key={pair.id} className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={pair.enabled}
            onChange={(e) => update(pair.id, "enabled", e.target.checked)}
            className="h-4 w-4 rounded border-border accent-primary"
          />
          <Input
            value={pair.key}
            onChange={(e) => update(pair.id, "key", e.target.value)}
            placeholder={keyPlaceholder}
            className="flex-1 h-8 text-sm font-mono"
          />
          <Input
            value={pair.value}
            onChange={(e) => update(pair.id, "value", e.target.value)}
            placeholder={valuePlaceholder}
            className="flex-1 h-8 text-sm font-mono"
          />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => remove(pair.id)}
            className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={add} className="h-7 text-xs">
        <Plus className="mr-1 h-3 w-3" />
        {addLabel}
      </Button>
    </div>
  )
}

function EnvVariableEditor({
  variables,
  onChange,
  hint,
  addLabel,
}: {
  variables: EnvVariable[]
  onChange: (vars: EnvVariable[]) => void
  hint: string
  addLabel: string
}) {
  const update = (id: string, field: "key" | "value", val: string) => {
    onChange(variables.map((v) => (v.id === id ? { ...v, [field]: val } : v)))
  }
  const remove = (id: string) => {
    const next = variables.filter((v) => v.id !== id)
    onChange(next.length > 0 ? next : [newEnvVar()])
  }
  const add = () => onChange([...variables, newEnvVar()])

  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground mb-2">
        {hint}
      </p>
      {variables.map((v) => (
        <div key={v.id} className="flex items-center gap-2">
          <Input
            value={v.key}
            onChange={(e) => update(v.id, "key", e.target.value)}
            placeholder="variable_name"
            className="flex-1 h-8 text-sm font-mono"
          />
          <Input
            value={v.value}
            onChange={(e) => update(v.id, "value", e.target.value)}
            placeholder="value"
            className="flex-1 h-8 text-sm font-mono"
          />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => remove(v.id)}
            className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={add} className="h-7 text-xs">
        <Plus className="mr-1 h-3 w-3" />
        {addLabel}
      </Button>
    </div>
  )
}

// ─── Response Panel ──────────────────────────────────────────────────────────

function ResponsePanel({
  response,
  error,
  loading,
}: {
  response: ResponseData | null
  error: string | null
  loading: boolean
}) {
  const t = useTranslations("pages.apiClient")
  const [copied, setCopied] = useState(false)
  const [responseTab, setResponseTab] = useState("pretty")

  const handleCopy = async (text: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent mb-3" />
        <p className="text-sm">{t("sending")}</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm font-medium text-destructive">{t("error")}</p>
        <p className="text-sm text-destructive/80 mt-1">{error}</p>
      </div>
    )
  }

  if (!response) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
        <Send className="h-10 w-10 mb-3 opacity-30" />
        <p className="text-sm">{t("emptyResponse")}</p>
      </div>
    )
  }

  const isJson = isJsonResponse(response.headers)
  const formattedBody = isJson ? tryFormatJson(response.body) : response.body

  return (
    <div className="space-y-4">
      {/* Status bar */}
      <div className="flex items-center gap-3 flex-wrap">
        <Badge variant="outline" className={cn("font-mono text-sm", statusColorClass(response.status))}>
          {response.status} {response.statusText}
        </Badge>
        <div className="flex items-center gap-1 text-sm text-muted-foreground">
          <Clock className="h-3.5 w-3.5" />
          <span>{t("elapsed", { ms: response.elapsed })}</span>
        </div>
        <div className="flex items-center gap-1 text-sm text-muted-foreground">
          <ArrowDownUp className="h-3.5 w-3.5" />
          <span>{formatBytes(response.size)}</span>
        </div>
      </div>

      {/* Response tabs */}
      <Tabs value={responseTab} onValueChange={setResponseTab}>
        <div className="flex items-center justify-between">
          <TabsList className="h-8">
            <TabsTrigger value="pretty" className="text-xs h-6 px-3">
              <Code2 className="mr-1 h-3 w-3" />
              {t("pretty")}
            </TabsTrigger>
            <TabsTrigger value="raw" className="text-xs h-6 px-3">
              <FileText className="mr-1 h-3 w-3" />
              {t("raw")}
            </TabsTrigger>
            <TabsTrigger value="headers" className="text-xs h-6 px-3">
              {t("responseHeaders")}
            </TabsTrigger>
          </TabsList>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => handleCopy(response.body)}
            className="h-7 text-xs"
          >
            {copied ? <Check className="mr-1 h-3 w-3" /> : <Copy className="mr-1 h-3 w-3" />}
            {t("copy")}
          </Button>
        </div>

        <TabsContent value="pretty" className="mt-2">
          <pre className="rounded-lg border border-border bg-muted/50 p-4 text-sm font-mono overflow-auto max-h-[500px] whitespace-pre-wrap break-all">
            {formattedBody || t("emptyBody")}
          </pre>
        </TabsContent>

        <TabsContent value="raw" className="mt-2">
          <pre className="rounded-lg border border-border bg-muted/50 p-4 text-sm font-mono overflow-auto max-h-[500px] whitespace-pre-wrap break-all">
            {response.body || t("emptyBody")}
          </pre>
        </TabsContent>

        <TabsContent value="headers" className="mt-2">
          <div className="rounded-lg border border-border bg-muted/50 overflow-hidden">
            <table className="w-full">
              <tbody>
                {Object.entries(response.headers).map(([key, value]) => (
                  <tr key={key} className="border-b border-border last:border-b-0">
                    <td className="px-3 py-2 text-sm font-mono font-medium text-muted-foreground whitespace-nowrap">
                      {key}
                    </td>
                    <td className="px-3 py-2 text-sm font-mono break-all">{value}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ─── Main Page ───────────────────────────────────────────────────────────────

type BodyType = "none" | "json" | "form" | "raw"

export default function ApiClientPage() {
  const t = useTranslations("pages.apiClient")

  // Request state
  const [method, setMethod] = useState<HttpMethod>("GET")
  const [url, setUrl] = useState("")
  const [headers, setHeaders] = useState<KeyValuePair[]>([newKeyValue()])
  const [queryParams, setQueryParams] = useState<KeyValuePair[]>([newKeyValue()])
  const [bodyType, setBodyType] = useState<BodyType>("none")
  const [bodyContent, setBodyContent] = useState("")
  const [formData, setFormData] = useState<KeyValuePair[]>([newKeyValue()])

  // Environment variables
  const [envVars, setEnvVars] = useState<EnvVariable[]>([newEnvVar()])

  // Response state
  const [response, setResponse] = useState<ResponseData | null>(null)
  const [responseError, setResponseError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  // Request tab
  const [requestTab, setRequestTab] = useState("params")

  // Sync URL ↔ query params
  const handleUrlChange = useCallback((newUrl: string) => {
    setUrl(newUrl)
    if (newUrl.includes("?")) {
      setQueryParams(parseQueryParams(newUrl))
    }
  }, [])

  const handleParamsChange = useCallback(
    (params: KeyValuePair[]) => {
      setQueryParams(params)
      if (url) {
        setUrl(buildUrlWithParams(url, params))
      }
    },
    [url]
  )

  // Build final request body
  const resolvedBody = useMemo(() => {
    if (method === "GET" || method === "HEAD") return undefined
    if (bodyType === "none") return undefined
    if (bodyType === "json" || bodyType === "raw") return substituteEnvVars(bodyContent, envVars)
    if (bodyType === "form") {
      const params = new URLSearchParams()
      for (const p of formData) {
        if (p.enabled && p.key) {
          params.set(p.key, substituteEnvVars(p.value, envVars))
        }
      }
      return params.toString()
    }
    return undefined
  }, [method, bodyType, bodyContent, formData, envVars])

  // Send request
  const handleSend = async () => {
    if (!url.trim()) return

    setLoading(true)
    setResponse(null)
    setResponseError(null)

    try {
      const resolvedUrl = substituteEnvVars(url, envVars)
      const resolvedHeaders: Record<string, string> = {}
      for (const h of headers) {
        if (h.enabled && h.key) {
          resolvedHeaders[substituteEnvVars(h.key, envVars)] = substituteEnvVars(h.value, envVars)
        }
      }

      // Auto-set content-type if not already set
      const hasContentType = Object.keys(resolvedHeaders).some(
        (k) => k.toLowerCase() === "content-type"
      )
      if (!hasContentType && bodyType === "json") {
        resolvedHeaders["Content-Type"] = "application/json"
      }
      if (!hasContentType && bodyType === "form") {
        resolvedHeaders["Content-Type"] = "application/x-www-form-urlencoded"
      }

      const res = await fetch("/api/http-proxy", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          method,
          url: resolvedUrl,
          headers: resolvedHeaders,
          body: resolvedBody,
        }),
      })

      const data = await res.json()

      if (data.error) {
        setResponseError(data.error)
      } else {
        setResponse(data as ResponseData)
      }
    } catch (err) {
      setResponseError(err instanceof Error ? err.message : "Request failed")
    } finally {
      setLoading(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("title")} description={t("description")} />

      <div className="p-6 space-y-6">
        {/* URL Bar */}
        <div className="flex items-center gap-2">
          <Select value={method} onValueChange={(v) => setMethod(v as HttpMethod)}>
            <SelectTrigger className="w-[130px] h-10">
              <SelectValue>
                <span className={cn("font-mono font-bold text-sm", METHOD_COLORS[method])}>
                  {method}
                </span>
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {HTTP_METHODS.map((m) => (
                <SelectItem key={m} value={m}>
                  <span className={cn("font-mono font-bold text-sm", METHOD_COLORS[m])}>{m}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Input
            value={url}
            onChange={(e) => handleUrlChange(e.target.value)}
            placeholder={t("urlPlaceholder")}
            className="flex-1 h-10 font-mono text-sm"
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSend()
            }}
          />

          <Button onClick={handleSend} disabled={loading || !url.trim()} className="h-10 px-6">
            <Send className="mr-2 h-4 w-4" />
            {t("send")}
          </Button>
        </div>

        {/* Request / Response Split */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Request Panel */}
          <div className="rounded-xl border border-border bg-card">
            <div className="border-b border-border px-4 py-3">
              <h3 className="text-sm font-semibold">{t("request")}</h3>
            </div>
            <div className="p-4">
              <Tabs value={requestTab} onValueChange={setRequestTab}>
                <TabsList className="h-8 mb-3">
                  <TabsTrigger value="params" className="text-xs h-6 px-3">
                    {t("queryParams")}
                    {queryParams.some((p) => p.key) && (
                      <Badge variant="secondary" className="ml-1 h-4 px-1 text-[10px]">
                        {queryParams.filter((p) => p.key).length}
                      </Badge>
                    )}
                  </TabsTrigger>
                  <TabsTrigger value="headers" className="text-xs h-6 px-3">
                    {t("headers")}
                    {headers.some((h) => h.key) && (
                      <Badge variant="secondary" className="ml-1 h-4 px-1 text-[10px]">
                        {headers.filter((h) => h.key).length}
                      </Badge>
                    )}
                  </TabsTrigger>
                  <TabsTrigger value="body" className="text-xs h-6 px-3">
                    {t("body")}
                  </TabsTrigger>
                  <TabsTrigger value="env" className="text-xs h-6 px-3">
                    <Variable className="mr-1 h-3 w-3" />
                    {t("variables")}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="params">
                  <KeyValueEditor
                    pairs={queryParams}
                    onChange={handleParamsChange}
                    keyPlaceholder={t("paramKey")}
                    valuePlaceholder={t("paramValue")}
                  />
                </TabsContent>

                <TabsContent value="headers">
                  <KeyValueEditor
                    pairs={headers}
                    onChange={setHeaders}
                    keyPlaceholder={t("headerKey")}
                    valuePlaceholder={t("headerValue")}
                  />
                </TabsContent>

                <TabsContent value="body">
                  <div className="space-y-3">
                    <div className="flex items-center gap-2">
                      {(["none", "json", "form", "raw"] as BodyType[]).map((bt) => (
                        <Button
                          key={bt}
                          variant={bodyType === bt ? "default" : "outline"}
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => setBodyType(bt)}
                        >
                          {t(`bodyType.${bt}`)}
                        </Button>
                      ))}
                    </div>

                    {bodyType === "none" && (
                      <p className="text-sm text-muted-foreground py-4 text-center">
                        {t("noBody")}
                      </p>
                    )}

                    {(bodyType === "json" || bodyType === "raw") && (
                      <textarea
                        value={bodyContent}
                        onChange={(e) => setBodyContent(e.target.value)}
                        placeholder={
                          bodyType === "json"
                            ? '{\n  "key": "value"\n}'
                            : t("rawBodyPlaceholder")
                        }
                        className="w-full h-48 rounded-lg border border-border bg-muted/50 p-3 text-sm font-mono resize-y focus:outline-none focus:ring-2 focus:ring-ring"
                      />
                    )}

                    {bodyType === "form" && (
                      <KeyValueEditor
                        pairs={formData}
                        onChange={setFormData}
                        keyPlaceholder={t("formKey")}
                        valuePlaceholder={t("formValue")}
                      />
                    )}
                  </div>
                </TabsContent>

                <TabsContent value="env">
                  <EnvVariableEditor
                    variables={envVars}
                    onChange={setEnvVars}
                    hint={t("envHint")}
                    addLabel={t("addVariable")}
                  />
                </TabsContent>
              </Tabs>
            </div>
          </div>

          {/* Response Panel */}
          <div className="rounded-xl border border-border bg-card">
            <div className="border-b border-border px-4 py-3">
              <h3 className="text-sm font-semibold">{t("response")}</h3>
            </div>
            <div className="p-4">
              <ResponsePanel response={response} error={responseError} loading={loading} />
            </div>
          </div>
        </div>
      </div>
    </DashboardLayout>
  )
}

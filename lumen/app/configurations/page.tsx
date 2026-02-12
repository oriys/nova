"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Pagination } from "@/components/pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { healthApi, snapshotsApi, configApi, aiApi, type HealthStatus, type AIModelEntry, type AIPromptTemplateMeta } from "@/lib/api"
import { RefreshCw, Server, Database, HardDrive, Trash2, Save, CheckCircle, Sparkles, Eye, EyeOff, Loader2, FileText } from "lucide-react"
import { cn } from "@/lib/utils"
import { useAutoRefresh } from "@/lib/use-auto-refresh"

interface Snapshot {
  function_id: string
  function_name: string
  snap_size: number
  mem_size: number
  total_size: number
  created_at: string
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const k = 1024
  const sizes = ["B", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i]
}

function isHealthyComponent(value: unknown): boolean {
  if (typeof value === "boolean") return value
  if (typeof value === "string") return value.toLowerCase().startsWith("healthy")
  return false
}

function componentStatusText(value: unknown): string {
  if (typeof value === "string" && value.trim() !== "") return value
  if (typeof value === "boolean") return value ? "healthy" : "unhealthy"
  return "unknown"
}

function componentBadgeClass(value: unknown): string {
  const text = componentStatusText(value).toLowerCase()
  if (isHealthyComponent(value) || text === "ok") {
    return "bg-success/10 text-success border-0"
  }
  if (text === "unknown") {
    return "bg-muted text-muted-foreground border-0"
  }
  return "bg-destructive/10 text-destructive border-0"
}

function formatComponentLabel(name: string): string {
  if (!name) return "Unknown"
  return name
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

export default function ConfigurationsPage() {
  const t = useTranslations("pages")
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [snapshotsPage, setSnapshotsPage] = useState(1)
  const [snapshotsPageSize, setSnapshotsPageSize] = useState(10)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // Settings from backend
  const [poolTTL, setPoolTTL] = useState("60")
  const [logLevel, setLogLevel] = useState("info")
  const [dirty, setDirty] = useState(false)

  // AI Settings
  const [aiEnabled, setAiEnabled] = useState(false)
  const [aiBaseUrl, setAiBaseUrl] = useState("https://api.openai.com/v1")
  const [aiApiKey, setAiApiKey] = useState("")
  const [aiApiKeyInitial, setAiApiKeyInitial] = useState("")
  const [aiModel, setAiModel] = useState("gpt-4o-mini")
  const [aiPromptDir, setAiPromptDir] = useState("configs/prompts/ai")
  const [aiDirty, setAiDirty] = useState(false)
  const [aiSaving, setAiSaving] = useState(false)
  const [aiSaved, setAiSaved] = useState(false)
  const [showApiKey, setShowApiKey] = useState(false)
  const [aiModels, setAiModels] = useState<AIModelEntry[]>([])
  const [aiModelsLoading, setAiModelsLoading] = useState(false)
  const [promptTemplates, setPromptTemplates] = useState<AIPromptTemplateMeta[]>([])
  const [selectedPrompt, setSelectedPrompt] = useState("")
  const [promptContent, setPromptContent] = useState("")
  const [promptLoading, setPromptLoading] = useState(false)
  const [promptSaving, setPromptSaving] = useState(false)
  const [promptDirty, setPromptDirty] = useState(false)
  const [promptSaved, setPromptSaved] = useState(false)

  const fetchModels = useCallback(async () => {
    try {
      setAiModelsLoading(true)
      const resp = await aiApi.listModels()
      setAiModels(resp.data || [])
    } catch {
      setAiModels([])
    } finally {
      setAiModelsLoading(false)
    }
  }, [])

  const fetchPromptTemplates = useCallback(async () => {
    try {
      const resp = await aiApi.listPromptTemplates()
      const items = resp.items || []
      setPromptTemplates(items)
      setSelectedPrompt((prev) => (
        prev && items.some((item) => item.name === prev) ? prev : (items[0]?.name || "")
      ))
    } catch {
      setPromptTemplates([])
      setSelectedPrompt("")
    }
  }, [])

  const loadPromptTemplate = useCallback(async (name: string) => {
    if (!name) {
      setPromptContent("")
      setPromptDirty(false)
      return
    }
    try {
      setPromptLoading(true)
      const tpl = await aiApi.getPromptTemplate(name)
      setPromptContent(tpl.content || "")
      setPromptDirty(false)
      setPromptSaved(false)
    } catch (err) {
      console.error("Failed to load prompt template:", err)
      setError(err instanceof Error ? err.message : "Failed to load prompt template")
    } finally {
      setPromptLoading(false)
    }
  }, [])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [healthData, snapshotsData, configData, aiConfigData] = await Promise.all([
        healthApi.check(),
        snapshotsApi.list().catch(() => []),
        configApi.get().catch(() => ({} as Record<string, string>)),
        aiApi.getConfig().catch(() => null),
      ])
      setHealth(healthData)
      setSnapshots(snapshotsData)

      // Apply config from backend
      if (configData["pool_ttl"]) setPoolTTL(configData["pool_ttl"])
      if (configData["log_level"]) setLogLevel(configData["log_level"])
      setDirty(false)

      // Apply AI config
      if (aiConfigData) {
        setAiEnabled(aiConfigData.enabled)
        setAiBaseUrl(aiConfigData.base_url || "https://api.openai.com/v1")
        const apiKey = aiConfigData.api_key || ""
        setAiApiKey(apiKey)
        setAiApiKeyInitial(apiKey)
        setAiModel(aiConfigData.model || "gpt-4o-mini")
        setAiPromptDir(aiConfigData.prompt_dir || "configs/prompts/ai")
        setAiDirty(false)
      }
    } catch (err) {
      console.error("Failed to fetch config data:", err)
      setError(err instanceof Error ? err.message : "Failed to load configuration")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    fetchModels()
    fetchPromptTemplates()
  }, [fetchData, fetchModels, fetchPromptTemplates])

  useEffect(() => {
    loadPromptTemplate(selectedPrompt)
  }, [selectedPrompt, loadPromptTemplate])

  const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh("configurations", fetchData, 30000)

  const snapshotsTotalPages = Math.max(1, Math.ceil(snapshots.length / snapshotsPageSize))
  useEffect(() => {
    if (snapshotsPage > snapshotsTotalPages) setSnapshotsPage(snapshotsTotalPages)
  }, [snapshotsPage, snapshotsTotalPages])

  const pagedSnapshots = snapshots.slice(
    (snapshotsPage - 1) * snapshotsPageSize,
    snapshotsPage * snapshotsPageSize
  )
  const selectedPromptMeta = promptTemplates.find((item) => item.name === selectedPrompt)
  const healthComponents = health?.components ?? {}
  const knownComponentConfig = [
    { key: "postgres", label: "PostgreSQL", icon: Database },
    { key: "zenith", label: "Zenith", icon: Server },
    { key: "nova", label: "Nova", icon: Server },
    { key: "comet", label: "Comet", icon: Server },
    { key: "corona", label: "Corona", icon: Server },
    { key: "nebula", label: "Nebula", icon: Server },
    { key: "aurora", label: "Aurora", icon: Server },
  ] as const
  const knownKeys = new Set<string>(knownComponentConfig.map((item) => item.key))
  const extraComponentKeys = Object.keys(healthComponents)
    .filter((name) => name !== "pool" && !knownKeys.has(name))
    .sort()

  const handleSave = async () => {
    try {
      setSaving(true)
      setSaved(false)
      await configApi.update({
        pool_ttl: poolTTL,
        log_level: logLevel,
      })
      setDirty(false)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save config:", err)
      setError(err instanceof Error ? err.message : "Failed to save configuration")
    } finally {
      setSaving(false)
    }
  }

  const handleAiSave = async () => {
    try {
      setAiSaving(true)
      setAiSaved(false)
      const payload: {
        enabled: boolean
        model: string
        base_url: string
        prompt_dir: string
        api_key?: string
      } = {
        enabled: aiEnabled,
        base_url: aiBaseUrl,
        model: aiModel,
        prompt_dir: aiPromptDir,
      }

      // Avoid sending masked key back unchanged (prevents overwriting real key).
      if (aiApiKey !== aiApiKeyInitial) {
        payload.api_key = aiApiKey
      }

      const updated = await aiApi.updateConfig(payload)
      setAiEnabled(updated.enabled)
      setAiBaseUrl(updated.base_url || "https://api.openai.com/v1")
      const updatedApiKey = updated.api_key || ""
      setAiApiKey(updatedApiKey)
      setAiApiKeyInitial(updatedApiKey)
      setAiModel(updated.model || "gpt-4o-mini")
      setAiPromptDir(updated.prompt_dir || "configs/prompts/ai")
      await fetchPromptTemplates()
      setAiDirty(false)
      setAiSaved(true)
      setShowApiKey(false)
      setTimeout(() => setAiSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save AI config:", err)
      setError(err instanceof Error ? err.message : "Failed to save AI configuration")
    } finally {
      setAiSaving(false)
    }
  }

  const handleDeleteSnapshot = async (functionName: string) => {
    try {
      await snapshotsApi.delete(functionName)
      fetchData()
    } catch (err) {
      console.error("Failed to delete snapshot:", err)
    }
  }

  const handlePromptSave = async () => {
    if (!selectedPrompt) return
    try {
      setPromptSaving(true)
      setPromptSaved(false)
      const updated = await aiApi.updatePromptTemplate(selectedPrompt, { content: promptContent })
      setPromptContent(updated.content || "")
      setPromptDirty(false)
      setPromptSaved(true)
      await fetchPromptTemplates()
      setTimeout(() => setPromptSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save prompt template:", err)
      setError(err instanceof Error ? err.message : "Failed to save prompt template")
    } finally {
      setPromptSaving(false)
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title={t("configurations.title")} description={t("configurations.description")} />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load configuration</p>
            <p className="text-sm mt-1">{error}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => { setError(null); fetchData(); }}>
              Retry
            </Button>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("configurations.title")} description={t("configurations.description")} />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-end gap-2">
          <Button
            variant={autoRefresh ? "default" : "outline"}
            size="sm"
            onClick={toggleAutoRefresh}
          >
            <span className={cn(
              "mr-2 h-2 w-2 rounded-full",
              autoRefresh ? "bg-success animate-pulse" : "bg-muted-foreground"
            )} />
            Auto
          </Button>
          <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            Refresh
          </Button>
        </div>

        {/* System Health */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            System Health
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Status</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    health?.status === "ok"
                      ? "bg-success/10 text-success border-0"
                      : "bg-warning/10 text-warning border-0"
                  )}
                >
                  {loading ? "..." : health?.status || "unknown"}
                </Badge>
              </div>
            </div>

            {knownComponentConfig.map((item) => {
              const Icon = item.icon
              const value = healthComponents[item.key]
              return (
                <div key={item.key} className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
                  <Icon className="h-8 w-8 text-primary" />
                  <div>
                    <p className="text-sm text-muted-foreground">{item.label}</p>
                    <Badge
                      variant="secondary"
                      className={cn(componentBadgeClass(value))}
                    >
                      {loading ? "..." : componentStatusText(value)}
                    </Badge>
                  </div>
                </div>
              )
            })}

            {extraComponentKeys.map((name) => (
              <div key={name} className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
                <Server className="h-8 w-8 text-primary" />
                <div>
                  <p className="text-sm text-muted-foreground">{formatComponentLabel(name)}</p>
                  <Badge
                    variant="secondary"
                    className={cn(componentBadgeClass(healthComponents[name]))}
                  >
                    {loading ? "..." : componentStatusText(healthComponents[name])}
                  </Badge>
                </div>
              </div>
            ))}

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Active VMs</p>
                <p className="text-lg font-semibold">
                  {loading ? "..." : healthComponents.pool?.active_vms ?? 0}
                </p>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Pool Count</p>
                <p className="text-lg font-semibold">
                  {loading ? "..." : healthComponents.pool?.total_pools ?? 0}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Pool Settings */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            Pool Settings
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div className="space-y-2">
              <Label htmlFor="poolTTL">Idle VM TTL (seconds)</Label>
              <Input
                id="poolTTL"
                type="number"
                value={poolTTL}
                onChange={(e) => { setPoolTTL(e.target.value); setDirty(true); }}
                min="10"
                max="3600"
              />
              <p className="text-xs text-muted-foreground">
                Time before idle VMs are terminated
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="logLevel">Log Level</Label>
              <Select value={logLevel} onValueChange={(v) => { setLogLevel(v); setDirty(true); }}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="debug">Debug</SelectItem>
                  <SelectItem value="info">Info</SelectItem>
                  <SelectItem value="warn">Warn</SelectItem>
                  <SelectItem value="error">Error</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Minimum log level to capture
              </p>
            </div>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button
              variant="outline"
              onClick={handleSave}
              disabled={saving || !dirty}
            >
              {saving ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save Settings
                </>
              )}
            </Button>
            {saved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                Saved
              </span>
            )}
          </div>
        </div>

        {/* AI Settings */}
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center gap-2 mb-4">
            <Sparkles className="h-5 w-5 text-purple-500" />
            <h3 className="text-lg font-semibold text-card-foreground">
              AI Settings
            </h3>
            <Badge
              variant="secondary"
              className={cn(
                aiEnabled
                  ? "bg-success/10 text-success border-0"
                  : "bg-muted text-muted-foreground border-0"
              )}
            >
              {aiEnabled ? "Enabled" : "Disabled"}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mb-4">
            Configure the OpenAI-compatible API for AI-powered code generation, review, and rewriting.
          </p>
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiBaseUrl">API Base URL</Label>
              <Input
                id="aiBaseUrl"
                type="url"
                value={aiBaseUrl}
                onChange={(e) => { setAiBaseUrl(e.target.value); setAiDirty(true); }}
                placeholder="https://api.openai.com/v1"
              />
              <p className="text-xs text-muted-foreground">
                OpenAI API endpoint. Change this to use a compatible provider (e.g., Azure OpenAI, local LLM).
              </p>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiPromptDir">Prompt Directory</Label>
              <Input
                id="aiPromptDir"
                value={aiPromptDir}
                onChange={(e) => { setAiPromptDir(e.target.value); setAiDirty(true); }}
                placeholder="configs/prompts/ai"
              />
              <p className="text-xs text-muted-foreground">
                Directory used for editable AI prompt templates.
              </p>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiApiKey">API Key</Label>
              <div className="relative">
                <Input
                  id="aiApiKey"
                  type={showApiKey ? "text" : "password"}
                  value={aiApiKey}
                  onChange={(e) => { setAiApiKey(e.target.value); setAiDirty(true); }}
                  placeholder="sk-..."
                  className="pr-10"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowApiKey(!showApiKey)}
                  aria-label={showApiKey ? "Hide API key" : "Show API key"}
                >
                  {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                Your OpenAI API key. The key is stored encrypted and shown masked after saving.
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <Label htmlFor="aiModel">Model</Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  onClick={fetchModels}
                  disabled={aiModelsLoading}
                  aria-label="Refresh models"
                >
                  {aiModelsLoading ? (
                    <Loader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <RefreshCw className="h-3 w-3" />
                  )}
                </Button>
              </div>
              <Select value={aiModel} onValueChange={(v) => { setAiModel(v); setAiDirty(true); }}>
                <SelectTrigger>
                  <SelectValue placeholder={aiModelsLoading ? "Loading models..." : "Select a model"} />
                </SelectTrigger>
                <SelectContent>
                  {aiModels.length > 0 ? (
                    aiModels.map((m) => (
                      <SelectItem key={m.id} value={m.id}>{m.id}</SelectItem>
                    ))
                  ) : aiModel ? (
                    <SelectItem value={aiModel}>{aiModel}</SelectItem>
                  ) : null}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                AI model for code generation. Click refresh to load available models from the API.
              </p>
            </div>

            <div className="space-y-2">
              <Label>Enable AI Features</Label>
              <div className="flex items-center gap-3 pt-1">
                <Button
                  variant={aiEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setAiEnabled(true); setAiDirty(true); }}
                >
                  Enabled
                </Button>
                <Button
                  variant={!aiEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setAiEnabled(false); setAiDirty(true); }}
                >
                  Disabled
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                Toggle AI-powered features (generate, review, rewrite)
              </p>
            </div>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button
              variant="outline"
              onClick={handleAiSave}
              disabled={aiSaving || !aiDirty}
            >
              {aiSaving ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save AI Settings
                </>
              )}
            </Button>
            {aiSaved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                Saved
              </span>
            )}
          </div>
        </div>

        {/* AI Prompt Templates */}
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center justify-between gap-2 mb-4">
            <div className="flex items-center gap-2">
              <FileText className="h-5 w-5 text-primary" />
              <h3 className="text-lg font-semibold text-card-foreground">
                Prompt Templates
              </h3>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={fetchPromptTemplates}
              disabled={promptLoading}
            >
              <RefreshCw className={cn("mr-2 h-4 w-4", promptLoading && "animate-spin")} />
              Refresh Templates
            </Button>
          </div>
          <p className="text-sm text-muted-foreground mb-4">
            Manage centralized AI prompts. Changes apply to subsequent AI requests immediately.
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="promptTemplate">Template</Label>
              <Select value={selectedPrompt} onValueChange={(value) => { setSelectedPrompt(value); setPromptSaved(false); }}>
                <SelectTrigger id="promptTemplate">
                  <SelectValue placeholder="Select a prompt template" />
                </SelectTrigger>
                <SelectContent>
                  {promptTemplates.map((tpl) => (
                    <SelectItem key={tpl.name} value={tpl.name}>
                      {tpl.label || tpl.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {selectedPromptMeta && (
                <p className="text-xs text-muted-foreground">
                  {selectedPromptMeta.description}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label>Template Status</Label>
              <div className="flex items-center gap-2 pt-2">
                <Badge
                  variant="secondary"
                  className={cn(
                    selectedPromptMeta?.customized
                      ? "bg-success/10 text-success border-0"
                      : "bg-muted text-muted-foreground border-0"
                  )}
                >
                  {selectedPromptMeta?.customized ? "Customized" : "Default"}
                </Badge>
                {selectedPromptMeta?.file && (
                  <Badge variant="outline" className="font-mono">
                    {selectedPromptMeta.file} ({selectedPromptMeta.name})
                  </Badge>
                )}
              </div>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="promptContent">Template Content</Label>
              <Textarea
                id="promptContent"
                rows={20}
                value={promptContent}
                onChange={(e) => { setPromptContent(e.target.value); setPromptDirty(true); }}
                disabled={promptLoading || !selectedPrompt}
                className="font-mono text-xs"
                placeholder={selectedPrompt ? "Prompt template content..." : "Select a template first"}
              />
            </div>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button
              variant="outline"
              onClick={handlePromptSave}
              disabled={promptSaving || promptLoading || !promptDirty || !selectedPrompt}
            >
              {promptSaving ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save Prompt Template
                </>
              )}
            </Button>
            {promptSaved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                Saved
              </span>
            )}
          </div>
        </div>

        {/* Snapshots */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            VM Snapshots
          </h3>
          {snapshots.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4">
              No snapshots created yet. Create snapshots from the function detail page.
            </p>
          ) : (
            <div className="space-y-3">
              {pagedSnapshots.map((snap) => (
                <div
                  key={snap.function_id}
                  className="flex items-center justify-between p-4 rounded-lg bg-muted/50"
                >
                  <div className="flex items-center gap-3">
                    <HardDrive className="h-5 w-5 text-muted-foreground" />
                    <div>
                      <p className="font-medium">{snap.function_name}</p>
                      <p className="text-sm text-muted-foreground">
                        {formatBytes(snap.total_size)} ·{" "}
                        {new Date(snap.created_at).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDeleteSnapshot(snap.function_name)}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              ))}

              <div className="pt-2">
                <Pagination
                  totalItems={snapshots.length}
                  page={snapshotsPage}
                  pageSize={snapshotsPageSize}
                  onPageChange={setSnapshotsPage}
                  onPageSizeChange={(size) => {
                    setSnapshotsPageSize(size)
                    setSnapshotsPage(1)
                  }}
                  pageSizeOptions={[5, 10, 20, 50]}
                  itemLabel="snapshots"
                />
              </div>
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

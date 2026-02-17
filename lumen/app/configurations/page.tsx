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

function formatComponentLabel(name: string, fallbackLabel: string): string {
  if (!name) return fallbackLabel
  return name
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

function isSensitiveConfigKey(key: string): boolean {
  const normalized = key.toLowerCase()
  return normalized.includes("api_key") || normalized.includes("secret") || normalized.includes("password") || normalized.includes("token") || normalized.endsWith(".dsn")
}

export default function ConfigurationsPage() {
  const t = useTranslations("pages")
  const tc = useTranslations("configurations")
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [snapshotsPage, setSnapshotsPage] = useState(1)
  const [snapshotsPageSize, setSnapshotsPageSize] = useState(10)
  const [snapshotsTotal, setSnapshotsTotal] = useState(0)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // Settings from backend
  const [poolTTL, setPoolTTL] = useState("60")
  const [logLevel, setLogLevel] = useState("info")
  const [maxGlobalVMs, setMaxGlobalVMs] = useState("0")
  const [dirty, setDirty] = useState(false)
  const [dbConfig, setDbConfig] = useState<Record<string, string>>({})
  const [dbConfigDirty, setDbConfigDirty] = useState(false)
  const [dbConfigSaving, setDbConfigSaving] = useState(false)
  const [dbConfigSaved, setDbConfigSaved] = useState(false)

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
      const items = await aiApi.listModels(500)
      setAiModels(items || [])
    } catch {
      setAiModels([])
    } finally {
      setAiModelsLoading(false)
    }
  }, [])

  const fetchPromptTemplates = useCallback(async () => {
    try {
      const items = await aiApi.listPromptTemplates(500)
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
      setError(err instanceof Error ? err.message : tc("failedToLoadPromptTemplate"))
    } finally {
      setPromptLoading(false)
    }
  }, [tc])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const snapshotOffset = (snapshotsPage - 1) * snapshotsPageSize
      const [healthData, snapshotsData, configData, aiConfigData] = await Promise.all([
        healthApi.check(),
        snapshotsApi.listPage(snapshotsPageSize, snapshotOffset).catch(() => ({ items: [], total: 0 })),
        configApi.get().catch(() => ({} as Record<string, string>)),
        aiApi.getConfig().catch(() => null),
      ])
      setHealth(healthData)
      setSnapshots(snapshotsData.items || [])
      setSnapshotsTotal(snapshotsData.total || 0)

      // Apply config from backend
      if (configData["pool_ttl"]) setPoolTTL(configData["pool_ttl"])
      if (configData["log_level"]) setLogLevel(configData["log_level"])
      if (configData["max_global_vms"]) setMaxGlobalVMs(configData["max_global_vms"])
      setDirty(false)
      setDbConfig(configData)
      setDbConfigDirty(false)

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
      setError(err instanceof Error ? err.message : tc("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [snapshotsPage, snapshotsPageSize, tc])

  useEffect(() => {
    fetchData()
    fetchModels()
    fetchPromptTemplates()
  }, [fetchData, fetchModels, fetchPromptTemplates])

  useEffect(() => {
    loadPromptTemplate(selectedPrompt)
  }, [selectedPrompt, loadPromptTemplate])

  const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh("configurations", fetchData, 30000)

  const snapshotsTotalPages = Math.max(1, Math.ceil(snapshotsTotal / snapshotsPageSize))
  useEffect(() => {
    if (snapshotsPage > snapshotsTotalPages) setSnapshotsPage(snapshotsTotalPages)
  }, [snapshotsPage, snapshotsTotalPages])

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
  const formatStatusLabel = (value: unknown) => {
    const status = componentStatusText(value)
    const normalized = status.toLowerCase()
    if (normalized === "healthy") return tc("healthy")
    if (normalized === "unhealthy") return tc("unhealthy")
    if (normalized === "unknown") return tc("unknown")
    if (normalized === "ok") return tc("ok")
    return status
  }
  const formatHealthStatusLabel = (status?: string) => {
    if (!status) return tc("unknown")
    const normalized = status.toLowerCase()
    if (normalized === "ok") return tc("ok")
    if (normalized === "unknown") return tc("unknown")
    return status
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      setSaved(false)
      await configApi.update({
        pool_ttl: poolTTL,
        log_level: logLevel,
        max_global_vms: maxGlobalVMs,
      })
      setDbConfig((prev) => ({
        ...prev,
        pool_ttl: poolTTL,
        log_level: logLevel,
        max_global_vms: maxGlobalVMs,
      }))
      setDirty(false)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save config:", err)
      setError(err instanceof Error ? err.message : tc("failedToSaveConfiguration"))
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
      setError(err instanceof Error ? err.message : tc("failedToSaveAiConfiguration"))
    } finally {
      setAiSaving(false)
    }
  }

  const handleDbConfigSave = async () => {
    try {
      setDbConfigSaving(true)
      setDbConfigSaved(false)
      const payload = Object.fromEntries(
        Object.entries(dbConfig).filter(([key]) => !isSensitiveConfigKey(key))
      )
      const updated = await configApi.update(payload)
      setDbConfig(updated)
      if (updated["pool_ttl"]) setPoolTTL(updated["pool_ttl"])
      if (updated["log_level"]) setLogLevel(updated["log_level"])
      if (updated["max_global_vms"]) setMaxGlobalVMs(updated["max_global_vms"])
      setDbConfigDirty(false)
      setDirty(false)
      setDbConfigSaved(true)
      setTimeout(() => setDbConfigSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save database config:", err)
      setError(err instanceof Error ? err.message : tc("failedToSaveConfiguration"))
    } finally {
      setDbConfigSaving(false)
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
      setError(err instanceof Error ? err.message : tc("failedToSavePromptTemplate"))
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
            <p className="font-medium">{tc("failedToLoad")}</p>
            <p className="text-sm mt-1">{error}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => { setError(null); fetchData(); }}>
              {tc("retry")}
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
            {tc("auto")}
          </Button>
          <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        {/* System Health */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            {tc("systemHealth")}
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">{tc("status")}</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    health?.status === "ok"
                      ? "bg-success/10 text-success border-0"
                      : "bg-warning/10 text-warning border-0"
                  )}
                >
                  {loading ? "..." : formatHealthStatusLabel(health?.status)}
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
                      {loading ? "..." : formatStatusLabel(value)}
                    </Badge>
                  </div>
                </div>
              )
            })}

            {extraComponentKeys.map((name) => (
              <div key={name} className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
                <Server className="h-8 w-8 text-primary" />
                <div>
                  <p className="text-sm text-muted-foreground">{formatComponentLabel(name, tc("unknownComponent"))}</p>
                  <Badge
                    variant="secondary"
                    className={cn(componentBadgeClass(healthComponents[name]))}
                  >
                    {loading ? "..." : formatStatusLabel(healthComponents[name])}
                  </Badge>
                </div>
              </div>
            ))}

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">{tc("activeVMs")}</p>
                <p className="text-lg font-semibold">
                  {loading ? "..." : healthComponents.pool?.active_vms ?? 0}
                </p>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">{tc("poolCount")}</p>
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
            {tc("poolSettings")}
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div className="space-y-2">
              <Label htmlFor="poolTTL">{tc("idleVmTTL")}</Label>
              <Input
                id="poolTTL"
                type="number"
                value={poolTTL}
                onChange={(e) => { setPoolTTL(e.target.value); setDirty(true); }}
                min="10"
                max="3600"
              />
              <p className="text-xs text-muted-foreground">
                {tc("idleVmTTLHelp")}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="logLevel">{tc("logLevel")}</Label>
              <Select value={logLevel} onValueChange={(v) => { setLogLevel(v); setDirty(true); }}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="debug">{tc("logLevelDebug")}</SelectItem>
                  <SelectItem value="info">{tc("logLevelInfo")}</SelectItem>
                  <SelectItem value="warn">{tc("logLevelWarn")}</SelectItem>
                  <SelectItem value="error">{tc("logLevelError")}</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                {tc("logLevelHelp")}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="maxGlobalVMs">{tc("maxGlobalVMs")}</Label>
              <Input
                id="maxGlobalVMs"
                type="number"
                value={maxGlobalVMs}
                onChange={(e) => { setMaxGlobalVMs(e.target.value); setDirty(true); }}
                min="0"
              />
              <p className="text-xs text-muted-foreground">
                {tc("maxGlobalVMsHelp")}
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
                  {tc("saving")}
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  {tc("saveSettings")}
                </>
              )}
            </Button>
            {saved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                {tc("saved")}
              </span>
            )}
          </div>
        </div>

        {/* AI Settings */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-2">
            Database Configuration
          </h3>
          <p className="text-sm text-muted-foreground mb-4">
            All configuration entries persisted in the database.
          </p>
          <div className="max-h-72 overflow-auto rounded-lg border border-border">
            <div className="grid grid-cols-2 gap-2 p-3 text-xs font-medium text-muted-foreground border-b border-border bg-muted/30">
              <span>Key</span>
              <span>Value</span>
            </div>
            <div className="space-y-2 p-3">
              {Object.keys(dbConfig).sort().map((key) => {
                const sensitive = isSensitiveConfigKey(key)
                return (
                  <div key={key} className="grid grid-cols-2 gap-2 items-center">
                    <code className="text-xs">{key}</code>
                    <Input
                      value={sensitive ? "********" : dbConfig[key]}
                      disabled={sensitive}
                      onChange={(e) => {
                        setDbConfig((prev) => ({ ...prev, [key]: e.target.value }))
                        setDbConfigDirty(true)
                      }}
                    />
                  </div>
                )
              })}
            </div>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button variant="outline" onClick={handleDbConfigSave} disabled={dbConfigSaving || !dbConfigDirty}>
              {dbConfigSaving ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  {tc("saving")}
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save Database Config
                </>
              )}
            </Button>
            {dbConfigSaved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                {tc("saved")}
              </span>
            )}
          </div>
        </div>

        {/* AI Settings */}
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center gap-2 mb-4">
            <Sparkles className="h-5 w-5 text-purple-500" />
            <h3 className="text-lg font-semibold text-card-foreground">
              {tc("aiSettings")}
            </h3>
            <Badge
              variant="secondary"
              className={cn(
                aiEnabled
                  ? "bg-success/10 text-success border-0"
                  : "bg-muted text-muted-foreground border-0"
              )}
            >
              {aiEnabled ? tc("enabled") : tc("disabled")}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mb-4">
            {tc("aiDescription")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiBaseUrl">{tc("apiBaseUrl")}</Label>
              <Input
                id="aiBaseUrl"
                type="url"
                value={aiBaseUrl}
                onChange={(e) => { setAiBaseUrl(e.target.value); setAiDirty(true); }}
                placeholder={tc("apiBaseUrlPlaceholder")}
              />
              <p className="text-xs text-muted-foreground">
                {tc("apiBaseUrlHelp")}
              </p>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiPromptDir">{tc("promptDirectory")}</Label>
              <Input
                id="aiPromptDir"
                value={aiPromptDir}
                onChange={(e) => { setAiPromptDir(e.target.value); setAiDirty(true); }}
                placeholder={tc("promptDirectoryPlaceholder")}
              />
              <p className="text-xs text-muted-foreground">
                {tc("promptDirectoryHelp")}
              </p>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="aiApiKey">{tc("apiKey")}</Label>
              <div className="relative">
                <Input
                  id="aiApiKey"
                  type={showApiKey ? "text" : "password"}
                  value={aiApiKey}
                  onChange={(e) => { setAiApiKey(e.target.value); setAiDirty(true); }}
                  placeholder={tc("apiKeyPlaceholder")}
                  className="pr-10"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowApiKey(!showApiKey)}
                  aria-label={showApiKey ? tc("hideApiKey") : tc("showApiKey")}
                >
                  {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                {tc("apiKeyHelp")}
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <Label htmlFor="aiModel">{tc("model")}</Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  onClick={fetchModels}
                  disabled={aiModelsLoading}
                  aria-label={tc("refreshModels")}
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
                  <SelectValue placeholder={aiModelsLoading ? tc("loadingModels") : tc("selectModel")} />
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
                {tc("modelHelp")}
              </p>
            </div>

            <div className="space-y-2">
              <Label>{tc("enableAiFeatures")}</Label>
              <div className="flex items-center gap-3 pt-1">
                <Button
                  variant={aiEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setAiEnabled(true); setAiDirty(true); }}
                >
                  {tc("enabled")}
                </Button>
                <Button
                  variant={!aiEnabled ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setAiEnabled(false); setAiDirty(true); }}
                >
                  {tc("disabled")}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                {tc("toggleAiHelp")}
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
                  {tc("saving")}
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  {tc("saveAiSettings")}
                </>
              )}
            </Button>
            {aiSaved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                {tc("saved")}
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
                {tc("promptTemplates")}
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
              {tc("refreshTemplates")}
            </Button>
          </div>
          <p className="text-sm text-muted-foreground mb-4">
            {tc("promptTemplatesDesc")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="promptTemplate">{tc("template")}</Label>
              <Select value={selectedPrompt} onValueChange={(value) => { setSelectedPrompt(value); setPromptSaved(false); }}>
                <SelectTrigger id="promptTemplate">
                  <SelectValue placeholder={tc("selectPromptTemplate")} />
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
              <Label>{tc("templateStatus")}</Label>
              <div className="flex items-center gap-2 pt-2">
                <Badge
                  variant="secondary"
                  className={cn(
                    selectedPromptMeta?.customized
                      ? "bg-success/10 text-success border-0"
                      : "bg-muted text-muted-foreground border-0"
                  )}
                >
                  {selectedPromptMeta?.customized ? tc("customized") : tc("default")}
                </Badge>
                {selectedPromptMeta?.file && (
                  <Badge variant="outline" className="font-mono">
                    {selectedPromptMeta.file} ({selectedPromptMeta.name})
                  </Badge>
                )}
              </div>
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="promptContent">{tc("templateContent")}</Label>
              <Textarea
                id="promptContent"
                rows={20}
                value={promptContent}
                onChange={(e) => { setPromptContent(e.target.value); setPromptDirty(true); }}
                disabled={promptLoading || !selectedPrompt}
                className="font-mono text-xs"
                placeholder={selectedPrompt ? tc("promptPlaceholder") : tc("selectFirst")}
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
                  {tc("saving")}
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  {tc("savePromptTemplate")}
                </>
              )}
            </Button>
            {promptSaved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                {tc("saved")}
              </span>
            )}
          </div>
        </div>

        {/* Snapshots */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            {tc("vmSnapshots")}
          </h3>
          {snapshotsTotal === 0 ? (
            <p className="text-sm text-muted-foreground py-4">
              {tc("noSnapshots")}
            </p>
          ) : (
            <div className="space-y-3">
              {snapshots.map((snap) => (
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
                  totalItems={snapshotsTotal}
                  page={snapshotsPage}
                  pageSize={snapshotsPageSize}
                  onPageChange={setSnapshotsPage}
                  onPageSizeChange={(size) => {
                    setSnapshotsPageSize(size)
                    setSnapshotsPage(1)
                  }}
                  pageSizeOptions={[5, 10, 20, 50]}
                  itemLabel={tc("snapshotsLabel")}
                />
              </div>
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

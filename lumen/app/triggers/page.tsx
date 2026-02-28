"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { triggersApi, functionsApi } from "@/lib/api"
import type { TriggerEntry, TriggerType, NovaFunction } from "@/lib/api"
import type { ParamMapping } from "@/lib/types"
import { ParamMappingEditor } from "@/components/param-mapping-editor"
import { Zap, Plus, Trash2, RefreshCw, Pencil, Power } from "lucide-react"
import { cn } from "@/lib/utils"

const TRIGGER_TYPES: TriggerType[] = ["kafka", "rabbitmq", "redis", "filesystem", "webhook"]

function configSummary(trigger: TriggerEntry): string {
  const c = trigger.config
  if (!c) return "-"
  switch (trigger.type) {
    case "kafka":
      return [c.brokers, c.topic].filter(Boolean).join(" / ") || "-"
    case "rabbitmq":
      return [c.url, c.queue].filter(Boolean).join(" / ") || "-"
    case "redis":
      return [c.addr, c.stream].filter(Boolean).join(" / ") || "-"
    case "filesystem":
      return [c.path, c.pattern].filter(Boolean).join(" ") || "-"
    case "webhook": {
      const parts: string[] = []
      if (c.listen_addr) parts.push(String(c.listen_addr))
      if (c.path) parts.push(String(c.path))
      const pm = c.param_mapping
      if (Array.isArray(pm) && pm.length > 0) parts.push(`${pm.length} mappings`)
      return parts.join(" ") || "-"
    }
    default:
      return "-"
  }
}

export default function TriggersPage() {
  const t = useTranslations("pages")
  const tt = useTranslations("triggersPage")
  const tc = useTranslations("common")
  const [triggers, setTriggers] = useState<TriggerEntry[]>([])
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editingTrigger, setEditingTrigger] = useState<TriggerEntry | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  // Form state
  const [newName, setNewName] = useState("")
  const [newType, setNewType] = useState<TriggerType>(TRIGGER_TYPES[0])
  const [newFunctionName, setNewFunctionName] = useState("")
  const [newEnabled, setNewEnabled] = useState(true)

  // Kafka config
  const [kafkaBrokers, setKafkaBrokers] = useState("localhost:9092")
  const [kafkaTopic, setKafkaTopic] = useState("")
  const [kafkaGroup, setKafkaGroup] = useState("")

  // RabbitMQ config
  const [rabbitURL, setRabbitURL] = useState("amqp://guest:guest@localhost:5672/")
  const [rabbitQueue, setRabbitQueue] = useState("")
  const [rabbitExchange, setRabbitExchange] = useState("")

  // Redis config
  const [redisAddr, setRedisAddr] = useState("localhost:6379")
  const [redisStream, setRedisStream] = useState("")
  const [redisGroup, setRedisGroup] = useState("")
  const [redisConsumer, setRedisConsumer] = useState("")

  // Filesystem config
  const [fsPath, setFsPath] = useState("")
  const [fsPattern, setFsPattern] = useState("*")
  const [fsPollInterval, setFsPollInterval] = useState("60")

  // Webhook config
  const [webhookListenAddr, setWebhookListenAddr] = useState(":8090")
  const [webhookPath, setWebhookPath] = useState("/webhook")
  const [webhookSecret, setWebhookSecret] = useState("")
  const [webhookParamMappings, setWebhookParamMappings] = useState<ParamMapping[]>([])

  const fetchTriggers = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await triggersApi.list()
      setTriggers(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tt])

  const fetchFunctions = useCallback(async () => {
    try {
      const data = await functionsApi.list()
      setFunctions(data || [])
    } catch {
      // silent — dropdown will just be empty
    }
  }, [])

  useEffect(() => {
    fetchTriggers()
    fetchFunctions()
  }, [fetchTriggers, fetchFunctions])

  const resetForm = () => {
    setEditingTrigger(null)
    setNewName("")
    setNewType(TRIGGER_TYPES[0])
    setNewFunctionName("")
    setNewEnabled(true)
    setKafkaBrokers("localhost:9092")
    setKafkaTopic("")
    setKafkaGroup("")
    setRabbitURL("amqp://guest:guest@localhost:5672/")
    setRabbitQueue("")
    setRabbitExchange("")
    setRedisAddr("localhost:6379")
    setRedisStream("")
    setRedisGroup("")
    setRedisConsumer("")
    setFsPath("")
    setFsPattern("*")
    setFsPollInterval("60")
    setWebhookListenAddr(":8090")
    setWebhookPath("/webhook")
    setWebhookSecret("")
    setWebhookParamMappings([])
  }

  const populateForm = (tr: TriggerEntry) => {
    setEditingTrigger(tr)
    setNewName(tr.name)
    setNewType(tr.type as TriggerType)
    setNewFunctionName(tr.function_name)
    setNewEnabled(tr.enabled)
    const c = tr.config || {}
    switch (tr.type) {
      case "kafka":
        setKafkaBrokers(String(c.brokers || "localhost:9092"))
        setKafkaTopic(String(c.topic || ""))
        setKafkaGroup(String(c.group || ""))
        break
      case "rabbitmq":
        setRabbitURL(String(c.url || "amqp://guest:guest@localhost:5672/"))
        setRabbitQueue(String(c.queue || ""))
        setRabbitExchange(String(c.exchange || ""))
        break
      case "redis":
        setRedisAddr(String(c.addr || "localhost:6379"))
        setRedisStream(String(c.stream || ""))
        setRedisGroup(String(c.group || ""))
        setRedisConsumer(String(c.consumer || ""))
        break
      case "filesystem":
        setFsPath(String(c.path || ""))
        setFsPattern(String(c.pattern || "*"))
        setFsPollInterval(String(c.poll_interval || "60"))
        break
      case "webhook":
        setWebhookListenAddr(String(c.listen_addr || ":8090"))
        setWebhookPath(String(c.path || "/webhook"))
        setWebhookSecret(String(c.secret || ""))
        setWebhookParamMappings(Array.isArray(c.param_mapping) ? c.param_mapping as ParamMapping[] : [])
        break
    }
  }

  const buildConfig = (): Record<string, unknown> => {
    switch (newType) {
      case "kafka": {
        const cfg: Record<string, unknown> = { brokers: kafkaBrokers, topic: kafkaTopic }
        if (kafkaGroup.trim()) cfg.group = kafkaGroup.trim()
        return cfg
      }
      case "rabbitmq": {
        const cfg: Record<string, unknown> = { url: rabbitURL, queue: rabbitQueue }
        if (rabbitExchange.trim()) cfg.exchange = rabbitExchange.trim()
        return cfg
      }
      case "redis": {
        const cfg: Record<string, unknown> = { addr: redisAddr, stream: redisStream }
        if (redisGroup.trim()) cfg.group = redisGroup.trim()
        if (redisConsumer.trim()) cfg.consumer = redisConsumer.trim()
        return cfg
      }
      case "filesystem": {
        const cfg: Record<string, unknown> = { path: fsPath }
        if (fsPattern.trim()) cfg.pattern = fsPattern.trim()
        if (fsPollInterval.trim()) cfg.poll_interval = Number(fsPollInterval) || 60
        return cfg
      }
      case "webhook": {
        const cfg: Record<string, unknown> = { listen_addr: webhookListenAddr, path: webhookPath }
        if (webhookSecret.trim()) cfg.secret = webhookSecret.trim()
        if (webhookParamMappings.length > 0) cfg.param_mapping = webhookParamMappings
        return cfg
      }
      default:
        return {}
    }
  }

  const handleSave = async () => {
    if (!newName.trim() || !newFunctionName.trim()) return
    try {
      setSaving(true)
      const config = buildConfig()
      if (editingTrigger) {
        await triggersApi.update(editingTrigger.id, { enabled: newEnabled, config })
      } else {
        await triggersApi.create({
          name: newName.trim(),
          type: newType,
          function_name: newFunctionName.trim(),
          enabled: newEnabled,
          config,
        })
      }
      setDialogOpen(false)
      resetForm()
      fetchTriggers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(tt("deleteConfirm"))) return
    try {
      await triggersApi.delete(id)
      fetchTriggers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    }
  }

  const handleToggle = async (tr: TriggerEntry) => {
    try {
      setTogglingId(tr.id)
      await triggersApi.update(tr.id, { enabled: !tr.enabled })
      fetchTriggers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setTogglingId(null)
    }
  }

  const handleEdit = (tr: TriggerEntry) => {
    resetForm()
    populateForm(tr)
    setDialogOpen(true)
  }

  const isEditing = !!editingTrigger
  const needsWideDialog = newType === "webhook"

  return (
    <DashboardLayout>
      <Header title={t("triggers.title")} description={t("triggers.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-between">
          <Dialog open={dialogOpen} onOpenChange={(open) => { setDialogOpen(open); if (!open) resetForm() }}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                {tt("createTrigger")}
              </Button>
            </DialogTrigger>
            <DialogContent className={cn(needsWideDialog ? "max-w-2xl" : "max-w-lg")}>
              <DialogHeader>
                <DialogTitle>{isEditing ? tt("editTrigger") : tt("createTrigger")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colName")}</label>
                  <Input
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder={tt("namePlaceholder")}
                    disabled={isEditing}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colType")}</label>
                  <Select
                    value={newType}
                    onValueChange={(v) => setNewType(v as TriggerType)}
                    disabled={isEditing}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {TRIGGER_TYPES.map((type) => (
                        <SelectItem key={type} value={type}>{type}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colFunction")}</label>
                  <Select
                    value={newFunctionName}
                    onValueChange={setNewFunctionName}
                    disabled={isEditing}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={tt("selectFunction")} />
                    </SelectTrigger>
                    <SelectContent className="max-h-64">
                      {functions.map((fn) => (
                        <SelectItem key={fn.name} value={fn.name}>{fn.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Kafka config */}
                {newType === "kafka" && (
                  <div className="space-y-3 rounded-lg border border-border p-4">
                    <p className="text-sm font-medium text-muted-foreground">{tt("kafkaConfig")}</p>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("kafkaBrokers")}</label>
                      <Input value={kafkaBrokers} onChange={(e) => setKafkaBrokers(e.target.value)} placeholder="localhost:9092,host2:9092" />
                      <p className="text-xs text-muted-foreground">{tt("kafkaBrokersHint")}</p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("kafkaTopic")}</label>
                      <Input value={kafkaTopic} onChange={(e) => setKafkaTopic(e.target.value)} placeholder="my-events" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("kafkaGroup")}</label>
                      <Input value={kafkaGroup} onChange={(e) => setKafkaGroup(e.target.value)} placeholder={tt("kafkaGroupPlaceholder")} />
                    </div>
                  </div>
                )}

                {/* RabbitMQ config */}
                {newType === "rabbitmq" && (
                  <div className="space-y-3 rounded-lg border border-border p-4">
                    <p className="text-sm font-medium text-muted-foreground">{tt("rabbitmqConfig")}</p>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("rabbitmqURL")}</label>
                      <Input value={rabbitURL} onChange={(e) => setRabbitURL(e.target.value)} placeholder="amqp://guest:guest@localhost:5672/" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("rabbitmqQueue")}</label>
                      <Input value={rabbitQueue} onChange={(e) => setRabbitQueue(e.target.value)} placeholder="my-queue" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("rabbitmqExchange")}</label>
                      <Input value={rabbitExchange} onChange={(e) => setRabbitExchange(e.target.value)} placeholder={tt("optionalPlaceholder")} />
                    </div>
                  </div>
                )}

                {/* Redis config */}
                {newType === "redis" && (
                  <div className="space-y-3 rounded-lg border border-border p-4">
                    <p className="text-sm font-medium text-muted-foreground">{tt("redisConfig")}</p>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("redisAddr")}</label>
                        <Input value={redisAddr} onChange={(e) => setRedisAddr(e.target.value)} placeholder="localhost:6379" />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("redisStream")}</label>
                        <Input value={redisStream} onChange={(e) => setRedisStream(e.target.value)} placeholder="my-stream" />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("redisGroup")}</label>
                        <Input value={redisGroup} onChange={(e) => setRedisGroup(e.target.value)} placeholder={tt("kafkaGroupPlaceholder")} />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("redisConsumer")}</label>
                        <Input value={redisConsumer} onChange={(e) => setRedisConsumer(e.target.value)} placeholder={tt("optionalPlaceholder")} />
                      </div>
                    </div>
                  </div>
                )}

                {/* Filesystem config */}
                {newType === "filesystem" && (
                  <div className="space-y-3 rounded-lg border border-border p-4">
                    <p className="text-sm font-medium text-muted-foreground">{tt("filesystemConfig")}</p>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("fsPath")}</label>
                      <Input value={fsPath} onChange={(e) => setFsPath(e.target.value)} placeholder="/var/data/incoming" />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("fsPattern")}</label>
                        <Input value={fsPattern} onChange={(e) => setFsPattern(e.target.value)} placeholder="*.json" />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("fsPollInterval")}</label>
                        <Input type="number" min={1} value={fsPollInterval} onChange={(e) => setFsPollInterval(e.target.value)} placeholder="60" />
                      </div>
                    </div>
                  </div>
                )}

                {/* Webhook config */}
                {newType === "webhook" && (
                  <div className="space-y-3 rounded-lg border border-border p-4">
                    <p className="text-sm font-medium text-muted-foreground">{tt("webhookConfig")}</p>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("listenAddr")}</label>
                        <Input value={webhookListenAddr} onChange={(e) => setWebhookListenAddr(e.target.value)} placeholder=":8090" />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tt("webhookPath")}</label>
                        <Input value={webhookPath} onChange={(e) => setWebhookPath(e.target.value)} placeholder="/webhook" />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("hmacSecret")}</label>
                      <Input value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value)} placeholder={tt("hmacSecretPlaceholder")} type="password" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tt("paramMapping")}</label>
                      <p className="text-xs text-muted-foreground">{tt("paramMappingDescription")}</p>
                      <ParamMappingEditor value={webhookParamMappings} onChange={setWebhookParamMappings} disabled={saving} />
                    </div>
                  </div>
                )}

                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="enabled"
                    checked={newEnabled}
                    onChange={(e) => setNewEnabled(e.target.checked)}
                    className="h-4 w-4 rounded border-border"
                  />
                  <label htmlFor="enabled" className="text-sm font-medium">{tt("colEnabled")}</label>
                </div>
                <Button
                  className="w-full"
                  onClick={handleSave}
                  disabled={saving || !newName.trim() || !newFunctionName.trim()}
                >
                  {saving ? tc("loading") : isEditing ? tc("save") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchTriggers} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colType")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colFunction")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colConfig")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colEnabled")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tt("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={6} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : triggers.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                    <Zap className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tt("noTriggers")}
                  </td>
                </tr>
              ) : (
                triggers.map((trigger) => (
                  <tr key={trigger.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Zap className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{trigger.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-muted text-muted-foreground">
                        {trigger.type}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm font-mono text-muted-foreground">{trigger.function_name}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground font-mono max-w-[300px] truncate">
                      {configSummary(trigger)}
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => handleToggle(trigger)}
                        disabled={togglingId === trigger.id}
                        className={cn(
                          "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium cursor-pointer transition-colors",
                          trigger.enabled
                            ? "bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400 dark:hover:bg-green-900/50"
                            : "bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700",
                          togglingId === trigger.id && "opacity-50 pointer-events-none"
                        )}
                      >
                        <Power className="h-3 w-3" />
                        {trigger.enabled ? "On" : "Off"}
                      </button>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => handleEdit(trigger)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => handleDelete(trigger.id)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </DashboardLayout>
  )
}

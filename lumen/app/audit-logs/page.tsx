"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { SubNav } from "@/components/sub-nav"
import { EmptyState } from "@/components/empty-state"
import { Pagination } from "@/components/pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { ErrorBanner } from "@/components/ui/error-banner"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { auditLogsApi, AuditLogEntry, AuditLogFilter } from "@/lib/api"
import {
  RefreshCw,
  Search,
  ShieldAlert,
  Loader2,
  ChevronDown,
  ChevronUp,
  User,
  Key,
  Plus,
  Pencil,
  Trash2,
  Zap,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { toUserErrorMessage } from "@/lib/error-map"

const PAGE_SIZE = 30

const ACTION_ICONS: Record<string, typeof Plus> = {
  create: Plus,
  update: Pencil,
  delete: Trash2,
  invoke: Zap,
}

const ACTION_COLORS: Record<string, string> = {
  create: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  update: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  delete: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
  invoke: "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
}

const RESOURCE_TYPES = [
  "function",
  "secret",
  "config",
  "api-key",
  "tenant",
  "namespace",
  "workflow",
  "trigger",
  "volume",
  "layer",
  "gateway-route",
  "schedule",
  "role",
  "permission",
]

export default function AuditLogsPage() {
  const t = useTranslations("auditLogsPage")
  const tp = useTranslations("pages")
  const [logs, setLogs] = useState<AuditLogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedLog, setSelectedLog] = useState<AuditLogEntry | null>(null)

  // Filters
  const [actorFilter, setActorFilter] = useState("")
  const [actionFilter, setActionFilter] = useState("all")
  const [resourceTypeFilter, setResourceTypeFilter] = useState("all")
  const [sortDesc, setSortDesc] = useState(true)

  const fetchLogs = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const filter: AuditLogFilter = {}
      if (actorFilter.trim()) filter.actor = actorFilter.trim()
      if (actionFilter !== "all") filter.action = actionFilter
      if (resourceTypeFilter !== "all") filter.resource_type = resourceTypeFilter

      const offset = (page - 1) * PAGE_SIZE
      const result = await auditLogsApi.list(filter, PAGE_SIZE, offset)
      let items = result.items || []

      if (!sortDesc) {
        items = [...items].reverse()
      }

      setLogs(items)
      setTotal(result.total)
    } catch (err) {
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [page, actorFilter, actionFilter, resourceTypeFilter, sortDesc])

  useEffect(() => {
    fetchLogs()
  }, [fetchLogs])

  const handleReset = () => {
    setActorFilter("")
    setActionFilter("all")
    setResourceTypeFilter("all")
    setPage(1)
  }

  const formatTime = (iso: string) => {
    const d = new Date(iso)
    return d.toLocaleString()
  }

  const statusColor = (code: number) => {
    if (code >= 200 && code < 300) return "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
    if (code >= 400 && code < 500) return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
    return "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
  }

  return (
    <DashboardLayout>
      <Header title={t("title")} description={t("description")} />
      <div className="px-6 pt-4">
        <SubNav items={[
          { label: tp("rbac.title"), href: "/rbac" },
          { label: t("title"), href: "/audit-logs" },
        ]} />
      </div>
      <div className="p-6 space-y-4">
        {error && <ErrorBanner error={error} onRetry={() => setError(null)} />}

        {/* Filters */}
        <div className="flex flex-wrap items-center gap-3">
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder={t("filters.actor")}
              value={actorFilter}
              onChange={(e) => { setActorFilter(e.target.value); setPage(1) }}
              className="pl-9 w-48"
            />
          </div>
          <Select value={actionFilter} onValueChange={(v) => { setActionFilter(v); setPage(1) }}>
            <SelectTrigger className="w-40">
              <SelectValue placeholder={t("filters.allActions")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("filters.allActions")}</SelectItem>
              <SelectItem value="create">{t("actions.create")}</SelectItem>
              <SelectItem value="update">{t("actions.update")}</SelectItem>
              <SelectItem value="delete">{t("actions.delete")}</SelectItem>
              <SelectItem value="invoke">{t("actions.invoke")}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={resourceTypeFilter} onValueChange={(v) => { setResourceTypeFilter(v); setPage(1) }}>
            <SelectTrigger className="w-44">
              <SelectValue placeholder={t("filters.allResourceTypes")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("filters.allResourceTypes")}</SelectItem>
              {RESOURCE_TYPES.map((rt) => (
                <SelectItem key={rt} value={rt}>{rt}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="outline" size="sm" onClick={handleReset}>
            {t("filters.reset")}
          </Button>
          <div className="ml-auto">
            <Button variant="outline" size="sm" onClick={fetchLogs} disabled={loading}>
              {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            </Button>
          </div>
        </div>

        {/* Table */}
        {loading && logs.length === 0 ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : logs.length === 0 ? (
          <EmptyState icon={ShieldAlert} title={t("empty")} description={t("emptyDescription")} />
        ) : (
          <div className="border rounded-lg overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th
                      className="text-left px-4 py-3 font-medium cursor-pointer select-none"
                      onClick={() => setSortDesc(!sortDesc)}
                    >
                      <span className="inline-flex items-center gap-1">
                        {t("columns.time")}
                        {sortDesc ? <ChevronDown className="h-3 w-3" /> : <ChevronUp className="h-3 w-3" />}
                      </span>
                    </th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.actor")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.action")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.resource")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.method")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.path")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.status")}</th>
                    <th className="text-left px-4 py-3 font-medium">{t("columns.ip")}</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map((log) => {
                    const ActionIcon = ACTION_ICONS[log.action] || Zap
                    return (
                      <tr
                        key={log.id}
                        className="border-b hover:bg-muted/30 cursor-pointer transition-colors"
                        onClick={() => setSelectedLog(log)}
                      >
                        <td className="px-4 py-3 text-muted-foreground whitespace-nowrap">
                          {formatTime(log.created_at)}
                        </td>
                        <td className="px-4 py-3">
                          <span className="inline-flex items-center gap-1.5">
                            {log.actor_type === "apikey" ? (
                              <Key className="h-3.5 w-3.5 text-amber-500" />
                            ) : (
                              <User className="h-3.5 w-3.5 text-blue-500" />
                            )}
                            <span className="font-mono text-xs">{log.actor}</span>
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant="secondary" className={cn("gap-1", ACTION_COLORS[log.action])}>
                            <ActionIcon className="h-3 w-3" />
                            {log.action}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          <span className="font-medium">{log.resource_type}</span>
                          {log.resource_name && (
                            <span className="text-muted-foreground ml-1">/ {log.resource_name}</span>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant="outline" className="font-mono text-xs">
                            {log.http_method}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground max-w-[200px] truncate">
                          {log.http_path}
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant="secondary" className={statusColor(log.status_code)}>
                            {log.status_code}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                          {log.ip_address}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {total > PAGE_SIZE && (
          <Pagination
            totalItems={total}
            page={page}
            pageSize={PAGE_SIZE}
            onPageChange={setPage}
          />
        )}

        {/* Detail Dialog */}
        <Dialog open={!!selectedLog} onOpenChange={(open) => { if (!open) setSelectedLog(null) }}>
          <DialogContent className="w-full max-w-2xl max-h-[85dvh] overflow-y-auto overflow-x-hidden">
            <DialogHeader>
              <DialogTitle>{t("detail.title")}</DialogTitle>
            </DialogHeader>
            {selectedLog && (
              <div className="space-y-4 min-w-0">
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.time")}</p>
                    <p className="font-medium">{formatTime(selectedLog.created_at)}</p>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.actor")}</p>
                    <p className="font-medium font-mono break-all">{selectedLog.actor} ({selectedLog.actor_type})</p>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.action")}</p>
                    <Badge variant="secondary" className={cn("gap-1", ACTION_COLORS[selectedLog.action])}>
                      {selectedLog.action}
                    </Badge>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.resource")}</p>
                    <p className="font-medium break-all">{selectedLog.resource_type} / {selectedLog.resource_name}</p>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.method")}</p>
                    <Badge variant="outline" className="font-mono">{selectedLog.http_method}</Badge>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.path")}</p>
                    <p className="font-mono text-xs break-all">{selectedLog.http_path}</p>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.status")}</p>
                    <Badge variant="secondary" className={statusColor(selectedLog.status_code)}>
                      {selectedLog.status_code}
                    </Badge>
                  </div>
                  <div className="min-w-0">
                    <p className="text-muted-foreground">{t("columns.ip")}</p>
                    <p className="font-mono text-xs break-all">{selectedLog.ip_address}</p>
                  </div>
                </div>
                <div>
                  <p className="text-muted-foreground text-sm mb-1">{t("detail.userAgent")}</p>
                  <p className="text-xs font-mono bg-muted p-2 rounded break-all">{selectedLog.user_agent}</p>
                </div>
                {selectedLog.request_body && (
                  <div>
                    <p className="text-muted-foreground text-sm mb-1">{t("detail.requestBody")}</p>
                    <pre className="text-xs font-mono bg-muted p-3 rounded overflow-auto max-h-48 max-w-full whitespace-pre-wrap break-all">
                      {(() => {
                        try {
                          return JSON.stringify(JSON.parse(selectedLog.request_body), null, 2)
                        } catch {
                          return selectedLog.request_body
                        }
                      })()}
                    </pre>
                  </div>
                )}
                {selectedLog.response_summary && (
                  <div>
                    <p className="text-muted-foreground text-sm mb-1">{t("detail.responseSummary")}</p>
                    <pre className="text-xs font-mono bg-muted p-3 rounded overflow-auto max-h-48 max-w-full whitespace-pre-wrap break-all">
                      {selectedLog.response_summary}
                    </pre>
                  </div>
                )}
              </div>
            )}
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  )
}

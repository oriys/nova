"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { functionsApi, type FunctionStateEntry, type DurableExecution, type PaginatedList } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Trash2, Plus, RefreshCw, Database, Clock, Layers, Eye } from "lucide-react"

interface FunctionStateProps {
  functionName: string
}

export function FunctionState({ functionName }: FunctionStateProps) {
  const t = useTranslations("functionDetailPage.state")
  const [stateEntries, setStateEntries] = useState<FunctionStateEntry[]>([])
  const [durableExecutions, setDurableExecutions] = useState<DurableExecution[]>([])
  const [stateTotal, setStateTotal] = useState(0)
  const [durableTotal, setDurableTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [prefix, setPrefix] = useState("")
  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [selectedExec, setSelectedExec] = useState<DurableExecution | null>(null)

  const loadState = useCallback(async () => {
    try {
      const res = await functionsApi.getState(functionName, undefined, prefix || undefined, 50)
      if (res && typeof res === "object" && "items" in res) {
        const paged = res as PaginatedList<FunctionStateEntry>
        setStateEntries(paged.items || [])
        setStateTotal(paged.total || 0)
      }
    } catch {
      setStateEntries([])
    }
  }, [functionName, prefix])

  const loadDurableExecutions = useCallback(async () => {
    try {
      const res = await functionsApi.listDurableExecutions(functionName, 20)
      setDurableExecutions(res.items || [])
      setDurableTotal(res.total || 0)
    } catch {
      setDurableExecutions([])
    }
  }, [functionName])

  useEffect(() => {
    setLoading(true)
    Promise.all([loadState(), loadDurableExecutions()]).finally(() => setLoading(false))
  }, [loadState, loadDurableExecutions])

  const handlePutState = async () => {
    if (!newKey.trim()) return
    try {
      const parsed = JSON.parse(newValue || "null")
      await functionsApi.putState(functionName, newKey, parsed)
      setNewKey("")
      setNewValue("")
      loadState()
    } catch {
      // ignore parse errors
    }
  }

  const handleDeleteState = async (key: string) => {
    try {
      await functionsApi.deleteState(functionName, key)
      loadState()
    } catch {
      // ignore
    }
  }

  const handleViewExecution = async (id: string) => {
    try {
      const exec = await functionsApi.getDurableExecution(id)
      setSelectedExec(exec)
    } catch {
      // ignore
    }
  }

  const statusColor = (status: string) => {
    switch (status) {
      case "completed": return "bg-green-500/10 text-green-500"
      case "failed": return "bg-red-500/10 text-red-500"
      case "running": return "bg-blue-500/10 text-blue-500"
      case "suspended": return "bg-yellow-500/10 text-yellow-500"
      default: return "bg-gray-500/10 text-gray-500"
    }
  }

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleString()
    } catch {
      return ts
    }
  }

  const formatJSON = (val: unknown) => {
    try {
      return JSON.stringify(val, null, 2)
    } catch {
      return String(val)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Stats Overview */}
      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center justify-between pb-2">
            <span className="text-sm font-medium text-card-foreground">{t("stateEntries")}</span>
            <Database className="h-4 w-4 text-muted-foreground" />
          </div>
          <div className="text-2xl font-bold">{stateTotal}</div>
        </div>
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center justify-between pb-2">
            <span className="text-sm font-medium text-card-foreground">{t("durableExecutions")}</span>
            <Layers className="h-4 w-4 text-muted-foreground" />
          </div>
          <div className="text-2xl font-bold">{durableTotal}</div>
        </div>
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center justify-between pb-2">
            <span className="text-sm font-medium text-card-foreground">{t("activeExecutions")}</span>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </div>
          <div className="text-2xl font-bold">
            {durableExecutions.filter(e => e.status === "running" || e.status === "suspended").length}
          </div>
        </div>
      </div>

      {/* KV State Section */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-6 py-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold text-card-foreground">{t("kvState")}</h3>
            <Button variant="outline" size="sm" onClick={loadState}>
              <RefreshCw className="h-4 w-4 mr-1" />
              {t("refresh")}
            </Button>
          </div>
          <div className="flex gap-2 mt-2">
            <Input
              placeholder={t("filterPrefix")}
              value={prefix}
              onChange={e => setPrefix(e.target.value)}
              className="max-w-xs"
            />
          </div>
        </div>
        <div className="p-6">
          {/* Add new entry */}
          <div className="flex gap-2 mb-4">
            <Input
              placeholder={t("keyPlaceholder")}
              value={newKey}
              onChange={e => setNewKey(e.target.value)}
              className="max-w-[200px]"
            />
            <Input
              placeholder={t("valuePlaceholder")}
              value={newValue}
              onChange={e => setNewValue(e.target.value)}
              className="flex-1"
            />
            <Button size="sm" onClick={handlePutState} disabled={!newKey.trim()}>
              <Plus className="h-4 w-4 mr-1" />
              {t("add")}
            </Button>
          </div>

          {stateEntries.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {t("noStateEntries")}
            </div>
          ) : (
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-muted/30">
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("key")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("value")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground w-[80px]">{t("version")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground w-[160px]">{t("updatedAt")}</th>
                  <th className="px-4 py-3 w-[60px]"></th>
                </tr>
              </thead>
              <tbody>
                {stateEntries.map(entry => (
                  <tr key={entry.key} className="border-b border-border last:border-0 hover:bg-muted/20 transition-colors">
                    <td className="px-4 py-3 font-mono text-sm">{entry.key}</td>
                    <td className="px-4 py-3 font-mono text-sm max-w-[300px] truncate">
                      {formatJSON(entry.value)}
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant="outline">v{entry.version}</Badge>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {formatTime(entry.updated_at)}
                    </td>
                    <td className="px-4 py-3">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDeleteState(entry.key)}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Durable Executions Section */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-6 py-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold text-card-foreground">{t("durableExecutionsTitle")}</h3>
            <Button variant="outline" size="sm" onClick={loadDurableExecutions}>
              <RefreshCw className="h-4 w-4 mr-1" />
              {t("refresh")}
            </Button>
          </div>
        </div>
        <div className="p-6">
          {durableExecutions.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {t("noDurableExecutions")}
            </div>
          ) : (
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-muted/30">
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("executionId")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("status")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("steps")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("createdAt")}</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-muted-foreground">{t("duration")}</th>
                  <th className="px-4 py-3 w-[60px]"></th>
                </tr>
              </thead>
              <tbody>
                {durableExecutions.map(exec => (
                  <tr key={exec.id} className="border-b border-border last:border-0 hover:bg-muted/20 transition-colors">
                    <td className="px-4 py-3 font-mono text-sm">{exec.id}</td>
                    <td className="px-4 py-3">
                      <Badge className={statusColor(exec.status)}>
                        {exec.status}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">{exec.steps?.length || 0}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {formatTime(exec.created_at)}
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {exec.completed_at
                        ? `${((new Date(exec.completed_at).getTime() - new Date(exec.created_at).getTime()) / 1000).toFixed(1)}s`
                        : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <Dialog>
                        <DialogTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleViewExecution(exec.id)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        </DialogTrigger>
                        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
                          <DialogHeader>
                            <DialogTitle>{t("executionDetail")}</DialogTitle>
                          </DialogHeader>
                          {selectedExec && (
                            <div className="space-y-4">
                              <div className="grid grid-cols-2 gap-4 text-sm">
                                <div>
                                  <span className="text-muted-foreground">{t("executionId")}:</span>{" "}
                                  <span className="font-mono">{selectedExec.id}</span>
                                </div>
                                <div>
                                  <span className="text-muted-foreground">{t("status")}:</span>{" "}
                                  <Badge className={statusColor(selectedExec.status)}>
                                    {selectedExec.status}
                                  </Badge>
                                </div>
                                <div>
                                  <span className="text-muted-foreground">{t("createdAt")}:</span>{" "}
                                  {formatTime(selectedExec.created_at)}
                                </div>
                                {selectedExec.completed_at && (
                                  <div>
                                    <span className="text-muted-foreground">{t("completedAt")}:</span>{" "}
                                    {formatTime(selectedExec.completed_at)}
                                  </div>
                                )}
                              </div>

                              {selectedExec.error && (
                                <div className="rounded-md bg-destructive/10 p-3">
                                  <p className="text-sm text-destructive font-mono">{selectedExec.error}</p>
                                </div>
                              )}

                              {selectedExec.input !== undefined && selectedExec.input !== null && (
                                <div>
                                  <h4 className="text-sm font-medium mb-1">{t("input")}</h4>
                                  <pre className="text-xs bg-muted p-2 rounded-md overflow-auto max-h-32 font-mono">
                                    {formatJSON(selectedExec.input)}
                                  </pre>
                                </div>
                              )}

                              {selectedExec.output !== undefined && selectedExec.output !== null && (
                                <div>
                                  <h4 className="text-sm font-medium mb-1">{t("output")}</h4>
                                  <pre className="text-xs bg-muted p-2 rounded-md overflow-auto max-h-32 font-mono">
                                    {formatJSON(selectedExec.output)}
                                  </pre>
                                </div>
                              )}

                              {selectedExec.steps && selectedExec.steps.length > 0 && (
                                <div>
                                  <h4 className="text-sm font-medium mb-2">{t("steps")}</h4>
                                  <div className="space-y-2">
                                    {selectedExec.steps.map((step, i) => (
                                      <div key={step.id} className="flex items-center gap-3 p-2 rounded-md border">
                                        <div className="flex items-center justify-center w-6 h-6 rounded-full bg-muted text-xs font-medium">
                                          {i + 1}
                                        </div>
                                        <div className="flex-1 min-w-0">
                                          <div className="text-sm font-medium">{step.name}</div>
                                          <div className="text-xs text-muted-foreground">
                                            {step.duration_ms}ms
                                          </div>
                                        </div>
                                        <Badge className={statusColor(step.status)}>
                                          {step.status}
                                        </Badge>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              )}
                            </div>
                          )}
                        </DialogContent>
                      </Dialog>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

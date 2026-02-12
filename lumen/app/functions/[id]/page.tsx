"use client"

import { use, useEffect, useMemo, useState, useCallback } from "react"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { FunctionMetrics } from "@/components/function-metrics"
import { FunctionCode } from "@/components/function-code"
import { FunctionLogs } from "@/components/function-logs"
import { FunctionConfig } from "@/components/function-config"
import { FunctionDiagnosticsPanel } from "@/components/function-diagnostics"
import { FunctionSLOPanel } from "@/components/function-slo-panel"
import { FunctionDocs } from "@/components/function-docs"
import { InvocationHeatmap } from "@/components/invocation-heatmap"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { cn } from "@/lib/utils"
import { functionsApi, schedulesApi } from "@/lib/api"
import { markOnboardingStep } from "@/lib/onboarding-state"
import { transformFunction, transformLog, FunctionData, LogEntry } from "@/lib/types"
import type {
  FunctionMetrics as FunctionMetricsType,
  FunctionVersionEntry,
  ScheduleEntry,
  AsyncInvocationJob,
} from "@/lib/api"
import {
  ArrowLeft,
  Play,
  RefreshCw,
  Loader2,
  Plus,
  Trash2,
  Pencil,
  ToggleLeft,
  ToggleRight,
} from "lucide-react"

export default function FunctionDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>
  searchParams: Promise<{ tab?: string; request_id?: string }>
}) {
  const { id } = use(params)
  const query = use(searchParams)
  const requestedLogID =
    typeof query.request_id === "string" ? query.request_id.trim() : ""
  const requestedTab = typeof query.tab === "string" ? query.tab.trim() : ""
  const normalizedTab = useMemo(() => {
    const validTabs = new Set([
      "overview",
      "code",
      "logs",
      "diagnostics",
      "config",
      "versions",
      "schedules",
      "docs",
    ])
    if (validTabs.has(requestedTab)) {
      return requestedTab
    }
    if (requestedLogID) {
      return "logs"
    }
    return "overview"
  }, [requestedLogID, requestedTab])

  const [activeTab, setActiveTab] = useState(normalizedTab)
  const [func, setFunc] = useState<FunctionData | null>(null)
  const [metrics, setMetrics] = useState<FunctionMetricsType | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [invoking, setInvoking] = useState(false)
  const [invokeInput, setInvokeInput] = useState("{\n  \n}")
  const [invokeOutput, setInvokeOutput] = useState<string | null>(null)
  const [invokeError, setInvokeError] = useState<string | null>(null)
  const [invokeMeta, setInvokeMeta] = useState<string | null>(null)
  const [invokeMode, setInvokeMode] = useState<"sync" | "async">("sync")
  const [asyncJobs, setAsyncJobs] = useState<AsyncInvocationJob[]>([])
  const [loadingAsyncJobs, setLoadingAsyncJobs] = useState(false)
  const [retryingAsyncJobId, setRetryingAsyncJobId] = useState<string | null>(null)
  const [versions, setVersions] = useState<FunctionVersionEntry[]>([])
  const [schedules, setSchedules] = useState<ScheduleEntry[]>([])
  const [schedDialogOpen, setSchedDialogOpen] = useState(false)
  const [newCron, setNewCron] = useState("")
  const [newSchedInput, setNewSchedInput] = useState("")
  const [creatingSchedule, setCreatingSchedule] = useState(false)
  const [editingSchedule, setEditingSchedule] = useState<ScheduleEntry | null>(null)
  const [editCron, setEditCron] = useState("")
  const [editDialogOpen, setEditDialogOpen] = useState(false)

  const refreshAsyncJobs = useCallback(async (functionName?: string) => {
    const targetName = functionName
    if (!targetName) return
    setLoadingAsyncJobs(true)
    try {
      const jobs = await functionsApi.listAsyncInvocations(targetName, 50)
      setAsyncJobs(jobs || [])
    } catch (err) {
      console.error("Failed to fetch async invocations:", err)
      setAsyncJobs([])
    } finally {
      setLoadingAsyncJobs(false)
    }
  }, [])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      // id could be function ID or name, try to get by name first
      const fn = await functionsApi.get(id)
      const [fnMetrics, fnLogs, requestedLog] = await Promise.all([
        functionsApi.metrics(fn.name).catch(() => null),
        functionsApi.logs(fn.name, 20).catch(() => []),
        requestedLogID
          ? functionsApi.logsByRequest(fn.name, requestedLogID).catch(() => null)
          : Promise.resolve(null),
      ])

      // Fetch versions and schedules (non-blocking)
      functionsApi.listVersions(fn.name).then(v => setVersions(v || [])).catch(() => setVersions([]))
      schedulesApi.list(fn.name).then(s => setSchedules(s || [])).catch(() => setSchedules([]))
      refreshAsyncJobs(fn.name)

      setMetrics(fnMetrics)
      setFunc(transformFunction(fn, fnMetrics ?? undefined))
      const mergedLogs = requestedLog
        ? [requestedLog, ...fnLogs.filter((entry) => entry.id !== requestedLog.id)]
        : fnLogs
      setLogs(mergedLogs.map(transformLog))
    } catch (err) {
      console.error("Failed to fetch function:", err)
      setError(err instanceof Error ? err.message : "Failed to load function")
    } finally {
      setLoading(false)
    }
  }, [id, refreshAsyncJobs, requestedLogID])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  useEffect(() => {
    setActiveTab(normalizedTab)
  }, [normalizedTab])

  const handleInvoke = async () => {
    if (!func) return

    try {
      setInvoking(true)
      setInvokeError(null)
      setInvokeMeta(null)
      let payload: unknown = {}

      if (invokeInput.trim()) {
        try {
          payload = JSON.parse(invokeInput)
        } catch (parseError) {
          setInvokeError(
            parseError instanceof Error
              ? parseError.message
              : "Invalid JSON payload"
          )
          return
        }
      }

      if (invokeMode === "async") {
        const job = await functionsApi.invokeAsync(func.name, payload)
        markOnboardingStep("function_invoked", true)
        setInvokeOutput(
          JSON.stringify(
            {
              job_id: job.id,
              status: job.status,
              next_run_at: job.next_run_at,
              max_attempts: job.max_attempts,
            },
            null,
            2
          )
        )
        setInvokeMeta(`job_id: ${job.id} · status: ${job.status} · attempts: ${job.attempt}/${job.max_attempts}`)
        refreshAsyncJobs(func.name)
      } else {
        const response = await functionsApi.invoke(func.name, payload)
        markOnboardingStep("function_invoked", true)
        setInvokeOutput(JSON.stringify(response.output ?? null, null, 2))
        setInvokeMeta(
          `request_id: ${response.request_id} · duration: ${response.duration_ms} ms · ${response.cold_start ? "cold" : "warm"} start`
        )
        if (response.error) {
          setInvokeError(response.error)
        }
        // Refresh data after invocation
        fetchData()
      }
    } catch (err) {
      console.error("Failed to invoke function:", err)
      setInvokeError(err instanceof Error ? err.message : "Invocation failed")
    } finally {
      setInvoking(false)
    }
  }

  const handleRetryAsyncJob = async (jobID: string) => {
    setRetryingAsyncJobId(jobID)
    setInvokeError(null)
    try {
      const requeued = await functionsApi.retryAsyncInvocation(jobID)
      setInvokeMeta(`job_id: ${requeued.id} requeued · attempts reset to ${requeued.attempt}/${requeued.max_attempts}`)
      if (func) {
        refreshAsyncJobs(func.name)
      }
    } catch (err) {
      console.error("Failed to retry async job:", err)
      setInvokeError(err instanceof Error ? err.message : "Retry failed")
    } finally {
      setRetryingAsyncJobId(null)
    }
  }

  if (loading) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center h-[60vh]">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </DashboardLayout>
    )
  }

  if (error || !func) {
    return (
      <DashboardLayout>
        <div className="flex flex-col items-center justify-center h-[60vh]">
          <p className="text-muted-foreground mb-4">
            {error || "Function not found"}
          </p>
          <Button asChild variant="outline">
            <Link href="/functions">Back to Functions</Link>
          </Button>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      {/* Header */}
      <header className="sticky top-0 z-30 border-b border-border bg-card/80 backdrop-blur-sm">
        <div className="flex items-center justify-between px-6 py-4">
          <div className="flex items-center gap-4">
            <Button variant="ghost" size="icon" asChild>
              <Link href="/functions">
                <ArrowLeft className="h-4 w-4" />
              </Link>
            </Button>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-xl font-semibold text-foreground">
                  {func.name}
                </h1>
                <Badge
                  variant="secondary"
                  className={cn(
                    "text-xs font-medium",
                    func.status === "active" && "bg-success/10 text-success border-0",
                    func.status === "error" && "bg-destructive/10 text-destructive border-0",
                    func.status === "inactive" && "bg-muted text-muted-foreground border-0"
                  )}
                >
                  {func.status}
                </Badge>
              </div>
              <p className="text-sm text-muted-foreground mt-0.5">
                {func.runtime} · {func.region}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
              Refresh
            </Button>
            <Button
              size="sm"
              onClick={handleInvoke}
              disabled={invoking}
            >
              {invoking ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Play className="mr-2 h-4 w-4" />
              )}
              {invokeMode === "async" ? "Enqueue" : "Invoke"}
            </Button>
          </div>
        </div>

        {/* Tabs */}
        <Tabs value={activeTab} onValueChange={setActiveTab} className="px-6">
          <TabsList className="h-12 w-full justify-start rounded-none border-0 bg-transparent p-0">
            <TabsTrigger
              value="overview"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Overview
            </TabsTrigger>
            <TabsTrigger
              value="code"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Code
            </TabsTrigger>
            <TabsTrigger
              value="logs"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Logs
            </TabsTrigger>
            <TabsTrigger
              value="diagnostics"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Diagnostics
            </TabsTrigger>
            <TabsTrigger
              value="config"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Configuration
            </TabsTrigger>
            <TabsTrigger
              value="versions"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Versions
            </TabsTrigger>
            <TabsTrigger
              value="schedules"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Schedules
            </TabsTrigger>
            <TabsTrigger
              value="docs"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Docs
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </header>

      {/* Content */}
      <div className="p-6">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsContent value="overview" className="mt-0 space-y-6">
            <FunctionMetrics func={func} metrics={metrics} />
            <InvocationHeatmap functionName={func.name} />
          </TabsContent>

          <TabsContent value="code" className="mt-0">
            <FunctionCode
              func={func}
              invokeInput={invokeInput}
              onInvokeInputChange={setInvokeInput}
              invokeOutput={invokeOutput}
              invokeError={invokeError}
              invokeMeta={invokeMeta}
              invoking={invoking}
              invokeMode={invokeMode}
              onInvokeModeChange={setInvokeMode}
              asyncJobs={asyncJobs}
              loadingAsyncJobs={loadingAsyncJobs}
              retryingJobId={retryingAsyncJobId}
              onRefreshAsyncJobs={() => refreshAsyncJobs(func.name)}
              onRetryAsyncJob={handleRetryAsyncJob}
              onInvoke={handleInvoke}
            />
          </TabsContent>

          <TabsContent value="logs" className="mt-0">
            <FunctionLogs logs={logs} onRefresh={fetchData} highlightedRequestId={requestedLogID || undefined} />
          </TabsContent>

          <TabsContent value="diagnostics" className="mt-0 space-y-4">
            <FunctionSLOPanel functionName={func.name} />
            <FunctionDiagnosticsPanel functionName={func.name} />
          </TabsContent>

          <TabsContent value="config" className="mt-0">
            <FunctionConfig func={func} onUpdate={fetchData} />
          </TabsContent>

          <TabsContent value="versions" className="mt-0">
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Version</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Code Hash</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Handler</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Memory</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Timeout</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Mode</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Created</th>
                  </tr>
                </thead>
                <tbody>
                  {versions.length === 0 ? (
                    <tr>
                      <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground">
                        No versions published yet
                      </td>
                    </tr>
                  ) : (
                    versions.map((v) => (
                      <tr key={v.version} className="border-b border-border hover:bg-muted/50">
                        <td className="px-4 py-3">
                          <Badge variant="secondary" className="text-xs">v{v.version}</Badge>
                        </td>
                        <td className="px-4 py-3">
                          <code className="text-xs text-muted-foreground bg-muted px-2 py-1 rounded">
                            {v.code_hash ? v.code_hash.slice(0, 12) + "..." : "-"}
                          </code>
                        </td>
                        <td className="px-4 py-3 text-sm">{v.handler || "-"}</td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">{v.memory_mb} MB</td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">{v.timeout_s}s</td>
                        <td className="px-4 py-3">
                          <Badge variant="secondary" className="text-xs">{v.mode || "process"}</Badge>
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {new Date(v.created_at).toLocaleString()}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          <TabsContent value="schedules" className="mt-0 space-y-4">
            <div className="flex items-center justify-between">
              <Dialog open={schedDialogOpen} onOpenChange={setSchedDialogOpen}>
                <DialogTrigger asChild>
                  <Button size="sm">
                    <Plus className="mr-2 h-4 w-4" />
                    Create Schedule
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Create Schedule</DialogTitle>
                  </DialogHeader>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Cron Expression</label>
                      <Input
                        value={newCron}
                        onChange={(e) => setNewCron(e.target.value)}
                        placeholder="@every 5m"
                      />
                      <div className="flex flex-wrap gap-1.5">
                        {[
                          { label: "Every 1m", value: "@every 1m" },
                          { label: "Every 5m", value: "@every 5m" },
                          { label: "Every 15m", value: "*/15 * * * *" },
                          { label: "Every 30m", value: "*/30 * * * *" },
                          { label: "Hourly", value: "@hourly" },
                          { label: "Daily", value: "@daily" },
                          { label: "Weekly", value: "@weekly" },
                        ].map((preset) => (
                          <button
                            key={preset.value}
                            type="button"
                            className={cn(
                              "px-2 py-0.5 rounded text-xs border transition-colors",
                              newCron === preset.value
                                ? "bg-primary text-primary-foreground border-primary"
                                : "border-border text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                            )}
                            onClick={() => setNewCron(preset.value)}
                          >
                            {preset.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Input (optional JSON)</label>
                      <Textarea
                        value={newSchedInput}
                        onChange={(e) => setNewSchedInput(e.target.value)}
                        placeholder='{"key": "value"}'
                        className="min-h-[80px] font-mono text-xs"
                      />
                    </div>
                    <Button
                      className="w-full"
                      onClick={async () => {
                        if (!func || !newCron.trim()) return
                        setCreatingSchedule(true)
                        try {
                          let input: unknown = undefined
                          if (newSchedInput.trim()) {
                            input = JSON.parse(newSchedInput)
                          }
                          await schedulesApi.create(func.name, newCron.trim(), input)
                          setSchedDialogOpen(false)
                          setNewCron("")
                          setNewSchedInput("")
                          const updated = await schedulesApi.list(func.name)
                          setSchedules(updated || [])
                        } catch (err) {
                          console.error("Failed to create schedule:", err)
                        } finally {
                          setCreatingSchedule(false)
                        }
                      }}
                      disabled={creatingSchedule || !newCron.trim()}
                    >
                      {creatingSchedule ? "Creating..." : "Create"}
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>

              {/* Edit Schedule Dialog */}
              <Dialog open={editDialogOpen} onOpenChange={(open) => {
                setEditDialogOpen(open)
                if (!open) setEditingSchedule(null)
              }}>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Edit Schedule</DialogTitle>
                  </DialogHeader>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Cron Expression</label>
                      <Input
                        value={editCron}
                        onChange={(e) => setEditCron(e.target.value)}
                        placeholder="@every 5m"
                      />
                      <div className="flex flex-wrap gap-1.5">
                        {[
                          { label: "Every 1m", value: "@every 1m" },
                          { label: "Every 5m", value: "@every 5m" },
                          { label: "Every 15m", value: "*/15 * * * *" },
                          { label: "Every 30m", value: "*/30 * * * *" },
                          { label: "Hourly", value: "@hourly" },
                          { label: "Daily", value: "@daily" },
                          { label: "Weekly", value: "@weekly" },
                        ].map((preset) => (
                          <button
                            key={preset.value}
                            type="button"
                            className={cn(
                              "px-2 py-0.5 rounded text-xs border transition-colors",
                              editCron === preset.value
                                ? "bg-primary text-primary-foreground border-primary"
                                : "border-border text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                            )}
                            onClick={() => setEditCron(preset.value)}
                          >
                            {preset.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    <Button
                      className="w-full"
                      onClick={async () => {
                        if (!func || !editingSchedule || !editCron.trim()) return
                        try {
                          await schedulesApi.updateCron(func.name, editingSchedule.id, editCron.trim())
                          setEditDialogOpen(false)
                          setEditingSchedule(null)
                          const updated = await schedulesApi.list(func.name)
                          setSchedules(updated || [])
                        } catch (err) {
                          console.error("Failed to update schedule:", err)
                        }
                      }}
                      disabled={!editCron.trim() || editCron.trim() === editingSchedule?.cron_expression}
                    >
                      Save
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>
            </div>

            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Cron</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Last Run</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Created</th>
                    <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {schedules.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                        No schedules configured
                      </td>
                    </tr>
                  ) : (
                    schedules.map((s) => (
                      <tr key={s.id} className="border-b border-border hover:bg-muted/50">
                        <td className="px-4 py-3">
                          <code className="text-sm font-mono">{s.cron_expression}</code>
                        </td>
                        <td className="px-4 py-3">
                          <Badge
                            variant="secondary"
                            className={cn(
                              "text-xs",
                              s.enabled
                                ? "bg-success/10 text-success border-0"
                                : "bg-muted text-muted-foreground border-0"
                            )}
                          >
                            {s.enabled ? "Active" : "Disabled"}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {s.last_run_at ? new Date(s.last_run_at).toLocaleString() : "Never"}
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {new Date(s.created_at).toLocaleString()}
                        </td>
                        <td className="px-4 py-3 text-right">
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => {
                                setEditingSchedule(s)
                                setEditCron(s.cron_expression)
                                setEditDialogOpen(true)
                              }}
                              title="Edit"
                            >
                              <Pencil className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={async () => {
                                if (!func) return
                                await schedulesApi.toggle(func.name, s.id, !s.enabled)
                                const updated = await schedulesApi.list(func.name)
                                setSchedules(updated || [])
                              }}
                              title={s.enabled ? "Disable" : "Enable"}
                            >
                              {s.enabled ? (
                                <ToggleRight className="h-4 w-4 text-success" />
                              ) : (
                                <ToggleLeft className="h-4 w-4 text-muted-foreground" />
                              )}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={async () => {
                                if (!func) return
                                await schedulesApi.delete(func.name, s.id)
                                const updated = await schedulesApi.list(func.name)
                                setSchedules(updated || [])
                              }}
                            >
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
          </TabsContent>

          <TabsContent value="docs" className="mt-0">
            <FunctionDocs func={func} />
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  )
}

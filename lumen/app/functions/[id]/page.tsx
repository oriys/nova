"use client"

import { use, useEffect, useMemo, useState, useCallback, useRef } from "react"
import Link from "next/link"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { FunctionMetrics } from "@/components/function-metrics"
import { FunctionCode } from "@/components/function-code"
import { FunctionLogs } from "@/components/function-logs"
import { FunctionConfig } from "@/components/function-config"
import { FunctionDiagnosticsPanel } from "@/components/function-diagnostics"
import { FunctionSLOPanel } from "@/components/function-slo-panel"
import { FunctionDocs } from "@/components/function-docs"
import { FunctionGateway } from "@/components/function-gateway"
import { FunctionTestSuite } from "@/components/function-test-suite"
import { FunctionDependencies } from "@/components/function-dependencies"
import { FunctionState } from "@/components/function-state"
import { FunctionVersions } from "@/components/function-versions"
import { FunctionSchedules } from "@/components/function-schedules"
import { InvocationHeatmap } from "@/components/invocation-heatmap"
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
} from "lucide-react"

export default function FunctionDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>
  searchParams: Promise<{ tab?: string; request_id?: string }>
}) {
  const t = useTranslations("functionDetailPage")
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
      "config",
      "state",
      "triggers",
      "tests",
      "docs",
    ])
    // Backwards-compatible mapping from old tab names
    const tabAliases: Record<string, string> = {
      diagnostics: "logs",
      versions: "config",
      dependencies: "code",
      schedules: "triggers",
      gateway: "triggers",
    }
    if (validTabs.has(requestedTab)) {
      return requestedTab
    }
    if (tabAliases[requestedTab]) {
      return tabAliases[requestedTab]
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
  const [logsPage, setLogsPage] = useState(1)
  const [logsPageSize, setLogsPageSize] = useState(20)
  const [logsTotal, setLogsTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
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
  const loadedFunctionIDRef = useRef<string | null>(null)

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
      if (loadedFunctionIDRef.current !== id) {
        setLoading(true)
      } else {
        setRefreshing(true)
      }
      setError(null)

      // id could be function ID or name, try to get by name first
      const fn = await functionsApi.get(id)
      const logOffset = (logsPage - 1) * logsPageSize
      const [fnMetrics, fnLogs, requestedLog] = await Promise.all([
        functionsApi.metrics(fn.name).catch(() => null),
        functionsApi.logsPage(fn.name, logsPageSize, logOffset).catch(() => ({ items: [], total: 0 })),
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
      setLogsTotal(fnLogs.total || 0)
      const mergedLogs = requestedLog
        ? [requestedLog, ...fnLogs.items.filter((entry) => entry.id !== requestedLog.id)]
        : fnLogs.items
      setLogs(mergedLogs.map(transformLog))
    } catch (err) {
      console.error("Failed to fetch function:", err)
      setError(err instanceof Error ? err.message : t("loadFailed"))
    } finally {
      setLoading(false)
      setRefreshing(false)
      loadedFunctionIDRef.current = id
    }
  }, [id, logsPage, logsPageSize, refreshAsyncJobs, requestedLogID, t])

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
              : t("invalidJsonPayload")
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
        setInvokeMeta(t("invokeMetaAsync", {
          jobId: job.id,
          status: job.status,
          attempt: job.attempt,
          maxAttempts: job.max_attempts,
        }))
        refreshAsyncJobs(func.name)
      } else {
        const response = await functionsApi.invoke(func.name, payload)
        markOnboardingStep("function_invoked", true)
        setInvokeOutput(JSON.stringify(response.output ?? null, null, 2))
        setInvokeMeta(t("invokeMetaSync", {
          requestId: response.request_id,
          duration: response.duration_ms,
          startState: response.cold_start ? t("coldStart") : t("warmStart"),
        }))
        if (response.error) {
          setInvokeError(response.error)
        }
        // Refresh data after invocation
        fetchData()
      }
    } catch (err) {
      console.error("Failed to invoke function:", err)
      setInvokeError(err instanceof Error ? err.message : t("invocationFailed"))
    } finally {
      setInvoking(false)
    }
  }

  const handleRetryAsyncJob = async (jobID: string) => {
    setRetryingAsyncJobId(jobID)
    setInvokeError(null)
    try {
      const requeued = await functionsApi.retryAsyncInvocation(jobID)
      setInvokeMeta(t("retryMeta", {
        jobId: requeued.id,
        attempt: requeued.attempt,
        maxAttempts: requeued.max_attempts,
      }))
      if (func) {
        refreshAsyncJobs(func.name)
      }
    } catch (err) {
      console.error("Failed to retry async job:", err)
      setInvokeError(err instanceof Error ? err.message : t("retryFailed"))
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
            {error || t("functionNotFound")}
          </p>
          <Button asChild variant="outline">
            <Link href="/functions">{t("backToFunctions")}</Link>
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
                {func.backend && (
                  <>
                    {" · "}
                    <Badge variant="outline" className="text-[10px] px-1.5 py-0 font-mono align-middle">
                      {func.backend}
                    </Badge>
                  </>
                )}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={fetchData} disabled={loading || refreshing}>
              <RefreshCw className={cn("mr-2 h-4 w-4", (loading || refreshing) && "animate-spin")} />
              {t("refresh")}
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
              {invokeMode === "async" ? t("enqueue") : t("invoke")}
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
              {t("tabs.overview")}
            </TabsTrigger>
            <TabsTrigger
              value="code"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.code")}
            </TabsTrigger>
            <TabsTrigger
              value="logs"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.logs")}
            </TabsTrigger>
            <TabsTrigger
              value="config"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.configuration")}
            </TabsTrigger>
            {func && func.mode === "durable" && (
              <TabsTrigger
                value="state"
                className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
              >
                {t("tabs.state")}
              </TabsTrigger>
            )}
            <TabsTrigger
              value="triggers"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.triggers")}
            </TabsTrigger>
            <TabsTrigger
              value="tests"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.tests")}
            </TabsTrigger>
            <TabsTrigger
              value="docs"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              {t("tabs.docs")}
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

          <TabsContent value="code" className="mt-0 space-y-6">
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
            <FunctionDependencies func={func} onDependenciesSaved={() => fetchData()} />
          </TabsContent>

          <TabsContent value="logs" className="mt-0 space-y-6">
            <FunctionLogs
              logs={logs}
              onRefresh={fetchData}
              loading={loading || refreshing}
              highlightedRequestId={requestedLogID || undefined}
              page={logsPage}
              pageSize={logsPageSize}
              totalItems={logsTotal}
              onPageChange={setLogsPage}
              onPageSizeChange={(size) => {
                setLogsPageSize(size)
                setLogsPage(1)
              }}
            />
            <FunctionSLOPanel functionName={func.name} />
            <FunctionDiagnosticsPanel functionName={func.name} />
          </TabsContent>

          <TabsContent value="config" className="mt-0 space-y-6">
            <FunctionConfig func={func} onUpdate={fetchData} />
            <FunctionVersions versions={versions} />
          </TabsContent>

          {func.mode === "durable" && (
            <TabsContent value="state" className="mt-0 space-y-6">
              <FunctionState functionName={func.name} />
            </TabsContent>
          )}

          <TabsContent value="triggers" className="mt-0 space-y-6">
            <FunctionSchedules
              functionName={func.name}
              schedules={schedules}
              onSchedulesChange={setSchedules}
            />
            <FunctionGateway functionName={func.name} />
          </TabsContent>

          <TabsContent value="tests" className="mt-0">
            <FunctionTestSuite functionName={func.name} runtime={func.runtimeId} />
          </TabsContent>

          <TabsContent value="docs" className="mt-0">
            <FunctionDocs func={func} />
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  )
}

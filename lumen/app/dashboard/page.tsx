"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { useRouter } from "next/navigation"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { StatsCard } from "@/components/stats-card"
import { DashboardCharts, TimeSeriesData, type TimeRange } from "@/components/dashboard-charts"
import { ActiveFunctionsTable } from "@/components/active-functions-table"
import { RecentLogs } from "@/components/recent-logs"
import { OnboardingFlow } from "@/components/onboarding-flow"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Activity, Zap, AlertTriangle, Clock, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { functionsApi, gatewayApi, metricsApi } from "@/lib/api"
import { transformFunction, transformLog, FunctionData, LogEntry } from "@/lib/types"
import { useAutoRefresh } from "@/lib/use-auto-refresh"
import { syncOnboardingStateFromData } from "@/lib/onboarding-state"
import { cn } from "@/lib/utils"
import { GlobalHeatmap } from "@/components/global-heatmap"

export default function DashboardPage() {
  const t = useTranslations("pages")
  const td = useTranslations("dashboard")
  const router = useRouter()
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([])
  const [globalMetrics, setGlobalMetrics] = useState<{
    total: number
    success: number
    failed: number
    avgLatency: number
  }>({ total: 0, success: 0, failed: 0, avgLatency: 0 })
  const [, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [timeRange, setTimeRange] = useState<TimeRange>("1h")
  const [gatewayRouteCount, setGatewayRouteCount] = useState(0)

  const fetchData = useCallback(async (isRefresh = false, range?: TimeRange) => {
    try {
      if (isRefresh) {
        setRefreshing(true)
      } else {
        setLoading(true)
      }
      setError(null)

      const currentRange = range || timeRange

      // Fetch functions, metrics, and time-series in parallel
      const [funcs, metrics, timeSeriesData, routes] = await Promise.all([
        functionsApi.list(),
        metricsApi.global(),
        metricsApi.timeseries(currentRange).catch(() => []),
        gatewayApi.listRoutes().catch(() => []),
      ])

      // Transform functions with their metrics
      const transformedFuncs = funcs.map((fn) => {
        const funcMetrics = metrics.functions?.[fn.id]
        return transformFunction(fn, funcMetrics ? {
          function_id: fn.id,
          function_name: fn.name,
          invocations: funcMetrics,
          pool: { active_vms: 0, busy_vms: 0, idle_vms: 0 },
        } : undefined)
      })

      setFunctions(transformedFuncs)
      setGatewayRouteCount(routes.length || 0)

      // Transform time-series data (tenant-scoped from backend store)
      const chartData: TimeSeriesData[] = timeSeriesData.map((point) => ({
        time: point.timestamp,
        invocations: point.invocations,
        errors: point.errors,
        avgDuration: point.avg_duration,
      }))
      setTimeSeries(chartData)

      // Dashboard summary cards use tenant-scoped time-series aggregation
      const total = chartData.reduce((sum, point) => sum + point.invocations, 0)
      const failed = chartData.reduce((sum, point) => sum + point.errors, 0)
      const weightedDuration = chartData.reduce((sum, point) => sum + (point.avgDuration * point.invocations), 0)
      setGlobalMetrics({
        total,
        success: Math.max(0, total - failed),
        failed,
        avgLatency: total > 0 ? Math.round(weightedDuration / total) : 0,
      })
      syncOnboardingStateFromData({
        hasFunctionCreated: transformedFuncs.length > 0,
        hasFunctionInvoked: total > 0,
        hasGatewayRouteCreated: (routes?.length || 0) > 0,
      })

      // Fetch logs for active functions (take first few)
      const logsPromises = funcs.slice(0, 3).map((fn) =>
        functionsApi.logs(fn.name, 5).catch(() => [])
      )
      const allLogs = await Promise.all(logsPromises)
      const flatLogs = allLogs.flat().map(transformLog)
      // Sort by timestamp descending
      flatLogs.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
      setLogs(flatLogs.slice(0, 8))
    } catch (err) {
      console.error("Failed to fetch dashboard data:", err)
      setError(err instanceof Error ? err.message : td("failedToLoad"))
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [td, timeRange])

  useEffect(() => {
    fetchData(false)
  }, [fetchData])

  const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh("dashboard", () => fetchData(true), 30000)

  const handleRangeChange = useCallback((range: TimeRange) => {
    setTimeRange(range)
    fetchData(true, range)
  }, [fetchData])

  const totalInvocations = globalMetrics.total
  const totalErrors = globalMetrics.failed
  const avgDuration = globalMetrics.avgLatency
  const activeFunctions = functions.filter((fn) => fn.status === "active").length
  const errorRate = totalInvocations > 0 ? ((totalErrors / totalInvocations) * 100).toFixed(2) : "0.00"

  if (error) {
    return (
      <DashboardLayout>
        <Header title={t("dashboard.title")} description={t("dashboard.description")} />
        <div className="p-6">
          <ErrorBanner error={error} title={td("failedToLoad")} onRetry={() => fetchData(false)} />
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("dashboard.title")} description={t("dashboard.description")} />

      <div className="p-6 space-y-6">
        {/* Controls */}
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
            {td("auto")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => fetchData(true)} disabled={refreshing}>
            <RefreshCw className={cn("mr-2 h-4 w-4", refreshing && "animate-spin")} />
            {t("dashboard.refresh")}
          </Button>
        </div>

        <OnboardingFlow
          hasFunctionCreated={functions.length > 0}
          hasFunctionInvoked={globalMetrics.total > 0}
          hasGatewayRouteCreated={gatewayRouteCount > 0}
          onCreateFunction={() => router.push("/functions")}
        />

        {/* Stats Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatsCard
            title={td("totalInvocations")}
            value={totalInvocations.toLocaleString()}
            change={td("successful", { count: globalMetrics.success })}
            changeType="neutral"
            icon={Activity}
          />
          <StatsCard
            title={td("activeFunctions")}
            value={activeFunctions}
            change={td("totalCount", { count: functions.length })}
            changeType="neutral"
            icon={Zap}
          />
          <StatsCard
            title={td("errorRate")}
            value={`${errorRate}%`}
            change={td("errorsCount", { count: totalErrors })}
            changeType={totalErrors > 0 ? "negative" : "positive"}
            icon={AlertTriangle}
          />
          <StatsCard
            title={td("avgDuration")}
            value={`${avgDuration}ms`}
            change={td("perInvocation")}
            changeType="neutral"
            icon={Clock}
          />
        </div>

        {/* Charts */}
        <DashboardCharts
          data={timeSeries}
          range={timeRange}
          onRangeChange={handleRangeChange}
        />

        {/* Heatmap */}
        <GlobalHeatmap />

        {/* Tables */}
        <div className="grid gap-6 lg:grid-cols-2">
          <ActiveFunctionsTable functions={functions.slice(0, 5)} />
          <RecentLogs logs={logs.slice(0, 6)} />
        </div>
      </div>
    </DashboardLayout>
  )
}

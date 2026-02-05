"use client"

import { useEffect, useState, useCallback } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { StatsCard } from "@/components/stats-card"
import { DashboardCharts, TimeSeriesData } from "@/components/dashboard-charts"
import { ActiveFunctionsTable } from "@/components/active-functions-table"
import { RecentLogs } from "@/components/recent-logs"
import { Activity, Zap, AlertTriangle, Clock, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { functionsApi, metricsApi } from "@/lib/api"
import { transformFunction, transformLog, FunctionData, LogEntry } from "@/lib/types"
import { useAutoRefresh } from "@/lib/use-auto-refresh"
import { cn } from "@/lib/utils"

export default function DashboardPage() {
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [timeSeries, setTimeSeries] = useState<TimeSeriesData[]>([])
  const [globalMetrics, setGlobalMetrics] = useState<{
    total: number
    success: number
    failed: number
    avgLatency: number
  }>({ total: 0, success: 0, failed: 0, avgLatency: 0 })
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchData = useCallback(async (isRefresh = false) => {
    try {
      if (isRefresh) {
        setRefreshing(true)
      } else {
        setLoading(true)
      }
      setError(null)

      // Fetch functions, metrics, and time-series in parallel
      const [funcs, metrics, timeSeriesData] = await Promise.all([
        functionsApi.list(),
        metricsApi.global(),
        metricsApi.timeseries().catch(() => []),
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

      // Set global metrics
      setGlobalMetrics({
        total: metrics.invocations?.total ?? 0,
        success: metrics.invocations?.success ?? 0,
        failed: metrics.invocations?.failed ?? 0,
        avgLatency: Math.round(metrics.latency_ms?.avg ?? 0),
      })

      // Transform time-series data
      const chartData: TimeSeriesData[] = timeSeriesData.map((point) => ({
        time: point.timestamp,
        invocations: point.invocations,
        errors: point.errors,
        avgDuration: point.avg_duration,
      }))
      setTimeSeries(chartData)

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
      setError(err instanceof Error ? err.message : "Failed to load dashboard")
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    fetchData(false)
  }, [fetchData])

  const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh("dashboard", () => fetchData(true), 30000)

  const totalInvocations = globalMetrics.total
  const totalErrors = globalMetrics.failed
  const avgDuration = globalMetrics.avgLatency
  const activeFunctions = functions.filter((fn) => fn.status === "active").length
  const errorRate = totalInvocations > 0 ? ((totalErrors / totalInvocations) * 100).toFixed(2) : "0.00"

  if (error) {
    return (
      <DashboardLayout>
        <Header title="Dashboard" description="Overview of your serverless functions" />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load dashboard</p>
            <p className="text-sm mt-1">{error}</p>
            <p className="text-sm mt-2 text-muted-foreground">
              Make sure the Nova daemon is running on port 9000
            </p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title="Dashboard" description="Overview of your serverless functions" />

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
            {autoRefresh ? "Auto" : "Auto"}
          </Button>
          <Button variant="outline" size="sm" onClick={() => fetchData(true)} disabled={refreshing}>
            <RefreshCw className={cn("mr-2 h-4 w-4", refreshing && "animate-spin")} />
            Refresh
          </Button>
        </div>

        {/* Stats Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatsCard
            title="Total Invocations"
            value={totalInvocations.toLocaleString()}
            change={`${globalMetrics.success} successful`}
            changeType="neutral"
            icon={Activity}
          />
          <StatsCard
            title="Active Functions"
            value={activeFunctions}
            change={`${functions.length} total`}
            changeType="neutral"
            icon={Zap}
          />
          <StatsCard
            title="Error Rate"
            value={`${errorRate}%`}
            change={`${totalErrors} errors`}
            changeType={totalErrors > 0 ? "negative" : "positive"}
            icon={AlertTriangle}
          />
          <StatsCard
            title="Avg Duration"
            value={`${avgDuration}ms`}
            change="per invocation"
            changeType="neutral"
            icon={Clock}
          />
        </div>

        {/* Charts */}
        <DashboardCharts data={timeSeries} />

        {/* Tables */}
        <div className="grid gap-6 lg:grid-cols-2">
          <ActiveFunctionsTable functions={functions.slice(0, 5)} />
          <RecentLogs logs={logs.slice(0, 6)} />
        </div>
      </div>
    </DashboardLayout>
  )
}

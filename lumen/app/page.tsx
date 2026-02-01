"use client"

import { useEffect, useState } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { StatsCard } from "@/components/stats-card"
import { DashboardCharts } from "@/components/dashboard-charts"
import { ActiveFunctionsTable } from "@/components/active-functions-table"
import { RecentLogs } from "@/components/recent-logs"
import { Activity, Zap, AlertTriangle, Clock } from "lucide-react"
import { functionsApi, metricsApi } from "@/lib/api"
import { transformFunction, transformLog, FunctionData, LogEntry } from "@/lib/types"

export default function DashboardPage() {
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [globalMetrics, setGlobalMetrics] = useState<{
    total: number
    success: number
    failed: number
    avgLatency: number
  }>({ total: 0, success: 0, failed: 0, avgLatency: 0 })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchData() {
      try {
        setLoading(true)
        setError(null)

        // Fetch functions and metrics in parallel
        const [funcs, metrics] = await Promise.all([
          functionsApi.list(),
          metricsApi.global(),
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
      }
    }

    fetchData()
    // Refresh every 30 seconds
    const interval = setInterval(fetchData, 30000)
    return () => clearInterval(interval)
  }, [])

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
        {/* Stats Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatsCard
            title="Total Invocations"
            value={loading ? "..." : totalInvocations.toLocaleString()}
            change={loading ? "" : `${globalMetrics.success} successful`}
            changeType="neutral"
            icon={Activity}
          />
          <StatsCard
            title="Active Functions"
            value={loading ? "..." : activeFunctions}
            change={loading ? "" : `${functions.length} total`}
            changeType="neutral"
            icon={Zap}
          />
          <StatsCard
            title="Error Rate"
            value={loading ? "..." : `${errorRate}%`}
            change={loading ? "" : `${totalErrors} errors`}
            changeType={totalErrors > 0 ? "negative" : "positive"}
            icon={AlertTriangle}
          />
          <StatsCard
            title="Avg Duration"
            value={loading ? "..." : `${avgDuration}ms`}
            change={loading ? "" : "per invocation"}
            changeType="neutral"
            icon={Clock}
          />
        </div>

        {/* Charts */}
        <DashboardCharts />

        {/* Tables */}
        <div className="grid gap-6 lg:grid-cols-2">
          <ActiveFunctionsTable functions={functions.slice(0, 5)} loading={loading} />
          <RecentLogs logs={logs.slice(0, 6)} loading={loading} />
        </div>
      </div>
    </DashboardLayout>
  )
}

"use client"

import { useMemo } from "react"
import { useTranslations } from "next-intl"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from "recharts"
import type { Workflow, WorkflowVersion, WorkflowRun } from "@/lib/api"
import { Activity, CheckCircle2, XCircle, Clock, GitBranch, Layers } from "lucide-react"
import { Badge } from "@/components/ui/badge"

interface WorkflowMetricsProps {
  workflow: Workflow
  versions: WorkflowVersion[]
  runs: WorkflowRun[]
}

export function WorkflowMetrics({ workflow, versions, runs }: WorkflowMetricsProps) {
  const t = useTranslations("workflowDetailPage.metrics")

  const runStats = useMemo(() => {
    let succeeded = 0
    let failed = 0
    let running = 0
    let pending = 0
    let totalDurationMs = 0
    let completedCount = 0

    for (const run of runs) {
      switch (run.status) {
        case "succeeded":
          succeeded++
          break
        case "failed":
          failed++
          break
        case "running":
          running++
          break
        default:
          pending++
          break
      }
      if (run.finished_at && run.started_at) {
        const dur = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
        if (dur > 0) {
          totalDurationMs += dur
          completedCount++
        }
      }
    }

    const avgDuration = completedCount > 0 ? totalDurationMs / completedCount : 0
    const successRate = runs.length > 0 ? (succeeded / runs.length) * 100 : 0

    return { succeeded, failed, running, pending, avgDuration, successRate }
  }, [runs])

  const stats = [
    {
      label: t("totalRuns"),
      value: runs.length.toString(),
      icon: Activity,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: t("succeeded"),
      value: runStats.succeeded.toString(),
      icon: CheckCircle2,
      color: "text-success",
      bg: "bg-success/10",
    },
    {
      label: t("failed"),
      value: runStats.failed.toString(),
      icon: XCircle,
      color: runStats.failed > 0 ? "text-destructive" : "text-muted-foreground",
      bg: runStats.failed > 0 ? "bg-destructive/10" : "bg-muted",
    },
    {
      label: t("avgDuration"),
      value: runStats.avgDuration > 0 ? `${Math.round(runStats.avgDuration)}ms` : "-",
      icon: Clock,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: t("currentVersion"),
      value: workflow.current_version > 0 ? `v${workflow.current_version}` : "-",
      icon: GitBranch,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: t("totalVersions"),
      value: versions.length.toString(),
      icon: Layers,
      color: "text-primary",
      bg: "bg-primary/10",
    },
  ]

  // Status breakdown for pie chart
  const statusData = useMemo(() => {
    const data = []
    if (runStats.succeeded > 0) data.push({ name: t("succeeded"), value: runStats.succeeded, color: "var(--success, #22c55e)" })
    if (runStats.failed > 0) data.push({ name: t("failed"), value: runStats.failed, color: "var(--destructive, #ef4444)" })
    if (runStats.running > 0) data.push({ name: t("running"), value: runStats.running, color: "var(--primary, #3b82f6)" })
    if (runStats.pending > 0) data.push({ name: t("pending"), value: runStats.pending, color: "var(--muted-foreground, #94a3b8)" })
    return data
  }, [runStats, t])

  // Runs over time (group by day)
  const runsOverTime = useMemo(() => {
    if (runs.length === 0) return []
    const buckets: Record<string, { succeeded: number; failed: number; other: number }> = {}
    for (const run of runs) {
      const date = new Date(run.started_at || run.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric" })
      if (!buckets[date]) buckets[date] = { succeeded: 0, failed: 0, other: 0 }
      if (run.status === "succeeded") buckets[date].succeeded++
      else if (run.status === "failed") buckets[date].failed++
      else buckets[date].other++
    }
    return Object.entries(buckets).map(([date, counts]) => ({ date, ...counts }))
  }, [runs])

  return (
    <div className="space-y-6">
      {/* Status badge */}
      <div className="flex items-center gap-3">
        <span className="text-sm font-medium text-muted-foreground">{t("status")}</span>
        <Badge variant={workflow.status === "active" ? "default" : "secondary"}>
          {workflow.status}
        </Badge>
        {runs.length > 0 && (
          <span className="text-sm text-muted-foreground">
            · {t("successRate", { rate: runStats.successRate.toFixed(1) })}
          </span>
        )}
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-6">
        {stats.map((stat) => (
          <div
            key={stat.label}
            className="rounded-xl border border-border bg-card p-4"
          >
            <div className="flex items-center gap-2 mb-2">
              <div className={`rounded-md p-1.5 ${stat.bg}`}>
                <stat.icon className={`h-4 w-4 ${stat.color}`} />
              </div>
              <span className="text-xs font-medium text-muted-foreground">
                {stat.label}
              </span>
            </div>
            <p className="text-2xl font-semibold text-card-foreground">
              {stat.value}
            </p>
          </div>
        ))}
      </div>

      {/* Charts */}
      {runs.length > 0 ? (
        <div className="grid gap-6 lg:grid-cols-2">
          {/* Runs Over Time */}
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-card-foreground">{t("runsOverTime")}</h3>
              <p className="text-xs text-muted-foreground">{t("runsOverTimeDesc")}</p>
            </div>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={runsOverTime}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                  <XAxis
                    dataKey="date"
                    tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                    axisLine={{ stroke: "var(--border)" }}
                    tickLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                    axisLine={false}
                    tickLine={false}
                    allowDecimals={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "var(--popover)",
                      border: "1px solid var(--border)",
                      borderRadius: "8px",
                      fontSize: "12px",
                      color: "var(--popover-foreground)",
                    }}
                  />
                  <Bar dataKey="succeeded" stackId="a" fill="var(--success, #22c55e)" name={t("succeeded")} radius={[0, 0, 0, 0]} />
                  <Bar dataKey="failed" stackId="a" fill="var(--destructive, #ef4444)" name={t("failed")} radius={[0, 0, 0, 0]} />
                  <Bar dataKey="other" stackId="a" fill="var(--muted-foreground, #94a3b8)" name={t("other")} radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Status Breakdown */}
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-card-foreground">{t("statusBreakdown")}</h3>
              <p className="text-xs text-muted-foreground">{t("statusBreakdownDesc")}</p>
            </div>
            <div className="h-64 flex items-center justify-center">
              {statusData.length > 0 ? (
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={statusData}
                      cx="50%"
                      cy="50%"
                      innerRadius={60}
                      outerRadius={90}
                      paddingAngle={2}
                      dataKey="value"
                      label={({ name, value }) => `${name}: ${value}`}
                    >
                      {statusData.map((entry, index) => (
                        <Cell key={`cell-${index}`} fill={entry.color} />
                      ))}
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "var(--popover)",
                        border: "1px solid var(--border)",
                        borderRadius: "8px",
                        fontSize: "12px",
                        color: "var(--popover-foreground)",
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
              ) : (
                <p className="text-sm text-muted-foreground">{t("noData")}</p>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div className="rounded-xl border border-border bg-card p-8 text-center">
          <p className="text-sm text-muted-foreground">{t("noRunsYet")}</p>
        </div>
      )}
    </div>
  )
}

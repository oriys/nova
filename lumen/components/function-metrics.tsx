"use client"

import { useMemo } from "react"
import {
  AreaChart,
  Area,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"
import { FunctionData } from "@/lib/types"
import type { FunctionMetrics as FunctionMetricsType } from "@/lib/api"
import { Activity, Clock, Zap, AlertTriangle, HardDrive, Server } from "lucide-react"

interface FunctionMetricsProps {
  func: FunctionData
  metrics?: FunctionMetricsType | null
}

export function FunctionMetrics({ func: fn, metrics }: FunctionMetricsProps) {
  const invocations = metrics?.invocations?.invocations ?? fn.invocations
  const failures = metrics?.invocations?.failures ?? fn.errors
  const avgMs = metrics?.invocations?.avg_ms ?? fn.avgDuration
  const coldStarts = metrics?.invocations?.cold_starts ?? 0
  const warmStarts = metrics?.invocations?.warm_starts ?? 0
  const activeVMs = metrics?.pool?.active_vms ?? 0

  const invocationData = useMemo(() => {
    if (metrics?.timeseries && metrics.timeseries.length > 0) {
      return metrics.timeseries.map((p) => ({
        time: new Date(p.timestamp).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
        value: p.invocations,
        errors: p.errors,
      }))
    }
    return []
  }, [metrics?.timeseries])

  const durationData = useMemo(() => {
    if (metrics?.timeseries && metrics.timeseries.length > 0) {
      return metrics.timeseries.map((p) => ({
        time: new Date(p.timestamp).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
        avgDuration: Math.round(p.avg_duration),
      }))
    }
    return []
  }, [metrics?.timeseries])

  const coldStartRate = (coldStarts + warmStarts) > 0
    ? ((coldStarts / (coldStarts + warmStarts)) * 100).toFixed(1)
    : "0.0"

  const stats = [
    {
      label: "Invocations",
      value: invocations.toLocaleString(),
      icon: Activity,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: "Errors",
      value: failures.toString(),
      icon: AlertTriangle,
      color: failures > 0 ? "text-destructive" : "text-muted-foreground",
      bg: failures > 0 ? "bg-destructive/10" : "bg-muted",
    },
    {
      label: "Avg Duration",
      value: `${Math.round(avgMs)}ms`,
      icon: Clock,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: "Memory",
      value: `${fn.memory} MB`,
      icon: HardDrive,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: "Active VMs",
      value: activeVMs.toString(),
      icon: Server,
      color: "text-primary",
      bg: "bg-primary/10",
    },
    {
      label: "Cold Start Rate",
      value: `${coldStartRate}%`,
      icon: Zap,
      color: "text-primary",
      bg: "bg-primary/10",
    },
  ]

  const hasTimeSeriesData = invocationData.length > 0

  return (
    <div className="space-y-6">
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
      {hasTimeSeriesData ? (
        <div className="grid gap-6 lg:grid-cols-2">
          {/* Invocations Chart */}
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-card-foreground">
                Invocations
              </h3>
              <p className="text-xs text-muted-foreground">Last 24 hours</p>
            </div>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={invocationData}>
                  <defs>
                    <linearGradient id="invGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="oklch(0.55 0.18 250)" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="oklch(0.55 0.18 250)" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="oklch(0.90 0 0)" vertical={false} />
                  <XAxis
                    dataKey="time"
                    tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                    axisLine={{ stroke: "oklch(0.90 0 0)" }}
                    tickLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "oklch(1 0 0)",
                      border: "1px solid oklch(0.90 0 0)",
                      borderRadius: "8px",
                      fontSize: "12px",
                    }}
                  />
                  <Area
                    type="monotone"
                    dataKey="value"
                    stroke="oklch(0.55 0.18 250)"
                    strokeWidth={2}
                    fill="url(#invGradient)"
                    name="Invocations"
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Duration Chart */}
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-card-foreground">
                Execution Time
              </h3>
              <p className="text-xs text-muted-foreground">Last 24 hours</p>
            </div>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={durationData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="oklch(0.90 0 0)" vertical={false} />
                  <XAxis
                    dataKey="time"
                    tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                    axisLine={{ stroke: "oklch(0.90 0 0)" }}
                    tickLine={false}
                  />
                  <YAxis
                    tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "oklch(1 0 0)",
                      border: "1px solid oklch(0.90 0 0)",
                      borderRadius: "8px",
                      fontSize: "12px",
                    }}
                  />
                  <Line
                    type="monotone"
                    dataKey="avgDuration"
                    stroke="oklch(0.55 0.18 250)"
                    strokeWidth={2}
                    dot={false}
                    name="Avg (ms)"
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        </div>
      ) : (
        <div className="rounded-xl border border-border bg-card p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No invocation data yet. Invoke this function to see charts.
          </p>
        </div>
      )}
    </div>
  )
}

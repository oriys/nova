"use client"

import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"

export interface TimeSeriesData {
  time: string
  invocations: number
  errors: number
  avgDuration: number
}

interface DashboardChartsProps {
  data: TimeSeriesData[]
  loading?: boolean
}

export function DashboardCharts({ data, loading }: DashboardChartsProps) {
  // Format time for display (show hour)
  const formattedData = data.map((d) => ({
    ...d,
    time: new Date(d.time).toLocaleTimeString("en-US", {
      hour: "2-digit",
      minute: "2-digit",
    }),
  }))

  if (loading) {
    return (
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="mb-4">
            <div className="h-5 w-32 bg-muted rounded animate-pulse" />
            <div className="h-4 w-20 bg-muted rounded animate-pulse mt-1" />
          </div>
          <div className="h-64 bg-muted/50 rounded animate-pulse" />
        </div>
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="mb-4">
            <div className="h-5 w-32 bg-muted rounded animate-pulse" />
            <div className="h-4 w-20 bg-muted rounded animate-pulse mt-1" />
          </div>
          <div className="h-64 bg-muted/50 rounded animate-pulse" />
        </div>
      </div>
    )
  }

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {/* Invocations Chart */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold text-card-foreground">
            Function Invocations
          </h3>
          <p className="text-xs text-muted-foreground">Last 24 hours</p>
        </div>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={formattedData}>
              <defs>
                <linearGradient id="invocationsGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="oklch(0.55 0.18 250)" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="oklch(0.55 0.18 250)" stopOpacity={0} />
                </linearGradient>
                <linearGradient id="errorsGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="oklch(0.55 0.18 25)" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="oklch(0.55 0.18 25)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="oklch(0.90 0 0)" vertical={false} />
              <XAxis
                dataKey="time"
                tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                axisLine={{ stroke: "oklch(0.90 0 0)" }}
                tickLine={false}
                interval="preserveStartEnd"
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
                dataKey="invocations"
                stroke="oklch(0.55 0.18 250)"
                strokeWidth={2}
                fill="url(#invocationsGradient)"
                name="Invocations"
              />
              <Area
                type="monotone"
                dataKey="errors"
                stroke="oklch(0.55 0.18 25)"
                strokeWidth={2}
                fill="url(#errorsGradient)"
                name="Errors"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Duration Chart */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold text-card-foreground">
            Avg Execution Time (ms)
          </h3>
          <p className="text-xs text-muted-foreground">Last 24 hours</p>
        </div>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={formattedData}>
              <CartesianGrid strokeDasharray="3 3" stroke="oklch(0.90 0 0)" vertical={false} />
              <XAxis
                dataKey="time"
                tick={{ fontSize: 11, fill: "oklch(0.45 0 0)" }}
                axisLine={{ stroke: "oklch(0.90 0 0)" }}
                tickLine={false}
                interval="preserveStartEnd"
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
                formatter={(value: number) => [`${Math.round(value)}ms`, "Avg Duration"]}
              />
              <Bar
                dataKey="avgDuration"
                fill="oklch(0.55 0.18 250)"
                radius={[4, 4, 0, 0]}
                name="Avg Duration"
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

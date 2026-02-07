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

export const TIME_RANGES = [
  { value: "1m", label: "1m" },
  { value: "5m", label: "5m" },
  { value: "15m", label: "15m" },
  { value: "1h", label: "1h" },
  { value: "3h", label: "3h" },
  { value: "6h", label: "6h" },
  { value: "12h", label: "12h" },
  { value: "24h", label: "24h" },
  { value: "3d", label: "3d" },
  { value: "7d", label: "7d" },
  { value: "21d", label: "21d" },
] as const

export type TimeRange = (typeof TIME_RANGES)[number]["value"]

interface DashboardChartsProps {
  data: TimeSeriesData[]
  range: TimeRange
  onRangeChange: (range: TimeRange) => void
  loading?: boolean
}

function formatTime(iso: string, range: TimeRange): string {
  const d = new Date(iso)
  // For ranges <= 1h, show HH:mm:ss; for larger ranges show HH:mm
  if (range === "1m" || range === "5m" || range === "15m") {
    return d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", second: "2-digit" })
  }
  if (range === "3d" || range === "7d" || range === "21d") {
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric" }) + " " +
      d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: false })
  }
  if (range === "12h" || range === "24h") {
    return d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: false })
  }
  return d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" })
}

export function DashboardCharts({ data, range, onRangeChange, loading }: DashboardChartsProps) {
  const formattedData = data.map((d) => ({
    ...d,
    time: formatTime(d.time, range),
  }))

  const rangeSelector = (
    <div className="inline-flex items-center rounded-lg border border-border bg-muted/50 p-0.5 gap-0.5">
      {TIME_RANGES.map((r) => (
        <button
          key={r.value}
          type="button"
          onClick={() => onRangeChange(r.value)}
          className={`px-2 py-1 text-xs font-medium rounded-md transition-colors ${
            range === r.value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {r.label}
        </button>
      ))}
    </div>
  )

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="flex justify-end">{rangeSelector}</div>
        <div className="grid gap-6 lg:grid-cols-2">
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <div className="h-5 w-32 bg-muted rounded animate-pulse" />
            </div>
            <div className="h-64 bg-muted/50 rounded animate-pulse" />
          </div>
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="mb-4">
              <div className="h-5 w-32 bg-muted rounded animate-pulse" />
            </div>
            <div className="h-64 bg-muted/50 rounded animate-pulse" />
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">{rangeSelector}</div>
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Invocations Chart */}
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="mb-4">
            <h3 className="text-sm font-semibold text-card-foreground">
              Function Invocations
            </h3>
          </div>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={formattedData}>
                <defs>
                  <linearGradient id="invocationsGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="var(--foreground)" stopOpacity={0.2} />
                    <stop offset="95%" stopColor="var(--foreground)" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="errorsGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="var(--destructive)" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="var(--destructive)" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                <XAxis
                  dataKey="time"
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  axisLine={{ stroke: "var(--border)" }}
                  tickLine={false}
                  interval="preserveStartEnd"
                />
                <YAxis
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  axisLine={false}
                  tickLine={false}
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
                <Area
                  type="monotone"
                  dataKey="invocations"
                  stroke="var(--foreground)"
                  strokeWidth={2}
                  fill="url(#invocationsGradient)"
                  name="Invocations"
                />
                <Area
                  type="monotone"
                  dataKey="errors"
                  stroke="var(--destructive)"
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
          </div>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={formattedData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                <XAxis
                  dataKey="time"
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  axisLine={{ stroke: "var(--border)" }}
                  tickLine={false}
                  interval="preserveStartEnd"
                />
                <YAxis
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  axisLine={false}
                  tickLine={false}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "var(--popover)",
                    border: "1px solid var(--border)",
                    borderRadius: "8px",
                    fontSize: "12px",
                    color: "var(--popover-foreground)",
                  }}
                  formatter={(value: number) => [`${Math.round(value)}ms`, "Avg Duration"]}
                />
                <Bar
                  dataKey="avgDuration"
                  fill="var(--foreground)"
                  fillOpacity={0.8}
                  radius={[4, 4, 0, 0]}
                  name="Avg Duration"
                />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>
    </div>
  )
}

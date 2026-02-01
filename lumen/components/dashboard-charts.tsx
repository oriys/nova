"use client"

import { useMemo } from "react"
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

// Generate sample chart data
function generateInvocationData(hours: number) {
  const data = []
  const now = new Date()

  for (let i = hours; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000)
    data.push({
      time: time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
      invocations: Math.floor(Math.random() * 500) + 100,
      errors: Math.floor(Math.random() * 20),
    })
  }

  return data
}

function generateDurationData(hours: number) {
  const data = []
  const now = new Date()

  for (let i = hours; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000)
    data.push({
      time: time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
      avgDuration: Math.floor(Math.random() * 150) + 20,
      p95Duration: Math.floor(Math.random() * 300) + 80,
    })
  }

  return data
}

export function DashboardCharts() {
  const invocationData = useMemo(() => generateInvocationData(12), [])
  const durationData = useMemo(() => generateDurationData(12), [])

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {/* Invocations Chart */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold text-card-foreground">
            Function Invocations
          </h3>
          <p className="text-xs text-muted-foreground">Last 12 hours</p>
        </div>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={invocationData}>
              <defs>
                <linearGradient id="invocationsGradient" x1="0" y1="0" x2="0" y2="1">
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
                dataKey="invocations"
                stroke="oklch(0.55 0.18 250)"
                strokeWidth={2}
                fill="url(#invocationsGradient)"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Duration Chart */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold text-card-foreground">
            Execution Time (ms)
          </h3>
          <p className="text-xs text-muted-foreground">Last 12 hours</p>
        </div>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={durationData}>
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
              <Bar
                dataKey="avgDuration"
                fill="oklch(0.55 0.18 250)"
                radius={[4, 4, 0, 0]}
                name="Avg Duration"
              />
              <Bar
                dataKey="p95Duration"
                fill="oklch(0.75 0.10 250)"
                radius={[4, 4, 0, 0]}
                name="P95 Duration"
              />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

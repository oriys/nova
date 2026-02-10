"use client"

import { useCallback, useEffect, useState } from "react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { functionsApi, type FunctionDiagnostics } from "@/lib/api"
import { AlertTriangle, Clock, Flame, Loader2, RefreshCw, Timer } from "lucide-react"

interface FunctionDiagnosticsProps {
  functionName: string
}

export function FunctionDiagnosticsPanel({ functionName }: FunctionDiagnosticsProps) {
  const [windowValue, setWindowValue] = useState("24h")
  const [sampleValue, setSampleValue] = useState("1000")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [data, setData] = useState<FunctionDiagnostics | null>(null)

  const load = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const sampleRaw = Number.parseInt(sampleValue, 10)
      const sample = Number.isFinite(sampleRaw) && sampleRaw > 0 ? Math.min(sampleRaw, 5000) : 1000
      const diagnostics = await functionsApi.diagnostics(functionName, windowValue, sample)
      setData(diagnostics)
    } catch (err) {
      console.error("Failed to fetch diagnostics:", err)
      setError(err instanceof Error ? err.message : "Failed to fetch diagnostics")
    } finally {
      setLoading(false)
    }
  }, [functionName, sampleValue, windowValue])

  useEffect(() => {
    load()
  }, [load])

  const statCards = [
    {
      label: "P95",
      value: `${Math.round(data?.p95_duration_ms || 0)}ms`,
      icon: Timer,
    },
    {
      label: "P99",
      value: `${Math.round(data?.p99_duration_ms || 0)}ms`,
      icon: Clock,
    },
    {
      label: "Error Rate",
      value: `${(data?.error_rate_pct || 0).toFixed(1)}%`,
      icon: AlertTriangle,
    },
    {
      label: "Cold Start Rate",
      value: `${(data?.cold_start_rate_pct || 0).toFixed(1)}%`,
      icon: Flame,
    },
    {
      label: "Slow Threshold",
      value: `${Math.round(data?.slow_threshold_ms || 0)}ms`,
      icon: Timer,
    },
    {
      label: "Samples",
      value: String(data?.total_invocations || 0),
      icon: Clock,
    },
  ]

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">Window</p>
            <Select value={windowValue} onValueChange={setWindowValue}>
              <SelectTrigger className="w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1h">Last 1h</SelectItem>
                <SelectItem value="6h">Last 6h</SelectItem>
                <SelectItem value="24h">Last 24h</SelectItem>
                <SelectItem value="7d">Last 7d</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">Sample Size</p>
            <Input
              className="w-[140px]"
              type="number"
              min="1"
              max="5000"
              value={sampleValue}
              onChange={(e) => setSampleValue(e.target.value)}
            />
          </div>

          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            {loading ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {!data && loading ? (
        <div className="flex items-center justify-center h-32">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-6">
            {statCards.map((card) => (
              <div key={card.label} className="rounded-xl border border-border bg-card p-4">
                <div className="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <card.icon className="h-4 w-4" />
                  <span>{card.label}</span>
                </div>
                <p className="text-2xl font-semibold text-card-foreground">{card.value}</p>
              </div>
            ))}
          </div>

          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="border-b border-border px-4 py-3">
              <h3 className="text-sm font-semibold text-card-foreground">Top Slow Invocations</h3>
              <p className="text-xs text-muted-foreground">
                Showing up to 10 entries over threshold ({Math.round(data?.slow_threshold_ms || 0)}ms)
              </p>
            </div>
            {data?.slow_invocations?.length ? (
              <div className="divide-y divide-border">
                {data.slow_invocations.map((entry) => (
                  <div key={entry.id} className="grid gap-2 px-4 py-3 sm:grid-cols-4 sm:items-center">
                    <div className="font-mono text-xs text-muted-foreground">{entry.id}</div>
                    <div className="text-sm text-card-foreground">{entry.duration_ms}ms</div>
                    <div className="text-xs text-muted-foreground">
                      {new Date(entry.created_at).toLocaleString()}
                    </div>
                    <div className="text-xs">
                      {entry.success ? (
                        <span className="text-success">success</span>
                      ) : (
                        <span className="text-destructive">{entry.error_message || "failed"}</span>
                      )}
                    </div>
                    <div className="sm:col-span-4 flex justify-end">
                      <Button asChild variant="outline" size="sm">
                        <Link
                          href={`/functions/${encodeURIComponent(functionName)}?tab=logs&request_id=${encodeURIComponent(entry.id)}`}
                        >
                          View Logs
                        </Link>
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="px-4 py-8 text-center text-sm text-muted-foreground">
                No slow invocations in current window.
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { RefreshCw, CheckCircle2, XCircle, FlaskConical, History, AlertTriangle, ArrowDownRight, ArrowUpRight } from "lucide-react"

interface FunctionSignals {
  function_id: string
  cold_starts: number
  warm_starts: number
  cold_ratio: number
  p50_latency: number
  p95_latency: number
  p99_latency: number
  avg_queue_wait: number
  avg_boot_time: number
  avg_exec_time: number
  snapshot_hit_rate: number
  vm_busy_ratio: number
  vm_count: number
  request_rate_per_sec: number
  error_rate: number
}

interface Recommendation {
  parameter: string
  current: string
  suggested: string
  reason: string
  confidence: "high" | "medium" | "low"
  impact: string
}

interface Experiment {
  id: string
  function_id: string
  parameter: string
  control_value: string
  experiment_value: string
  traffic_percent: number
  status: string
  started_at: string
  completed_at?: string
  verdict?: string
}

interface TuningResponse {
  function_id: string
  signals: FunctionSignals
  recommendations: Recommendation[]
  active_experiments: Experiment[]
  generated_at: string
}

interface TuningHistoryEntry {
  id: string
  function_id: string
  parameter: string
  old_value: string
  new_value: string
  source: string
  applied_at: string
}

function formatDuration(ns: number): string {
  if (ns <= 0) return "—"
  const ms = ns / 1_000_000
  if (ms < 1) return `${(ns / 1000).toFixed(0)}µs`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function formatPercent(v: number): string {
  return `${(v * 100).toFixed(1)}%`
}

function confidenceBadge(c: string) {
  const colors: Record<string, string> = {
    high: "bg-green-500/10 text-green-600 border-green-500/20",
    medium: "bg-yellow-500/10 text-yellow-600 border-yellow-500/20",
    low: "bg-muted text-muted-foreground border-border",
  }
  return (
    <span className={cn("inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium", colors[c] || colors.low)}>
      {c}
    </span>
  )
}

function statusBadge(s: string) {
  const colors: Record<string, string> = {
    running: "bg-blue-500/10 text-blue-600 border-blue-500/20",
    promoted: "bg-green-500/10 text-green-600 border-green-500/20",
    rolled_back: "bg-red-500/10 text-red-600 border-red-500/20",
    pending: "bg-muted text-muted-foreground border-border",
    failed: "bg-red-500/10 text-red-600 border-red-500/20",
  }
  return (
    <span className={cn("inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium", colors[s] || colors.pending)}>
      {s.replace("_", " ")}
    </span>
  )
}

export default function TuningPage() {
  const t = useTranslations("pages")
  const tt = useTranslations("tuningPage")
  const tc = useTranslations("common")
  const [functions, setFunctions] = useState<string[]>([])
  const [selectedFn, setSelectedFn] = useState<string>("")
  const [tuning, setTuning] = useState<TuningResponse | null>(null)
  const [history, setHistory] = useState<TuningHistoryEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchFunctions = useCallback(async () => {
    try {
      const res = await fetch("/api/functions")
      if (res.ok) {
        const data = await res.json()
        const names = (data || []).map((f: { name: string }) => f.name)
        setFunctions(names)
        if (names.length > 0 && !selectedFn) setSelectedFn(names[0])
      }
    } catch {
      /* ignore */
    }
  }, [selectedFn])

  const fetchTuning = useCallback(async () => {
    if (!selectedFn) return
    try {
      setLoading(true)
      setError(null)
      const res = await fetch(`/api/functions/${selectedFn}/tuning`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data: TuningResponse = await res.json()
      setTuning(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [selectedFn, tt])

  const fetchHistory = useCallback(async () => {
    if (!selectedFn) return
    try {
      const res = await fetch(`/api/functions/${selectedFn}/tuning/history`)
      if (res.ok) {
        const data = await res.json()
        setHistory(data || [])
      }
    } catch {
      /* ignore */
    }
  }, [selectedFn])

  useEffect(() => {
    fetchFunctions()
  }, [fetchFunctions])

  useEffect(() => {
    if (selectedFn) {
      fetchTuning()
      fetchHistory()
    }
  }, [selectedFn, fetchTuning, fetchHistory])

  const handleApply = async (index: number) => {
    try {
      const res = await fetch(`/api/functions/${selectedFn}/tuning/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ recommendation_index: index }),
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      fetchTuning()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToApply"))
    }
  }

  const signals = tuning?.signals

  return (
    <DashboardLayout>
      <Header title={t("tuning.title")} description={t("tuning.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        {/* Function selector + refresh */}
        <div className="flex items-center gap-4">
          <select
            value={selectedFn}
            onChange={(e) => setSelectedFn(e.target.value)}
            className="rounded-lg border border-border bg-card px-3 py-2 text-sm"
          >
            {functions.map((fn) => (
              <option key={fn} value={fn}>{fn}</option>
            ))}
          </select>
          <Button variant="outline" size="sm" onClick={() => { fetchTuning(); fetchHistory() }} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        {/* Signal summary cards */}
        {signals && (
          <div>
            <h2 className="text-lg font-semibold mb-3">{tt("signals")}</h2>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
              <SignalCard label={tt("coldWarmRatio")} value={`${signals.cold_starts} / ${signals.warm_starts}`} sub={formatPercent(signals.cold_ratio) + " cold"} warn={signals.cold_ratio > 0.3} />
              <SignalCard label="P50" value={formatDuration(signals.p50_latency)} />
              <SignalCard label="P95" value={formatDuration(signals.p95_latency)} />
              <SignalCard label="P99" value={formatDuration(signals.p99_latency)} warn={signals.p99_latency > 1_000_000_000} />
              <SignalCard label={tt("errorRate")} value={formatPercent(signals.error_rate)} warn={signals.error_rate > 0.05} />
              <SignalCard label={tt("requestRate")} value={`${signals.request_rate_per_sec.toFixed(1)} rps`} />
            </div>
          </div>
        )}

        {/* Recommendations */}
        {tuning && tuning.recommendations && tuning.recommendations.length > 0 && (
          <div>
            <h2 className="text-lg font-semibold mb-3">{tt("recommendations")}</h2>
            <div className="space-y-3">
              {tuning.recommendations.map((rec, i) => (
                <div key={i} className="rounded-xl border border-border bg-card p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1 space-y-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm font-mono">{rec.parameter}</span>
                        {confidenceBadge(rec.confidence)}
                      </div>
                      <p className="text-sm text-muted-foreground">{rec.reason}</p>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{rec.current}</span>
                        <ArrowDownRight className="h-3 w-3" />
                        <span className="font-medium text-foreground">{rec.suggested}</span>
                      </div>
                      {rec.impact && <p className="text-xs text-muted-foreground">{tt("impact")}: {rec.impact}</p>}
                    </div>
                    <div className="flex gap-2 flex-shrink-0">
                      <Button size="sm" onClick={() => handleApply(i)}>
                        <CheckCircle2 className="mr-1 h-4 w-4" />
                        {tt("accept")}
                      </Button>
                      <Button variant="ghost" size="sm">
                        <XCircle className="mr-1 h-4 w-4" />
                        {tt("reject")}
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Active experiments */}
        {tuning && tuning.active_experiments && tuning.active_experiments.length > 0 && (
          <div>
            <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
              <FlaskConical className="h-5 w-5" />
              {tt("activeExperiments")}
            </h2>
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colExperiment")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colParameter")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colTraffic")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colStatus")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colVerdict")}</th>
                  </tr>
                </thead>
                <tbody>
                  {tuning.active_experiments.map((exp) => (
                    <tr key={exp.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 text-sm font-mono">{exp.id}</td>
                      <td className="px-4 py-3 text-sm">
                        <span className="font-mono">{exp.parameter}</span>
                        <span className="text-muted-foreground ml-2">{exp.control_value} → {exp.experiment_value}</span>
                      </td>
                      <td className="px-4 py-3 text-sm">{formatPercent(exp.traffic_percent)}</td>
                      <td className="px-4 py-3 text-sm">{statusBadge(exp.status)}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{exp.verdict || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Tuning history */}
        {history.length > 0 && (
          <div>
            <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
              <History className="h-5 w-5" />
              {tt("history")}
            </h2>
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colParameter")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colChange")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colSource")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colAppliedAt")}</th>
                  </tr>
                </thead>
                <tbody>
                  {history.map((h) => (
                    <tr key={h.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 text-sm font-mono">{h.parameter}</td>
                      <td className="px-4 py-3 text-sm">
                        <span className="text-muted-foreground">{h.old_value}</span>
                        <ArrowUpRight className="inline h-3 w-3 mx-1" />
                        <span className="font-medium">{h.new_value}</span>
                      </td>
                      <td className="px-4 py-3 text-sm">{h.source}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(h.applied_at).toLocaleString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Empty state */}
        {!loading && !tuning && !error && (
          <div className="text-center py-12 text-muted-foreground">
            <AlertTriangle className="mx-auto h-8 w-8 mb-2 opacity-50" />
            {tt("noData")}
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}

function SignalCard({ label, value, sub, warn }: { label: string; value: string; sub?: string; warn?: boolean }) {
  return (
    <div className={cn(
      "rounded-xl border bg-card p-4",
      warn ? "border-yellow-500/50" : "border-border"
    )}>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={cn("text-lg font-semibold mt-1", warn && "text-yellow-600")}>{value}</p>
      {sub && <p className="text-xs text-muted-foreground mt-0.5">{sub}</p>}
    </div>
  )
}

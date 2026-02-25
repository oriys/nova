"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { RefreshCw, Play, ChevronLeft, ChevronRight, CheckCircle2, AlertTriangle, XCircle } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Recording, ReplayResult, TimeTravelState } from "@/lib/types"

const API_BASE = "/api"

async function fetchRecordings(): Promise<Recording[]> {
  const res = await fetch(`${API_BASE}/recordings`)
  if (!res.ok) throw new Error(`Failed to fetch recordings: ${res.statusText}`)
  return res.json()
}

async function triggerReplay(functionName: string, invocationId: string): Promise<ReplayResult> {
  const res = await fetch(`${API_BASE}/functions/${functionName}/invocations/${invocationId}/replay`, {
    method: "POST",
  })
  if (!res.ok) throw new Error(`Replay failed: ${res.statusText}`)
  return res.json()
}

async function fetchTimeTravelState(replayId: string, step: number): Promise<TimeTravelState> {
  const res = await fetch(`${API_BASE}/replays/${replayId}/steps/${step}`)
  if (!res.ok) throw new Error(`Failed to fetch step: ${res.statusText}`)
  return res.json()
}

function StatusBadge({ status }: { status: ReplayResult["status"] }) {
  const colors = {
    success: "bg-green-500/10 text-green-500",
    diverged: "bg-yellow-500/10 text-yellow-500",
    failed: "bg-red-500/10 text-red-500",
  }
  const icons = {
    success: CheckCircle2,
    diverged: AlertTriangle,
    failed: XCircle,
  }
  const Icon = icons[status]
  return (
    <span className={cn("inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium", colors[status])}>
      <Icon className="h-3 w-3" />
      {status}
    </span>
  )
}

export default function ReplayPage() {
  const t = useTranslations("pages")
  const tr = useTranslations("replayPage")
  const tc = useTranslations("common")

  const [recordings, setRecordings] = useState<Recording[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [replayingId, setReplayingId] = useState<string | null>(null)
  const [replayResult, setReplayResult] = useState<ReplayResult | null>(null)
  const [timeTravelState, setTimeTravelState] = useState<TimeTravelState | null>(null)

  const loadRecordings = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await fetchRecordings()
      setRecordings(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tr])

  useEffect(() => {
    loadRecordings()
  }, [loadRecordings])

  const handleReplay = async (rec: Recording) => {
    try {
      setReplayingId(rec.id)
      setReplayResult(null)
      setTimeTravelState(null)
      const result = await triggerReplay(rec.function_id, rec.invocation_id)
      setReplayResult(result)
      // Load initial time-travel state if available
      try {
        const state = await fetchTimeTravelState(result.replay_id, 0)
        setTimeTravelState(state)
      } catch {
        // Time-travel not available for this replay
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("replayFailed"))
    } finally {
      setReplayingId(null)
    }
  }

  const handleStepForward = async () => {
    if (!replayResult || !timeTravelState) return
    try {
      const state = await fetchTimeTravelState(replayResult.replay_id, timeTravelState.step + 1)
      setTimeTravelState(state)
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("stepFailed"))
    }
  }

  const handleStepBackward = async () => {
    if (!replayResult || !timeTravelState || timeTravelState.step <= 0) return
    try {
      const state = await fetchTimeTravelState(replayResult.replay_id, timeTravelState.step - 1)
      setTimeTravelState(state)
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("stepFailed"))
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("replay.title")} description={t("replay.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        {/* Toolbar */}
        <div className="flex items-center justify-end">
          <Button variant="outline" size="sm" onClick={loadRecordings} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        {/* Recordings Table */}
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="px-4 py-3 border-b border-border">
            <h3 className="text-sm font-medium">{tr("recordings")}</h3>
          </div>
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colFunction")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colRuntime")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colEvents")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tr("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={5} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : recordings.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                    <Play className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tr("noRecordings")}
                  </td>
                </tr>
              ) : (
                recordings.map((rec) => (
                  <tr key={rec.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <span className="font-medium text-sm font-mono">{rec.function_id}</span>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{rec.runtime}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{rec.events_count}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(rec.created_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleReplay(rec)}
                        disabled={replayingId === rec.id}
                      >
                        {replayingId === rec.id ? (
                          <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                        ) : (
                          <Play className="mr-2 h-4 w-4" />
                        )}
                        {tr("replayButton")}
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Replay Result */}
        {replayResult && (
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="px-4 py-3 border-b border-border flex items-center justify-between">
              <h3 className="text-sm font-medium">{tr("replayResult")}</h3>
              <StatusBadge status={replayResult.status} />
            </div>
            <div className="p-4 space-y-4">
              <div className="grid grid-cols-3 gap-4 text-sm">
                <div>
                  <span className="text-muted-foreground">{tr("status")}</span>
                  <p className="font-medium mt-1">{replayResult.status}</p>
                </div>
                <div>
                  <span className="text-muted-foreground">{tr("duration")}</span>
                  <p className="font-medium mt-1">{replayResult.duration_ms}ms</p>
                </div>
                <div>
                  <span className="text-muted-foreground">{tr("eventsReplayed")}</span>
                  <p className="font-medium mt-1">{replayResult.events_replayed}</p>
                </div>
              </div>

              {/* Divergences */}
              <div>
                <h4 className="text-sm font-medium mb-2">{tr("divergences")}</h4>
                {replayResult.divergences.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{tr("noDivergences")}</p>
                ) : (
                  <div className="space-y-2">
                    {replayResult.divergences.map((div, i) => (
                      <div key={i} className="rounded-lg border border-yellow-500/30 bg-yellow-500/5 p-3 text-sm">
                        <div className="flex items-center gap-2 mb-1">
                          <AlertTriangle className="h-4 w-4 text-yellow-500" />
                          <span className="font-medium">Event #{div.event_seq} — {div.type}</span>
                        </div>
                        <p className="text-muted-foreground">{div.message}</p>
                        <div className="mt-2 grid grid-cols-2 gap-2 font-mono text-xs">
                          <div>
                            <span className="text-muted-foreground">{tr("expected")}:</span>
                            <pre className="mt-1 p-2 bg-muted rounded">{div.expected}</pre>
                          </div>
                          <div>
                            <span className="text-muted-foreground">{tr("actual")}:</span>
                            <pre className="mt-1 p-2 bg-muted rounded">{div.actual}</pre>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Time Travel Debug Panel */}
        {timeTravelState && (
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="px-4 py-3 border-b border-border flex items-center justify-between">
              <h3 className="text-sm font-medium">{tr("timeTravel")}</h3>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleStepBackward}
                  disabled={timeTravelState.step <= 0}
                >
                  <ChevronLeft className="mr-1 h-4 w-4" />
                  {tr("stepBackward")}
                </Button>
                <span className="text-sm text-muted-foreground font-mono">
                  {tr("stepLabel", { step: timeTravelState.step })}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleStepForward}
                  disabled={timeTravelState.completed}
                >
                  {tr("stepForward")}
                  <ChevronRight className="ml-1 h-4 w-4" />
                </Button>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-0 divide-x divide-border">
              {/* Variables Panel */}
              <div className="p-4">
                <h4 className="text-sm font-medium mb-2">{tr("variables")}</h4>
                <div className="space-y-1 font-mono text-xs">
                  {Object.entries(timeTravelState.variables).map(([key, value]) => (
                    <div key={key} className="flex justify-between py-1 border-b border-border/50">
                      <span className="text-muted-foreground">{key}</span>
                      <span>{value}</span>
                    </div>
                  ))}
                  {Object.keys(timeTravelState.variables).length === 0 && (
                    <p className="text-muted-foreground">{tr("noVariables")}</p>
                  )}
                </div>
              </div>

              {/* Call Stack Panel */}
              <div className="p-4">
                <h4 className="text-sm font-medium mb-2">{tr("callStack")}</h4>
                <div className="space-y-1 font-mono text-xs">
                  {timeTravelState.call_stack.map((frame, i) => (
                    <div key={i} className={cn(
                      "py-1 px-2 rounded",
                      i === 0 && "bg-primary/10 text-primary"
                    )}>
                      <span className="font-medium">{frame.function}</span>
                      <span className="text-muted-foreground ml-2">{frame.file}:{frame.line}</span>
                    </div>
                  ))}
                  {timeTravelState.call_stack.length === 0 && (
                    <p className="text-muted-foreground">{tr("emptyStack")}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Output */}
            {timeTravelState.output && (
              <div className="border-t border-border p-4">
                <h4 className="text-sm font-medium mb-2">{tr("output")}</h4>
                <pre className="p-3 bg-muted rounded-lg text-xs font-mono overflow-x-auto">
                  {timeTravelState.output}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}

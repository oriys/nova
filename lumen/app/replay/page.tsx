"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { RefreshCw, Play, CheckCircle2, AlertTriangle, XCircle } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Recording, ReplayResult } from "@/lib/types"
import { functionsApi, invocationsApi } from "@/lib/api"

type ReplayRecording = Recording & {
  input_payload?: unknown
}

async function fetchRecordings(): Promise<ReplayRecording[]> {
  const { items } = await invocationsApi.listPage(100, 0)
  return items.map((invocation) => ({
    id: invocation.id,
    function_id: invocation.function_name,
    invocation_id: invocation.id,
    runtime: invocation.runtime,
    arch: "unknown",
    created_at: invocation.created_at,
    events_count: 1,
    input_payload: invocation.input ?? {},
  }))
}

async function triggerReplay(functionName: string, payload: unknown): Promise<ReplayResult> {
  const startedAt = Date.now()
  try {
    const result = await functionsApi.invoke(functionName, payload ?? {})
    return {
      replay_id: result.request_id,
      status: result.error ? "failed" : "success",
      divergences: [],
      duration_ms: result.duration_ms,
      events_replayed: 1,
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : "Replay failed"
    return {
      replay_id: `replay-${startedAt}`,
      status: "failed",
      divergences: [
        {
          event_seq: 1,
          type: "invoke_error",
          expected: "function invocation succeeds",
          actual: "function invocation failed",
          message,
        },
      ],
      duration_ms: Math.max(1, Date.now() - startedAt),
      events_replayed: 0,
    }
  }
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

  const [recordings, setRecordings] = useState<ReplayRecording[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [replayingId, setReplayingId] = useState<string | null>(null)
  const [replayResult, setReplayResult] = useState<ReplayResult | null>(null)

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

  const handleReplay = async (rec: ReplayRecording) => {
    try {
      setReplayingId(rec.id)
      setReplayResult(null)
      const result = await triggerReplay(rec.function_id, rec.input_payload ?? {})
      setReplayResult(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("replayFailed"))
    } finally {
      setReplayingId(null)
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

      </div>
    </DashboardLayout>
  )
}

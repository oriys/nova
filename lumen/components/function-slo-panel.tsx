"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  functionsApi,
  type FunctionSLOStatus,
  type SLOObjectives,
  type SLOPolicy,
  type SLONotificationTarget,
} from "@/lib/api"
import { AlertTriangle, CheckCircle2, Loader2, RefreshCw, Save, ShieldCheck } from "lucide-react"

interface FunctionSLOPanelProps {
  functionName: string
}

const DEFAULT_POLICY: SLOPolicy = {
  enabled: false,
  window_s: 900,
  min_samples: 20,
  objectives: {
    success_rate_pct: 99.5,
    p95_duration_ms: 800,
    cold_start_rate_pct: 15,
  },
  notifications: [{ type: "bell", url: "" }],
}

type Preset = {
  id: string
  label: string
  policy: Partial<SLOPolicy>
}

const PRESETS: Preset[] = [
  {
    id: "strict",
    label: "Strict",
    policy: {
      window_s: 300,
      min_samples: 20,
      objectives: { success_rate_pct: 99.9, p95_duration_ms: 350, cold_start_rate_pct: 8 },
    },
  },
  {
    id: "balanced",
    label: "Balanced",
    policy: {
      window_s: 900,
      min_samples: 30,
      objectives: { success_rate_pct: 99.5, p95_duration_ms: 800, cold_start_rate_pct: 15 },
    },
  },
  {
    id: "cost",
    label: "Cost-aware",
    policy: {
      window_s: 1800,
      min_samples: 40,
      objectives: { success_rate_pct: 99, p95_duration_ms: 1200, cold_start_rate_pct: 30 },
    },
  },
]

function normalizeNotifications(notifications?: SLONotificationTarget[]): SLONotificationTarget[] {
  if (!notifications || notifications.length === 0) {
    return [{ type: "bell", url: "" }]
  }
  return notifications
}

function mergePolicy(base: SLOPolicy, partial: Partial<SLOPolicy>): SLOPolicy {
  return {
    ...base,
    ...partial,
    objectives: {
      ...base.objectives,
      ...(partial.objectives || {}),
    },
    notifications: normalizeNotifications(partial.notifications || base.notifications),
  }
}

export function FunctionSLOPanel({ functionName }: FunctionSLOPanelProps) {
  const [policy, setPolicy] = useState<SLOPolicy>(DEFAULT_POLICY)
  const [status, setStatus] = useState<FunctionSLOStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [remotePolicy, remoteStatus] = await Promise.all([
        functionsApi.getSLOPolicy(functionName),
        functionsApi.sloStatus(functionName),
      ])
      setPolicy(mergePolicy(DEFAULT_POLICY, remotePolicy || {}))
      setStatus(remoteStatus)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load SLO policy")
    } finally {
      setLoading(false)
    }
  }, [functionName])

  useEffect(() => {
    load()
  }, [load])

  const setObjective = (key: keyof SLOObjectives, value: string) => {
    const parsed = Number(value)
    const next = Number.isFinite(parsed) ? parsed : 0
    setPolicy((prev) => ({
      ...prev,
      objectives: {
        ...prev.objectives,
        [key]: next,
      },
    }))
    setSaved(false)
  }

  const setNumberField = (key: "window_s" | "min_samples", value: string) => {
    const parsed = Number(value)
    const next = Number.isFinite(parsed) ? Math.max(0, Math.floor(parsed)) : 0
    setPolicy((prev) => ({ ...prev, [key]: next }))
    setSaved(false)
  }

  const applyPreset = (preset: Preset) => {
    setPolicy((prev) => mergePolicy(prev, preset.policy))
    setSaved(false)
  }

  const normalizedPolicy = useMemo<SLOPolicy>(() => {
    const objectives = policy.objectives || {}
    return {
      ...policy,
      window_s: Math.max(60, Math.floor(policy.window_s || 900)),
      min_samples: Math.max(1, Math.floor(policy.min_samples || 20)),
      objectives: {
        success_rate_pct: Math.min(100, Math.max(0, objectives.success_rate_pct || 0)),
        p95_duration_ms: Math.max(0, Math.floor(objectives.p95_duration_ms || 0)),
        cold_start_rate_pct: Math.min(100, Math.max(0, objectives.cold_start_rate_pct || 0)),
      },
      notifications: normalizeNotifications(policy.notifications),
    }
  }, [policy])

  const savePolicy = async () => {
    setSaving(true)
    setError(null)
    setSaved(false)
    try {
      await functionsApi.setSLOPolicy(functionName, normalizedPolicy)
      await load()
      setSaved(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save SLO policy")
    } finally {
      setSaving(false)
    }
  }

  const toggleEnabled = () => {
    setPolicy((prev) => ({ ...prev, enabled: !prev.enabled }))
    setSaved(false)
  }

  const hasBreaches = (status?.breaches?.length || 0) > 0

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold text-card-foreground">SLO Policy</h3>
          {!policy.enabled ? (
            <Badge variant="secondary" className="text-[10px] bg-muted text-muted-foreground border-0">
              disabled
            </Badge>
          ) : hasBreaches ? (
            <Badge variant="secondary" className="text-[10px] bg-warning/10 text-warning border-0">
              breached
            </Badge>
          ) : (
            <Badge variant="secondary" className="text-[10px] bg-success/10 text-success border-0">
              healthy
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button variant={policy.enabled ? "secondary" : "outline"} size="sm" onClick={toggleEnabled}>
            {policy.enabled ? "Enabled" : "Enable"}
          </Button>
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
            Refresh
          </Button>
          <Button size="sm" onClick={savePolicy} disabled={saving}>
            {saving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            Save
          </Button>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        {PRESETS.map((preset) => (
          <Button key={preset.id} variant="outline" size="xs" onClick={() => applyPreset(preset)}>
            {preset.label}
          </Button>
        ))}
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-5">
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">Window (s)</p>
          <Input
            type="number"
            min={60}
            value={policy.window_s ?? 900}
            onChange={(e) => setNumberField("window_s", e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">Min samples</p>
          <Input
            type="number"
            min={1}
            value={policy.min_samples ?? 20}
            onChange={(e) => setNumberField("min_samples", e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">Success rate % (min)</p>
          <Input
            type="number"
            min={0}
            max={100}
            value={policy.objectives?.success_rate_pct ?? 0}
            onChange={(e) => setObjective("success_rate_pct", e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">P95 latency ms (max)</p>
          <Input
            type="number"
            min={0}
            value={policy.objectives?.p95_duration_ms ?? 0}
            onChange={(e) => setObjective("p95_duration_ms", e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">Cold start % (max)</p>
          <Input
            type="number"
            min={0}
            max={100}
            value={policy.objectives?.cold_start_rate_pct ?? 0}
            onChange={(e) => setObjective("cold_start_rate_pct", e.target.value)}
          />
        </div>
      </div>

      {error && (
        <div className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}
      {saved && (
        <div className="mt-3 rounded-md border border-success/40 bg-success/10 px-3 py-2 text-xs text-success">
          SLO policy saved.
        </div>
      )}

      <div className="mt-4 grid gap-3 md:grid-cols-4">
        <div className="rounded-md border border-border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">Invocations</p>
          <p className="mt-1 text-lg font-semibold text-card-foreground">{status?.snapshot?.total_invocations ?? 0}</p>
        </div>
        <div className="rounded-md border border-border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">Success rate</p>
          <p className="mt-1 text-lg font-semibold text-card-foreground">
            {(status?.snapshot?.success_rate_pct ?? 0).toFixed(2)}%
          </p>
        </div>
        <div className="rounded-md border border-border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">P95 latency</p>
          <p className="mt-1 text-lg font-semibold text-card-foreground">{status?.snapshot?.p95_duration_ms ?? 0}ms</p>
        </div>
        <div className="rounded-md border border-border bg-muted/20 p-3">
          <p className="text-xs text-muted-foreground">Cold start rate</p>
          <p className="mt-1 text-lg font-semibold text-card-foreground">
            {(status?.snapshot?.cold_start_rate_pct ?? 0).toFixed(2)}%
          </p>
        </div>
      </div>

      {(status?.breaches?.length || 0) > 0 && (
        <div className="mt-3 rounded-md border border-warning/40 bg-warning/10 px-3 py-2">
          <div className="flex items-center gap-2 text-xs text-warning">
            <AlertTriangle className="h-3.5 w-3.5" />
            Active breaches:
            {status?.breaches?.map((b) => (
              <Badge key={b} variant="secondary" className="text-[10px] bg-warning/20 text-warning border-0">
                {b}
              </Badge>
            ))}
          </div>
        </div>
      )}

      {policy.enabled && !hasBreaches && (
        <div className="mt-3 rounded-md border border-success/40 bg-success/10 px-3 py-2 text-xs text-success">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-3.5 w-3.5" />
            Current window is within SLO thresholds.
          </div>
        </div>
      )}
    </div>
  )
}

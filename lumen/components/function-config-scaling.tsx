"use client"

import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { FunctionData } from "@/lib/types"
import {
  functionsApi,
  type AutoScalePolicy,
  type CapacityPolicy,
  type ScaleThresholds,
} from "@/lib/api"
import { SectionHeader } from "@/components/section-header"
import { Save, Loader2 } from "lucide-react"

interface FunctionConfigScalingProps {
  func: FunctionData
  onUpdate?: () => void
}

const defaultScaleThresholds: ScaleThresholds = {
  queue_depth: 0,
  queue_wait_ms: 0,
  avg_latency_ms: 0,
  cold_start_pct: 0,
  idle_pct: 0,
  target_concurrency: 0,
}

function normalizeAutoScalePolicy(policy: AutoScalePolicy | undefined, func: FunctionData): AutoScalePolicy {
  const scaleUp = { ...defaultScaleThresholds, ...(policy?.scale_up_thresholds || {}) }
  const scaleDown = { ...defaultScaleThresholds, ...(policy?.scale_down_thresholds || {}) }
  return {
    enabled: Boolean(policy?.enabled),
    min_replicas: policy?.min_replicas ?? func.minReplicas ?? 0,
    max_replicas: policy?.max_replicas ?? func.maxReplicas ?? 0,
    target_utilization: policy?.target_utilization ?? 0.7,
    scale_up_thresholds: scaleUp,
    scale_down_thresholds: scaleDown,
    cooldown_scale_up_s: policy?.cooldown_scale_up_s ?? 15,
    cooldown_scale_down_s: policy?.cooldown_scale_down_s ?? 60,
    scale_down_step: policy?.scale_down_step ?? 1,
    scale_up_step_max: policy?.scale_up_step_max ?? 4,
    scale_down_stabilization_s: policy?.scale_down_stabilization_s ?? 90,
    min_sample_count: policy?.min_sample_count ?? 3,
  }
}

function normalizeCapacityPolicy(policy: CapacityPolicy | undefined): CapacityPolicy {
  return {
    enabled: Boolean(policy?.enabled),
    max_inflight: policy?.max_inflight ?? 0,
    max_queue_depth: policy?.max_queue_depth ?? 0,
    max_queue_wait_ms: policy?.max_queue_wait_ms ?? 0,
    shed_status_code: policy?.shed_status_code ?? 503,
    retry_after_s: policy?.retry_after_s ?? 1,
    breaker_error_pct: policy?.breaker_error_pct ?? 0,
    breaker_window_s: policy?.breaker_window_s ?? 0,
    breaker_open_s: policy?.breaker_open_s ?? 0,
    half_open_probes: policy?.half_open_probes ?? 0,
  }
}

export function FunctionConfigScaling({ func, onUpdate }: FunctionConfigScalingProps) {
  const [policyLoading, setPolicyLoading] = useState(false)
  const [savingAutoScale, setSavingAutoScale] = useState(false)
  const [savingCapacity, setSavingCapacity] = useState(false)
  const [autoScaleMessage, setAutoScaleMessage] = useState<{ type: "success" | "error"; text: string } | null>(null)
  const [capacityMessage, setCapacityMessage] = useState<{ type: "success" | "error"; text: string } | null>(null)
  const [autoScalePolicy, setAutoScalePolicy] = useState<AutoScalePolicy>(
    normalizeAutoScalePolicy(undefined, func)
  )
  const [capacityPolicy, setCapacityPolicy] = useState<CapacityPolicy>(
    normalizeCapacityPolicy(undefined)
  )

  const loadPolicies = useCallback(async () => {
    setPolicyLoading(true)
    try {
      const [scaling, capacity] = await Promise.all([
        functionsApi.getScalingPolicy(func.name).catch(() => ({ enabled: false } as AutoScalePolicy)),
        functionsApi.getCapacityPolicy(func.name).catch(() => ({ enabled: false } as CapacityPolicy)),
      ])
      setAutoScalePolicy(normalizeAutoScalePolicy(scaling, func))
      setCapacityPolicy(normalizeCapacityPolicy(capacity))
    } catch (err) {
      console.error("Failed to load scaling/capacity policies:", err)
      setAutoScalePolicy(normalizeAutoScalePolicy(undefined, func))
      setCapacityPolicy(normalizeCapacityPolicy(undefined))
    } finally {
      setPolicyLoading(false)
    }
  }, [func])

  useEffect(() => {
    loadPolicies()
  }, [loadPolicies])

  const setScaleUpThreshold = (key: keyof ScaleThresholds, value: number) => {
    setAutoScalePolicy((prev) => ({
      ...prev,
      scale_up_thresholds: {
        ...(prev.scale_up_thresholds || defaultScaleThresholds),
        [key]: value,
      },
    }))
  }

  const setScaleDownThreshold = (key: keyof ScaleThresholds, value: number) => {
    setAutoScalePolicy((prev) => ({
      ...prev,
      scale_down_thresholds: {
        ...(prev.scale_down_thresholds || defaultScaleThresholds),
        [key]: value,
      },
    }))
  }

  const handleSaveAutoScale = async () => {
    try {
      setSavingAutoScale(true)
      setAutoScaleMessage(null)
      if (!autoScalePolicy.enabled) {
        await functionsApi.deleteScalingPolicy(func.name)
      } else {
        await functionsApi.setScalingPolicy(func.name, autoScalePolicy)
      }
      await loadPolicies()
      setAutoScaleMessage({ type: "success", text: "Auto-scaling policy saved" })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to save auto-scaling policy:", err)
      setAutoScaleMessage({ type: "error", text: err instanceof Error ? err.message : "Failed to save auto-scaling policy" })
    } finally {
      setSavingAutoScale(false)
    }
  }

  const handleDisableAutoScale = async () => {
    try {
      setSavingAutoScale(true)
      setAutoScaleMessage(null)
      await functionsApi.deleteScalingPolicy(func.name)
      await loadPolicies()
      setAutoScaleMessage({ type: "success", text: "Auto-scaling policy disabled" })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to disable auto-scaling policy:", err)
      setAutoScaleMessage({ type: "error", text: err instanceof Error ? err.message : "Failed to disable auto-scaling policy" })
    } finally {
      setSavingAutoScale(false)
    }
  }

  const handleSaveCapacity = async () => {
    try {
      setSavingCapacity(true)
      setCapacityMessage(null)
      if (!capacityPolicy.enabled) {
        await functionsApi.deleteCapacityPolicy(func.name)
      } else {
        await functionsApi.setCapacityPolicy(func.name, capacityPolicy)
      }
      await loadPolicies()
      setCapacityMessage({ type: "success", text: "Capacity policy saved" })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to save capacity policy:", err)
      setCapacityMessage({ type: "error", text: err instanceof Error ? err.message : "Failed to save capacity policy" })
    } finally {
      setSavingCapacity(false)
    }
  }

  const handleDisableCapacity = async () => {
    try {
      setSavingCapacity(true)
      setCapacityMessage(null)
      await functionsApi.deleteCapacityPolicy(func.name)
      await loadPolicies()
      setCapacityMessage({ type: "success", text: "Capacity policy disabled" })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to disable capacity policy:", err)
      setCapacityMessage({ type: "error", text: err instanceof Error ? err.message : "Failed to disable capacity policy" })
    } finally {
      setSavingCapacity(false)
    }
  }

  return (
    <>
      {/* Auto Scaling Policy */}
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader
          className="mb-4"
          title="Auto Scaling Policy"
          description="Configure replica scaling behavior based on load and queue pressure."
          titleClassName="text-lg font-semibold text-card-foreground"
          descriptionClassName="text-sm"
          action={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                size="sm"
                onClick={handleSaveAutoScale}
                disabled={policyLoading || savingAutoScale}
              >
                {savingAutoScale ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Save className="mr-2 h-4 w-4" />
                )}
                Save
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleDisableAutoScale}
                disabled={policyLoading || savingAutoScale || !autoScalePolicy.enabled}
              >
                Disable
              </Button>
            </div>
          }
        />

        {policyLoading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading policy...
          </div>
        ) : (
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Enabled</Label>
                <Select
                  value={autoScalePolicy.enabled ? "true" : "false"}
                  onValueChange={(v) => setAutoScalePolicy((prev) => ({ ...prev, enabled: v === "true" }))}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="true">Enabled</SelectItem>
                    <SelectItem value="false">Disabled</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Min Replicas</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.min_replicas ?? 0}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, min_replicas: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Max Replicas</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.max_replicas ?? 0}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, max_replicas: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Target Utilization (0-1)</Label>
                <Input
                  type="number"
                  min="0"
                  max="1"
                  step="0.05"
                  value={autoScalePolicy.target_utilization ?? 0.7}
                  onChange={(e) =>
                    setAutoScalePolicy((prev) => ({
                      ...prev,
                      target_utilization: Math.min(1, Math.max(0, Number(e.target.value) || 0)),
                    }))
                  }
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Scale Up Cooldown (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.cooldown_scale_up_s ?? 15}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, cooldown_scale_up_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Scale Down Cooldown (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.cooldown_scale_down_s ?? 60}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, cooldown_scale_down_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Scale Up Step Max</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_up_step_max ?? 4}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, scale_up_step_max: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Scale Down Step</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_down_step ?? 1}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, scale_down_step: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Down Stabilization (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_down_stabilization_s ?? 90}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, scale_down_stabilization_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Min Sample Count</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.min_sample_count ?? 3}
                  onChange={(e) => setAutoScalePolicy((prev) => ({ ...prev, min_sample_count: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Up Threshold: Queue Depth</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_up_thresholds?.queue_depth ?? 0}
                  onChange={(e) => setScaleUpThreshold("queue_depth", Math.max(0, Number(e.target.value) || 0))}
                />
              </div>
              <div className="space-y-2">
                <Label>Up Threshold: Queue Wait (ms)</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_up_thresholds?.queue_wait_ms ?? 0}
                  onChange={(e) => setScaleUpThreshold("queue_wait_ms", Math.max(0, Number(e.target.value) || 0))}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Up Threshold: Avg Latency (ms)</Label>
                <Input
                  type="number"
                  min="0"
                  value={autoScalePolicy.scale_up_thresholds?.avg_latency_ms ?? 0}
                  onChange={(e) => setScaleUpThreshold("avg_latency_ms", Math.max(0, Number(e.target.value) || 0))}
                />
              </div>
              <div className="space-y-2">
                <Label>Up Threshold: Cold Start %</Label>
                <Input
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  value={autoScalePolicy.scale_up_thresholds?.cold_start_pct ?? 0}
                  onChange={(e) => setScaleUpThreshold("cold_start_pct", Math.min(100, Math.max(0, Number(e.target.value) || 0)))}
                />
              </div>
              <div className="space-y-2">
                <Label>Up Threshold: Concurrency</Label>
                <Input
                  type="number"
                  min="0"
                  step="0.05"
                  value={autoScalePolicy.scale_up_thresholds?.target_concurrency ?? 0}
                  onChange={(e) => setScaleUpThreshold("target_concurrency", Math.max(0, Number(e.target.value) || 0))}
                />
              </div>
              <div className="space-y-2">
                <Label>Down Threshold: Idle %</Label>
                <Input
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  value={autoScalePolicy.scale_down_thresholds?.idle_pct ?? 0}
                  onChange={(e) => setScaleDownThreshold("idle_pct", Math.min(100, Math.max(0, Number(e.target.value) || 0)))}
                />
              </div>
            </div>

            {autoScaleMessage && (
              <div className={`text-sm p-3 rounded-lg ${autoScaleMessage.type === "success" ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive"}`}>
                {autoScaleMessage.text}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Capacity Policy */}
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader
          className="mb-4"
          title="Capacity Protection Policy"
          description="Protect the function from overload with in-flight, queue, and shed controls."
          titleClassName="text-lg font-semibold text-card-foreground"
          descriptionClassName="text-sm"
          action={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                size="sm"
                onClick={handleSaveCapacity}
                disabled={policyLoading || savingCapacity}
              >
                {savingCapacity ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Save className="mr-2 h-4 w-4" />
                )}
                Save
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleDisableCapacity}
                disabled={policyLoading || savingCapacity || !capacityPolicy.enabled}
              >
                Disable
              </Button>
            </div>
          }
        />

        {policyLoading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading policy...
          </div>
        ) : (
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Enabled</Label>
                <Select
                  value={capacityPolicy.enabled ? "true" : "false"}
                  onValueChange={(v) => setCapacityPolicy((prev) => ({ ...prev, enabled: v === "true" }))}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="true">Enabled</SelectItem>
                    <SelectItem value="false">Disabled</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Max Inflight</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.max_inflight ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, max_inflight: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Max Queue Depth</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.max_queue_depth ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, max_queue_depth: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Max Queue Wait (ms)</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.max_queue_wait_ms ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, max_queue_wait_ms: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Shed Status Code</Label>
                <Select
                  value={String(capacityPolicy.shed_status_code ?? 503)}
                  onValueChange={(v) => setCapacityPolicy((prev) => ({ ...prev, shed_status_code: Number(v) }))}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="503">503 Service Unavailable</SelectItem>
                    <SelectItem value="429">429 Too Many Requests</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Retry-After (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.retry_after_s ?? 1}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, retry_after_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Breaker Error %</Label>
                <Input
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  value={capacityPolicy.breaker_error_pct ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, breaker_error_pct: Math.min(100, Math.max(0, Number(e.target.value) || 0)) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Breaker Window (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.breaker_window_s ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, breaker_window_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <Label>Breaker Open (s)</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.breaker_open_s ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, breaker_open_s: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Half-open Probes</Label>
                <Input
                  type="number"
                  min="0"
                  value={capacityPolicy.half_open_probes ?? 0}
                  onChange={(e) => setCapacityPolicy((prev) => ({ ...prev, half_open_probes: Math.max(0, Number(e.target.value) || 0) }))}
                />
              </div>
            </div>

            {capacityMessage && (
              <div className={`text-sm p-3 rounded-lg ${capacityMessage.type === "success" ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive"}`}>
                {capacityMessage.text}
              </div>
            )}
          </div>
        )}
      </div>
    </>
  )
}

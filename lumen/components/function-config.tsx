"use client"

import { useCallback, useEffect, useState } from "react"
import { useRouter } from "next/navigation"
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { FunctionData } from "@/lib/types"
import {
  functionsApi,
  snapshotsApi,
  type AutoScalePolicy,
  type CapacityPolicy,
  type NetworkPolicy,
  type RolloutPolicy,
  type ResourceLimits,
  type ScaleThresholds,
} from "@/lib/api"
import { Save, Plus, Trash2, Eye, EyeOff, Key, Loader2, Camera, AlertTriangle } from "lucide-react"

interface FunctionConfigProps {
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

type EditableEgressRule = {
  host: string
  port: string
  protocol: string
}

type EditableIngressRule = {
  source: string
  port: string
  protocol: string
}

function normalizeIngressRules(policy: NetworkPolicy | undefined): EditableIngressRule[] {
  if (!policy?.ingress_rules || policy.ingress_rules.length === 0) {
    return []
  }
  return policy.ingress_rules.map((rule) => ({
    source: rule.source || "",
    port: rule.port ? String(rule.port) : "",
    protocol: rule.protocol || "tcp",
  }))
}

function normalizeEgressRules(policy: NetworkPolicy | undefined): EditableEgressRule[] {
  if (!policy?.egress_rules || policy.egress_rules.length === 0) {
    return []
  }
  return policy.egress_rules.map((rule) => ({
    host: rule.host || "",
    port: rule.port ? String(rule.port) : "",
    protocol: rule.protocol || "tcp",
  }))
}

export function FunctionConfig({ func, onUpdate }: FunctionConfigProps) {
  const router = useRouter()
  const [memory, setMemory] = useState(func.memory.toString())
  const [timeout, setTimeout] = useState(func.timeout.toString())
  const [handler, setHandler] = useState(func.handler)
  const [maxReplicas, setMaxReplicas] = useState((func.maxReplicas ?? 0).toString())
  const [saving, setSaving] = useState(false)
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({})

  // Resource limits state
  const [vcpus, setVcpus] = useState((func.limits?.vcpus || 1).toString())
  const [diskIops, setDiskIops] = useState((func.limits?.disk_iops || 0).toString())
  const [diskBandwidth, setDiskBandwidth] = useState((func.limits?.disk_bandwidth || 0).toString())
  const [netRx, setNetRx] = useState((func.limits?.net_rx_bandwidth || 0).toString())
  const [netTx, setNetTx] = useState((func.limits?.net_tx_bandwidth || 0).toString())

  // Network policy state
  const [isolationMode, setIsolationMode] = useState(func.networkPolicy?.isolation_mode || "egress-only")
  const [denyExternalAccess, setDenyExternalAccess] = useState(func.networkPolicy?.deny_external_access ? "true" : "false")
  const [ingressRules, setIngressRules] = useState<EditableIngressRule[]>(
    normalizeIngressRules(func.networkPolicy)
  )
  const [egressRules, setEgressRules] = useState<EditableEgressRule[]>(
    normalizeEgressRules(func.networkPolicy)
  )
  const [rolloutEnabled, setRolloutEnabled] = useState(func.rolloutPolicy?.enabled ? "true" : "false")
  const [canaryFunction, setCanaryFunction] = useState(func.rolloutPolicy?.canary_function || "")
  const [canaryPercent, setCanaryPercent] = useState(String(func.rolloutPolicy?.canary_percent ?? 10))

  // Environment variables state
  const [envVarsState, setEnvVarsState] = useState<Record<string, string>>(func.envVars || {})
  const [newEnvKey, setNewEnvKey] = useState("")
  const [newEnvValue, setNewEnvValue] = useState("")
  const [showAddEnvDialog, setShowAddEnvDialog] = useState(false)
  const [savingEnv, setSavingEnv] = useState(false)

  // Delete state
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [deleteConfirmName, setDeleteConfirmName] = useState("")

  // Snapshot state
  const [creatingSnapshot, setCreatingSnapshot] = useState(false)
  const [snapshotMessage, setSnapshotMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null)

  // Autoscaling + capacity policy state
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

  // Use function's env vars
  const envVars = Object.entries(envVarsState).map(([key, value], idx) => ({
    id: `env-${idx}`,
    key,
    value,
    type: key.toLowerCase().includes("secret") || key.toLowerCase().includes("key") || key.toLowerCase().includes("password")
      ? "secret" as const
      : "string" as const,
  }))

  const toggleSecret = (id: string) => {
    setShowSecrets((prev) => ({ ...prev, [id]: !prev[id] }))
  }

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

  const addEgressRule = () => {
    setEgressRules((prev) => [...prev, { host: "", port: "", protocol: "tcp" }])
  }

  const addIngressRule = () => {
    setIngressRules((prev) => [...prev, { source: "", port: "", protocol: "tcp" }])
  }

  const updateEgressRule = (index: number, field: keyof EditableEgressRule, value: string) => {
    setEgressRules((prev) =>
      prev.map((rule, i) => (i === index ? { ...rule, [field]: value } : rule))
    )
  }

  const removeEgressRule = (index: number) => {
    setEgressRules((prev) => prev.filter((_, i) => i !== index))
  }

  const updateIngressRule = (index: number, field: keyof EditableIngressRule, value: string) => {
    setIngressRules((prev) =>
      prev.map((rule, i) => (i === index ? { ...rule, [field]: value } : rule))
    )
  }

  const removeIngressRule = (index: number) => {
    setIngressRules((prev) => prev.filter((_, i) => i !== index))
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      const limits: ResourceLimits = {
        vcpus: parseInt(vcpus) || 1,
        disk_iops: parseInt(diskIops) || 0,
        disk_bandwidth: parseInt(diskBandwidth) || 0,
        net_rx_bandwidth: parseInt(netRx) || 0,
        net_tx_bandwidth: parseInt(netTx) || 0,
      }
      const parsedIngressRules: NonNullable<NetworkPolicy["ingress_rules"]> = []
      for (const rule of ingressRules) {
        const source = rule.source.trim()
        if (!source) {
          continue
        }
        const port = Number.parseInt(rule.port, 10)
        const protocol = rule.protocol.trim().toLowerCase()
        parsedIngressRules.push({
          source,
          port: Number.isFinite(port) && port > 0 ? port : undefined,
          protocol: protocol === "udp" ? "udp" : "tcp",
        })
      }
      const parsedEgressRules: NonNullable<NetworkPolicy["egress_rules"]> = []
      for (const rule of egressRules) {
        const host = rule.host.trim()
        if (!host) {
          continue
        }
        const port = Number.parseInt(rule.port, 10)
        const protocol = rule.protocol.trim().toLowerCase()
        parsedEgressRules.push({
          host,
          port: Number.isFinite(port) && port > 0 ? port : undefined,
          protocol: protocol === "udp" ? "udp" : "tcp",
        })
      }
      const networkPolicy: NetworkPolicy = {
        isolation_mode: isolationMode,
        ingress_rules: parsedIngressRules,
        egress_rules: parsedEgressRules,
        deny_external_access: denyExternalAccess === "true",
      }
      const parsedCanaryPercentRaw = Number.parseInt(canaryPercent, 10)
      const parsedCanaryPercent = Number.isFinite(parsedCanaryPercentRaw)
        ? Math.max(0, Math.min(100, parsedCanaryPercentRaw))
        : 0
      const trimmedCanaryFunction = canaryFunction.trim()
      const rolloutPolicy: RolloutPolicy = {
        enabled: rolloutEnabled === "true" && trimmedCanaryFunction.length > 0 && parsedCanaryPercent > 0,
        canary_function: trimmedCanaryFunction,
        canary_percent: parsedCanaryPercent,
      }
      await functionsApi.update(func.name, {
        handler,
        memory_mb: parseInt(memory),
        timeout_s: parseInt(timeout),
        max_replicas: parseInt(maxReplicas) || 0,
        limits,
        network_policy: networkPolicy,
        rollout_policy: rolloutPolicy,
      })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to save configuration:", err)
    } finally {
      setSaving(false)
    }
  }

  const handleAddEnvVar = async () => {
    if (!newEnvKey.trim()) return

    try {
      setSavingEnv(true)
      const updatedEnvVars = { ...envVarsState, [newEnvKey]: newEnvValue }
      await functionsApi.update(func.name, { env_vars: updatedEnvVars })
      setEnvVarsState(updatedEnvVars)
      setNewEnvKey("")
      setNewEnvValue("")
      setShowAddEnvDialog(false)
      onUpdate?.()
    } catch (err) {
      console.error("Failed to add environment variable:", err)
    } finally {
      setSavingEnv(false)
    }
  }

  const handleDeleteEnvVar = async (key: string) => {
    try {
      const updatedEnvVars = { ...envVarsState }
      delete updatedEnvVars[key]
      await functionsApi.update(func.name, { env_vars: updatedEnvVars })
      setEnvVarsState(updatedEnvVars)
      onUpdate?.()
    } catch (err) {
      console.error("Failed to delete environment variable:", err)
    }
  }

  const handleDelete = async () => {
    if (deleteConfirmName !== func.name) return

    try {
      setDeleting(true)
      await functionsApi.delete(func.name)
      router.push("/functions")
    } catch (err) {
      console.error("Failed to delete function:", err)
      setDeleting(false)
    }
  }

  const handleCreateSnapshot = async () => {
    try {
      setCreatingSnapshot(true)
      setSnapshotMessage(null)
      await snapshotsApi.create(func.name)
      setSnapshotMessage({ type: 'success', text: 'Snapshot created successfully' })
    } catch (err) {
      console.error("Failed to create snapshot:", err)
      setSnapshotMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to create snapshot' })
    } finally {
      setCreatingSnapshot(false)
    }
  }

  return (
    <div className="space-y-6">
      {/* General Settings */}
      <div className="rounded-xl border border-border bg-card p-6">
        <h3 className="text-lg font-semibold text-card-foreground mb-4">
          General Settings
        </h3>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="handler">Handler</Label>
            <Input
              id="handler"
              value={handler}
              onChange={(e) => setHandler(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              The entry point for your function
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="timeout">Timeout (seconds)</Label>
            <Input
              id="timeout"
              type="number"
              min="1"
              max="900"
              value={timeout}
              onChange={(e) => setTimeout(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Maximum execution time
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="maxReplicas">Max VMs</Label>
            <Input
              id="maxReplicas"
              type="number"
              min="0"
              value={maxReplicas}
              onChange={(e) => setMaxReplicas(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Maximum concurrent VMs for this function (0 = unlimited)
            </p>
          </div>
        </div>

        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Code Hash</Label>
            <Input value={func.codeHash || ""} readOnly className="bg-muted/50 font-mono text-sm" />
            <p className="text-xs text-muted-foreground">
              SHA256 hash of the function source code
            </p>
          </div>

          <div className="space-y-2">
            <Label>Execution Mode</Label>
            <Input value={func.mode || "process"} readOnly className="bg-muted/50" />
            <p className="text-xs text-muted-foreground">
              Function execution mode
            </p>
          </div>
        </div>
      </div>

      {/* Resource Limits */}
      <div className="rounded-xl border border-border bg-card p-6">
        <h3 className="text-lg font-semibold text-card-foreground mb-1">
          Resource Limits
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          Configure CPU, disk I/O, and network bandwidth for the VM. Set to 0 for unlimited.
        </p>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="memory">Memory</Label>
            <Select value={memory} onValueChange={setMemory}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="128">128 MB</SelectItem>
                <SelectItem value="256">256 MB</SelectItem>
                <SelectItem value="512">512 MB</SelectItem>
                <SelectItem value="1024">1024 MB</SelectItem>
                <SelectItem value="2048">2048 MB</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Allocated memory for execution
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="vcpus">vCPUs</Label>
            <Select value={vcpus} onValueChange={setVcpus}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[1, 2, 4, 8, 16, 32].map((v) => (
                  <SelectItem key={v} value={v.toString()}>{v} vCPU{v > 1 ? "s" : ""}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Virtual CPU cores allocated
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="diskIops">Disk IOPS</Label>
            <Input
              id="diskIops"
              type="number"
              min="0"
              value={diskIops}
              onChange={(e) => setDiskIops(e.target.value)}
              placeholder="0 = unlimited"
            />
            <p className="text-xs text-muted-foreground">
              Max disk I/O operations per second
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="diskBandwidth">Disk Bandwidth (bytes/s)</Label>
            <Input
              id="diskBandwidth"
              type="number"
              min="0"
              value={diskBandwidth}
              onChange={(e) => setDiskBandwidth(e.target.value)}
              placeholder="0 = unlimited"
            />
            <p className="text-xs text-muted-foreground">
              Max disk throughput in bytes per second
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="netRx">Network RX Bandwidth (bytes/s)</Label>
            <Input
              id="netRx"
              type="number"
              min="0"
              value={netRx}
              onChange={(e) => setNetRx(e.target.value)}
              placeholder="0 = unlimited"
            />
            <p className="text-xs text-muted-foreground">
              Max inbound network throughput
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="netTx">Network TX Bandwidth (bytes/s)</Label>
            <Input
              id="netTx"
              type="number"
              min="0"
              value={netTx}
              onChange={(e) => setNetTx(e.target.value)}
              placeholder="0 = unlimited"
            />
            <p className="text-xs text-muted-foreground">
              Max outbound network throughput
            </p>
          </div>
        </div>
      </div>

      {/* Network Policy */}
      <div className="rounded-xl border border-border bg-card p-6">
        <h3 className="text-lg font-semibold text-card-foreground mb-1">
          Network Policy
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          Configure ingress and egress controls.
        </p>

        <div className="grid gap-6 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="isolationMode">Isolation Mode</Label>
            <Select value={isolationMode} onValueChange={setIsolationMode}>
              <SelectTrigger id="isolationMode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">none</SelectItem>
                <SelectItem value="egress-only">egress-only</SelectItem>
                <SelectItem value="strict">strict</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              `egress-only` is the default. `strict` blocks all outbound unless explicitly allowed.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="denyExternalAccess">Deny External Access</Label>
            <Select value={denyExternalAccess} onValueChange={setDenyExternalAccess}>
              <SelectTrigger id="denyExternalAccess">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="false">false</SelectItem>
                <SelectItem value="true">true</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              When true, blocks non-private external destinations.
            </p>
          </div>
        </div>

        <div className="mt-4 space-y-3">
          <div className="flex items-center justify-between">
            <Label>Ingress Rules</Label>
            <Button variant="outline" size="sm" onClick={addIngressRule}>
              <Plus className="mr-2 h-4 w-4" />
              Add Rule
            </Button>
          </div>

          {ingressRules.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No ingress rules configured.
            </p>
          ) : (
            ingressRules.map((rule, index) => (
              <div key={`ingress-rule-${index}`} className="grid gap-3 sm:grid-cols-[1fr_120px_140px_auto] items-end">
                <div className="space-y-1">
                  <Label>Source</Label>
                  <Input
                    value={rule.source}
                    onChange={(e) => updateIngressRule(index, "source", e.target.value)}
                    placeholder="caller-func or 10.0.0.0/8"
                  />
                </div>
                <div className="space-y-1">
                  <Label>Port</Label>
                  <Input
                    type="number"
                    min="0"
                    max="65535"
                    value={rule.port}
                    onChange={(e) => updateIngressRule(index, "port", e.target.value)}
                    placeholder="0"
                  />
                </div>
                <div className="space-y-1">
                  <Label>Protocol</Label>
                  <Select
                    value={rule.protocol || "tcp"}
                    onValueChange={(value) => updateIngressRule(index, "protocol", value)}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="tcp">tcp</SelectItem>
                      <SelectItem value="udp">udp</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-destructive hover:text-destructive"
                  onClick={() => removeIngressRule(index)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))
          )}
        </div>

        <div className="mt-4 space-y-3">
          <div className="flex items-center justify-between">
            <Label>Egress Rules</Label>
            <Button variant="outline" size="sm" onClick={addEgressRule}>
              <Plus className="mr-2 h-4 w-4" />
              Add Rule
            </Button>
          </div>

          {egressRules.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No egress rules configured. Behavior depends on isolation mode.
            </p>
          ) : (
            egressRules.map((rule, index) => (
              <div key={`egress-rule-${index}`} className="grid gap-3 sm:grid-cols-[1fr_120px_140px_auto] items-end">
                <div className="space-y-1">
                  <Label>Host / CIDR</Label>
                  <Input
                    value={rule.host}
                    onChange={(e) => updateEgressRule(index, "host", e.target.value)}
                    placeholder="example.com or 10.0.0.0/8"
                  />
                </div>
                <div className="space-y-1">
                  <Label>Port</Label>
                  <Input
                    type="number"
                    min="0"
                    max="65535"
                    value={rule.port}
                    onChange={(e) => updateEgressRule(index, "port", e.target.value)}
                    placeholder="0"
                  />
                </div>
                <div className="space-y-1">
                  <Label>Protocol</Label>
                  <Select
                    value={rule.protocol || "tcp"}
                    onValueChange={(value) => updateEgressRule(index, "protocol", value)}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="tcp">tcp</SelectItem>
                      <SelectItem value="udp">udp</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-destructive hover:text-destructive"
                  onClick={() => removeEgressRule(index)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))
          )}
        </div>
      </div>

      {/* Canary Rollout */}
      <div className="rounded-xl border border-border bg-card p-6">
        <h3 className="text-lg font-semibold text-card-foreground mb-1">
          Canary Rollout
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          Split invocation traffic from this function to a canary function.
        </p>

        <div className="grid gap-6 sm:grid-cols-3">
          <div className="space-y-2">
            <Label>Enabled</Label>
            <Select value={rolloutEnabled} onValueChange={setRolloutEnabled}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="true">enabled</SelectItem>
                <SelectItem value="false">disabled</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="canaryFunction">Canary Function</Label>
            <Input
              id="canaryFunction"
              value={canaryFunction}
              onChange={(e) => setCanaryFunction(e.target.value)}
              placeholder="function name"
            />
            <p className="text-xs text-muted-foreground">
              Requests for this function will be partially routed to this target function.
            </p>
          </div>
        </div>

        <div className="mt-4 space-y-2">
          <Label htmlFor="canaryPercent">Canary Traffic (%)</Label>
          <Input
            id="canaryPercent"
            type="number"
            min="0"
            max="100"
            value={canaryPercent}
            onChange={(e) => setCanaryPercent(e.target.value)}
            placeholder="10"
          />
          <div className="flex flex-wrap gap-2">
            {[1, 10, 25, 50].map((v) => (
              <Button key={`canary-${v}`} type="button" variant="outline" size="sm" onClick={() => setCanaryPercent(String(v))}>
                {v}%
              </Button>
            ))}
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => {
                setRolloutEnabled("false")
                setCanaryPercent("0")
              }}
            >
              Rollback
            </Button>
          </div>
        </div>
      </div>

      {/* Save button for General + Resource settings */}
      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={saving}>
          {saving ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Save className="mr-2 h-4 w-4" />
          )}
          Save Changes
        </Button>
      </div>

      {/* Environment Variables */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">
              Environment Variables
            </h3>
            <p className="text-sm text-muted-foreground">
              Manage secrets and configuration values
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setShowAddEnvDialog(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Variable
          </Button>
        </div>

        <div className="space-y-3">
          {envVars.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              No environment variables configured
            </p>
          ) : (
            envVars.map((config) => (
              <div
                key={config.id}
                className="flex items-center gap-4 rounded-lg border border-border bg-muted/30 p-4"
              >
                <div className="flex items-center gap-2 shrink-0">
                  <Key className="h-4 w-4 text-muted-foreground" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground font-mono">
                    {config.key}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {config.type === "secret" ? (
                    <>
                      <Input
                        type={showSecrets[config.id] ? "text" : "password"}
                        value={showSecrets[config.id] ? config.value : "••••••••"}
                        readOnly
                        className="w-40 font-mono text-sm"
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => toggleSecret(config.id)}
                      >
                        {showSecrets[config.id] ? (
                          <EyeOff className="h-4 w-4" />
                        ) : (
                          <Eye className="h-4 w-4" />
                        )}
                      </Button>
                    </>
                  ) : (
                    <Input
                      value={config.value}
                      readOnly
                      className="w-40 font-mono text-sm"
                    />
                  )}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-destructive hover:text-destructive"
                    onClick={() => handleDeleteEnvVar(config.key)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {/* Snapshots */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">
              VM Snapshot
            </h3>
            <p className="text-sm text-muted-foreground">
              Create a snapshot for faster cold starts
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={handleCreateSnapshot}
            disabled={creatingSnapshot}
          >
            {creatingSnapshot ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Camera className="mr-2 h-4 w-4" />
            )}
            Create Snapshot
          </Button>
        </div>
        {snapshotMessage && (
          <div className={`text-sm p-3 rounded-lg ${
            snapshotMessage.type === 'success'
              ? 'bg-success/10 text-success'
              : 'bg-destructive/10 text-destructive'
          }`}>
            {snapshotMessage.text}
          </div>
        )}
      </div>

      {/* Auto Scaling Policy */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">
              Auto Scaling Policy
            </h3>
            <p className="text-sm text-muted-foreground">
              Configure replica scaling behavior based on load and queue pressure.
            </p>
          </div>
        </div>

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

            <div className="flex items-center gap-2">
              <Button onClick={handleSaveAutoScale} disabled={savingAutoScale}>
                {savingAutoScale ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
                Save Auto Scaling
              </Button>
              <Button variant="outline" onClick={handleDisableAutoScale} disabled={savingAutoScale}>
                Disable
              </Button>
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
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">
              Capacity Protection Policy
            </h3>
            <p className="text-sm text-muted-foreground">
              Protect the function from overload with in-flight, queue, and shed controls.
            </p>
          </div>
        </div>

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

            <div className="flex items-center gap-2">
              <Button onClick={handleSaveCapacity} disabled={savingCapacity}>
                {savingCapacity ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
                Save Capacity Policy
              </Button>
              <Button variant="outline" onClick={handleDisableCapacity} disabled={savingCapacity}>
                Disable
              </Button>
            </div>
            {capacityMessage && (
              <div className={`text-sm p-3 rounded-lg ${capacityMessage.type === "success" ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive"}`}>
                {capacityMessage.text}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Danger Zone */}
      <div className="rounded-xl border border-destructive/50 bg-card p-6">
        <h3 className="text-lg font-semibold text-destructive mb-2">
          Danger Zone
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          These actions are irreversible. Please proceed with caution.
        </p>
        <div className="flex gap-3">
          <Button
            variant="outline"
            className="border-destructive text-destructive hover:bg-destructive hover:text-destructive-foreground bg-transparent"
            onClick={() => setShowDeleteDialog(true)}
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Delete Function
          </Button>
        </div>
      </div>

      {/* Add Environment Variable Dialog */}
      <Dialog open={showAddEnvDialog} onOpenChange={setShowAddEnvDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Environment Variable</DialogTitle>
            <DialogDescription>
              Add a new environment variable to this function.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="envKey">Key</Label>
              <Input
                id="envKey"
                placeholder="MY_VARIABLE"
                value={newEnvKey}
                onChange={(e) => setNewEnvKey(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '_'))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="envValue">Value</Label>
              <Input
                id="envValue"
                placeholder="value"
                value={newEnvValue}
                onChange={(e) => setNewEnvValue(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowAddEnvDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleAddEnvVar} disabled={!newEnvKey.trim() || savingEnv}>
              {savingEnv ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Plus className="mr-2 h-4 w-4" />
              )}
              Add Variable
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              Delete Function
            </DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the function
              and all its versions and aliases.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-muted-foreground">
              Please type <span className="font-mono font-semibold text-foreground">{func.name}</span> to confirm.
            </p>
            <Input
              placeholder="Enter function name"
              value={deleteConfirmName}
              onChange={(e) => setDeleteConfirmName(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setShowDeleteDialog(false); setDeleteConfirmName(""); }}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteConfirmName !== func.name || deleting}
            >
              {deleting ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="mr-2 h-4 w-4" />
              )}
              Delete Function
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

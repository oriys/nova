"use client"

import { useState, useEffect } from "react"
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
  type NetworkPolicy,
  type RolloutPolicy,
  type ResourceLimits,
  type AsyncDestinations,
  type NovaFunction,
} from "@/lib/api"
import { Save, Plus, Trash2, Loader2 } from "lucide-react"
import { FunctionConfigEnv } from "@/components/function-config-env"
import { FunctionConfigSnapshots } from "@/components/function-config-snapshots"
import { FunctionConfigScaling } from "@/components/function-config-scaling"
import { FunctionConfigDanger } from "@/components/function-config-danger"
import { SectionHeader } from "@/components/section-header"

interface FunctionConfigProps {
  func: FunctionData
  onUpdate?: () => void
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
  const [memory, setMemory] = useState(func.memory.toString())
  const [timeout, setTimeout] = useState(func.timeout.toString())
  const [handler, setHandler] = useState(func.handler)
  const [maxReplicas, setMaxReplicas] = useState((func.maxReplicas ?? 0).toString())
  const [logRetentionDays, setLogRetentionDays] = useState((func.logRetentionDays ?? 0).toString())
  const [saving, setSaving] = useState(false)

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

  // Tags state
  const [tags, setTags] = useState<Record<string, string>>(func.tags ?? {})
  const [newTagKey, setNewTagKey] = useState("")
  const [newTagValue, setNewTagValue] = useState("")

  const addTag = () => {
    const key = newTagKey.trim()
    if (!key) return
    setTags((prev) => ({ ...prev, [key]: newTagValue.trim() }))
    setNewTagKey("")
    setNewTagValue("")
  }

  const removeTag = (key: string) => {
    setTags((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
  }

  // Async destinations state
  const [onSuccessType, setOnSuccessType] = useState(func.asyncDestinations?.on_success?.type || "")
  const [onSuccessTarget, setOnSuccessTarget] = useState(func.asyncDestinations?.on_success?.target || "")
  const [onFailureType, setOnFailureType] = useState(func.asyncDestinations?.on_failure?.type || "")
  const [onFailureTarget, setOnFailureTarget] = useState(func.asyncDestinations?.on_failure?.target || "")
  const [allFunctions, setAllFunctions] = useState<NovaFunction[]>([])

  useEffect(() => {
    functionsApi.list().then(setAllFunctions).catch(() => {})
  }, [])

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

  const buildAsyncDestinations = (): AsyncDestinations | undefined => {
    const dest: AsyncDestinations = {}
    if (onSuccessType && onSuccessTarget.trim()) {
      dest.on_success = { type: onSuccessType as 'function' | 'topic', target: onSuccessTarget.trim() }
    }
    if (onFailureType && onFailureTarget.trim()) {
      dest.on_failure = { type: onFailureType as 'function' | 'topic', target: onFailureTarget.trim() }
    }
    return (dest.on_success || dest.on_failure) ? dest : undefined
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
        log_retention_days: parseInt(logRetentionDays) || 0,
        limits,
        network_policy: networkPolicy,
        rollout_policy: rolloutPolicy,
        tags,
        async_destinations: buildAsyncDestinations(),
      })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to save configuration:", err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
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

      {/* General Settings */}
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader title="General Settings" className="mb-4" />
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

          <div className="space-y-2">
            <Label htmlFor="logRetention">Log Retention (days)</Label>
            <Input
              id="logRetention"
              type="number"
              min="0"
              max="365"
              value={logRetentionDays}
              onChange={(e) => setLogRetentionDays(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Auto-delete logs older than this (0 = use global default)
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
        <SectionHeader
          title="Resource Limits"
          description="Configure CPU, disk I/O, and network bandwidth for the VM. Set to 0 for unlimited."
          className="mb-4"
        />
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
        <SectionHeader
          title="Network Policy"
          description="Configure ingress and egress controls."
          className="mb-4"
        />

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
        <SectionHeader
          title="Canary Rollout"
          description="Split invocation traffic from this function to a canary function."
          className="mb-4"
        />

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

      {/* Async Destinations */}
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader
          title="Async Destinations"
          description="Automatically invoke a function or publish to a topic after async invocation completes."
          className="mb-4"
        />

        <div className="grid gap-6 sm:grid-cols-2">
          <div className="space-y-3">
            <Label className="font-medium">On Success</Label>
            <Select value={onSuccessType} onValueChange={setOnSuccessType}>
              <SelectTrigger>
                <SelectValue placeholder="Disabled" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Disabled</SelectItem>
                <SelectItem value="function">Function</SelectItem>
                <SelectItem value="topic">Topic</SelectItem>
              </SelectContent>
            </Select>
            {onSuccessType && onSuccessType !== "none" && (
              onSuccessType === "function" ? (
                <Select value={onSuccessTarget} onValueChange={setOnSuccessTarget}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select function" />
                  </SelectTrigger>
                  <SelectContent>
                    {allFunctions.filter(f => f.name !== func.name).map(f => (
                      <SelectItem key={f.name} value={f.name}>{f.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={onSuccessTarget}
                  onChange={(e) => setOnSuccessTarget(e.target.value)}
                  placeholder="topic name"
                />
              )
            )}
          </div>

          <div className="space-y-3">
            <Label className="font-medium">On Failure</Label>
            <Select value={onFailureType} onValueChange={setOnFailureType}>
              <SelectTrigger>
                <SelectValue placeholder="Disabled" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Disabled</SelectItem>
                <SelectItem value="function">Function</SelectItem>
                <SelectItem value="topic">Topic</SelectItem>
              </SelectContent>
            </Select>
            {onFailureType && onFailureType !== "none" && (
              onFailureType === "function" ? (
                <Select value={onFailureTarget} onValueChange={setOnFailureTarget}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select function" />
                  </SelectTrigger>
                  <SelectContent>
                    {allFunctions.filter(f => f.name !== func.name).map(f => (
                      <SelectItem key={f.name} value={f.name}>{f.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={onFailureTarget}
                  onChange={(e) => setOnFailureTarget(e.target.value)}
                  placeholder="topic name"
                />
              )
            )}
          </div>
        </div>
      </div>

      {/* Tags */}
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader
          title="Tags"
          description="Key-value labels for grouping and filtering functions."
          className="mb-4"
        />

        {Object.keys(tags).length > 0 && (
          <div className="flex flex-wrap gap-2 mb-4">
            {Object.entries(tags).map(([k, v]) => (
              <span
                key={k}
                className="inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/50 px-2.5 py-1 text-sm"
              >
                <span className="font-medium text-foreground">{k}</span>
                {v && <span className="text-muted-foreground">= {v}</span>}
                <button
                  type="button"
                  onClick={() => removeTag(k)}
                  className="ml-0.5 text-muted-foreground hover:text-destructive transition-colors"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </span>
            ))}
          </div>
        )}

        <div className="flex gap-2 items-end">
          <div className="space-y-1 flex-1">
            <Label htmlFor="tagKey">Key</Label>
            <Input
              id="tagKey"
              value={newTagKey}
              onChange={(e) => setNewTagKey(e.target.value)}
              placeholder="env"
              onKeyDown={(e) => e.key === "Enter" && addTag()}
            />
          </div>
          <div className="space-y-1 flex-1">
            <Label htmlFor="tagValue">Value</Label>
            <Input
              id="tagValue"
              value={newTagValue}
              onChange={(e) => setNewTagValue(e.target.value)}
              placeholder="production"
              onKeyDown={(e) => e.key === "Enter" && addTag()}
            />
          </div>
          <Button type="button" variant="outline" size="icon" onClick={addTag} disabled={!newTagKey.trim()}>
            <Plus className="h-4 w-4" />
          </Button>
        </div>
      </div>

      <FunctionConfigEnv func={func} onUpdate={onUpdate} />
      <FunctionConfigSnapshots functionName={func.name} />
      <FunctionConfigScaling func={func} onUpdate={onUpdate} />
      <FunctionConfigDanger func={func} />
    </div>
  )
}

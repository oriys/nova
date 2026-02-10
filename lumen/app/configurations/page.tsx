"use client"

import { useEffect, useState, useCallback } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Pagination } from "@/components/pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { healthApi, snapshotsApi, configApi, type HealthStatus } from "@/lib/api"
import { RefreshCw, Server, Database, HardDrive, Trash2, Save, CheckCircle } from "lucide-react"
import { cn } from "@/lib/utils"
import { useAutoRefresh } from "@/lib/use-auto-refresh"

interface Snapshot {
  function_id: string
  function_name: string
  snap_size: number
  mem_size: number
  total_size: number
  created_at: string
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const k = 1024
  const sizes = ["B", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i]
}

function isHealthyComponent(value: unknown): boolean {
  if (typeof value === "boolean") return value
  if (typeof value === "string") return value.toLowerCase().startsWith("healthy")
  return false
}

function componentStatusText(value: unknown): string {
  if (typeof value === "string" && value.trim() !== "") return value
  if (typeof value === "boolean") return value ? "healthy" : "unhealthy"
  return "unknown"
}

export default function ConfigurationsPage() {
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [snapshotsPage, setSnapshotsPage] = useState(1)
  const [snapshotsPageSize, setSnapshotsPageSize] = useState(10)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // Settings from backend
  const [poolTTL, setPoolTTL] = useState("60")
  const [logLevel, setLogLevel] = useState("info")
  const [dirty, setDirty] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [healthData, snapshotsData, configData] = await Promise.all([
        healthApi.check(),
        snapshotsApi.list().catch(() => []),
        configApi.get().catch(() => ({} as Record<string, string>)),
      ])
      setHealth(healthData)
      setSnapshots(snapshotsData)

      // Apply config from backend
      if (configData["pool_ttl"]) setPoolTTL(configData["pool_ttl"])
      if (configData["log_level"]) setLogLevel(configData["log_level"])
      setDirty(false)
    } catch (err) {
      console.error("Failed to fetch config data:", err)
      setError(err instanceof Error ? err.message : "Failed to load configuration")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh("configurations", fetchData, 30000)

  const snapshotsTotalPages = Math.max(1, Math.ceil(snapshots.length / snapshotsPageSize))
  useEffect(() => {
    if (snapshotsPage > snapshotsTotalPages) setSnapshotsPage(snapshotsTotalPages)
  }, [snapshotsPage, snapshotsTotalPages])

  const pagedSnapshots = snapshots.slice(
    (snapshotsPage - 1) * snapshotsPageSize,
    snapshotsPage * snapshotsPageSize
  )

  const handleSave = async () => {
    try {
      setSaving(true)
      setSaved(false)
      await configApi.update({
        pool_ttl: poolTTL,
        log_level: logLevel,
      })
      setDirty(false)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    } catch (err) {
      console.error("Failed to save config:", err)
      setError(err instanceof Error ? err.message : "Failed to save configuration")
    } finally {
      setSaving(false)
    }
  }

  const handleDeleteSnapshot = async (functionName: string) => {
    try {
      await snapshotsApi.delete(functionName)
      fetchData()
    } catch (err) {
      console.error("Failed to delete snapshot:", err)
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title="Configurations" description="System settings and health" />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load configuration</p>
            <p className="text-sm mt-1">{error}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => { setError(null); fetchData(); }}>
              Retry
            </Button>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title="Configurations" description="System settings and health" />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-end gap-2">
          <Button
            variant={autoRefresh ? "default" : "outline"}
            size="sm"
            onClick={toggleAutoRefresh}
          >
            <span className={cn(
              "mr-2 h-2 w-2 rounded-full",
              autoRefresh ? "bg-success animate-pulse" : "bg-muted-foreground"
            )} />
            Auto
          </Button>
          <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            Refresh
          </Button>
        </div>

        {/* System Health */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            System Health
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Status</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    health?.status === "ok"
                      ? "bg-success/10 text-success border-0"
                      : "bg-warning/10 text-warning border-0"
                  )}
                >
                  {loading ? "..." : health?.status || "unknown"}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Database className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">PostgreSQL</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    isHealthyComponent(health?.components?.postgres)
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : isHealthyComponent(health?.components?.postgres) ? "Connected" : "Disconnected"}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Zenith</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    isHealthyComponent(health?.components?.zenith)
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : componentStatusText(health?.components?.zenith)}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Nova</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    isHealthyComponent(health?.components?.nova)
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : componentStatusText(health?.components?.nova)}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Comet</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    isHealthyComponent(health?.components?.comet)
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : componentStatusText(health?.components?.comet)}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Server className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Active VMs</p>
                <p className="text-lg font-semibold">
                  {loading ? "..." : health?.components?.pool?.active_vms ?? 0}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Pool Settings */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            Pool Settings
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div className="space-y-2">
              <Label htmlFor="poolTTL">Idle VM TTL (seconds)</Label>
              <Input
                id="poolTTL"
                type="number"
                value={poolTTL}
                onChange={(e) => { setPoolTTL(e.target.value); setDirty(true); }}
                min="10"
                max="3600"
              />
              <p className="text-xs text-muted-foreground">
                Time before idle VMs are terminated
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="logLevel">Log Level</Label>
              <Select value={logLevel} onValueChange={(v) => { setLogLevel(v); setDirty(true); }}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="debug">Debug</SelectItem>
                  <SelectItem value="info">Info</SelectItem>
                  <SelectItem value="warn">Warn</SelectItem>
                  <SelectItem value="error">Error</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Minimum log level to capture
              </p>
            </div>
          </div>
          <div className="mt-4 flex items-center gap-3">
            <Button
              variant="outline"
              onClick={handleSave}
              disabled={saving || !dirty}
            >
              {saving ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save Settings
                </>
              )}
            </Button>
            {saved && (
              <span className="flex items-center gap-1 text-sm text-success">
                <CheckCircle className="h-4 w-4" />
                Saved
              </span>
            )}
          </div>
        </div>

        {/* Snapshots */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            VM Snapshots
          </h3>
          {snapshots.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4">
              No snapshots created yet. Create snapshots from the function detail page.
            </p>
          ) : (
            <div className="space-y-3">
              {pagedSnapshots.map((snap) => (
                <div
                  key={snap.function_id}
                  className="flex items-center justify-between p-4 rounded-lg bg-muted/50"
                >
                  <div className="flex items-center gap-3">
                    <HardDrive className="h-5 w-5 text-muted-foreground" />
                    <div>
                      <p className="font-medium">{snap.function_name}</p>
                      <p className="text-sm text-muted-foreground">
                        {formatBytes(snap.total_size)} ·{" "}
                        {new Date(snap.created_at).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDeleteSnapshot(snap.function_name)}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              ))}

              <div className="pt-2">
                <Pagination
                  totalItems={snapshots.length}
                  page={snapshotsPage}
                  pageSize={snapshotsPageSize}
                  onPageChange={setSnapshotsPage}
                  onPageSizeChange={(size) => {
                    setSnapshotsPageSize(size)
                    setSnapshotsPage(1)
                  }}
                  pageSizeOptions={[5, 10, 20, 50]}
                  itemLabel="snapshots"
                />
              </div>
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

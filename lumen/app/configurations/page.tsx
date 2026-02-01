"use client"

import { useEffect, useState, useCallback } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
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
import { healthApi, snapshotsApi } from "@/lib/api"
import { RefreshCw, Server, Database, HardDrive, Trash2 } from "lucide-react"
import { cn } from "@/lib/utils"

interface Snapshot {
  function_id: string
  function_name: string
  snap_size: number
  mem_size: number
  total_size: number
  created_at: string
}

interface HealthStatus {
  status: string
  components?: {
    postgres: boolean
    redis: boolean
    pool: {
      active_vms: number
      total_pools: number | null
    }
  }
  uptime_seconds?: number
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const k = 1024
  const sizes = ["B", "KB", "MB", "GB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i]
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h ${mins}m`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

export default function ConfigurationsPage() {
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Settings (local state only - would connect to backend in production)
  const [poolTTL, setPoolTTL] = useState("60")
  const [logLevel, setLogLevel] = useState("info")

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [healthData, snapshotsData] = await Promise.all([
        healthApi.check(),
        snapshotsApi.list().catch(() => []),
      ])
      setHealth(healthData)
      setSnapshots(snapshotsData)
    } catch (err) {
      console.error("Failed to fetch config data:", err)
      setError(err instanceof Error ? err.message : "Failed to load configuration")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 30000)
    return () => clearInterval(interval)
  }, [fetchData])

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
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title="Configurations" description="System settings and health" />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-end">
          <Button variant="outline" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            Refresh
          </Button>
        </div>

        {/* System Health */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h3 className="text-lg font-semibold text-card-foreground mb-4">
            System Health
          </h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
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
                    health?.components?.postgres
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : health?.components?.postgres ? "Connected" : "Disconnected"}
                </Badge>
              </div>
            </div>

            <div className="flex items-center gap-3 p-4 rounded-lg bg-muted/50">
              <Database className="h-8 w-8 text-primary" />
              <div>
                <p className="text-sm text-muted-foreground">Redis</p>
                <Badge
                  variant="secondary"
                  className={cn(
                    health?.components?.redis
                      ? "bg-success/10 text-success border-0"
                      : "bg-destructive/10 text-destructive border-0"
                  )}
                >
                  {loading ? "..." : health?.components?.redis ? "Connected" : "Disconnected"}
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
                onChange={(e) => setPoolTTL(e.target.value)}
                min="10"
                max="3600"
              />
              <p className="text-xs text-muted-foreground">
                Time before idle VMs are terminated
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="logLevel">Log Level</Label>
              <Select value={logLevel} onValueChange={setLogLevel}>
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
          <div className="mt-4">
            <Button variant="outline" disabled>
              Save Settings (requires restart)
            </Button>
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
              {snapshots.map((snap) => (
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
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

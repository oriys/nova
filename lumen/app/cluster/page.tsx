"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { clusterApi } from "@/lib/api"
import type { ClusterNodeEntry } from "@/lib/api"
import { Server, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

export default function ClusterPage() {
  const t = useTranslations("pages")
  const tc = useTranslations("common")
  const tp = useTranslations("clusterPage")
  const [nodes, setNodes] = useState<ClusterNodeEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchNodes = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await clusterApi.listNodes()
      setNodes(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tp("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tp])

  useEffect(() => {
    fetchNodes()
  }, [fetchNodes])

  const handleRemove = async (id: string) => {
    if (!confirm(tp("removeConfirm"))) return
    try { await clusterApi.deleteNode(id); fetchNodes() }
    catch (err) { setError(err instanceof Error ? err.message : tp("failedToLoad")) }
  }

  const stateBadge = (state: string) => {
    const styles: Record<string, string> = {
      active: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
      inactive: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
      drained: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
    }
    return (
      <span className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        styles[state] || "bg-muted text-muted-foreground"
      )}>
        {state}
      </span>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("cluster.title")} description={t("cluster.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-end">
          <Button variant="outline" size="sm" onClick={fetchNodes} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colAddress")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colState")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colVMs")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colQueue")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tp("colHeartbeat")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tp("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={7} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : nodes.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground">
                    <Server className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tp("noNodes")}
                  </td>
                </tr>
              ) : (
                nodes.map((node) => (
                  <tr key={node.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Server className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{node.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground font-mono">{node.address}</td>
                    <td className="px-4 py-3">{stateBadge(node.state)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{node.active_vms}/{node.max_vms}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{node.queue_depth}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(node.last_heartbeat).toLocaleString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemove(node.id)}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </DashboardLayout>
  )
}

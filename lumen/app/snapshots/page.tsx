"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { snapshotsApi } from "@/lib/api"
import { Camera, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

interface SnapshotInfo {
  function_id: string;
  function_name: string;
  snap_size: number;
  mem_size: number;
  total_size: number;
  created_at: string;
}

function formatSize(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`
  return `${mb} MB`
}

export default function SnapshotsPage() {
  const t = useTranslations("pages")
  const ts = useTranslations("snapshotsPage")
  const tc = useTranslations("common")
  const [snapshots, setSnapshots] = useState<SnapshotInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchSnapshots = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await snapshotsApi.list()
      setSnapshots(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : ts("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [ts])

  useEffect(() => {
    fetchSnapshots()
  }, [fetchSnapshots])

  const handleDelete = async (functionName: string) => {
    try {
      await snapshotsApi.delete(functionName)
      fetchSnapshots()
    } catch (err) {
      setError(err instanceof Error ? err.message : ts("failedToDelete"))
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("snapshots.title")} description={t("snapshots.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-end">
          <Button variant="outline" size="sm" onClick={fetchSnapshots} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ts("colFunctionName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ts("colSnapSize")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ts("colMemSize")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ts("colTotalSize")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ts("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{ts("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={6} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : snapshots.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                    <Camera className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {ts("noSnapshots")}
                  </td>
                </tr>
              ) : (
                snapshots.map((snap) => (
                  <tr key={snap.function_id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <span className="font-medium text-sm font-mono">{snap.function_name}</span>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{formatSize(snap.snap_size)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{formatSize(snap.mem_size)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{formatSize(snap.total_size)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(snap.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(snap.function_name)}
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

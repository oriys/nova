"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { notificationsApi } from "@/lib/api"
import type { NotificationEntry } from "@/lib/api"
import { Activity, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

export default function AlertsPage() {
  const t = useTranslations("pages")
  const tc = useTranslations("common")
  const ta = useTranslations("alertsPage")
  const [alerts, setAlerts] = useState<NotificationEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchAlerts = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await notificationsApi.list("all", 100)
      setAlerts((data || []).filter((n) => n.type === "slo_alert"))
    } catch (err) {
      setError(err instanceof Error ? err.message : ta("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [ta])

  useEffect(() => {
    fetchAlerts()
  }, [fetchAlerts])

  const severityBadge = (severity: string) => {
    const styles: Record<string, string> = {
      warning: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
      info: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    }
    return (
      <span className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        styles[severity] || "bg-muted text-muted-foreground"
      )}>
        {severity}
      </span>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("alerts.title")} description={t("alerts.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-end">
          <Button variant="outline" size="sm" onClick={fetchAlerts} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colTitle")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colSeverity")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colFunction")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colTime")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colStatus")}</th>
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
              ) : alerts.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                    <Activity className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {ta("noAlerts")}
                  </td>
                </tr>
              ) : (
                alerts.map((alert) => (
                  <tr key={alert.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Activity className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm">{alert.title}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">{severityBadge(alert.severity)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{alert.function_name || "—"}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(alert.created_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-3">
                      <span className={cn(
                        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
                        alert.status === "unread" ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400" : "bg-muted text-muted-foreground"
                      )}>
                        {alert.status}
                      </span>
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

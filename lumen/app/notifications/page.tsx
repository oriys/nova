"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { notificationsApi } from "@/lib/api"
import type { NotificationEntry, NotificationStatus } from "@/lib/api"
import { Bell, RefreshCw, CheckCheck, Check } from "lucide-react"
import { cn } from "@/lib/utils"

const severityStyles: Record<string, string> = {
  info: "bg-blue-500/10 text-blue-500 border-blue-500/20",
  warning: "bg-yellow-500/10 text-yellow-500 border-yellow-500/20",
  error: "bg-red-500/10 text-red-500 border-red-500/20",
  critical: "bg-red-500/10 text-red-600 border-red-500/20 font-bold",
}

export default function NotificationsPage() {
  const t = useTranslations("pages")
  const tn = useTranslations("notificationsPage")
  const tc = useTranslations("common")
  const [notifications, setNotifications] = useState<NotificationEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<NotificationStatus>("all")

  const fetchNotifications = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await notificationsApi.list(filter, 100)
      setNotifications(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tn("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [filter, tn])

  useEffect(() => {
    fetchNotifications()
  }, [fetchNotifications])

  const handleMarkRead = async (id: string) => {
    try {
      await notificationsApi.markRead(id)
      fetchNotifications()
    } catch (err) {
      setError(err instanceof Error ? err.message : tn("failedToLoad"))
    }
  }

  const handleMarkAllRead = async () => {
    try {
      await notificationsApi.markAllRead()
      fetchNotifications()
    } catch (err) {
      setError(err instanceof Error ? err.message : tn("failedToLoad"))
    }
  }

  const filters: { value: NotificationStatus; label: string }[] = [
    { value: "all", label: tn("all") },
    { value: "unread", label: tn("unread") },
    { value: "read", label: tn("read") },
  ]

  return (
    <DashboardLayout>
      <Header title={t("notifications.title")} description={t("notifications.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {filters.map((f) => (
              <Button
                key={f.value}
                variant={filter === f.value ? "default" : "outline"}
                size="sm"
                onClick={() => setFilter(f.value)}
              >
                {f.label}
              </Button>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleMarkAllRead}>
              <CheckCheck className="mr-2 h-4 w-4" />
              {tn("markAllRead")}
            </Button>
            <Button variant="outline" size="sm" onClick={fetchNotifications} disabled={loading}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
              {tc("refresh")}
            </Button>
          </div>
        </div>

        <div className="space-y-3">
          {loading ? (
            Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="rounded-xl border border-border bg-card p-4">
                <div className="h-4 bg-muted rounded animate-pulse" />
              </div>
            ))
          ) : notifications.length === 0 ? (
            <div className="rounded-xl border border-border bg-card p-8 text-center text-muted-foreground">
              <Bell className="mx-auto h-8 w-8 mb-2 opacity-50" />
              {tn("noNotifications")}
            </div>
          ) : (
            notifications.map((n) => (
              <div
                key={n.id}
                className={cn(
                  "rounded-xl border border-border bg-card p-4 hover:bg-muted/50",
                  n.status === "unread" && "border-l-4 border-l-primary"
                )}
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1 space-y-1">
                    <div className="flex items-center gap-2">
                      <span
                        className={cn(
                          "inline-flex items-center rounded-md border px-2 py-0.5 text-xs",
                          severityStyles[n.severity] || severityStyles.info
                        )}
                      >
                        {n.severity}
                      </span>
                      <span className="font-medium text-sm">{n.title}</span>
                    </div>
                    <p className="text-sm text-muted-foreground">{n.message}</p>
                    <div className="flex items-center gap-4 text-xs text-muted-foreground">
                      {n.function_name && (
                        <span>
                          {tn("function")}: <span className="font-mono">{n.function_name}</span>
                        </span>
                      )}
                      {n.source && (
                        <span>
                          {tn("source")}: {n.source}
                        </span>
                      )}
                      <span>{new Date(n.created_at).toLocaleString()}</span>
                    </div>
                  </div>
                  {n.status === "unread" && (
                    <Button variant="ghost" size="sm" onClick={() => handleMarkRead(n.id)}>
                      <Check className="mr-1 h-4 w-4" />
                      {tn("markRead")}
                    </Button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

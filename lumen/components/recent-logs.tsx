"use client"

import Link from "next/link"
import { useTranslations } from "next-intl"
import { cn } from "@/lib/utils"
import { LogEntry } from "@/lib/types"
import { ArrowRight, Info, AlertTriangle, XCircle, Bug, Loader2 } from "lucide-react"

interface RecentLogsProps {
  logs: LogEntry[]
  loading?: boolean
}

const levelConfig = {
  info: { icon: Info, color: "text-primary", bg: "bg-primary/10" },
  warn: { icon: AlertTriangle, color: "text-warning", bg: "bg-warning/10" },
  error: { icon: XCircle, color: "text-destructive", bg: "bg-destructive/10" },
  debug: { icon: Bug, color: "text-muted-foreground", bg: "bg-muted" },
}

export function RecentLogs({ logs, loading }: RecentLogsProps) {
  const t = useTranslations("recentLogs")

  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card">
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <div>
            <h3 className="text-sm font-semibold text-card-foreground">
              {t("title")}
            </h3>
            <p className="text-xs text-muted-foreground">
              {t("description")}
            </p>
          </div>
        </div>
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-6 py-4">
        <div>
          <h3 className="text-sm font-semibold text-card-foreground">
            {t("title")}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("description")}
          </p>
        </div>
        <Link
          href="/functions"
          className="flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          {t("viewAll")}
          <ArrowRight className="h-3 w-3" />
        </Link>
      </div>
      {logs.length === 0 ? (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">{t("noLogs")}</p>
        </div>
      ) : (
        <div className="divide-y divide-border">
          {logs.map((log) => {
            const config = levelConfig[log.level]
            const Icon = config.icon
            const time = new Date(log.timestamp).toLocaleTimeString("en-US", {
              hour: "2-digit",
              minute: "2-digit",
              second: "2-digit",
            })

            return (
              <div
                key={log.id}
                className="flex items-start gap-3 px-6 py-3 hover:bg-muted/20 transition-colors"
              >
                <div className={cn("rounded-md p-1.5 mt-0.5", config.bg)}>
                  <Icon className={cn("h-3.5 w-3.5", config.color)} />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-xs font-medium text-foreground truncate">
                      {log.functionName}
                    </span>
                    <span className="text-xs text-muted-foreground">{time}</span>
                  </div>
                  <p className="text-xs text-muted-foreground line-clamp-1">
                    {log.message}
                  </p>
                </div>
                {log.duration && (
                  <span className="text-xs text-muted-foreground whitespace-nowrap">
                    {t("durationMs", { duration: log.duration })}
                  </span>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

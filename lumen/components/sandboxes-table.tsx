"use client"

import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Trash2, Terminal, RefreshCw } from "lucide-react"
import type { SandboxData } from "@/lib/types"

interface SandboxesTableProps {
  data: SandboxData[]
  loading: boolean
  onOpen: (id: string) => void
  onDestroy: (id: string) => void
  onKeepalive: (id: string) => void
}

const statusVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  running: "default",
  creating: "secondary",
  paused: "outline",
  stopped: "secondary",
  error: "destructive",
}

export function SandboxesTable({ data, loading, onOpen, onDestroy, onKeepalive }: SandboxesTableProps) {
  const t = useTranslations("pages.sandboxes")

  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <p className="text-sm text-muted-foreground">{t("loading")}</p>
      </div>
    )
  }

  if (data.length === 0) {
    return null
  }

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-muted/50">
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.id")}</th>
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.template")}</th>
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.status")}</th>
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.resources")}</th>
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.created")}</th>
            <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t("table.expires")}</th>
            <th className="px-4 py-3 text-right font-medium text-muted-foreground">{t("table.actions")}</th>
          </tr>
        </thead>
        <tbody>
          {data.map((sb) => (
            <tr key={sb.id} className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
              <td className="px-4 py-3">
                <button
                  onClick={() => onOpen(sb.id)}
                  className="font-mono text-xs text-primary hover:underline"
                >
                  {sb.id.slice(0, 12)}…
                </button>
              </td>
              <td className="px-4 py-3 font-medium">{sb.template}</td>
              <td className="px-4 py-3">
                <Badge variant={statusVariant[sb.status] ?? "secondary"}>
                  {sb.status}
                </Badge>
              </td>
              <td className="px-4 py-3 text-muted-foreground">
                {sb.vcpus} vCPU · {sb.memoryMB} MB
              </td>
              <td className="px-4 py-3 text-muted-foreground text-xs">
                {new Date(sb.createdAt).toLocaleString()}
              </td>
              <td className="px-4 py-3 text-muted-foreground text-xs">
                {new Date(sb.expiresAt).toLocaleString()}
              </td>
              <td className="px-4 py-3">
                <div className="flex items-center justify-end gap-1">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onOpen(sb.id)}
                    title={t("buttons.open")}
                    disabled={sb.status !== "running"}
                  >
                    <Terminal className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onKeepalive(sb.id)}
                    title={t("buttons.keepalive")}
                    disabled={sb.status !== "running"}
                  >
                    <RefreshCw className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onDestroy(sb.id)}
                    title={t("buttons.destroy")}
                    className="text-destructive hover:text-destructive"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

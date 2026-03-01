"use client"

import { useState, useEffect, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { RefreshCw, XCircle } from "lucide-react"
import { sandboxApi, type SandboxProcessInfo } from "@/lib/api"

interface SandboxProcessesProps {
  sandboxId: string
}

export function SandboxProcesses({ sandboxId }: SandboxProcessesProps) {
  const t = useTranslations("pages.sandboxes.processes")
  const [processes, setProcesses] = useState<SandboxProcessInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadProcesses = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const procs = await sandboxApi.processList(sandboxId)
      setProcesses(procs)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [sandboxId])

  useEffect(() => {
    loadProcesses()
    const timer = setInterval(loadProcesses, 5000)
    return () => clearInterval(timer)
  }, [loadProcesses])

  const handleKill = async (pid: number) => {
    try {
      await sandboxApi.processKill(sandboxId, pid)
      loadProcesses()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="flex h-full flex-col rounded-lg border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-muted-foreground">{t("title")}</span>
          <Badge variant="secondary" className="text-[10px]">{processes.length}</Badge>
        </div>
        <Button variant="ghost" size="icon-sm" onClick={loadProcesses} title={t("refresh")}>
          <RefreshCw className="h-3.5 w-3.5" />
        </Button>
      </div>

      {error && (
        <div className="border-b border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="flex-1 overflow-y-auto">
        {loading && processes.length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t("loading")}</p>
        ) : processes.length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t("empty")}</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">PID</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("command")}</th>
                <th className="px-3 py-2 text-right font-medium text-muted-foreground">CPU</th>
                <th className="px-3 py-2 text-right font-medium text-muted-foreground">{t("memory")}</th>
                <th className="px-3 py-2 text-right font-medium text-muted-foreground" />
              </tr>
            </thead>
            <tbody>
              {processes.map((proc) => (
                <tr key={proc.pid} className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
                  <td className="px-3 py-2 font-mono text-xs">{proc.pid}</td>
                  <td className="px-3 py-2 font-mono text-xs max-w-xs truncate">{proc.command}</td>
                  <td className="px-3 py-2 text-right text-xs text-muted-foreground">{proc.cpu || "—"}</td>
                  <td className="px-3 py-2 text-right text-xs text-muted-foreground">{proc.memory || "—"}</td>
                  <td className="px-3 py-2 text-right">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleKill(proc.pid)}
                      title={t("kill")}
                      disabled={proc.pid <= 2}
                      className="text-destructive hover:text-destructive"
                    >
                      <XCircle className="h-3.5 w-3.5" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

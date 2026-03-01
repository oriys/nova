"use client"

import { useMemo, useState } from "react"
import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Loader2 } from "lucide-react"
import { functionsApi, type FunctionVersionEntry, type FunctionVersionDiffResponse } from "@/lib/api"

interface FunctionVersionsProps {
  functionName: string
  versions: FunctionVersionEntry[]
  currentVersion?: number
  onVersionActivated?: () => void
}

type VersionNotice = {
  kind: "success" | "error"
  text: string
}

function formatDiffValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "-"
  }
  if (typeof value === "string") {
    return value
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export function FunctionVersions({
  functionName,
  versions,
  currentVersion,
  onVersionActivated,
}: FunctionVersionsProps) {
  const t = useTranslations("functionDetailPage")
  const tc = useTranslations("common")
  const [activatingVersion, setActivatingVersion] = useState<number | null>(null)
  const [diffingVersion, setDiffingVersion] = useState<number | null>(null)
  const [notice, setNotice] = useState<VersionNotice | null>(null)
  const [diffResult, setDiffResult] = useState<FunctionVersionDiffResponse | null>(null)

  const sortedVersions = useMemo(
    () => [...versions].sort((a, b) => b.version - a.version),
    [versions]
  )

  const handleActivate = async (version: number) => {
    try {
      setNotice(null)
      setActivatingVersion(version)
      await functionsApi.activateVersion(functionName, version)
      setNotice({ kind: "success", text: t("versions.activateSuccess", { version }) })
      onVersionActivated?.()
    } catch (err) {
      setNotice({
        kind: "error",
        text: err instanceof Error ? err.message : t("versions.activateFailed"),
      })
    } finally {
      setActivatingVersion(null)
    }
  }

  const handleDiffWithCurrent = async (version: number) => {
    if (!currentVersion || currentVersion === version) {
      return
    }
    try {
      setNotice(null)
      setDiffingVersion(version)
      const result = await functionsApi.compareVersions(functionName, currentVersion, version)
      setDiffResult(result)
    } catch (err) {
      setNotice({
        kind: "error",
        text: err instanceof Error ? err.message : t("versions.diffFailed"),
      })
    } finally {
      setDiffingVersion(null)
    }
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium text-muted-foreground mb-3">{t("tabs.versions")}</h3>
      {notice && (
        <div
          className={`rounded-lg border px-3 py-2 text-sm ${
            notice.kind === "success"
              ? "border-success/50 bg-success/10 text-success"
              : "border-destructive/50 bg-destructive/10 text-destructive"
          }`}
        >
          {notice.text}
        </div>
      )}
      {diffResult && (
        <div className="rounded-lg border border-border bg-muted/30 px-4 py-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-sm font-medium">
              {t("versions.diffTitle", { from: diffResult.v1, to: diffResult.v2 })}
            </p>
            <Button variant="ghost" size="sm" onClick={() => setDiffResult(null)}>
              {tc("dismiss")}
            </Button>
          </div>
          {diffResult.changes.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("versions.diffEmpty")}</p>
          ) : (
            <div className="space-y-2">
              {diffResult.changes.map((change, idx) => (
                <div key={`${change.field}-${idx}`} className="rounded border border-border bg-card px-3 py-2">
                  <p className="text-xs font-medium text-muted-foreground">{change.field}</p>
                  <p className="text-xs font-mono text-muted-foreground mt-1">
                    {formatDiffValue(change.from)} {"->"} {formatDiffValue(change.to)}
                  </p>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border">
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colVersion")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colCodeHash")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colHandler")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colMemory")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colTimeout")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colMode")}</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("versions.colCreated")}</th>
              <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{t("versions.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {versions.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-muted-foreground">
                  {t("versions.empty")}
                </td>
              </tr>
            ) : (
              sortedVersions.map((v) => {
                const isCurrent = typeof currentVersion === "number" && currentVersion === v.version
                return (
                <tr key={v.version} className="border-b border-border hover:bg-muted/50">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <Badge variant={isCurrent ? "default" : "secondary"} className="text-xs">
                        {t("versions.versionBadge", { version: v.version })}
                      </Badge>
                      {isCurrent && <Badge variant="outline" className="text-[10px]">{t("versions.current")}</Badge>}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <code className="text-xs text-muted-foreground bg-muted px-2 py-1 rounded">
                      {v.code_hash ? v.code_hash.slice(0, 12) + "..." : "-"}
                    </code>
                  </td>
                  <td className="px-4 py-3 text-sm">{v.handler || "-"}</td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">{v.memory_mb} MB</td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">
                    {t("versions.timeoutSeconds", { seconds: v.timeout_s })}
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant="secondary" className="text-xs">{v.mode || t("modeProcess")}</Badge>
                  </td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">
                    {new Date(v.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleDiffWithCurrent(v.version)}
                        disabled={isCurrent || !!diffingVersion}
                      >
                        {diffingVersion === v.version ? (
                          <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                        ) : null}
                        {t("versions.diffWithCurrent")}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleActivate(v.version)}
                        disabled={isCurrent || !!activatingVersion}
                      >
                        {activatingVersion === v.version ? (
                          <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
                        ) : null}
                        {t("versions.activate")}
                      </Button>
                    </div>
                  </td>
                </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

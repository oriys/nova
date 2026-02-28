"use client"

import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import type { FunctionVersionEntry } from "@/lib/api"

interface FunctionVersionsProps {
  versions: FunctionVersionEntry[]
}

export function FunctionVersions({ versions }: FunctionVersionsProps) {
  const t = useTranslations("functionDetailPage")

  return (
    <div>
      <h3 className="text-sm font-medium text-muted-foreground mb-3">{t("tabs.versions")}</h3>
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
            </tr>
          </thead>
          <tbody>
            {versions.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground">
                  {t("versions.empty")}
                </td>
              </tr>
            ) : (
              versions.map((v) => (
                <tr key={v.version} className="border-b border-border hover:bg-muted/50">
                  <td className="px-4 py-3">
                    <Badge variant="secondary" className="text-xs">v{v.version}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <code className="text-xs text-muted-foreground bg-muted px-2 py-1 rounded">
                      {v.code_hash ? v.code_hash.slice(0, 12) + "..." : "-"}
                    </code>
                  </td>
                  <td className="px-4 py-3 text-sm">{v.handler || "-"}</td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">{v.memory_mb} MB</td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">{v.timeout_s}s</td>
                  <td className="px-4 py-3">
                    <Badge variant="secondary" className="text-xs">{v.mode || t("modeProcess")}</Badge>
                  </td>
                  <td className="px-4 py-3 text-sm text-muted-foreground">
                    {new Date(v.created_at).toLocaleString()}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

"use client"

import Link from "next/link"
import { useTranslations } from "next-intl"
import { cn } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { FunctionData } from "@/lib/types"
import {
  Eye,
  Edit,
  Trash2,
  Loader2,
} from "lucide-react"

interface FunctionsTableProps {
  functions: FunctionData[]
  onDelete: (name: string) => void
  loading?: boolean
  refreshing?: boolean
  selectedNames?: Set<string>
  onToggleSelect?: (name: string, checked: boolean) => void
  onToggleSelectAll?: (checked: boolean, names: string[]) => void
}

export function FunctionsTable({
  functions,
  onDelete,
  loading,
  refreshing,
  selectedNames,
  onToggleSelect,
  onToggleSelectAll,
}: FunctionsTableProps) {
  const t = useTranslations("functionsTable")
  const tc = useTranslations("common")
  const allSelected = functions.length > 0 && functions.every((fn) => selectedNames?.has(fn.name))

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  const formatBandwidth = (bytesPerSecond?: number) => {
    if (!bytesPerSecond || bytesPerSecond <= 0) {
      return t("unlimited")
    }
    const mbps = bytesPerSecond / (1024 * 1024)
    return `${mbps >= 100 ? mbps.toFixed(0) : mbps.toFixed(1)} MB/s`
  }

  const formatIO = (fn: FunctionData) => {
    const iops = fn.limits?.disk_iops ?? 0
    const bandwidth = fn.limits?.disk_bandwidth ?? 0
    if (iops <= 0 && bandwidth <= 0) {
      return { primary: t("unlimited"), secondary: "" }
    }
    return {
      primary: iops > 0 ? `${iops.toLocaleString()} IOPS` : `${t("unlimited")} IOPS`,
      secondary: `BW ${formatBandwidth(bandwidth)}`,
    }
  }

  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <Loader2 className="h-6 w-6 animate-spin mx-auto text-muted-foreground" />
        <p className="text-sm text-muted-foreground mt-2">{t("loadingFunctions")}</p>
      </div>
    )
  }

  if (functions.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <p className="text-sm text-muted-foreground">
          {t("noFunctions")}
        </p>
      </div>
    )
  }

  return (
    <div className="relative rounded-xl border border-border bg-card overflow-hidden">
      {refreshing && (
        <div className="pointer-events-none absolute right-3 top-3 z-10 rounded-md bg-card/90 p-1.5 shadow-sm">
          <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border bg-muted/30">
              <th className="px-4 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={(event) => onToggleSelectAll?.(event.target.checked, functions.map((fn) => fn.name))}
                  className="h-4 w-4 rounded border-border"
                />
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("function")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("runtime")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("status")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("vcpu")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("memory")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("io")}
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("invocations")}
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("errors")}
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("lastModified")}
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {t("actions")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {functions.map((fn) => {
              const ioInfo = formatIO(fn)
              const checked = Boolean(selectedNames?.has(fn.name))
              return (
              <tr key={fn.id} className="hover:bg-muted/20 transition-colors">
                <td className="px-4 py-4">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(event) => onToggleSelect?.(fn.name, event.target.checked)}
                    className="h-4 w-4 rounded border-border"
                  />
                </td>
                <td className="px-6 py-4">
                  <Link
                    href={`/functions/${fn.name}`}
                    className="group"
                  >
                    <p className="text-sm font-medium text-foreground group-hover:text-primary transition-colors">
                      {fn.name}
                    </p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {fn.region}
                    </p>
                  </Link>
                </td>
                <td className="px-6 py-4">
                  <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
                    <span className="h-2 w-2 rounded-full bg-primary" />
                    {fn.runtime}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-xs font-medium",
                      fn.status === "active" && "bg-success/10 text-success border-0",
                      fn.status === "error" && "bg-destructive/10 text-destructive border-0",
                      fn.status === "inactive" && "bg-muted text-muted-foreground border-0"
                    )}
                  >
                    {fn.status}
                  </Badge>
                </td>
                <td className="px-6 py-4">
                  <span className="text-sm text-muted-foreground">
                    {fn.limits?.vcpus && fn.limits.vcpus > 0 ? fn.limits.vcpus : 1}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <span className="text-sm text-muted-foreground">
                    {fn.memory} MB
                  </span>
                </td>
                <td className="px-6 py-4">
                  <div className="text-sm text-muted-foreground">
                    <div>{ioInfo.primary}</div>
                    {ioInfo.secondary && (
                      <div className="text-xs text-muted-foreground/80 mt-0.5">
                        {ioInfo.secondary}
                      </div>
                    )}
                  </div>
                </td>
                <td className="px-6 py-4 text-right">
                  <span className="text-sm font-medium text-foreground">
                    {fn.invocations.toLocaleString()}
                  </span>
                </td>
                <td className="px-6 py-4 text-right">
                  <span className={cn(
                    "text-sm font-medium",
                    fn.errors > 0 ? "text-destructive" : "text-muted-foreground"
                  )}>
                    {fn.errors}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <span className="text-sm text-muted-foreground">
                    {formatDate(fn.lastModified)}
                  </span>
                </td>
                <td className="px-6 py-4 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <Button variant="ghost" size="icon" className="h-8 w-8" asChild title={t("view")}>
                      <Link href={`/functions/${fn.name}`} aria-label={`${t("view")} ${fn.name}`}>
                        <Eye className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Button variant="ghost" size="icon" className="h-8 w-8" asChild title={t("edit")}>
                      <Link href={`/functions/${fn.name}?tab=config`} aria-label={`${t("edit")} ${fn.name}`}>
                        <Edit className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => onDelete(fn.name)}
                      title={tc("delete")}
                      aria-label={`${tc("delete")} ${fn.name}`}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </td>
              </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

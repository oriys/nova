"use client"

import Link from "next/link"
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
  selectedNames?: Set<string>
  onToggleSelect?: (name: string, checked: boolean) => void
  onToggleSelectAll?: (checked: boolean, names: string[]) => void
}

export function FunctionsTable({
  functions,
  onDelete,
  loading,
  selectedNames,
  onToggleSelect,
  onToggleSelectAll,
}: FunctionsTableProps) {
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
      return "Unlimited"
    }
    const mbps = bytesPerSecond / (1024 * 1024)
    return `${mbps >= 100 ? mbps.toFixed(0) : mbps.toFixed(1)} MB/s`
  }

  const formatIO = (fn: FunctionData) => {
    const iops = fn.limits?.disk_iops ?? 0
    const bandwidth = fn.limits?.disk_bandwidth ?? 0
    if (iops <= 0 && bandwidth <= 0) {
      return { primary: "Unlimited", secondary: "" }
    }
    return {
      primary: iops > 0 ? `${iops.toLocaleString()} IOPS` : "Unlimited IOPS",
      secondary: `BW ${formatBandwidth(bandwidth)}`,
    }
  }

  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <Loader2 className="h-6 w-6 animate-spin mx-auto text-muted-foreground" />
        <p className="text-sm text-muted-foreground mt-2">Loading functions...</p>
      </div>
    )
  }

  if (functions.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <p className="text-sm text-muted-foreground">
          No functions found. Create your first function to get started.
        </p>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
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
                Function
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Runtime
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                vCPU
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Memory
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                IO
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Invocations
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Errors
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Last Modified
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Actions
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
                    <Button variant="ghost" size="icon" className="h-8 w-8" asChild title="View">
                      <Link href={`/functions/${fn.name}`} aria-label={`View ${fn.name}`}>
                        <Eye className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Button variant="ghost" size="icon" className="h-8 w-8" asChild title="Edit">
                      <Link href={`/functions/${fn.name}?tab=config`} aria-label={`Edit ${fn.name}`}>
                        <Edit className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => onDelete(fn.name)}
                      title="Delete"
                      aria-label={`Delete ${fn.name}`}
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

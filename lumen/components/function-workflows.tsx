"use client"

import { useCallback, useEffect, useState } from "react"
import Link from "next/link"
import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import { Pagination } from "@/components/pagination"
import { SectionEmptyHint } from "@/components/section-empty-hint"
import { SectionHeader } from "@/components/section-header"
import { SectionTableFrame } from "@/components/section-table-frame"
import { cn } from "@/lib/utils"
import { functionsApi, type Workflow } from "@/lib/api"

interface FunctionWorkflowsProps {
  functionName: string
}

export function FunctionWorkflows({ functionName }: FunctionWorkflowsProps) {
  const t = useTranslations("functionDetailPage")
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [loading, setLoading] = useState(true)

  const fetchData = useCallback(async () => {
    try {
      const offset = (page - 1) * pageSize
      const result = await functionsApi.listFunctionWorkflows(functionName, pageSize, offset)
      setWorkflows(result.items || [])
      setTotal(result.total)
    } catch {
      setWorkflows([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [functionName, page, pageSize])

  useEffect(() => { fetchData() }, [fetchData])

  if (loading) return null
  if (total === 0 && workflows.length === 0) {
    return (
      <div className="space-y-4">
        <SectionHeader title={t("workflows_section.title")} />
        <SectionEmptyHint>{t("workflows_section.empty")}</SectionEmptyHint>
      </div>
    )
  }

  const statusColor = (status: string) => {
    switch (status) {
      case "active":
        return "bg-success/10 text-success border-0"
      case "paused":
        return "bg-warning/10 text-warning border-0"
      default:
        return "bg-muted text-muted-foreground border-0"
    }
  }

  return (
    <div className="space-y-4">
      <SectionHeader title={t("workflows_section.title")} />
      <SectionTableFrame>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("workflows_section.colName")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("workflows_section.colDescription")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("workflows_section.colStatus")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("workflows_section.colVersion")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("workflows_section.colUpdated")}</th>
            </tr>
          </thead>
          <tbody>
            {workflows.map((wf) => (
              <tr key={wf.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="px-4 py-2.5">
                  <Link href={`/workflows/${encodeURIComponent(wf.name)}`} className="text-sm font-medium hover:underline">
                    {wf.name}
                  </Link>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground max-w-[200px] truncate">
                  {wf.description || "—"}
                </td>
                <td className="px-4 py-2.5">
                  <Badge variant="secondary" className={cn("text-[10px]", statusColor(wf.status))}>
                    {wf.status}
                  </Badge>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground font-mono">
                  {t("versions.versionBadge", { version: wf.current_version })}
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground">
                  {new Date(wf.updated_at).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {total > pageSize && (
          <div className="border-t border-border p-4">
            <Pagination
              totalItems={total}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(size) => { setPageSize(size); setPage(1) }}
              itemLabel="workflows"
            />
          </div>
        )}
      </SectionTableFrame>
    </div>
  )
}

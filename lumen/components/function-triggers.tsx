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
import { functionsApi, type TriggerEntry } from "@/lib/api"

interface FunctionTriggersProps {
  functionName: string
}

export function FunctionTriggers({ functionName }: FunctionTriggersProps) {
  const t = useTranslations("functionDetailPage")
  const [triggers, setTriggers] = useState<TriggerEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [loading, setLoading] = useState(true)

  const fetchData = useCallback(async () => {
    try {
      const offset = (page - 1) * pageSize
      const result = await functionsApi.listFunctionTriggers(functionName, pageSize, offset)
      setTriggers(result.items || [])
      setTotal(result.total)
    } catch {
      setTriggers([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [functionName, page, pageSize])

  useEffect(() => { fetchData() }, [fetchData])

  if (loading) return null
  if (total === 0 && triggers.length === 0) {
    return (
      <div className="space-y-4">
        <SectionHeader title={t("triggers_section.title")} />
        <SectionEmptyHint>{t("triggers_section.empty")}</SectionEmptyHint>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <SectionHeader title={t("triggers_section.title")} />
      <SectionTableFrame>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("triggers_section.colName")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("triggers_section.colType")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("triggers_section.colEnabled")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("triggers_section.colCreated")}</th>
            </tr>
          </thead>
          <tbody>
            {triggers.map((trigger) => (
              <tr key={trigger.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="px-4 py-2.5">
                  <Link href="/triggers" className="text-sm font-medium hover:underline">
                    {trigger.name}
                  </Link>
                </td>
                <td className="px-4 py-2.5">
                  <Badge variant="outline" className="text-[10px] font-mono">
                    {trigger.type}
                  </Badge>
                </td>
                <td className="px-4 py-2.5">
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px]",
                      trigger.enabled
                        ? "bg-success/10 text-success border-0"
                        : "bg-muted text-muted-foreground border-0"
                    )}
                  >
                    {trigger.enabled ? t("schedules.statusActive") : t("schedules.statusDisabled")}
                  </Badge>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground">
                  {new Date(trigger.created_at).toLocaleString()}
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
              itemLabel="triggers"
            />
          </div>
        )}
      </SectionTableFrame>
    </div>
  )
}

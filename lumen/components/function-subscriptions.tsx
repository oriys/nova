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
import { functionsApi, type EventSubscription } from "@/lib/api"

interface FunctionSubscriptionsProps {
  functionName: string
}

export function FunctionSubscriptions({ functionName }: FunctionSubscriptionsProps) {
  const t = useTranslations("functionDetailPage")
  const [subs, setSubs] = useState<EventSubscription[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [loading, setLoading] = useState(true)

  const fetchData = useCallback(async () => {
    try {
      const offset = (page - 1) * pageSize
      const result = await functionsApi.listFunctionSubscriptions(functionName, pageSize, offset)
      setSubs(result.items || [])
      setTotal(result.total)
    } catch {
      setSubs([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [functionName, page, pageSize])

  useEffect(() => { fetchData() }, [fetchData])

  if (loading) return null
  if (total === 0 && subs.length === 0) {
    return (
      <div className="space-y-4">
        <SectionHeader title={t("subscriptions.title")} />
        <SectionEmptyHint>{t("subscriptions.empty")}</SectionEmptyHint>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <SectionHeader title={t("subscriptions.title")} />
      <SectionTableFrame>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colName")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colTopic")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colGroup")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colEnabled")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colLag")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("subscriptions.colCreated")}</th>
            </tr>
          </thead>
          <tbody>
            {subs.map((sub) => (
              <tr key={sub.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="px-4 py-2.5">
                  <Link href="/events" className="text-sm font-medium hover:underline">
                    {sub.name}
                  </Link>
                </td>
                <td className="px-4 py-2.5">
                  <Badge variant="outline" className="text-[10px] font-mono">
                    {sub.topic_name}
                  </Badge>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground font-mono">
                  {sub.consumer_group}
                </td>
                <td className="px-4 py-2.5">
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px]",
                      sub.enabled
                        ? "bg-success/10 text-success border-0"
                        : "bg-muted text-muted-foreground border-0"
                    )}
                  >
                    {sub.enabled ? t("schedules.statusActive") : t("schedules.statusDisabled")}
                  </Badge>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground font-mono">
                  {sub.lag ?? 0}
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground">
                  {new Date(sub.created_at).toLocaleString()}
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
              itemLabel="subscriptions"
            />
          </div>
        )}
      </SectionTableFrame>
    </div>
  )
}

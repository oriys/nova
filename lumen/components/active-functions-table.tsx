"use client"

import Link from "next/link"
import { useTranslations } from "next-intl"
import { cn } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { FunctionData } from "@/lib/types"
import { ArrowRight, Loader2 } from "lucide-react"

interface ActiveFunctionsTableProps {
  functions: FunctionData[]
  loading?: boolean
}

export function ActiveFunctionsTable({ functions, loading }: ActiveFunctionsTableProps) {
  const t = useTranslations("activeFunctionsTable")

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
      {functions.length === 0 ? (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">{t("noFunctions")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-6 py-3 text-left text-xs font-medium text-muted-foreground">
                  {t("name")}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-muted-foreground">
                  {t("runtime")}
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-muted-foreground">
                  {t("status")}
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-muted-foreground">
                  {t("invocations")}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {functions.map((fn) => (
                <tr key={fn.id} className="hover:bg-muted/20 transition-colors">
                  <td className="px-6 py-4">
                    <Link
                      href={`/functions/${fn.name}`}
                      className="text-sm font-medium text-foreground hover:text-primary"
                    >
                      {fn.name}
                    </Link>
                  </td>
                  <td className="px-6 py-4">
                    <span className="text-sm text-muted-foreground">
                      {fn.runtime}
                    </span>
                  </td>
                  <td className="px-6 py-4">
                    <Badge
                      variant="secondary"
                      className={cn(
                        "text-xs",
                        fn.status === "active" && "bg-success/10 text-success border-0",
                        fn.status === "error" && "bg-destructive/10 text-destructive border-0",
                        fn.status === "inactive" && "bg-muted text-muted-foreground border-0"
                      )}
                    >
                      {fn.status}
                    </Badge>
                  </td>
                  <td className="px-6 py-4 text-right">
                    <span className="text-sm font-medium text-foreground">
                      {fn.invocations.toLocaleString()}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

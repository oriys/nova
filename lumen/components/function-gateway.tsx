"use client"

import { useEffect, useState, useCallback } from "react"
import Link from "next/link"
import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { gatewayApi, type GatewayRoute } from "@/lib/api"
import { ExternalLink, Loader2, Globe } from "lucide-react"

interface FunctionGatewayProps {
  functionName: string
}

export function FunctionGateway({ functionName }: FunctionGatewayProps) {
  const t = useTranslations("functionDetailPage.gateway")
  const [routes, setRoutes] = useState<GatewayRoute[]>([])
  const [loading, setLoading] = useState(true)

  const fetchRoutes = useCallback(async () => {
    setLoading(true)
    try {
      const allRoutes = await gatewayApi.listRoutes()
      setRoutes(allRoutes.filter((r) => r.function_name === functionName))
    } catch {
      setRoutes([])
    } finally {
      setLoading(false)
    }
  }, [functionName])

  useEffect(() => {
    fetchRoutes()
  }, [fetchRoutes])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (routes.length === 0) {
    return (
      <div className="rounded-lg border border-border bg-card p-8 text-center">
        <Globe className="mx-auto h-10 w-10 text-muted-foreground/50 mb-3" />
        <p className="text-sm font-medium text-foreground mb-1">{t("empty")}</p>
        <p className="text-xs text-muted-foreground mb-4">{t("emptyHint")}</p>
        <Button variant="outline" size="sm" asChild>
          <Link href="/gateway">
            <ExternalLink className="mr-2 h-3.5 w-3.5" />
            {t("goToGateway")}
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">{t("title")}</h3>
          <p className="text-xs text-muted-foreground mt-0.5">{t("description")}</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t("routeCount", { count: routes.length })}
          </span>
          <Button variant="outline" size="sm" asChild>
            <Link href="/gateway">
              <ExternalLink className="mr-2 h-3.5 w-3.5" />
              {t("goToGateway")}
            </Link>
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("domain")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("path")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("methods")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("auth")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("rateLimit")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("status")}</th>
            </tr>
          </thead>
          <tbody>
            {routes.map((route) => (
              <tr key={route.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="px-4 py-2.5 font-mono text-xs">{route.domain || "*"}</td>
                <td className="px-4 py-2.5 font-mono text-xs">{route.path}</td>
                <td className="px-4 py-2.5">
                  {route.methods && route.methods.length > 0 ? (
                    <div className="flex gap-1 flex-wrap">
                      {route.methods.map((m) => (
                        <Badge key={m} variant="outline" className="text-[10px] px-1.5 py-0">
                          {m}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-xs text-muted-foreground">{t("allMethods")}</span>
                  )}
                </td>
                <td className="px-4 py-2.5">
                  <Badge variant="secondary" className="text-[10px]">
                    {route.auth_strategy || "none"}
                  </Badge>
                </td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground">
                  {route.rate_limit
                    ? t("rps", { rps: route.rate_limit.requests_per_second })
                    : t("noLimit")}
                </td>
                <td className="px-4 py-2.5">
                  <Badge
                    variant="secondary"
                    className={
                      route.enabled
                        ? "bg-success/10 text-success border-0 text-[10px]"
                        : "bg-muted text-muted-foreground border-0 text-[10px]"
                    }
                  >
                    {route.enabled ? t("enabled") : t("disabled")}
                  </Badge>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

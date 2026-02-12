"use client"

import Link from "next/link"
import { useEffect, useMemo, useState } from "react"
import { useParams } from "next/navigation"
import { useTranslations } from "next-intl"
import { ArrowLeft, RefreshCw } from "lucide-react"

import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { TenantGovernancePanel } from "@/components/tenant-governance-panel"
import { Button } from "@/components/ui/button"
import { NamespaceEntry, TenantEntry, tenantsApi } from "@/lib/api"
import { DEFAULT_NAMESPACE, setTenantScope } from "@/lib/tenant-scope"

function formatDate(value?: string): string {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export default function TenantDetailPage() {
  const t = useTranslations("tenantDetailPage")
  const params = useParams<{ tenantId: string }>()
  const rawTenantId = Array.isArray(params?.tenantId) ? params.tenantId[0] : params?.tenantId
  const tenantId = useMemo(() => {
    if (!rawTenantId) return ""
    try {
      return decodeURIComponent(rawTenantId)
    } catch {
      return rawTenantId
    }
  }, [rawTenantId])

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [tenant, setTenant] = useState<TenantEntry | null>(null)
  const [namespaces, setNamespaces] = useState<NamespaceEntry[]>([])

  useEffect(() => {
    if (!tenantId) {
      setTenant(null)
      setNamespaces([])
      setLoading(false)
      setError(t("tenantIdRequired"))
      return
    }

    let active = true

    const load = async () => {
      setLoading(true)
      setError("")
      try {
        const tenantList = await tenantsApi.list()

        if (!active) return

        const selected = tenantList.find((item) => item.id === tenantId) ?? null
        if (!selected) {
          setTenant(null)
          setNamespaces([])
          setError(t("notFound", { tenantId }))
          return
        }

        const namespaceList = await tenantsApi.listNamespaces(tenantId)
        if (!active) return

        setTenant(selected)
        setNamespaces(namespaceList)
      } catch (err) {
        if (!active) return
        setTenant(null)
        setNamespaces([])
        setError(err instanceof Error ? err.message : t("loadFailed"))
      } finally {
        if (active) {
          setLoading(false)
        }
      }
    }

    void load()

    return () => {
      active = false
    }
  }, [tenantId])

  const applyScope = () => {
    const fallbackNamespace =
      namespaces.find((ns) => ns.name === DEFAULT_NAMESPACE)?.name ??
      namespaces[0]?.name ??
      DEFAULT_NAMESPACE

    setTenantScope({ tenantId, namespace: fallbackNamespace })
    if (typeof window !== "undefined") {
      window.location.reload()
    }
  }

  return (
    <DashboardLayout>
      <Header
        title={tenantId ? t("titleWithTenant", { tenantId }) : t("title")}
        description={t("description")}
      />

      <div className="space-y-6 p-6">
        <div className="flex items-center gap-3">
          <Button asChild variant="outline" size="sm">
            <Link href="/tenancy">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              {t("backToTenancy")}
            </Link>
          </Button>
          <Button variant="ghost" size="sm" onClick={applyScope} disabled={!tenantId || loading || !!error}>
            <RefreshCw className="mr-1.5 h-4 w-4" />
            {t("useThisTenant")}
          </Button>
        </div>

        {loading && (
          <div className="rounded-xl border border-border bg-card p-6 text-sm text-muted-foreground">
            {t("loading")}
          </div>
        )}

        {!loading && error && (
          <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
            {error}
          </div>
        )}

        {!loading && !error && tenant && (
          <>
            <div className="rounded-xl border border-border bg-card p-6">
              <h3 className="text-lg font-semibold text-card-foreground">{t("detailCardTitle")}</h3>
              <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.id")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{tenant.id}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.name")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{tenant.name || tenant.id}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.status")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{tenant.status}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.tier")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{tenant.tier}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.created")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{formatDate(tenant.created_at)}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.updated")}</div>
                  <div className="mt-1 text-sm font-medium text-foreground">{formatDate(tenant.updated_at)}</div>
                </div>
              </div>

              <div className="mt-5 rounded-lg border border-border bg-muted/20 p-3">
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{t("labels.namespaces")}</div>
                <div className="mt-2 flex flex-wrap gap-2">
                  {namespaces.length === 0 ? (
                    <span className="text-xs text-muted-foreground">{t("noNamespaces")}</span>
                  ) : (
                    namespaces.map((ns) => (
                      <span
                        key={ns.id}
                        className="rounded-md bg-foreground/5 px-2 py-1 text-xs font-medium text-foreground"
                      >
                        {ns.name}
                      </span>
                    ))
                  )}
                </div>
              </div>
            </div>

            <TenantGovernancePanel tenantId={tenant.id} />
          </>
        )}
      </div>
    </DashboardLayout>
  )
}

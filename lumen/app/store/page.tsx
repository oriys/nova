"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { StoreFlowRoadmap, type FlowStep } from "@/components/store-flow-roadmap"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { storeApi, type AppStoreApp, type AppStoreInstallation } from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { Clock3, Package, PackageCheck, RefreshCw, Store as StoreIcon, Tag, User } from "lucide-react"

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString()
}

function statusBadgeVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  if (status === "succeeded") return "default"
  if (status === "failed") return "destructive"
  if (status === "pending" || status === "planning" || status === "applying" || status === "validating") {
    return "secondary"
  }
  return "outline"
}

export default function StorePage() {
  const t = useTranslations("pages")
  const [apps, setApps] = useState<AppStoreApp[]>([])
  const [installations, setInstallations] = useState<AppStoreInstallation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const appsByID = useMemo(() => {
    const lookup = new Map<string, AppStoreApp>()
    for (const app of apps) {
      lookup.set(app.id, app)
    }
    return lookup
  }, [apps])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [appsResp, installsResp] = await Promise.all([
        storeApi.listApps({ limit: 100 }),
        storeApi.listInstallations(100),
      ])
      setApps(appsResp.apps || [])
      setInstallations(installsResp.installations || [])
    } catch (err) {
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  const firstApp = apps[0] || null
  const hasInstallations = installations.length > 0

  const consumerSteps: FlowStep[] = [
    {
      id: "discover",
      title: "Discover app packages",
      description: "Browse catalog cards and pick a package that matches your scenario.",
      status: apps.length > 0 ? "done" : "current",
      action: {
        label: "Refresh Catalog",
        onClick: fetchData,
        disabled: loading,
      },
    },
    {
      id: "inspect",
      title: "Open app detail and inspect releases",
      description: "Check function metadata and workflow DAG before install.",
      status: firstApp ? "current" : "pending",
      action: firstApp
        ? {
            label: "Open Detail",
            href: `/store/${encodeURIComponent(firstApp.slug)}`,
          }
        : undefined,
    },
    {
      id: "install-use",
      title: "Install and run in current scope",
      description: "Plan/apply installation and invoke installed resources.",
      status: hasInstallations ? "done" : firstApp ? "pending" : "pending",
      action: hasInstallations
        ? {
            label: "Manage Installations",
            href: firstApp ? `/store/${encodeURIComponent(firstApp.slug)}#installations` : "/store",
          }
        : undefined,
    },
  ]

  useEffect(() => {
    fetchData()
  }, [fetchData])

  return (
    <DashboardLayout>
      <Header title={t("appStore.title")} description={t("appStore.description")} />

      <div className="p-6 space-y-6">
        {error ? <ErrorBanner error={error} title="Failed to Load App Store" onRetry={fetchData} /> : null}

        <div className="flex items-center justify-between">
          <div className="text-sm text-muted-foreground">
            {loading ? "Loading..." : `${apps.length} apps · ${installations.length} installations`}
          </div>
          <div className="flex items-center gap-2">
            <Button asChild variant="outline">
              <Link href="/my-apps">Manage My Apps</Link>
            </Button>
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Published Apps</p>
            <p className="mt-2 text-2xl font-semibold">{loading ? "..." : apps.length}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Installed in Current Scope</p>
            <p className="mt-2 text-2xl font-semibold">{loading ? "..." : installations.length}</p>
          </div>
        </div>

        <StoreFlowRoadmap
          title="Consumer Workflow"
          description="Use this path to browse, inspect, and install app bundles."
          steps={consumerSteps}
        />

        <section className="rounded-lg border border-border bg-card">
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            <StoreIcon className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Available Apps</h2>
          </div>

          {!loading && apps.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No Apps Yet"
                description="Publish your first bundle to make it available in App Store."
                icon={Package}
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4 sm:grid-cols-2 xl:grid-cols-3">
              {loading
                ? Array.from({ length: 6 }).map((_, index) => (
                    <div key={`app-skeleton-${index}`} className="rounded-lg border border-border/80 bg-card/70 p-4">
                      <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-1/3 animate-pulse rounded bg-muted" />
                      <div className="mt-4 h-3 w-full animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-4/5 animate-pulse rounded bg-muted" />
                      <div className="mt-4 h-3 w-1/2 animate-pulse rounded bg-muted" />
                    </div>
                  ))
                : apps.map((app) => (
                    <article key={app.id} className="rounded-lg border border-border/80 bg-card/70 p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <h3 className="truncate text-sm font-semibold text-foreground">{app.title}</h3>
                          <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{app.slug}</p>
                        </div>
                        <Badge variant={app.visibility === "public" ? "default" : "secondary"}>
                          {app.visibility}
                        </Badge>
                      </div>

                      <p className="mt-3 line-clamp-3 text-sm text-muted-foreground">
                        {app.summary || app.description || "No description provided."}
                      </p>

                      {app.tags && app.tags.length > 0 ? (
                        <div className="mt-3 flex flex-wrap gap-1.5">
                          {app.tags.slice(0, 4).map((tag) => (
                            <Badge key={`${app.id}-${tag}`} variant="outline" className="gap-1">
                              <Tag className="h-3 w-3" />
                              {tag}
                            </Badge>
                          ))}
                        </div>
                      ) : null}

                      <div className="mt-4 space-y-1.5 text-xs text-muted-foreground">
                        <div className="flex items-center gap-2">
                          <User className="h-3.5 w-3.5" />
                          <span className="truncate">{app.owner}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Clock3 className="h-3.5 w-3.5" />
                          <span>{formatDateTime(app.updated_at)}</span>
                        </div>
                      </div>

                      <div className="mt-4 flex items-center gap-2">
                        <Button asChild size="sm">
                          <Link href={`/store/${encodeURIComponent(app.slug)}`}>View Details</Link>
                        </Button>
                      </div>
                    </article>
                  ))}
            </div>
          )}
        </section>

        <section className="rounded-lg border border-border bg-card">
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            <Package className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Installations</h2>
          </div>

          {!loading && installations.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No Installations"
                description="Install an app bundle to create managed resources in this tenant/namespace."
                icon={PackageCheck}
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4 sm:grid-cols-2 xl:grid-cols-3">
              {loading
                ? Array.from({ length: 3 }).map((_, index) => (
                    <div key={`installation-skeleton-${index}`} className="rounded-lg border border-border/80 bg-card/70 p-4">
                      <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
                      <div className="mt-3 h-3 w-1/3 animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-4/5 animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-2/3 animate-pulse rounded bg-muted" />
                    </div>
                  ))
                : installations.map((inst) => (
                    <article key={inst.id} className="rounded-lg border border-border/80 bg-card/70 p-4">
                      <p className="truncate text-xs text-muted-foreground">
                        {appsByID.get(inst.app_id)?.title ?? inst.app_id}
                      </p>
                      <div className="flex items-start justify-between gap-3">
                        <h3 className="truncate text-sm font-semibold text-foreground">{inst.install_name}</h3>
                        <Badge variant={statusBadgeVariant(inst.status)}>{inst.status}</Badge>
                      </div>

                      <div className="mt-4 space-y-1.5 text-xs text-muted-foreground">
                        <div className="flex items-center gap-2">
                          <User className="h-3.5 w-3.5" />
                          <span>{inst.created_by || "-"}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Tag className="h-3.5 w-3.5" />
                          <span className="truncate">{inst.tenant_id}/{inst.namespace}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Clock3 className="h-3.5 w-3.5" />
                          <span>{formatDateTime(inst.updated_at)}</span>
                        </div>
                      </div>

                      {appsByID.get(inst.app_id)?.slug ? (
                        <div className="mt-4">
                          <Button asChild size="sm" variant="outline">
                            <Link href={`/store/${encodeURIComponent(appsByID.get(inst.app_id)!.slug)}#installations`}>
                              Manage Installation
                            </Link>
                          </Button>
                        </div>
                      ) : null}
                    </article>
                  ))}
            </div>
          )}
        </section>
      </div>
    </DashboardLayout>
  )
}

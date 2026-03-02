"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { systemApi, type DockerImageInfo, type RootfsImageInfo } from "@/lib/api"
import { RefreshCw, Container, HardDrive, Info } from "lucide-react"
import { RuntimeIcon, getRuntimeColor } from "@/components/runtime-logos"
import { cn } from "@/lib/utils"

function formatBytes(bytes: number): string {
  if (bytes < 1024) return bytes + " B"
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB"
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + " MB"
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB"
}

export default function ImagesPage() {
  const t = useTranslations("pages")
  const [dockerImages, setDockerImages] = useState<DockerImageInfo[]>([])
  const [rootfsImages, setRootfsImages] = useState<RootfsImageInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await systemApi.listImages()
      setDockerImages(result.docker_images || [])
      setRootfsImages(result.rootfs_images || [])
    } catch (err) {
      console.error("Failed to fetch images:", err)
      setError(err instanceof Error ? err.message : t("images.failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  if (error) {
    return (
      <DashboardLayout>
        <Header title={t("images.title")} description={t("images.description")} />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">{t("images.failedToLoad")}</p>
            <p className="text-sm mt-1">{error}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => { setError(null); fetchData() }}>
              {t("images.retry")}
            </Button>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("images.title")} description={t("images.description")} />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-end">
          <Button variant="outline" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {t("images.refresh")}
          </Button>
        </div>

        {/* Docker Images Section */}
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Container className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-lg font-semibold">{t("images.dockerImages")}</h2>
            {!loading && (
              <span className="text-sm text-muted-foreground">({dockerImages.length})</span>
            )}
          </div>

          {loading ? (
            <div className="grid gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="rounded-lg border border-border bg-card p-3 animate-pulse">
                  <div className="flex items-center gap-2">
                    <div className="h-8 w-8 rounded bg-muted" />
                    <div className="flex-1 space-y-1">
                      <div className="h-4 w-16 bg-muted rounded" />
                      <div className="h-3 w-12 bg-muted rounded" />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : dockerImages.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border bg-card/50 p-6 text-center">
              <Info className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
              <p className="text-sm text-muted-foreground">{t("images.noDockerImages")}</p>
              <p className="text-xs text-muted-foreground mt-1 font-mono">{t("images.dockerHint")}</p>
            </div>
          ) : (
            <div className="grid gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {dockerImages.map((img) => {
                const bgColor = getRuntimeColor(img.runtime)
                return (
                  <div
                    key={img.repository + ":" + img.tag}
                    className="rounded-lg border border-border bg-card p-3 transition-shadow hover:shadow-sm"
                  >
                    <div className="flex items-center gap-2">
                      <div className={cn(
                        "flex h-8 w-8 items-center justify-center rounded",
                        bgColor,
                        img.runtime === "bun" ? "text-black" : "text-white"
                      )}>
                        <RuntimeIcon runtimeId={img.runtime} className="text-base" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium truncate">{img.runtime}</p>
                        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          <span>{img.tag}</span>
                          <span>·</span>
                          <span>{img.size}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {/* Rootfs Images Section */}
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <HardDrive className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-lg font-semibold">{t("images.rootfsImages")}</h2>
            {!loading && (
              <span className="text-sm text-muted-foreground">({rootfsImages.length})</span>
            )}
          </div>

          {loading ? (
            <div className="grid gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="rounded-lg border border-border bg-card p-3 animate-pulse">
                  <div className="flex items-center gap-2">
                    <div className="h-8 w-8 rounded bg-muted" />
                    <div className="flex-1 space-y-1">
                      <div className="h-4 w-16 bg-muted rounded" />
                      <div className="h-3 w-12 bg-muted rounded" />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : rootfsImages.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border bg-card/50 p-6 text-center">
              <Info className="h-8 w-8 text-muted-foreground mx-auto mb-2" />
              <p className="text-sm text-muted-foreground">{t("images.noRootfsImages")}</p>
              <p className="text-xs text-muted-foreground mt-1 font-mono">{t("images.rootfsHint")}</p>
            </div>
          ) : (
            <div className="grid gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
              {rootfsImages.map((img) => {
                const bgColor = getRuntimeColor(img.runtime)
                return (
                  <div
                    key={img.filename}
                    className="rounded-lg border border-border bg-card p-3 transition-shadow hover:shadow-sm"
                  >
                    <div className="flex items-center gap-2">
                      <div className={cn(
                        "flex h-8 w-8 items-center justify-center rounded",
                        bgColor,
                        img.runtime === "bun" ? "text-black" : "text-white"
                      )}>
                        <RuntimeIcon runtimeId={img.runtime} className="text-base" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium truncate">{img.runtime}</p>
                        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          <span>{img.filename}</span>
                          <span>·</span>
                          <span>{formatBytes(img.size)}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

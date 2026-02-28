"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams, useRouter } from "next/navigation"
import Link from "next/link"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Pagination } from "@/components/pagination"
import { layersApi } from "@/lib/api"
import type { LayerEntry, LayerFunctionRef } from "@/lib/api"
import { ArrowLeft, Layers, File, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

export default function LayerDetailPage() {
  const params = useParams()
  const router = useRouter()
  const layerName = decodeURIComponent(params.name as string)
  const t = useTranslations("layersPage")
  const td = useTranslations("layersPage.detail")
  const tc = useTranslations("common")

  const [layer, setLayer] = useState<LayerEntry | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [functions, setFunctions] = useState<LayerFunctionRef[]>([])
  const [funcTotal, setFuncTotal] = useState(0)
  const [funcPage, setFuncPage] = useState(1)
  const [funcPageSize, setFuncPageSize] = useState(10)

  const fetchLayer = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await layersApi.get(layerName)
      setLayer(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : td("layerNotFound"))
    } finally {
      setLoading(false)
    }
  }, [layerName, td])

  const fetchFunctions = useCallback(async () => {
    try {
      const offset = (funcPage - 1) * funcPageSize
      const result = await layersApi.getLayerFunctions(layerName, funcPageSize, offset)
      setFunctions(result.items || [])
      setFuncTotal(result.total)
    } catch {
      setFunctions([])
      setFuncTotal(0)
    }
  }, [layerName, funcPage, funcPageSize])

  useEffect(() => { fetchLayer() }, [fetchLayer])
  useEffect(() => { fetchFunctions() }, [fetchFunctions])

  const handleDelete = async () => {
    if (!confirm(t("deleteConfirmDesc", { name: layerName }))) return
    try {
      await layersApi.delete(layerName)
      router.push("/layers")
    } catch (err) {
      setError(err instanceof Error ? err.message : t("failedToDelete"))
    }
  }

  if (loading) {
    return (
      <DashboardLayout>
        <div className="p-6">
          <div className="h-8 w-48 bg-muted rounded animate-pulse mb-4" />
          <div className="h-64 bg-muted rounded animate-pulse" />
        </div>
      </DashboardLayout>
    )
  }

  if (!layer) {
    return (
      <DashboardLayout>
        <div className="p-6">
          <Link href="/layers" className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground mb-4">
            <ArrowLeft className="h-4 w-4" />
            {td("backToLayers")}
          </Link>
          <div className="rounded-xl border border-border bg-card p-8 text-center">
            <Layers className="mx-auto h-10 w-10 mb-3 opacity-50" />
            <p className="text-muted-foreground">{td("layerNotFound")}</p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <Link href="/layers" className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground mb-2">
              <ArrowLeft className="h-4 w-4" />
              {td("backToLayers")}
            </Link>
            <div className="flex items-center gap-3">
              <Layers className="h-6 w-6 text-muted-foreground" />
              <h1 className="text-2xl font-bold font-mono">{layer.name}</h1>
              <Badge variant="outline">{layer.runtime}</Badge>
              <Badge variant="secondary">v{layer.version}</Badge>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={fetchLayer}>
              <RefreshCw className="mr-2 h-4 w-4" />
              {tc("refresh")}
            </Button>
            <Button variant="destructive" size="sm" onClick={handleDelete}>
              <Trash2 className="mr-2 h-4 w-4" />
              {tc("delete")}
            </Button>
          </div>
        </div>

        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        {/* Metadata */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-sm font-semibold mb-4">{td("metadata")}</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-xs text-muted-foreground mb-1">{t("colSize")}</p>
              <p className="text-sm font-medium">{layer.size_mb} MB</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground mb-1">{t("colFiles")}</p>
              <p className="text-sm font-medium">{t("filesCount", { count: layer.files?.length ?? 0 })}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground mb-1">{td("createdAt")}</p>
              <p className="text-sm font-medium">{new Date(layer.created_at).toLocaleString()}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground mb-1">{td("updatedAt")}</p>
              <p className="text-sm font-medium">{new Date(layer.updated_at).toLocaleString()}</p>
            </div>
            {layer.content_hash && (
              <div className="col-span-2 md:col-span-4">
                <p className="text-xs text-muted-foreground mb-1">{td("contentHash")}</p>
                <p className="text-sm font-mono text-muted-foreground break-all">{layer.content_hash}</p>
              </div>
            )}
          </div>
        </div>

        {/* Files */}
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="p-4 border-b border-border">
            <h2 className="text-sm font-semibold">{td("filesSection")}</h2>
          </div>
          {layer.files && layer.files.length > 0 ? (
            <div className="divide-y divide-border">
              {layer.files.map((file) => (
                <div key={file} className="px-4 py-2.5 flex items-center gap-2 hover:bg-muted/50">
                  <File className="h-4 w-4 text-muted-foreground shrink-0" />
                  <span className="text-sm font-mono">{file}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="p-6 text-center text-muted-foreground text-sm">
              {td("noFiles")}
            </div>
          )}
        </div>

        {/* Functions using this layer */}
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="p-4 border-b border-border">
            <h2 className="text-sm font-semibold">{td("functionsSection")}</h2>
          </div>
          {functions.length > 0 ? (
            <>
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{td("functionName")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{td("functionRuntime")}</th>
                  </tr>
                </thead>
                <tbody>
                  {functions.map((fn) => (
                    <tr key={fn.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3">
                        <Link href={`/functions/${encodeURIComponent(fn.name)}`} className="text-sm font-medium font-mono hover:underline">
                          {fn.name}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="outline" className="text-xs">{fn.runtime}</Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {funcTotal > funcPageSize && (
                <div className="border-t border-border p-4">
                  <Pagination
                    totalItems={funcTotal}
                    page={funcPage}
                    pageSize={funcPageSize}
                    onPageChange={setFuncPage}
                    onPageSizeChange={(size) => { setFuncPageSize(size); setFuncPage(1) }}
                    itemLabel="functions"
                  />
                </div>
              )}
            </>
          ) : (
            <div className="p-6 text-center text-muted-foreground text-sm">
              {td("noFunctions")}
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

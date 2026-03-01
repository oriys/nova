"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { useRouter } from "next/navigation"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { SandboxesTable } from "@/components/sandboxes-table"
import { CreateSandboxDialog } from "@/components/create-sandbox-dialog"
import { EmptyState } from "@/components/empty-state"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Button } from "@/components/ui/button"
import { sandboxApi, type CreateSandboxRequest } from "@/lib/api"
import { transformSandbox, type SandboxData } from "@/lib/types"
import { toUserErrorMessage } from "@/lib/error-map"
import { Plus, RefreshCw, Terminal } from "lucide-react"

type Notice = { kind: "success" | "error" | "info"; text: string }

export default function SandboxesPage() {
  const t = useTranslations("pages.sandboxes")
  const router = useRouter()
  const [sandboxes, setSandboxes] = useState<SandboxData[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | undefined>()
  const [notice, setNotice] = useState<Notice | undefined>()
  const [showCreate, setShowCreate] = useState(false)

  const loadSandboxes = useCallback(async () => {
    try {
      setLoading(true)
      const list = await sandboxApi.list()
      setSandboxes(list.map(transformSandbox))
      setError(undefined)
    } catch (e) {
      setError(toUserErrorMessage(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSandboxes()
    const timer = setInterval(loadSandboxes, 10000)
    return () => clearInterval(timer)
  }, [loadSandboxes])

  const handleCreate = async (data: CreateSandboxRequest) => {
    try {
      const sb = await sandboxApi.create(data)
      setNotice({ kind: "success", text: t("notices.created") })
      loadSandboxes()
      router.push(`/sandboxes/${sb.id}`)
    } catch (e) {
      setNotice({ kind: "error", text: toUserErrorMessage(e) })
      throw e
    }
  }

  const handleDestroy = async (id: string) => {
    try {
      await sandboxApi.destroy(id)
      setNotice({ kind: "success", text: t("notices.destroyed") })
      loadSandboxes()
    } catch (e) {
      setNotice({ kind: "error", text: toUserErrorMessage(e) })
    }
  }

  const handleKeepalive = async (id: string) => {
    try {
      await sandboxApi.keepalive(id)
      setNotice({ kind: "info", text: t("notices.keepalive") })
      loadSandboxes()
    } catch (e) {
      setNotice({ kind: "error", text: toUserErrorMessage(e) })
    }
  }

  const handleOpen = (id: string) => {
    router.push(`/sandboxes/${id}`)
  }

  return (
    <DashboardLayout>
      <Header title={t("title")} description={t("description")} />
      <div className="space-y-6 p-6">
        {error && <ErrorBanner error={error} />}

        {notice && (
          <div
            className={`rounded-lg border px-4 py-3 text-sm ${
              notice.kind === "error"
                ? "border-destructive/50 bg-destructive/10 text-destructive"
                : notice.kind === "success"
                  ? "border-green-500/50 bg-green-500/10 text-green-700 dark:text-green-400"
                  : "border-blue-500/50 bg-blue-500/10 text-blue-700 dark:text-blue-400"
            }`}
          >
            {notice.text}
            <button className="ml-2 font-medium underline" onClick={() => setNotice(undefined)}>
              ✕
            </button>
          </div>
        )}

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Button onClick={() => setShowCreate(true)}>
              <Plus className="mr-2 h-4 w-4" />
              {t("buttons.create")}
            </Button>
          </div>
          <Button variant="outline" size="sm" onClick={loadSandboxes} disabled={loading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("buttons.refresh")}
          </Button>
        </div>

        {!loading && sandboxes.length === 0 ? (
          <EmptyState
            title={t("empty")}
            description={t("emptyDescription")}
            icon={Terminal}
            primaryAction={{
              label: t("buttons.create"),
              onClick: () => setShowCreate(true),
            }}
          />
        ) : (
          <SandboxesTable
            data={sandboxes}
            loading={loading}
            onOpen={handleOpen}
            onDestroy={handleDestroy}
            onKeepalive={handleKeepalive}
          />
        )}
      </div>

      <CreateSandboxDialog
        open={showCreate}
        onOpenChange={setShowCreate}
        onCreate={handleCreate}
      />
    </DashboardLayout>
  )
}

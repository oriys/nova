"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams, useRouter } from "next/navigation"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { SandboxTerminal } from "@/components/sandbox-terminal"
import { SandboxFileBrowser } from "@/components/sandbox-file-browser"
import { SandboxProcesses } from "@/components/sandbox-processes"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { sandboxApi, type Sandbox } from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { ArrowLeft, RefreshCw, Trash2, Terminal, FolderOpen, Activity, Code2 } from "lucide-react"

const statusVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  running: "default",
  creating: "secondary",
  paused: "outline",
  stopped: "secondary",
  error: "destructive",
}

export default function SandboxDetailPage() {
  const t = useTranslations("pages.sandboxes.detail")
  const params = useParams()
  const router = useRouter()
  const sandboxId = params.id as string

  const [sandbox, setSandbox] = useState<Sandbox | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | undefined>()

  const loadSandbox = useCallback(async () => {
    try {
      const sb = await sandboxApi.get(sandboxId)
      setSandbox(sb)
      setError(undefined)
    } catch (e) {
      setError(toUserErrorMessage(e))
    } finally {
      setLoading(false)
    }
  }, [sandboxId])

  useEffect(() => {
    loadSandbox()
    const timer = setInterval(loadSandbox, 5000)
    return () => clearInterval(timer)
  }, [loadSandbox])

  // Auto-keepalive while the detail page is open
  useEffect(() => {
    if (!sandbox || sandbox.status !== "running") return
    const timer = setInterval(() => {
      sandboxApi.keepalive(sandboxId).catch(() => {})
    }, 30000)
    return () => clearInterval(timer)
  }, [sandboxId, sandbox?.status]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleDestroy = async () => {
    try {
      await sandboxApi.destroy(sandboxId)
      router.push("/sandboxes")
    } catch (e) {
      setError(toUserErrorMessage(e))
    }
  }

  if (loading) {
    return (
      <DashboardLayout>
        <Header title={t("loading")} />
        <div className="flex items-center justify-center p-12">
          <RefreshCw className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </DashboardLayout>
    )
  }

  if (error || !sandbox) {
    return (
      <DashboardLayout>
        <Header title={t("error")} />
        <div className="p-6">
          <ErrorBanner error={error || t("notFound")} />
          <Button variant="outline" className="mt-4" onClick={() => router.push("/sandboxes")}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            {t("backToList")}
          </Button>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header
        title={`${t("titlePrefix")} ${sandbox.id.slice(0, 12)}…`}
        description={`${sandbox.template} · ${sandbox.vcpus} vCPU · ${sandbox.memory_mb} MB`}
      />

      <div className="space-y-4 p-6">
        {/* Metadata bar */}
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="outline" size="sm" onClick={() => router.push("/sandboxes")}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            {t("backToList")}
          </Button>

          <Badge variant={statusVariant[sandbox.status] ?? "secondary"} className="text-xs">
            {sandbox.status}
          </Badge>

          <span className="text-xs text-muted-foreground">
            {t("network")}: {sandbox.network_policy}
          </span>

          <span className="text-xs text-muted-foreground">
            {t("expires")}: {new Date(sandbox.expires_at).toLocaleTimeString()}
          </span>

          <div className="ml-auto flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => sandboxApi.keepalive(sandboxId).then(loadSandbox)}
            >
              <RefreshCw className="mr-2 h-4 w-4" />
              {t("keepalive")}
            </Button>
            <Button variant="destructive" size="sm" onClick={handleDestroy}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("destroy")}
            </Button>
          </div>
        </div>

        {sandbox.error && <ErrorBanner error={sandbox.error} />}

        {/* Main content tabs */}
        <Tabs defaultValue="terminal" className="w-full">
          <TabsList>
            <TabsTrigger value="terminal" className="gap-2">
              <Terminal className="h-4 w-4" />
              {t("tabs.terminal")}
            </TabsTrigger>
            <TabsTrigger value="files" className="gap-2">
              <FolderOpen className="h-4 w-4" />
              {t("tabs.files")}
            </TabsTrigger>
            <TabsTrigger value="processes" className="gap-2">
              <Activity className="h-4 w-4" />
              {t("tabs.processes")}
            </TabsTrigger>
            <TabsTrigger value="code" className="gap-2">
              <Code2 className="h-4 w-4" />
              {t("tabs.code")}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="terminal" className="mt-4 h-[calc(100vh-320px)] min-h-[400px]">
            {sandbox.status === "running" ? (
              <SandboxTerminal sandboxId={sandboxId} />
            ) : (
              <div className="flex h-full items-center justify-center rounded-lg border border-border bg-card">
                <p className="text-sm text-muted-foreground">{t("notRunning")}</p>
              </div>
            )}
          </TabsContent>

          <TabsContent value="files" className="mt-4 h-[calc(100vh-320px)] min-h-[400px]">
            {sandbox.status === "running" ? (
              <SandboxFileBrowser sandboxId={sandboxId} />
            ) : (
              <div className="flex h-full items-center justify-center rounded-lg border border-border bg-card">
                <p className="text-sm text-muted-foreground">{t("notRunning")}</p>
              </div>
            )}
          </TabsContent>

          <TabsContent value="processes" className="mt-4 h-[calc(100vh-320px)] min-h-[400px]">
            {sandbox.status === "running" ? (
              <SandboxProcesses sandboxId={sandboxId} />
            ) : (
              <div className="flex h-full items-center justify-center rounded-lg border border-border bg-card">
                <p className="text-sm text-muted-foreground">{t("notRunning")}</p>
              </div>
            )}
          </TabsContent>

          <TabsContent value="code" className="mt-4">
            <CodeExecPanel sandboxId={sandboxId} running={sandbox.status === "running"} />
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  )
}

function CodeExecPanel({ sandboxId, running }: { sandboxId: string; running: boolean }) {
  const t = useTranslations("pages.sandboxes.codeExec")
  const [code, setCode] = useState("")
  const [language, setLanguage] = useState("python")
  const [output, setOutput] = useState<{ stdout: string; stderr: string; exit_code: number } | null>(null)
  const [executing, setExecuting] = useState(false)

  const handleExec = async () => {
    if (!code.trim()) return
    setExecuting(true)
    setOutput(null)
    try {
      const resp = await sandboxApi.execCode(sandboxId, { code, language })
      setOutput(resp)
    } catch (err) {
      setOutput({
        stdout: "",
        stderr: err instanceof Error ? err.message : String(err),
        exit_code: -1,
      })
    } finally {
      setExecuting(false)
    }
  }

  if (!running) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-card">
        <p className="text-sm text-muted-foreground">{t("notRunning")}</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <select
          value={language}
          onChange={(e) => setLanguage(e.target.value)}
          className="rounded-md border border-border bg-background px-3 py-2 text-sm"
        >
          <option value="python">Python</option>
          <option value="javascript">JavaScript</option>
          <option value="bash">Bash</option>
          <option value="ruby">Ruby</option>
          <option value="go">Go</option>
        </select>
        <Button onClick={handleExec} disabled={executing || !code.trim()}>
          <Code2 className="mr-2 h-4 w-4" />
          {executing ? t("executing") : t("run")}
        </Button>
      </div>

      <textarea
        value={code}
        onChange={(e) => setCode(e.target.value)}
        placeholder={t("placeholder")}
        className="h-48 w-full resize-none rounded-lg border border-border bg-black/95 p-3 font-mono text-sm text-foreground outline-none placeholder:text-muted-foreground"
        spellCheck={false}
      />

      {output && (
        <div className="rounded-lg border border-border bg-card">
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <span className="text-xs font-medium text-muted-foreground">{t("output")}</span>
            <Badge variant={output.exit_code === 0 ? "default" : "destructive"}>
              exit {output.exit_code}
            </Badge>
          </div>
          <div className="p-3 font-mono text-sm">
            {output.stdout && <pre className="whitespace-pre-wrap text-foreground">{output.stdout}</pre>}
            {output.stderr && <pre className="whitespace-pre-wrap text-red-400">{output.stderr}</pre>}
            {!output.stdout && !output.stderr && (
              <p className="text-muted-foreground">{t("noOutput")}</p>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

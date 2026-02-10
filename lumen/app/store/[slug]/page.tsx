"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { useCallback, useEffect, useMemo, useState } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { StoreFlowRoadmap, type FlowStep } from "@/components/store-flow-roadmap"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { DagViewer } from "@/components/workflow/dag-viewer"
import {
  functionsApi,
  storeApi,
  type AppStoreApp,
  type AppStoreInstallJob,
  type AppStoreInstallPlan,
  type AppStoreInstallation,
  type AppStoreInstallationResource,
  type AppStoreRelease,
  type WorkflowVersion,
} from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import {
  ArrowLeft,
  Package,
  PackageCheck,
  Play,
  RefreshCw,
  Store as StoreIcon,
  Wrench,
} from "lucide-react"

type ManifestFunction = {
  key: string
  name?: string
  runtime?: string
  handler?: string
  files?: string[]
  memory_mb?: number
  timeout_s?: number
  env_vars?: Record<string, string>
  description?: string
}

type ManifestWorkflowNode = {
  node_key: string
  function_ref: string
  input_mapping?: unknown
  retry_policy?: {
    max_attempts?: number
    base_ms?: number
    max_backoff_ms?: number
  }
  timeout_s?: number
}

type ManifestWorkflowEdge = {
  from: string
  to: string
}

type ManifestWorkflow = {
  name?: string
  description?: string
  definition?: {
    nodes?: ManifestWorkflowNode[]
    edges?: ManifestWorkflowEdge[]
  }
}

type ReleaseManifest = {
  name?: string
  version?: string
  type?: string
  description?: string
  functions?: ManifestFunction[]
  workflow?: ManifestWorkflow
}

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString()
}

function statusBadgeVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  if (status === "succeeded" || status === "published") return "default"
  if (status === "failed") return "destructive"
  if (status === "pending" || status === "planning" || status === "applying" || status === "validating") {
    return "secondary"
  }
  return "outline"
}

function isTerminalJobStatus(status: string): boolean {
  return status === "succeeded" || status === "failed"
}

function parseManifestStats(manifest: unknown): { functionCount: number; hasWorkflow: boolean } {
  if (!manifest || typeof manifest !== "object") {
    return { functionCount: 0, hasWorkflow: false }
  }
  const data = manifest as Record<string, unknown>
  return {
    functionCount: Array.isArray(data.functions) ? data.functions.length : 0,
    hasWorkflow: !!data.workflow,
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function parseReleaseManifest(manifest: unknown): ReleaseManifest {
  if (!isObject(manifest)) {
    return {}
  }
  const data = manifest as ReleaseManifest
  return data
}

function buildWorkflowPreviewVersion(manifest: ReleaseManifest): WorkflowVersion | null {
  const workflow = manifest.workflow
  if (!workflow?.definition) {
    return null
  }

  const nodesInput = Array.isArray(workflow.definition.nodes) ? workflow.definition.nodes : []
  const edgesInput = Array.isArray(workflow.definition.edges) ? workflow.definition.edges : []
  if (nodesInput.length === 0) {
    return null
  }

  const now = new Date().toISOString()
  return {
    id: "preview-version",
    workflow_id: "preview-workflow",
    version: 1,
    definition: workflow.definition,
    nodes: nodesInput.map((node, index) => ({
      id: node.node_key,
      version_id: "preview-version",
      node_key: node.node_key,
      function_name: node.function_ref,
      input_mapping: node.input_mapping,
      retry_policy: node.retry_policy
        ? {
            max_attempts: node.retry_policy.max_attempts ?? 1,
            base_ms: node.retry_policy.base_ms ?? 100,
            max_backoff_ms: node.retry_policy.max_backoff_ms ?? 1000,
          }
        : undefined,
      timeout_s: node.timeout_s ?? 0,
      position: index,
    })),
    edges: edgesInput.map((edge, index) => ({
      id: `preview-edge-${index}`,
      version_id: "preview-version",
      from_node_id: edge.from,
      to_node_id: edge.to,
    })),
    created_at: now,
  }
}

function parseValuesInput(valuesText: string): { ok: true; values: Record<string, unknown> } | { ok: false; error: string } {
  if (!valuesText.trim()) {
    return { ok: true, values: {} }
  }
  try {
    const parsed = JSON.parse(valuesText)
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ok: false, error: "Values must be a JSON object." }
    }
    return { ok: true, values: parsed as Record<string, unknown> }
  } catch {
    return { ok: false, error: "Values must be valid JSON." }
  }
}

export default function AppStoreDetailPage() {
  const params = useParams<{ slug: string | string[] }>()
  const rawSlug = params?.slug
  const slug = decodeURIComponent(Array.isArray(rawSlug) ? rawSlug[0] || "" : rawSlug || "")

  const [app, setApp] = useState<AppStoreApp | null>(null)
  const [releases, setReleases] = useState<AppStoreRelease[]>([])
  const [installations, setInstallations] = useState<AppStoreInstallation[]>([])
  const [installationResources, setInstallationResources] = useState<Record<string, AppStoreInstallationResource[]>>({})
  const [resourceLoading, setResourceLoading] = useState<Record<string, boolean>>({})
  const [resourceError, setResourceError] = useState<Record<string, string>>({})

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const [selectedVersion, setSelectedVersion] = useState("")
  const [installName, setInstallName] = useState("")
  const [namePrefix, setNamePrefix] = useState("")
  const [valuesText, setValuesText] = useState("{}")
  const [planning, setPlanning] = useState(false)
  const [planResult, setPlanResult] = useState<AppStoreInstallPlan | null>(null)
  const [installing, setInstalling] = useState(false)
  const [installJobID, setInstallJobID] = useState<string | null>(null)
  const [installJob, setInstallJob] = useState<AppStoreInstallJob | null>(null)
  const [uninstallingID, setUninstallingID] = useState<string | null>(null)

  const [invokeOpen, setInvokeOpen] = useState(false)
  const [invokeFunctionName, setInvokeFunctionName] = useState("")
  const [invokePayload, setInvokePayload] = useState("{\n  \"name\": \"Nova\"\n}")
  const [invokeResult, setInvokeResult] = useState("")
  const [invoking, setInvoking] = useState(false)
  const [invokeError, setInvokeError] = useState<string | null>(null)

  const [previewRelease, setPreviewRelease] = useState<AppStoreRelease | null>(null)
  const [previewTab, setPreviewTab] = useState("functions")
  const [focusedFunctionRef, setFocusedFunctionRef] = useState<string | null>(null)
  const [hasInspectedRelease, setHasInspectedRelease] = useState(false)
  const [hasInvokedFunction, setHasInvokedFunction] = useState(false)

  const loadInstallationResources = useCallback(async (installationID: string) => {
    try {
      setResourceLoading((prev) => ({ ...prev, [installationID]: true }))
      setResourceError((prev) => ({ ...prev, [installationID]: "" }))
      const detail = await storeApi.getInstallation(installationID)
      setInstallationResources((prev) => ({ ...prev, [installationID]: detail.resources || [] }))
    } catch (err) {
      setResourceError((prev) => ({ ...prev, [installationID]: toUserErrorMessage(err) }))
    } finally {
      setResourceLoading((prev) => ({ ...prev, [installationID]: false }))
    }
  }, [])

  const fetchData = useCallback(async () => {
    if (!slug) {
      setError("Missing app slug.")
      setLoading(false)
      return
    }

    try {
      setLoading(true)
      setError(null)
      setActionError(null)

      const appData = await storeApi.getApp(slug)
      const [releasesResp, installsResp] = await Promise.all([
        storeApi.listReleases(slug, 100),
        storeApi.listInstallations(200),
      ])

      const releaseList = releasesResp.releases || []
      const installList = (installsResp.installations || []).filter((item) => item.app_id === appData.id)

      setApp(appData)
      setReleases(releaseList)
      setInstallations(installList)
      setInstallationResources({})
      setResourceError({})
      setResourceLoading({})
    } catch (err) {
      setError(toUserErrorMessage(err))
      setApp(null)
      setReleases([])
      setInstallations([])
    } finally {
      setLoading(false)
    }
  }, [slug])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  useEffect(() => {
    setHasInspectedRelease(false)
    setHasInvokedFunction(false)
    setSelectedVersion("")
  }, [slug])

  useEffect(() => {
    if (!selectedVersion && releases.length > 0) {
      setSelectedVersion(releases[0].version)
    }
  }, [releases, selectedVersion])

  useEffect(() => {
    if (app && !installName) {
      setInstallName(`${app.slug}-default`)
    }
  }, [app, installName])

  useEffect(() => {
    for (const installation of installations) {
      if (!installationResources[installation.id] && !resourceLoading[installation.id]) {
        void loadInstallationResources(installation.id)
      }
    }
  }, [installations, installationResources, loadInstallationResources, resourceLoading])

  useEffect(() => {
    if (!installJobID) {
      return
    }

    let active = true
    let timer = 0

    const poll = async () => {
      if (!active) return
      try {
        const job = await storeApi.getInstallJob(installJobID)
        if (!active) return
        setInstallJob(job)
        if (isTerminalJobStatus(job.status)) {
          window.clearInterval(timer)
          await fetchData()
        }
      } catch (err) {
        if (!active) return
        setActionError(toUserErrorMessage(err))
        window.clearInterval(timer)
      }
    }

    void poll()
    timer = window.setInterval(() => {
      void poll()
    }, 2000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [fetchData, installJobID])

  const handlePlan = async () => {
    if (!selectedVersion) {
      setActionError("Please select a version before planning installation.")
      return
    }
    if (!installName.trim()) {
      setActionError("Install name is required.")
      return
    }

    const parsed = parseValuesInput(valuesText)
    if (!parsed.ok) {
      setActionError(parsed.error)
      return
    }

    try {
      setPlanning(true)
      setActionError(null)
      const plan = await storeApi.planInstall({
        app_slug: slug,
        version: selectedVersion,
        install_name: installName.trim(),
        name_prefix: namePrefix.trim() || undefined,
        values: parsed.values,
      })
      setPlanResult(plan)
    } catch (err) {
      setActionError(toUserErrorMessage(err))
      setPlanResult(null)
    } finally {
      setPlanning(false)
    }
  }

  const handleInstall = async () => {
    if (!selectedVersion) {
      setActionError("Please select a version before installation.")
      return
    }
    if (!installName.trim()) {
      setActionError("Install name is required.")
      return
    }

    const parsed = parseValuesInput(valuesText)
    if (!parsed.ok) {
      setActionError(parsed.error)
      return
    }

    try {
      setInstalling(true)
      setActionError(null)
      const result = await storeApi.install({
        app_slug: slug,
        version: selectedVersion,
        install_name: installName.trim(),
        name_prefix: namePrefix.trim() || undefined,
        values: parsed.values,
      })
      setInstallJobID(result.job_id)
      await fetchData()
    } catch (err) {
      setActionError(toUserErrorMessage(err))
    } finally {
      setInstalling(false)
    }
  }

  const handleUninstall = async (installationID: string) => {
    try {
      setUninstallingID(installationID)
      setActionError(null)
      await storeApi.uninstall(installationID)
      await fetchData()
    } catch (err) {
      setActionError(toUserErrorMessage(err))
    } finally {
      setUninstallingID(null)
    }
  }

  const openInvokeDialog = (functionName: string) => {
    setInvokeFunctionName(functionName)
    setInvokePayload("{\n  \"name\": \"Nova\"\n}")
    setInvokeResult("")
    setInvokeError(null)
    setInvokeOpen(true)
  }

  const openReleasePreview = (release: AppStoreRelease) => {
    setPreviewRelease(release)
    setPreviewTab("functions")
    setFocusedFunctionRef(null)
    setHasInspectedRelease(true)
  }

  const previewManifest = useMemo(() => parseReleaseManifest(previewRelease?.manifest_json), [previewRelease])
  const previewFunctions = useMemo(() => {
    return Array.isArray(previewManifest.functions) ? previewManifest.functions : []
  }, [previewManifest])
  const previewWorkflowVersion = useMemo(
    () => buildWorkflowPreviewVersion(previewManifest),
    [previewManifest]
  )

  const handleWorkflowFunctionClick = (functionRef: string) => {
    setFocusedFunctionRef(functionRef)
    setPreviewTab("functions")
  }

  const handleInvoke = async () => {
    if (!invokeFunctionName) {
      setInvokeError("Function name is required.")
      return
    }
    let payload: unknown = {}
    if (invokePayload.trim()) {
      try {
        payload = JSON.parse(invokePayload)
      } catch {
        setInvokeError("Payload must be valid JSON.")
        return
      }
    }

    try {
      setInvoking(true)
      setInvokeError(null)
      const response = await functionsApi.invoke(invokeFunctionName, payload)
      setInvokeResult(JSON.stringify(response, null, 2))
      setHasInvokedFunction(true)
    } catch (err) {
      setInvokeError(toUserErrorMessage(err))
      setInvokeResult("")
    } finally {
      setInvoking(false)
    }
  }

  const selectedRelease =
    releases.find((release) => release.version === selectedVersion) || releases[0] || null

  const allInstalledResources = useMemo(
    () => Object.values(installationResources).flat(),
    [installationResources]
  )

  const firstFunctionResource = allInstalledResources.find((item) => item.resource_type === "function")
  const firstWorkflowResource = allInstalledResources.find((item) => item.resource_type === "workflow")
  const hasInstalledResources = allInstalledResources.length > 0
  const hasInstallations = installations.length > 0

  const consumerSteps: FlowStep[] = [
    {
      id: "inspect-release",
      title: "Inspect release package",
      description: "Review bundled functions, runtime metadata, and workflow DAG.",
      status: releases.length === 0 ? "pending" : hasInspectedRelease ? "done" : "current",
      action: selectedRelease
        ? {
            label: hasInspectedRelease ? "Inspect Again" : "Inspect",
            onClick: () => openReleasePreview(selectedRelease),
          }
        : undefined,
    },
    {
      id: "plan-install",
      title: "Plan installation",
      description: "Validate quota, runtime dependencies, and resource conflicts before apply.",
      status:
        releases.length === 0
          ? "pending"
          : planResult?.valid
            ? "done"
            : hasInspectedRelease
              ? "current"
              : "pending",
      action:
        releases.length > 0
          ? {
              label: "Go to Install",
              href: "#install",
            }
          : undefined,
    },
    {
      id: "install-release",
      title: "Install selected version",
      description: "Create managed functions/workflows in the current tenant namespace.",
      status: hasInstallations ? "done" : releases.length > 0 ? "current" : "pending",
      action:
        releases.length > 0
          ? {
              label: "Open Install Section",
              href: "#install",
            }
          : undefined,
    },
    {
      id: "use-resources",
      title: "Use installed resources",
      description: "Open functions/workflows, run invocations, and verify output.",
      status: hasInvokedFunction ? "done" : hasInstalledResources ? "current" : "pending",
      action: firstFunctionResource
        ? {
            label: "Open Function",
            href: `/functions/${encodeURIComponent(firstFunctionResource.resource_name)}`,
          }
        : firstWorkflowResource
          ? {
              label: "Open Workflow",
              href: `/workflows/${encodeURIComponent(firstWorkflowResource.resource_name)}`,
            }
          : undefined,
    },
    {
      id: "uninstall-cleanup",
      title: "Uninstall when no longer needed",
      description: "Cleanly remove managed resources from this scope.",
      status: hasInstallations ? "current" : "pending",
      action:
        hasInstallations
          ? {
              label: "Manage Installations",
              href: "#installations",
            }
          : undefined,
    },
  ]

  if (loading) {
    return (
      <DashboardLayout>
        <Header title="App Store" description="Loading app details..." />
        <div className="p-6">
          <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
            Loading app details...
          </div>
        </div>
      </DashboardLayout>
    )
  }

  if (error || !app) {
    return (
      <DashboardLayout>
        <Header title="App Store" description="App detail" />
        <div className="space-y-4 p-6">
          <ErrorBanner error={error || "App not found"} title="Failed to Load App" onRetry={fetchData} />
          <Button asChild variant="outline">
            <Link href="/store">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to App Store
            </Link>
          </Button>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={app.title} description={`Install, use, and uninstall ${app.slug}`} />

      <div className="space-y-6 p-6">
        {actionError ? <ErrorBanner error={actionError} title="Action Failed" /> : null}

        <div className="flex flex-wrap items-center gap-2">
          <Button asChild variant="outline">
            <Link href="/store">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Link>
          </Button>
          <Button variant="outline" onClick={fetchData}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          <Button asChild variant="outline">
            <Link href="/my-apps">Manage My Apps</Link>
          </Button>
          <Button asChild variant="secondary">
            <a href="#install">
              <PackageCheck className="mr-2 h-4 w-4" />
              Install
            </a>
          </Button>
        </div>

        <section className="rounded-lg border border-border bg-card p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-base font-semibold text-foreground">{app.title}</h2>
              <p className="mt-1 font-mono text-xs text-muted-foreground">{app.slug}</p>
            </div>
            <Badge variant={app.visibility === "public" ? "default" : "secondary"}>
              {app.visibility}
            </Badge>
          </div>
          <p className="mt-3 text-sm text-muted-foreground">
            {app.summary || app.description || "No description provided."}
          </p>
          <div className="mt-3 grid grid-cols-1 gap-2 text-xs text-muted-foreground sm:grid-cols-2">
            <p>Owner: {app.owner}</p>
            <p>Updated: {formatDateTime(app.updated_at)}</p>
            <p>Releases: {releases.length}</p>
            <p>Installations in scope: {installations.length}</p>
          </div>
        </section>

        <StoreFlowRoadmap
          title="Consumer Workflow"
          description="From inspection to install/use/uninstall, follow this sequence in the current scope."
          steps={consumerSteps}
        />

        <section id="releases" className="rounded-lg border border-border bg-card">
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            <StoreIcon className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Releases</h2>
          </div>
          {releases.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No Releases Yet"
                description="No public versions are available yet."
                icon={Package}
                primaryAction={{
                  label: "Go to My Apps",
                  href: "/my-apps",
                }}
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4 md:grid-cols-2 xl:grid-cols-3">
              {releases.map((release) => {
                const stats = parseManifestStats(release.manifest_json)
                return (
                  <article key={release.id} className="rounded-lg border border-border/80 bg-card/70 p-4">
                    <div className="flex items-center justify-between gap-2">
                      <h3 className="font-mono text-sm font-semibold">{release.version}</h3>
                      <Badge variant={statusBadgeVariant(release.status)}>{release.status}</Badge>
                    </div>
                    <div className="mt-3 space-y-1 text-xs text-muted-foreground">
                      <p>Functions: {stats.functionCount}</p>
                      <p>Workflow: {stats.hasWorkflow ? "yes" : "no"}</p>
                      <p>Updated: {formatDateTime(release.updated_at)}</p>
                    </div>
                    <div className="mt-4 flex items-center gap-2">
                      <Button
                        size="sm"
                        onClick={() => {
                          setSelectedVersion(release.version)
                          document.getElementById("install")?.scrollIntoView({ behavior: "smooth", block: "start" })
                        }}
                      >
                        Use This Version
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => openReleasePreview(release)}
                      >
                        Inspect
                      </Button>
                    </div>
                  </article>
                )
              })}
            </div>
          )}
        </section>

        <section id="install" className="rounded-lg border border-border bg-card p-4">
          <div className="mb-3 flex items-center gap-2">
            <Wrench className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Install</h2>
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div className="space-y-2">
              <Label>Version</Label>
              <Select value={selectedVersion} onValueChange={setSelectedVersion}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select version" />
                </SelectTrigger>
                <SelectContent>
                  {releases.map((release) => (
                    <SelectItem key={release.id} value={release.version}>
                      {release.version}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="install-name">Install Name</Label>
              <Input
                id="install-name"
                value={installName}
                onChange={(event) => setInstallName(event.target.value)}
                placeholder="hello-world-prod"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="name-prefix">Name Prefix (optional)</Label>
              <Input
                id="name-prefix"
                value={namePrefix}
                onChange={(event) => setNamePrefix(event.target.value)}
                placeholder="hw-prod"
              />
            </div>
          </div>

          <div className="mt-4 space-y-2">
            <Label htmlFor="install-values">Values JSON</Label>
            <Textarea
              id="install-values"
              rows={5}
              value={valuesText}
              onChange={(event) => setValuesText(event.target.value)}
            />
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button variant="outline" onClick={handlePlan} disabled={planning}>
              {planning ? "Planning..." : "Plan Install"}
            </Button>
            <Button onClick={handleInstall} disabled={installing}>
              {installing ? "Installing..." : "Install"}
            </Button>
          </div>

          {planResult ? (
            <div className="mt-4 rounded-lg border border-border/80 bg-card/70 p-4 text-sm">
              <p className="font-medium">
                Plan: {planResult.valid ? "valid" : "invalid"}
              </p>
              <div className="mt-2 grid grid-cols-1 gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                <p>Resources to create: {planResult.to_create?.length || 0}</p>
                <p>Conflicts: {planResult.conflicts?.length || 0}</p>
                <p>Missing runtimes: {planResult.missing_runtimes?.length || 0}</p>
                <p>Quota check: {planResult.quota_check?.ok ? "ok" : "failed"}</p>
              </div>
              {(planResult.errors || []).length > 0 ? (
                <ul className="mt-3 list-disc space-y-1 pl-5 text-xs text-destructive">
                  {planResult.errors.map((item, index) => (
                    <li key={`plan-error-${index}`}>{item}</li>
                  ))}
                </ul>
              ) : null}
            </div>
          ) : null}
        </section>

        {installJob ? (
          <section className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold">Latest Install Job</h2>
              <Badge variant={statusBadgeVariant(installJob.status)}>{installJob.status}</Badge>
            </div>
            <div className="mt-2 space-y-1 text-xs text-muted-foreground">
              <p>Job ID: {installJob.id}</p>
              <p>Step: {installJob.step || "-"}</p>
              <p>Started: {formatDateTime(installJob.started_at)}</p>
              {installJob.finished_at ? <p>Finished: {formatDateTime(installJob.finished_at)}</p> : null}
              {installJob.error ? <p className="text-destructive">Error: {installJob.error}</p> : null}
            </div>
          </section>
        ) : null}

        <section id="installations" className="rounded-lg border border-border bg-card">
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            <PackageCheck className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Installations</h2>
          </div>

          {installations.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No Installations"
                description="Install a release to create resources in this tenant/namespace."
                icon={PackageCheck}
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4">
              {installations.map((installation) => {
                const resources = installationResources[installation.id] || []
                const functionResources = resources.filter((item) => item.resource_type === "function")
                const workflowResources = resources.filter((item) => item.resource_type === "workflow")
                return (
                  <article key={installation.id} className="rounded-lg border border-border/80 bg-card/70 p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <p className="text-sm font-semibold text-foreground">{installation.install_name}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {installation.tenant_id}/{installation.namespace}
                        </p>
                      </div>
                      <Badge variant={statusBadgeVariant(installation.status)}>
                        {installation.status}
                      </Badge>
                    </div>

                    <div className="mt-3 grid grid-cols-1 gap-1 text-xs text-muted-foreground sm:grid-cols-2">
                      <p>Created by: {installation.created_by || "-"}</p>
                      <p>Updated: {formatDateTime(installation.updated_at)}</p>
                      <p>Functions: {functionResources.length}</p>
                      <p>Workflows: {workflowResources.length}</p>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => loadInstallationResources(installation.id)}
                        disabled={resourceLoading[installation.id]}
                      >
                        {resourceLoading[installation.id] ? "Loading..." : "Reload Resources"}
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => handleUninstall(installation.id)}
                        disabled={uninstallingID === installation.id}
                      >
                        {uninstallingID === installation.id ? "Uninstalling..." : "Uninstall"}
                      </Button>
                    </div>

                    {resourceError[installation.id] ? (
                      <p className="mt-3 text-xs text-destructive">{resourceError[installation.id]}</p>
                    ) : null}

                    <div className="mt-3 space-y-2">
                      {resources.length === 0 ? (
                        <p className="text-xs text-muted-foreground">No resource records yet.</p>
                      ) : (
                        resources.map((resource) => (
                          <div
                            key={resource.id}
                            className="rounded-md border border-border/70 bg-background/30 p-3"
                          >
                            <div className="flex flex-wrap items-start justify-between gap-2">
                              <div>
                                <div className="flex items-center gap-2">
                                  <Badge variant="outline">{resource.resource_type}</Badge>
                                  <span className="text-sm font-medium text-foreground">
                                    {resource.resource_name}
                                  </span>
                                </div>
                                <p className="mt-1 text-xs text-muted-foreground">
                                  Resource ID: {resource.resource_id}
                                </p>
                              </div>

                              {resource.resource_type === "function" ? (
                                <div className="flex items-center gap-2">
                                  <Button
                                    size="sm"
                                    variant="outline"
                                    onClick={() => openInvokeDialog(resource.resource_name)}
                                  >
                                    <Play className="mr-2 h-3.5 w-3.5" />
                                    Invoke
                                  </Button>
                                  <Button asChild size="sm" variant="outline">
                                    <Link href={`/functions/${encodeURIComponent(resource.resource_name)}`}>
                                      Open Function
                                    </Link>
                                  </Button>
                                </div>
                              ) : resource.resource_type === "workflow" ? (
                                <Button asChild size="sm" variant="outline">
                                  <Link href={`/workflows/${encodeURIComponent(resource.resource_name)}`}>
                                    Open Workflow
                                  </Link>
                                </Button>
                              ) : null}
                            </div>
                          </div>
                        ))
                      )}
                    </div>
                  </article>
                )
              })}
            </div>
          )}
        </section>
      </div>

      <Dialog
        open={!!previewRelease}
        onOpenChange={(open) => {
          if (!open) {
            setPreviewRelease(null)
            setFocusedFunctionRef(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>
              Release Inspector {previewRelease ? `· ${previewRelease.version}` : ""}
            </DialogTitle>
          </DialogHeader>

          <Tabs value={previewTab} onValueChange={setPreviewTab}>
            <TabsList>
              <TabsTrigger value="functions">Functions</TabsTrigger>
              <TabsTrigger value="workflow">Workflow</TabsTrigger>
              <TabsTrigger value="manifest">Manifest JSON</TabsTrigger>
            </TabsList>

            <TabsContent value="functions" className="mt-4">
              {previewFunctions.length === 0 ? (
                <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
                  No function entries in this release manifest.
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                  {previewFunctions.map((fn) => {
                    const isFocused = focusedFunctionRef === fn.key
                    return (
                      <article
                        key={fn.key}
                        className={`rounded-lg border bg-card/70 p-4 ${isFocused ? "border-primary" : "border-border/80"}`}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div>
                            <h3 className="text-sm font-semibold text-foreground">
                              {fn.name || fn.key}
                            </h3>
                            <p className="mt-1 font-mono text-xs text-muted-foreground">
                              key: {fn.key}
                            </p>
                          </div>
                          {fn.runtime ? <Badge variant="outline">{fn.runtime}</Badge> : null}
                        </div>

                        <div className="mt-3 grid grid-cols-1 gap-1 text-xs text-muted-foreground sm:grid-cols-2">
                          <p>Handler: {fn.handler || "-"}</p>
                          <p>Timeout: {fn.timeout_s ?? "-"}s</p>
                          <p>Memory: {fn.memory_mb ?? "-"} MB</p>
                          <p>Files: {Array.isArray(fn.files) ? fn.files.length : 0}</p>
                        </div>

                        {Array.isArray(fn.files) && fn.files.length > 0 ? (
                          <div className="mt-3 rounded-md border border-border/70 bg-background/40 p-2">
                            <p className="text-xs font-medium text-foreground">Files</p>
                            <ul className="mt-1 space-y-1">
                              {fn.files.map((file) => (
                                <li key={`${fn.key}-${file}`} className="font-mono text-xs text-muted-foreground">
                                  {file}
                                </li>
                              ))}
                            </ul>
                          </div>
                        ) : null}

                        {fn.description ? (
                          <p className="mt-3 text-xs text-muted-foreground">{fn.description}</p>
                        ) : null}

                        {fn.env_vars && Object.keys(fn.env_vars).length > 0 ? (
                          <div className="mt-3 rounded-md border border-border/70 bg-background/40 p-2">
                            <p className="text-xs font-medium text-foreground">Env Vars</p>
                            <div className="mt-1 flex flex-wrap gap-1.5">
                              {Object.entries(fn.env_vars).map(([key, value]) => (
                                <Badge key={`${fn.key}-env-${key}`} variant="outline" className="font-mono text-[10px]">
                                  {key}={String(value)}
                                </Badge>
                              ))}
                            </div>
                          </div>
                        ) : null}
                      </article>
                    )
                  })}
                </div>
              )}
            </TabsContent>

            <TabsContent value="workflow" className="mt-4">
              {previewWorkflowVersion ? (
                <div className="space-y-3">
                  <div className="rounded-lg border border-border bg-card p-3">
                    <p className="text-sm font-medium text-foreground">
                      {previewManifest.workflow?.name || "Workflow"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {previewManifest.workflow?.description || "Workflow DAG preview from release manifest."}
                    </p>
                  </div>
                  <DagViewer version={previewWorkflowVersion} onFunctionClick={handleWorkflowFunctionClick} />
                </div>
              ) : (
                <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
                  This release does not include a workflow DAG.
                </div>
              )}
            </TabsContent>

            <TabsContent value="manifest" className="mt-4">
              <Textarea
                rows={18}
                value={JSON.stringify(previewRelease?.manifest_json ?? {}, null, 2)}
                readOnly
                className="font-mono text-xs"
              />
            </TabsContent>
          </Tabs>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setPreviewRelease(null)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={invokeOpen} onOpenChange={setInvokeOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Invoke Function: {invokeFunctionName}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            {invokeError ? <ErrorBanner error={invokeError} title="Invocation Failed" /> : null}
            <div className="space-y-2">
              <Label htmlFor="invoke-payload">Payload JSON</Label>
              <Textarea
                id="invoke-payload"
                rows={8}
                value={invokePayload}
                onChange={(event) => setInvokePayload(event.target.value)}
              />
            </div>

            {invokeResult ? (
              <div className="space-y-2">
                <Label htmlFor="invoke-result">Result</Label>
                <Textarea id="invoke-result" rows={10} value={invokeResult} readOnly />
              </div>
            ) : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setInvokeOpen(false)} disabled={invoking}>
              Close
            </Button>
            <Button type="button" onClick={handleInvoke} disabled={invoking}>
              {invoking ? "Invoking..." : "Invoke"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  )
}

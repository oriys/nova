"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { StoreFlowRoadmap, type FlowStep } from "@/components/store-flow-roadmap"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import {
  functionsApi,
  storeApi,
  workflowsApi,
  type AppStoreApp,
  type AppStoreRelease,
  type NovaFunction,
  type Workflow,
} from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { Clock3, Package, Plus, RefreshCw, Store as StoreIcon, Upload } from "lucide-react"

const MY_OWNER = "system"

function formatDateTime(value: string): string {
  return new Date(value).toLocaleString()
}

function toSlug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80)
}

function releaseStatusBadgeVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  if (status === "published") return "default"
  if (status === "yanked") return "destructive"
  if (status === "draft") return "secondary"
  return "outline"
}

export default function MyAppsPage() {
  const [apps, setApps] = useState<AppStoreApp[]>([])
  const [releasesBySlug, setReleasesBySlug] = useState<Record<string, AppStoreRelease[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createForm, setCreateForm] = useState({
    slug: "",
    title: "",
    summary: "",
    description: "",
    tags: "",
    visibility: "public" as "public" | "private",
  })

  const [publishOpen, setPublishOpen] = useState(false)
  const [publishTarget, setPublishTarget] = useState<AppStoreApp | null>(null)
  const [publishing, setPublishing] = useState(false)
  const [publishError, setPublishError] = useState<string | null>(null)
  const [publishVersion, setPublishVersion] = useState("")
  const [publishChangelog, setPublishChangelog] = useState("")
  const [resourceLoading, setResourceLoading] = useState(false)
  const [availableFunctions, setAvailableFunctions] = useState<NovaFunction[]>([])
  const [availableWorkflows, setAvailableWorkflows] = useState<Workflow[]>([])
  const [selectedFunctions, setSelectedFunctions] = useState<string[]>([])
  const [selectedWorkflow, setSelectedWorkflow] = useState("")

  const totalReleases = useMemo(() => {
    return Object.values(releasesBySlug).reduce((sum, items) => sum + items.length, 0)
  }, [releasesBySlug])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const appsResp = await storeApi.listApps({ owner: MY_OWNER, limit: 200 })
      const myApps = appsResp.apps || []
      setApps(myApps)

      const releaseEntries = await Promise.all(
        myApps.map(async (app) => {
          try {
            const releasesResp = await storeApi.listReleases(app.slug, 50)
            return [app.slug, releasesResp.releases || []] as const
          } catch {
            return [app.slug, []] as const
          }
        })
      )
      setReleasesBySlug(Object.fromEntries(releaseEntries))
    } catch (err) {
      setError(toUserErrorMessage(err))
      setApps([])
      setReleasesBySlug({})
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const resetCreateForm = () => {
    setCreateForm({
      slug: "",
      title: "",
      summary: "",
      description: "",
      tags: "",
      visibility: "public",
    })
    setCreateError(null)
  }

  const handleCreateApp = async (event: FormEvent) => {
    event.preventDefault()
    if (!createForm.slug.trim()) {
      setCreateError("Slug is required.")
      return
    }
    if (!createForm.title.trim()) {
      setCreateError("Title is required.")
      return
    }

    const tags = createForm.tags
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean)

    try {
      setCreating(true)
      setCreateError(null)
      await storeApi.createApp({
        slug: createForm.slug.trim(),
        title: createForm.title.trim(),
        summary: createForm.summary.trim() || undefined,
        description: createForm.description.trim() || undefined,
        tags: tags.length > 0 ? tags : undefined,
        visibility: createForm.visibility,
      })
      setCreateOpen(false)
      resetCreateForm()
      await fetchData()
    } catch (err) {
      setCreateError(toUserErrorMessage(err))
    } finally {
      setCreating(false)
    }
  }

  const loadPublishResources = useCallback(async () => {
    try {
      setResourceLoading(true)
      const [functions, workflows] = await Promise.all([
        functionsApi.list("", 500),
        workflowsApi.list(),
      ])
      setAvailableFunctions(
        [...functions].sort((a, b) => a.name.localeCompare(b.name))
      )
      setAvailableWorkflows(
        [...workflows].sort((a, b) => a.name.localeCompare(b.name))
      )
    } catch (err) {
      setPublishError(toUserErrorMessage(err))
      setAvailableFunctions([])
      setAvailableWorkflows([])
    } finally {
      setResourceLoading(false)
    }
  }, [])

  const openPublishDialog = (app: AppStoreApp) => {
    setPublishTarget(app)
    setPublishVersion("")
    setPublishChangelog("")
    setSelectedFunctions([])
    setSelectedWorkflow("")
    setPublishError(null)
    setPublishOpen(true)
    void loadPublishResources()
  }

  const toggleFunctionSelection = (name: string) => {
    setSelectedFunctions((prev) =>
      prev.includes(name) ? prev.filter((item) => item !== name) : [...prev, name]
    )
  }

  const handlePublish = async (event: FormEvent) => {
    event.preventDefault()
    if (!publishTarget) {
      setPublishError("Please choose an app to publish.")
      return
    }
    if (!publishVersion.trim()) {
      setPublishError("Version is required.")
      return
    }

    try {
      setPublishing(true)
      setPublishError(null)
      if (selectedFunctions.length === 0 && !selectedWorkflow) {
        setPublishError("Select at least one function or one workflow.")
        return
      }
      await storeApi.publishReleaseFromResources(publishTarget.slug, {
        version: publishVersion.trim(),
        changelog: publishChangelog.trim() || undefined,
        function_names: selectedFunctions,
        workflow_names: selectedWorkflow ? [selectedWorkflow] : [],
      })
      const releasesResp = await storeApi.listReleases(publishTarget.slug, 50)
      setReleasesBySlug((prev) => ({
        ...prev,
        [publishTarget.slug]: releasesResp.releases || [],
      }))
      setPublishOpen(false)
    } catch (err) {
      setPublishError(toUserErrorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

  const hasApps = apps.length > 0
  const hasReleases = totalReleases > 0
  const firstApp = apps[0] || null
  const firstReleasedApp =
    apps.find((app) => (releasesBySlug[app.slug] || []).length > 0) || null

  const publisherSteps: FlowStep[] = [
    {
      id: "create-app",
      title: "Create app metadata",
      description: "Define slug, title, summary, tags, and visibility for your package.",
      status: hasApps ? "done" : "current",
      action: {
        label: "Create App",
        onClick: () => {
          resetCreateForm()
          setCreateOpen(true)
        },
      },
    },
    {
      id: "publish-release",
      title: "Publish a release",
      description: "Select existing functions/workflow and assign a semantic version.",
      status: hasReleases ? "done" : hasApps ? "current" : "pending",
      action: firstApp
        ? {
            label: "Publish",
            onClick: () => openPublishDialog(firstApp),
          }
        : undefined,
    },
    {
      id: "verify-release",
      title: "Verify functions and workflow",
      description: "Open the app detail page and inspect function metadata and workflow DAG.",
      status: hasReleases ? "current" : "pending",
      action: firstReleasedApp
        ? {
            label: "Open Detail",
            href: `/store/${encodeURIComponent(firstReleasedApp.slug)}`,
          }
        : undefined,
    },
    {
      id: "distribute",
      title: "Distribute for installation",
      description: "Share the app with tenants so they can plan, install, and use it.",
      status: "pending",
      action: {
        label: "Browse App Store",
        href: "/store",
      },
    },
  ]

  return (
    <DashboardLayout>
      <Header title="My Apps" description="Manage apps you publish to the App Store" />

      <div className="space-y-6 p-6">
        {error ? <ErrorBanner error={error} title="Failed to Load My Apps" onRetry={fetchData} /> : null}

        <div className="flex items-center justify-between gap-3">
          <div className="text-sm text-muted-foreground">
            {loading ? "Loading..." : `${apps.length} apps · ${totalReleases} releases`}
          </div>
          <div className="flex items-center gap-2">
            <Button
              onClick={() => {
                resetCreateForm()
                setCreateOpen(true)
              }}
            >
              <Plus className="mr-2 h-4 w-4" />
              Create App
            </Button>
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button asChild variant="outline">
              <Link href="/store">Browse App Store</Link>
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">My Published Apps</p>
            <p className="mt-2 text-2xl font-semibold">{loading ? "..." : apps.length}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Total Releases</p>
            <p className="mt-2 text-2xl font-semibold">{loading ? "..." : totalReleases}</p>
          </div>
        </div>

        <StoreFlowRoadmap
          title="Publisher Workflow"
          description="Follow this sequence to move an app from draft to consumable package."
          steps={publisherSteps}
        />

        <section id="managed-apps" className="rounded-lg border border-border bg-card">
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            <StoreIcon className="h-4 w-4" />
            <h2 className="text-sm font-semibold">Managed Apps</h2>
          </div>

          {!loading && apps.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No Apps Yet"
                description="Create your first app and publish a release from this page."
                icon={Package}
                primaryAction={{
                  label: "Create App",
                  onClick: () => {
                    resetCreateForm()
                    setCreateOpen(true)
                  },
                }}
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4 p-4 lg:grid-cols-2">
              {loading
                ? Array.from({ length: 4 }).map((_, index) => (
                    <div key={`my-app-skeleton-${index}`} className="rounded-lg border border-border/80 bg-card/70 p-4">
                      <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-1/3 animate-pulse rounded bg-muted" />
                      <div className="mt-4 h-3 w-full animate-pulse rounded bg-muted" />
                      <div className="mt-2 h-3 w-5/6 animate-pulse rounded bg-muted" />
                    </div>
                  ))
                : apps.map((app) => {
                    const releases = releasesBySlug[app.slug] || []
                    const latest = releases[0]
                    return (
                      <article key={app.id} className="rounded-lg border border-border/80 bg-card/70 p-4">
                        <div className="flex items-start justify-between gap-2">
                          <div>
                            <h3 className="text-sm font-semibold text-foreground">{app.title}</h3>
                            <p className="mt-1 font-mono text-xs text-muted-foreground">{app.slug}</p>
                          </div>
                          <Badge variant={app.visibility === "public" ? "default" : "secondary"}>
                            {app.visibility}
                          </Badge>
                        </div>

                        <p className="mt-3 text-sm text-muted-foreground">
                          {app.summary || app.description || "No description provided."}
                        </p>

                        <div className="mt-3 grid grid-cols-1 gap-1 text-xs text-muted-foreground sm:grid-cols-2">
                          <p>Releases: {releases.length}</p>
                          <p>Updated: {formatDateTime(app.updated_at)}</p>
                          <p>Latest Version: {latest?.version || "-"}</p>
                          <p>Owner: {app.owner}</p>
                        </div>

                        <div className="mt-4 flex flex-wrap items-center gap-2">
                          <Button size="sm" onClick={() => openPublishDialog(app)}>
                            <Upload className="mr-2 h-3.5 w-3.5" />
                            Publish
                          </Button>
                          <Button asChild size="sm" variant="outline">
                            <Link href={`/store/${encodeURIComponent(app.slug)}`}>Open Detail</Link>
                          </Button>
                          <Button asChild size="sm" variant="outline">
                            <Link href={`/store/${encodeURIComponent(app.slug)}#install`}>Install Flow</Link>
                          </Button>
                        </div>

                        <div className="mt-4 space-y-2">
                          {releases.length === 0 ? (
                            <p className="text-xs text-muted-foreground">No releases yet.</p>
                          ) : (
                            releases.slice(0, 3).map((release) => (
                              <div
                                key={release.id}
                                className="flex items-center justify-between rounded-md border border-border/70 bg-background/30 px-3 py-2"
                              >
                                <div className="min-w-0">
                                  <p className="truncate font-mono text-xs text-foreground">{release.version}</p>
                                  <p className="text-xs text-muted-foreground">{formatDateTime(release.updated_at)}</p>
                                </div>
                                <Badge variant={releaseStatusBadgeVariant(release.status)}>{release.status}</Badge>
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

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Create App</DialogTitle>
          </DialogHeader>

          <form className="space-y-4" onSubmit={handleCreateApp}>
            {createError ? <ErrorBanner error={createError} title="Failed to Create App" /> : null}

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="my-app-title">Title</Label>
                <Input
                  id="my-app-title"
                  value={createForm.title}
                  onChange={(event) => {
                    const title = event.target.value
                    setCreateForm((prev) => ({
                      ...prev,
                      title,
                      slug: prev.slug || toSlug(title),
                    }))
                  }}
                  placeholder="Data Processor Toolkit"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="my-app-slug">Slug</Label>
                <Input
                  id="my-app-slug"
                  value={createForm.slug}
                  onChange={(event) =>
                    setCreateForm((prev) => ({ ...prev, slug: toSlug(event.target.value) }))
                  }
                  placeholder="data-processor-toolkit"
                  required
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="my-app-summary">Summary</Label>
              <Input
                id="my-app-summary"
                value={createForm.summary}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, summary: event.target.value }))}
                placeholder="Reusable serverless bundle"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="my-app-description">Description</Label>
              <Textarea
                id="my-app-description"
                rows={4}
                value={createForm.description}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, description: event.target.value }))}
                placeholder="Describe what this app package does."
              />
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="my-app-tags">Tags (comma separated)</Label>
                <Input
                  id="my-app-tags"
                  value={createForm.tags}
                  onChange={(event) => setCreateForm((prev) => ({ ...prev, tags: event.target.value }))}
                  placeholder="etl, workflow, sample"
                />
              </div>

              <div className="space-y-2">
                <Label>Visibility</Label>
                <Select
                  value={createForm.visibility}
                  onValueChange={(value) =>
                    setCreateForm((prev) => ({ ...prev, visibility: value as "public" | "private" }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Select visibility" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="public">public</SelectItem>
                    <SelectItem value="private">private</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)} disabled={creating}>
                Cancel
              </Button>
              <Button type="submit" disabled={creating}>
                {creating ? "Creating..." : "Create App"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={publishOpen} onOpenChange={setPublishOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Publish Release {publishTarget ? `· ${publishTarget.slug}` : ""}</DialogTitle>
          </DialogHeader>

          <form className="space-y-4" onSubmit={handlePublish}>
            {publishError ? <ErrorBanner error={publishError} title="Failed to Publish Release" /> : null}

            <div className="space-y-2">
              <Label htmlFor="publish-version">Version</Label>
              <Input
                id="publish-version"
                value={publishVersion}
                onChange={(event) => setPublishVersion(event.target.value)}
                placeholder="1.0.0"
                required
              />
            </div>

            <div className="space-y-4 rounded-lg border border-border/80 bg-card/70 p-4">
              <div className="flex items-center justify-between gap-2">
                <p className="text-sm font-medium">Select Functions and Workflow</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => void loadPublishResources()}
                  disabled={resourceLoading}
                >
                  <RefreshCw className={`mr-2 h-3.5 w-3.5 ${resourceLoading ? "animate-spin" : ""}`} />
                  Reload
                </Button>
              </div>

              <p className="text-xs text-muted-foreground">
                Select one or more functions. If you select a workflow, required workflow functions are auto-included.
              </p>

              <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div className="space-y-2">
                  <Label>Functions ({selectedFunctions.length} selected)</Label>
                  <div className="max-h-56 space-y-2 overflow-auto rounded-md border border-border/70 bg-background/40 p-3">
                    {resourceLoading ? (
                      <p className="text-xs text-muted-foreground">Loading functions...</p>
                    ) : availableFunctions.length === 0 ? (
                      <p className="text-xs text-muted-foreground">No functions found in current scope.</p>
                    ) : (
                      availableFunctions.map((fn) => (
                        <label
                          key={`publish-fn-${fn.id}`}
                          className="flex cursor-pointer items-start gap-2 rounded-md border border-border/60 bg-card/70 px-2 py-2"
                        >
                          <input
                            type="checkbox"
                            checked={selectedFunctions.includes(fn.name)}
                            onChange={() => toggleFunctionSelection(fn.name)}
                            className="mt-0.5"
                          />
                          <span className="min-w-0">
                            <span className="block truncate font-mono text-xs text-foreground">{fn.name}</span>
                            <span className="block text-[11px] text-muted-foreground">
                              {fn.runtime} · {fn.handler}
                            </span>
                          </span>
                        </label>
                      ))
                    )}
                  </div>
                </div>

                <div className="space-y-2">
                  <Label>Workflow (optional)</Label>
                  <div className="max-h-56 space-y-2 overflow-auto rounded-md border border-border/70 bg-background/40 p-3">
                    <label className="flex cursor-pointer items-center gap-2 rounded-md border border-border/60 bg-card/70 px-2 py-2">
                      <input
                        type="radio"
                        name="publish-workflow"
                        checked={selectedWorkflow === ""}
                        onChange={() => setSelectedWorkflow("")}
                      />
                      <span className="text-xs text-muted-foreground">No workflow</span>
                    </label>
                    {resourceLoading ? (
                      <p className="text-xs text-muted-foreground">Loading workflows...</p>
                    ) : availableWorkflows.length === 0 ? (
                      <p className="text-xs text-muted-foreground">No workflows found in current scope.</p>
                    ) : (
                      availableWorkflows.map((workflow) => (
                        <label
                          key={`publish-wf-${workflow.id}`}
                          className="flex cursor-pointer items-start gap-2 rounded-md border border-border/60 bg-card/70 px-2 py-2"
                        >
                          <input
                            type="radio"
                            name="publish-workflow"
                            checked={selectedWorkflow === workflow.name}
                            onChange={() => setSelectedWorkflow(workflow.name)}
                            className="mt-0.5"
                          />
                          <span className="min-w-0">
                            <span className="block truncate font-mono text-xs text-foreground">{workflow.name}</span>
                            <span className="block text-[11px] text-muted-foreground">
                              version {workflow.current_version}
                            </span>
                          </span>
                        </label>
                      ))
                    )}
                  </div>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="publish-changelog">Changelog</Label>
              <Textarea
                id="publish-changelog"
                rows={4}
                value={publishChangelog}
                onChange={(event) => setPublishChangelog(event.target.value)}
                placeholder="Describe release changes."
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setPublishOpen(false)} disabled={publishing}>
                Cancel
              </Button>
              <Button type="submit" disabled={publishing}>
                {publishing ? "Publishing..." : "Publish Release"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  )
}

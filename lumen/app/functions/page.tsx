"use client"

import { useEffect, useState, useCallback, useRef, useMemo } from "react"
import { useTranslations } from "next-intl"
import { useRouter } from "next/navigation"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { FunctionsTable } from "@/components/functions-table"
import { Pagination } from "@/components/pagination"
import { CreateFunctionDialog } from "@/components/create-function-dialog"
import { EmptyState } from "@/components/empty-state"
import { OnboardingFlow } from "@/components/onboarding-flow"
import { ErrorBanner } from "@/components/ui/error-banner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  functionsApi,
  gatewayApi,
  metricsApi,
  runtimesApi,
  type CreateFunctionRequest,
  type NetworkPolicy,
  type NovaFunction,
  type ResourceLimits,
  type RolloutPolicy,
} from "@/lib/api"
import { transformFunction, FunctionData, RuntimeInfo, transformRuntime } from "@/lib/types"
import {
  FUNCTION_SEARCH_EVENT,
  type FunctionSearchDetail,
  dispatchFunctionSearch,
  readFunctionSearchFromLocation,
} from "@/lib/function-search"
import { markOnboardingStep, syncOnboardingStateFromData } from "@/lib/onboarding-state"
import { toUserErrorMessage } from "@/lib/error-map"
import { Plus, Search, Filter, RefreshCw, Download, Upload } from "lucide-react"

type Notice = {
  kind: "success" | "error" | "info"
  text: string
}

type JsonObject = Record<string, unknown>

function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function readPositiveInt(value: unknown): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined
  }
  const next = Math.floor(value)
  return next > 0 ? next : undefined
}

function readStringMap(value: unknown): Record<string, string> | undefined {
  if (!isJsonObject(value)) {
    return undefined
  }
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string"
  )
  if (entries.length === 0) {
    return undefined
  }
  return Object.fromEntries(entries)
}

function parseFunctionImportPayload(payload: unknown): {
  items: CreateFunctionRequest[]
  invalid: number
} {
  const rows: unknown[] = Array.isArray(payload)
    ? payload
    : isJsonObject(payload) && Array.isArray(payload.functions)
      ? payload.functions
      : []

  const items: CreateFunctionRequest[] = []
  let invalid = 0

  rows.forEach((row) => {
    if (!isJsonObject(row)) {
      invalid += 1
      return
    }
    const name = typeof row.name === "string" ? row.name.trim() : ""
    const runtime = typeof row.runtime === "string" ? row.runtime.trim() : ""
    const code =
      typeof row.code === "string"
        ? row.code
        : typeof row.source_code === "string"
          ? row.source_code
          : ""

    if (!name || !runtime || !code) {
      invalid += 1
      return
    }

    const req: CreateFunctionRequest = {
      name,
      runtime,
      code,
    }

    const handler = typeof row.handler === "string" ? row.handler.trim() : ""
    if (handler) {
      req.handler = handler
    }

    const memoryMB = readPositiveInt(row.memory_mb ?? row.memory)
    if (memoryMB !== undefined) {
      req.memory_mb = memoryMB
    }

    const timeoutS = readPositiveInt(row.timeout_s ?? row.timeout)
    if (timeoutS !== undefined) {
      req.timeout_s = timeoutS
    }

    const minReplicas = readPositiveInt(row.min_replicas ?? row.minReplicas)
    if (minReplicas !== undefined) {
      req.min_replicas = minReplicas
    }

    const maxReplicas = readPositiveInt(row.max_replicas ?? row.maxReplicas)
    if (maxReplicas !== undefined) {
      req.max_replicas = maxReplicas
    }

    const mode = typeof row.mode === "string" ? row.mode.trim() : ""
    if (mode) {
      req.mode = mode
    }

    const envVars = readStringMap(row.env_vars ?? row.envVars)
    if (envVars) {
      req.env_vars = envVars
    }

    if (isJsonObject(row.limits)) {
      req.limits = row.limits as ResourceLimits
    }

    if (isJsonObject(row.network_policy ?? row.networkPolicy)) {
      req.network_policy = (row.network_policy ?? row.networkPolicy) as NetworkPolicy
    }

    if (isJsonObject(row.rollout_policy ?? row.rolloutPolicy)) {
      req.rollout_policy = (row.rollout_policy ?? row.rolloutPolicy) as RolloutPolicy
    }

    items.push(req)
  })

  return { items, invalid }
}

function downloadJSON(filename: string, payload: unknown) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export default function FunctionsPage() {
  const t = useTranslations("pages")
  const router = useRouter()
  const createOpenedByQueryRef = useRef(false)
  const importInputRef = useRef<HTMLInputElement | null>(null)
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [rawFunctions, setRawFunctions] = useState<NovaFunction[]>([])
  const [runtimes, setRuntimes] = useState<RuntimeInfo[]>([])
  const [searchQuery, setSearchQuery] = useState("")
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [runtimeFilter, setRuntimeFilter] = useState<string>("all")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [hasInvocations, setHasInvocations] = useState(false)
  const [hasGatewayRoutes, setHasGatewayRoutes] = useState(false)
  const [selectedFunctionNames, setSelectedFunctionNames] = useState<Set<string>>(new Set())
  const [bulkDeleting, setBulkDeleting] = useState(false)
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)
  const [ioBusy, setIoBusy] = useState(false)
  const [notice, setNotice] = useState<Notice | null>(null)

  // Debounce search query
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  useEffect(() => {
    const initialQuery = readFunctionSearchFromLocation()
    setSearchQuery(initialQuery)
    setDebouncedSearchQuery(initialQuery)
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const shouldOpenCreate = params.get("create") === "1"
    if (!shouldOpenCreate || createOpenedByQueryRef.current) {
      return
    }
    createOpenedByQueryRef.current = true
    setIsCreateOpen(true)
    params.delete("create")
    const qs = params.toString()
    router.replace(qs ? `/functions?${qs}` : "/functions", { scroll: false })
  }, [router])

  useEffect(() => {
    const handleFunctionSearch = (event: Event) => {
      const custom = event as CustomEvent<FunctionSearchDetail>
      const next = custom.detail?.query ?? ""
      setSearchQuery((prev) => (prev === next ? prev : next))
    }

    window.addEventListener(FUNCTION_SEARCH_EVENT, handleFunctionSearch)
    return () => {
      window.removeEventListener(FUNCTION_SEARCH_EVENT, handleFunctionSearch)
    }
  }, [])

  useEffect(() => {
    const current = readFunctionSearchFromLocation()
    const next = debouncedSearchQuery.trim()
    if (current === next) {
      return
    }

    const params = new URLSearchParams(window.location.search)
    if (next) {
      params.set("q", next)
    } else {
      params.delete("q")
    }
    const qs = params.toString()
    router.replace(qs ? `/functions?${qs}` : "/functions", { scroll: false })
    dispatchFunctionSearch(next)
  }, [debouncedSearchQuery, router])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const [funcs, metrics, rts, routes] = await Promise.all([
        functionsApi.list(debouncedSearchQuery),
        metricsApi.global(),
        runtimesApi.list(),
        gatewayApi.listRoutes().catch(() => []),
      ])

      // Transform functions with their metrics
      const transformedFuncs = funcs.map((fn) => {
        const funcMetrics = metrics.functions?.[fn.id]
        return transformFunction(fn, funcMetrics ? {
          function_id: fn.id,
          function_name: fn.name,
          invocations: funcMetrics,
          pool: { active_vms: 0, busy_vms: 0, idle_vms: 0 },
        } : undefined)
      })

      setFunctions(transformedFuncs)
      setRawFunctions(funcs)
      setRuntimes(rts.map(transformRuntime))
      const nextHasInvocations = (metrics.invocations?.total || 0) > 0
      const nextHasGatewayRoutes = (routes?.length || 0) > 0
      setHasInvocations(nextHasInvocations)
      setHasGatewayRoutes(nextHasGatewayRoutes)
      syncOnboardingStateFromData({
        hasFunctionCreated: transformedFuncs.length > 0,
        hasFunctionInvoked: nextHasInvocations,
        hasGatewayRouteCreated: nextHasGatewayRoutes,
      })
    } catch (err) {
      console.error("Failed to fetch functions:", err)
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [debouncedSearchQuery])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredFunctions = useMemo(
    () =>
      functions.filter((fn) => {
        const matchesStatus = statusFilter === "all" || fn.status === statusFilter
        const matchesRuntime =
          runtimeFilter === "all" || fn.runtime.toLowerCase().includes(runtimeFilter.toLowerCase())
        return matchesStatus && matchesRuntime
      }),
    [functions, statusFilter, runtimeFilter]
  )

  const uniqueRuntimes = useMemo(
    () => [...new Set(runtimes.map((r) => r.name.split(" ")[0]))],
    [runtimes]
  )

  useEffect(() => {
    setSelectedFunctionNames((prev) => {
      const valid = new Set(filteredFunctions.map((fn) => fn.name))
      const next = new Set<string>()
      prev.forEach((name) => {
        if (valid.has(name)) {
          next.add(name)
        }
      })
      if (next.size === prev.size) {
        let same = true
        prev.forEach((name) => {
          if (!next.has(name)) {
            same = false
          }
        })
        if (same) {
          return prev
        }
      }
      return next
    })
    setConfirmBulkDelete((prev) => (prev ? false : prev))
  }, [filteredFunctions])

  useEffect(() => {
    setPage(1)
  }, [searchQuery, statusFilter, runtimeFilter])

  const totalPages = Math.max(1, Math.ceil(filteredFunctions.length / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const pagedFunctions = filteredFunctions.slice((page - 1) * pageSize, page * pageSize)

  const handleCreate = async (
    name: string,
    runtime: string,
    handler: string,
    memory: number,
    timeout: number,
    code: string,
    limits?: ResourceLimits,
    networkPolicy?: NetworkPolicy,
    dependencyFiles?: Record<string, string>
  ) => {
    try {
      await functionsApi.create({
        name,
        runtime,
        handler,
        code,
        memory_mb: memory,
        timeout_s: timeout,
        limits,
        network_policy: networkPolicy,
        dependency_files: dependencyFiles,
      })
      markOnboardingStep("function_created", true)
      setIsCreateOpen(false)
      fetchData() // Refresh the list
    } catch (err) {
      console.error("Failed to create function:", err)
      throw err
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await functionsApi.delete(name)
      setSelectedFunctionNames((prev) => {
        const next = new Set(prev)
        next.delete(name)
        return next
      })
      fetchData() // Refresh the list
    } catch (err) {
      console.error("Failed to delete function:", err)
      setError(toUserErrorMessage(err))
    }
  }

  const toggleFunctionSelect = (name: string, checked: boolean) => {
    setSelectedFunctionNames((prev) => {
      const next = new Set(prev)
      if (checked) {
        next.add(name)
      } else {
        next.delete(name)
      }
      return next
    })
    setConfirmBulkDelete(false)
  }

  const toggleFunctionSelectAll = (checked: boolean, names: string[]) => {
    setSelectedFunctionNames((prev) => {
      const next = new Set(prev)
      if (checked) {
        names.forEach((name) => next.add(name))
      } else {
        names.forEach((name) => next.delete(name))
      }
      return next
    })
    setConfirmBulkDelete(false)
  }

  const handleBulkDelete = async () => {
    const targets = Array.from(selectedFunctionNames)
    if (targets.length === 0) return
    if (!confirmBulkDelete) {
      setConfirmBulkDelete(true)
      return
    }

    try {
      setBulkDeleting(true)
      const results = await Promise.allSettled(targets.map((name) => functionsApi.delete(name)))
      const failed = results.filter((result) => result.status === "rejected")
      if (failed.length > 0) {
        setError(`Bulk delete completed with ${failed.length} failures. Please review and retry.`)
      } else {
        setError(null)
      }
      setSelectedFunctionNames(new Set())
      setConfirmBulkDelete(false)
      await fetchData()
    } finally {
      setBulkDeleting(false)
    }
  }

  const handleExportFunctions = async () => {
    const targets = selectedFunctionNames.size > 0
      ? Array.from(selectedFunctionNames)
      : filteredFunctions.map((fn) => fn.name)
    if (targets.length === 0) {
      setNotice({ kind: "info", text: "No functions available for export." })
      return
    }

    try {
      setIoBusy(true)
      setError(null)
      setNotice(null)

      const rawByName = new Map(rawFunctions.map((fn) => [fn.name, fn]))
      const codeResults = await Promise.allSettled(targets.map((name) => functionsApi.getCode(name)))
      const failedReads = codeResults
        .map((result, index) => (result.status === "rejected" ? targets[index] : ""))
        .filter(Boolean)

      if (failedReads.length > 0) {
        throw new Error(`Failed to read source code for: ${failedReads.join(", ")}`)
      }

      const rows: CreateFunctionRequest[] = []
      for (let i = 0; i < targets.length; i += 1) {
        const name = targets[i]
        const fn = rawByName.get(name)
        if (!fn) {
          continue
        }
        const codeResult = codeResults[i]
        const code =
          codeResult.status === "fulfilled"
            ? (codeResult.value.source_code || fn.source_code || "")
            : ""
        if (!code.trim()) {
          throw new Error(`Function ${name} has no source code and cannot be exported`)
        }

        rows.push({
          name: fn.name,
          runtime: fn.runtime,
          handler: fn.handler,
          code,
          memory_mb: fn.memory_mb,
          timeout_s: fn.timeout_s,
          min_replicas: fn.min_replicas,
          max_replicas: fn.max_replicas,
          mode: fn.mode,
          env_vars: fn.env_vars,
          limits: fn.limits,
          network_policy: fn.network_policy,
          rollout_policy: fn.rollout_policy,
        })
      }

      if (rows.length === 0) {
        setNotice({ kind: "info", text: "No functions available for export." })
        return
      }

      const ts = new Date().toISOString().replace(/[:.]/g, "-")
      downloadJSON(`nova-functions-${ts}.json`, {
        kind: "nova.functions.export",
        version: 1,
        exported_at: new Date().toISOString(),
        count: rows.length,
        functions: rows,
      })
      setNotice({ kind: "success", text: `Exported ${rows.length} function(s).` })
    } catch (err) {
      const message = toUserErrorMessage(err)
      setError(message)
      setNotice({ kind: "error", text: message })
    } finally {
      setIoBusy(false)
    }
  }

  const handleImportFunctions = async (input: HTMLInputElement) => {
    const file = input.files?.[0]
    input.value = ""
    if (!file) {
      return
    }

    try {
      setIoBusy(true)
      setError(null)
      setNotice(null)

      const rawText = await file.text()
      const parsed: unknown = JSON.parse(rawText)
      const { items, invalid } = parseFunctionImportPayload(parsed)

      if (items.length === 0) {
        setError("No valid function definitions were found in the import file.")
        setNotice({ kind: "error", text: "Import failed: invalid file format or missing required fields." })
        return
      }

      const results = await Promise.allSettled(items.map((item) => functionsApi.create(item)))
      const failed = results.filter((result) => result.status === "rejected").length
      const succeeded = results.length - failed
      const invalidSuffix = invalid > 0 ? `, skipped ${invalid} invalid record(s)` : ""

      setSelectedFunctionNames(new Set())
      await fetchData()

      if (failed > 0) {
        setNotice({
          kind: "error",
          text: `Import finished: ${succeeded} succeeded, ${failed} failed${invalidSuffix}.`,
        })
      } else {
        setNotice({
          kind: "success",
          text: `Import succeeded: ${succeeded} function(s)${invalidSuffix}.`,
        })
      }

      if (succeeded > 0) {
        markOnboardingStep("function_created", true)
      }
    } catch (err) {
      const message = toUserErrorMessage(err)
      setError(message)
      setNotice({ kind: "error", text: `Import failed: ${message}` })
    } finally {
      setIoBusy(false)
    }
  }

  const noFunctions =
    !loading &&
    functions.length === 0 &&
    !searchQuery.trim() &&
    statusFilter === "all" &&
    runtimeFilter === "all"
  const noFilterResult = !loading && functions.length > 0 && filteredFunctions.length === 0

  return (
    <DashboardLayout>
      <Header
        title={t("functions.title")}
        description={t("functions.description")}
      />

      <div className="p-6 space-y-6">
        <OnboardingFlow
          hasFunctionCreated={functions.length > 0}
          hasFunctionInvoked={hasInvocations}
          hasGatewayRouteCreated={hasGatewayRoutes}
          onCreateFunction={() => setIsCreateOpen(true)}
        />

        {error && (
          <ErrorBanner error={error} title="Operation Result" onRetry={fetchData} />
        )}

        {notice && (
          <div
            className={`rounded-lg border p-4 text-sm ${
              notice.kind === "success"
                ? "border-success/50 bg-success/10 text-success"
                : notice.kind === "error"
                  ? "border-destructive/50 bg-destructive/10 text-destructive"
                  : "border-primary/40 bg-primary/10 text-primary"
            }`}
          >
            <div className="flex items-center justify-between gap-3">
              <p>{notice.text}</p>
              <Button variant="ghost" size="sm" onClick={() => setNotice(null)}>
                Dismiss
              </Button>
            </div>
          </div>
        )}

        {/* Actions Bar */}
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-1 items-center gap-3">
            <div className="relative flex-1 max-w-sm">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="search"
                placeholder="Search functions..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>

            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-32">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="inactive">Inactive</SelectItem>
                <SelectItem value="error">Error</SelectItem>
              </SelectContent>
            </Select>

            <Select value={runtimeFilter} onValueChange={setRuntimeFilter}>
              <SelectTrigger className="w-36">
                <SelectValue placeholder="Runtime" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Runtimes</SelectItem>
                {uniqueRuntimes.map((runtime) => (
                  <SelectItem key={runtime} value={runtime}>
                    {runtime}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center gap-2">
            <input
              ref={importInputRef}
              type="file"
              accept=".json,application/json"
              className="hidden"
              onChange={(event) => {
                void handleImportFunctions(event.target)
              }}
            />
            <Button
              variant="outline"
              onClick={handleExportFunctions}
              disabled={loading || ioBusy || filteredFunctions.length === 0}
            >
              <Download className="mr-2 h-4 w-4" />
              {selectedFunctionNames.size > 0 ? `Export Selected (${selectedFunctionNames.size})` : "Export Filtered"}
            </Button>
            <Button
              variant="outline"
              onClick={() => importInputRef.current?.click()}
              disabled={loading || ioBusy}
            >
              <Upload className="mr-2 h-4 w-4" />
              Import JSON
            </Button>
            <Button variant="outline" onClick={fetchData} disabled={loading || ioBusy}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button onClick={() => setIsCreateOpen(true)} disabled={ioBusy}>
              <Plus className="mr-2 h-4 w-4" />
              Create Function
            </Button>
          </div>
        </div>

        {selectedFunctionNames.size > 0 && (
          <div className="rounded-lg border border-border bg-card p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground">
                Selected <span className="font-medium text-foreground">{selectedFunctionNames.size}</span> function(s)
              </p>
              <div className="flex items-center gap-2">
                {confirmBulkDelete ? (
                  <>
                    <Button
                      size="sm"
                      variant="destructive"
                      onClick={handleBulkDelete}
                      disabled={bulkDeleting}
                    >
                      {bulkDeleting ? "Deleting..." : "Confirm Bulk Delete"}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setConfirmBulkDelete(false)}
                      disabled={bulkDeleting}
                    >
                      Cancel
                    </Button>
                  </>
                ) : (
                  <Button
                    size="sm"
                    variant="destructive"
                    onClick={handleBulkDelete}
                    disabled={bulkDeleting}
                  >
                    Bulk Delete
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setSelectedFunctionNames(new Set())}
                  disabled={bulkDeleting}
                >
                  Clear Selection
                </Button>
              </div>
            </div>
          </div>
        )}

        {/* Functions Table */}
        {noFunctions ? (
          <EmptyState
            title="No Functions Yet"
            description="Create your first function, then invoke it or bind it to the gateway."
            primaryAction={{ label: "Create Function", onClick: () => setIsCreateOpen(true) }}
            secondaryAction={{ label: "View Docs", href: "/docs/installation" }}
          />
        ) : noFilterResult ? (
          <EmptyState
            title="No Matching Functions"
            description="No results match the current filters. Try clearing them."
            primaryAction={{
              label: "Clear Filters",
              onClick: () => {
                setSearchQuery("")
                setStatusFilter("all")
                setRuntimeFilter("all")
              },
            }}
          />
        ) : (
          <FunctionsTable
            functions={pagedFunctions}
            onDelete={handleDelete}
            loading={loading}
            selectedNames={selectedFunctionNames}
            onToggleSelect={toggleFunctionSelect}
            onToggleSelectAll={toggleFunctionSelectAll}
          />
        )}

        {!loading && filteredFunctions.length > 0 && (
          <Pagination
            totalItems={filteredFunctions.length}
            page={page}
            pageSize={pageSize}
            onPageChange={setPage}
            onPageSizeChange={(size) => {
              setPageSize(size)
              setPage(1)
            }}
            itemLabel="functions"
            className="rounded-xl border border-border bg-card p-4"
          />
        )}

        {/* Create Dialog */}
        <CreateFunctionDialog
          open={isCreateOpen}
          onOpenChange={setIsCreateOpen}
          onCreate={handleCreate}
          runtimes={runtimes}
        />
      </div>
    </DashboardLayout>
  )
}

"use client"

import { useEffect, useState, useCallback } from "react"
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
import { functionsApi, gatewayApi, metricsApi, runtimesApi, type NetworkPolicy, type ResourceLimits } from "@/lib/api"
import { transformFunction, FunctionData, RuntimeInfo, transformRuntime } from "@/lib/types"
import {
  FUNCTION_SEARCH_EVENT,
  type FunctionSearchDetail,
  dispatchFunctionSearch,
  readFunctionSearchFromLocation,
} from "@/lib/function-search"
import { markOnboardingStep, syncOnboardingStateFromData } from "@/lib/onboarding-state"
import { Plus, Search, Filter, RefreshCw } from "lucide-react"

export default function FunctionsPage() {
  const router = useRouter()
  const [functions, setFunctions] = useState<FunctionData[]>([])
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
      setError(err instanceof Error ? err.message : "Failed to load functions")
    } finally {
      setLoading(false)
    }
  }, [debouncedSearchQuery])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredFunctions = functions.filter((fn) => {
    const matchesStatus = statusFilter === "all" || fn.status === statusFilter
    const matchesRuntime = runtimeFilter === "all" || fn.runtime.toLowerCase().includes(runtimeFilter.toLowerCase())
    return matchesStatus && matchesRuntime
  })

  const uniqueRuntimes = [...new Set(runtimes.map((r) => r.name.split(" ")[0]))]

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
    networkPolicy?: NetworkPolicy
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
      fetchData() // Refresh the list
    } catch (err) {
      console.error("Failed to delete function:", err)
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title="Functions" description="Manage and monitor your serverless functions" />
        <div className="p-6">
          <ErrorBanner error={error} title="加载函数失败" onRetry={fetchData} />
        </div>
      </DashboardLayout>
    )
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
        title="Functions"
        description="Manage and monitor your serverless functions"
      />

      <div className="p-6 space-y-6">
        <OnboardingFlow
          hasFunctionCreated={functions.length > 0}
          hasFunctionInvoked={hasInvocations}
          hasGatewayRouteCreated={hasGatewayRoutes}
          onCreateFunction={() => setIsCreateOpen(true)}
        />

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
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button onClick={() => setIsCreateOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create Function
            </Button>
          </div>
        </div>

        {/* Functions Table */}
        {noFunctions ? (
          <EmptyState
            title="还没有函数"
            description="先创建第一个函数，然后可以直接调用并挂到网关。"
            primaryAction={{ label: "创建函数", onClick: () => setIsCreateOpen(true) }}
            secondaryAction={{ label: "查看文档", href: "/docs/installation" }}
          />
        ) : noFilterResult ? (
          <EmptyState
            title="没有匹配的函数"
            description="当前筛选条件下没有结果，试试清空筛选。"
            primaryAction={{
              label: "清空筛选",
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

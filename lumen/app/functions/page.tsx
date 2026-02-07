"use client"

import { useEffect, useState, useCallback } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { FunctionsTable } from "@/components/functions-table"
import { Pagination } from "@/components/pagination"
import { CreateFunctionDialog } from "@/components/create-function-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { functionsApi, metricsApi, runtimesApi, type ResourceLimits } from "@/lib/api"
import { transformFunction, FunctionData, RuntimeInfo, transformRuntime } from "@/lib/types"
import { Plus, Search, Filter, RefreshCw } from "lucide-react"

export default function FunctionsPage() {
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [runtimes, setRuntimes] = useState<RuntimeInfo[]>([])
  const [searchQuery, setSearchQuery] = useState("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [runtimeFilter, setRuntimeFilter] = useState<string>("all")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const [funcs, metrics, rts] = await Promise.all([
        functionsApi.list(),
        metricsApi.global(),
        runtimesApi.list(),
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
    } catch (err) {
      console.error("Failed to fetch functions:", err)
      setError(err instanceof Error ? err.message : "Failed to load functions")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredFunctions = functions.filter((fn) => {
    const matchesSearch = fn.name.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesStatus = statusFilter === "all" || fn.status === statusFilter
    const matchesRuntime = runtimeFilter === "all" || fn.runtime.toLowerCase().includes(runtimeFilter.toLowerCase())
    return matchesSearch && matchesStatus && matchesRuntime
  })

  const uniqueRuntimes = [...new Set(functions.map((fn) => fn.runtime.split(" ")[0]))]

  useEffect(() => {
    setPage(1)
  }, [searchQuery, statusFilter, runtimeFilter])

  const totalPages = Math.max(1, Math.ceil(filteredFunctions.length / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const pagedFunctions = filteredFunctions.slice((page - 1) * pageSize, page * pageSize)

  const handleCreate = async (name: string, runtime: string, handler: string, memory: number, timeout: number, code: string, limits?: ResourceLimits) => {
    try {
      await functionsApi.create({
        name,
        runtime,
        handler,
        code,
        memory_mb: memory,
        timeout_s: timeout,
        limits,
      })
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
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load functions</p>
            <p className="text-sm mt-1">{error}</p>
            <p className="text-sm mt-2 text-muted-foreground">
              Make sure the nova backend is running on port 9000
            </p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header
        title="Functions"
        description="Manage and monitor your serverless functions"
      />

      <div className="p-6 space-y-6">
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

        {/* Summary Stats */}
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Total Functions</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : functions.length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Active</p>
            <p className="text-2xl font-semibold text-success">
              {loading ? "..." : functions.filter((f) => f.status === "active").length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Errors</p>
            <p className="text-2xl font-semibold text-destructive">
              {loading ? "..." : functions.filter((f) => f.status === "error").length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Total Invocations</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : functions.reduce((sum, fn) => sum + fn.invocations, 0).toLocaleString()}
            </p>
          </div>
        </div>

        {/* Functions Table */}
        <FunctionsTable
          functions={pagedFunctions}
          onDelete={handleDelete}
          loading={loading}
        />

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

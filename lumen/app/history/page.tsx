"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { Pagination } from "@/components/pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { ErrorBanner } from "@/components/ui/error-banner"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { functionsApi, invocationsApi } from "@/lib/api"
import { transformFunction, FunctionData } from "@/lib/types"
import {
  RefreshCw,
  Search,
  Filter,
  CheckCircle,
  XCircle,
  Clock,
  ExternalLink,
  Zap,
  Snowflake,
  Flame,
  RotateCcw,
  Loader2,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { toUserErrorMessage } from "@/lib/error-map"

interface InvocationRecord {
  id: string
  functionId: string
  functionName: string
  timestamp: string
  status: "success" | "failed"
  duration: number
  coldStart: boolean
  input?: string
  output?: string
  inputTitle?: string
  outputTitle?: string
}

export default function HistoryPage() {
  const tp = useTranslations("pages")
  const th = useTranslations("history")
  const tc = useTranslations("common")
  const [invocations, setInvocations] = useState<InvocationRecord[]>([])
  const [functions, setFunctions] = useState<FunctionData[]>([])
  const [searchQuery, setSearchQuery] = useState("")
  const [statusFilter, setStatusFilter] = useState<string>("all")
  const [functionFilter, setFunctionFilter] = useState<string>("all")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [replayingId, setReplayingId] = useState<string | null>(null)
  const [replayResult, setReplayResult] = useState<Record<string, string>>({})

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const [funcs, logs] = await Promise.all([
        functionsApi.list(),
        invocationsApi.list(200),
      ])

      // Transform functions
      const transformedFuncs = funcs.map((fn) => transformFunction(fn))
      setFunctions(transformedFuncs)

      // Transform logs to invocation records
      const formatPayload = (payload: unknown) => {
        if (payload === undefined) return ""
        try {
          return JSON.stringify(payload)
        } catch {
          return String(payload)
        }
      }

      const prettyPayload = (payload: unknown) => {
        if (payload === undefined) return ""
        try {
          return JSON.stringify(payload, null, 2)
        } catch {
          return String(payload)
        }
      }

      const records: InvocationRecord[] = logs.map((log) => {
        const input = formatPayload(log.input)
        const output = formatPayload(log.output)

        return {
          id: log.id,
          functionId: log.function_id,
          functionName: log.function_name,
          timestamp: log.created_at,
          status: log.success ? "success" : "failed",
          duration: log.duration_ms,
          coldStart: log.cold_start,
          input,
          output,
          inputTitle: input ? prettyPayload(log.input) : th("noInputCaptured"),
          outputTitle: output ? prettyPayload(log.output) : th("noOutputCaptured"),
        }
      })

      setInvocations(records)
    } catch (err) {
      console.error("Failed to fetch history:", err)
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleReplay = async (inv: InvocationRecord) => {
    try {
      setReplayingId(inv.id)
      let payload: unknown = {}
      if (inv.input && inv.input !== "-") {
        try {
          payload = JSON.parse(inv.input)
        } catch {
          payload = {}
        }
      }
      const result = await functionsApi.invoke(inv.functionName, payload)
      setReplayResult((prev) => ({
        ...prev,
        [inv.id]: `OK ${result.duration_ms}ms`,
      }))
      fetchData()
    } catch (err) {
      setReplayResult((prev) => ({
        ...prev,
        [inv.id]: err instanceof Error ? err.message : "Failed",
      }))
    } finally {
      setReplayingId(null)
    }
  }

  const filteredInvocations = invocations.filter((inv) => {
    const matchesSearch =
      inv.functionName.toLowerCase().includes(searchQuery.toLowerCase()) ||
      inv.id.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesStatus = statusFilter === "all" || inv.status === statusFilter
    const matchesFunction =
      functionFilter === "all" || inv.functionName === functionFilter
    return matchesSearch && matchesStatus && matchesFunction
  })

  useEffect(() => {
    setPage(1)
  }, [searchQuery, statusFilter, functionFilter])

  const totalPages = Math.max(1, Math.ceil(filteredInvocations.length / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const pagedInvocations = filteredInvocations.slice((page - 1) * pageSize, page * pageSize)

  const formatTimestamp = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleString()
  }

  const totalInvocations = invocations.length
  const successCount = invocations.filter((i) => i.status === "success").length
  const failedCount = invocations.filter((i) => i.status === "failed").length
  const coldStartCount = invocations.filter((i) => i.coldStart).length
  const avgDuration =
    invocations.length > 0
      ? Math.round(
          invocations.reduce((sum, i) => sum + i.duration, 0) / invocations.length
        )
      : 0

  if (error) {
    return (
      <DashboardLayout>
        <Header title={tp("history.title")} description={tp("history.description")} />
        <div className="p-6">
          <ErrorBanner error={error} title={th("failedToLoad")} onRetry={fetchData} />
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={tp("history.title")} description={tp("history.description")} />

      <div className="p-6 space-y-6">
        {/* Filters */}
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-1 items-center gap-3">
            <div className="relative flex-1 max-w-sm">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="search"
                placeholder={th("searchPlaceholder")}
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
                <SelectItem value="all">{th("allStatus")}</SelectItem>
                <SelectItem value="success">{th("success")}</SelectItem>
                <SelectItem value="failed">{th("failed")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={functionFilter} onValueChange={setFunctionFilter}>
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Function" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{th("allFunctions")}</SelectItem>
                {functions.map((fn) => (
                  <SelectItem key={fn.id} value={fn.name}>
                    {fn.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Button variant="outline" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
          <div className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2">
              <Zap className="h-4 w-4 text-primary" />
              <p className="text-sm text-muted-foreground">{th("totalInvocations")}</p>
            </div>
            <p className="text-2xl font-semibold text-foreground mt-1">
              {loading ? "..." : totalInvocations}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2">
              <CheckCircle className="h-4 w-4 text-success" />
              <p className="text-sm text-muted-foreground">{th("successful")}</p>
            </div>
            <p className="text-2xl font-semibold text-success mt-1">
              {loading ? "..." : successCount}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2">
              <XCircle className="h-4 w-4 text-destructive" />
              <p className="text-sm text-muted-foreground">{th("failed")}</p>
            </div>
            <p className="text-2xl font-semibold text-destructive mt-1">
              {loading ? "..." : failedCount}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2">
              <Snowflake className="h-4 w-4 text-blue-500" />
              <p className="text-sm text-muted-foreground">{th("coldStarts")}</p>
            </div>
            <p className="text-2xl font-semibold text-blue-500 mt-1">
              {loading ? "..." : coldStartCount}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-primary" />
              <p className="text-sm text-muted-foreground">{th("avgDuration")}</p>
            </div>
            <p className="text-2xl font-semibold text-foreground mt-1">
              {loading ? "..." : `${avgDuration}ms`}
            </p>
          </div>
        </div>

        {/* Invocations Table */}
        {!loading && invocations.length === 0 ? (
          <EmptyState
            title={th("noRecords")}
            description={th("noRecordsDesc")}
            primaryAction={{ label: th("goToFunctions"), href: "/functions" }}
          />
        ) : (
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colStatus")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colTimestamp")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colFunction")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colRequestId")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colDuration")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colColdStart")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colInput")}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    {th("colOutput")}
                  </th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">
                    {th("colActions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td colSpan={9} className="px-4 py-3">
                        <div className="h-4 bg-muted rounded animate-pulse" />
                      </td>
                    </tr>
                  ))
                ) : filteredInvocations.length === 0 ? (
                  <tr>
                    <td
                      colSpan={9}
                      className="px-4 py-8 text-center text-muted-foreground"
                    >
                      {th("noInvocations")}
                    </td>
                  </tr>
                ) : (
                  pagedInvocations.map((inv) => (
                    <tr
                      key={inv.id}
                      className="border-b border-border hover:bg-muted/50"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          {inv.status === "success" ? (
                            <CheckCircle className="h-4 w-4 text-success" />
                          ) : (
                            <XCircle className="h-4 w-4 text-destructive" />
                          )}
                          <Badge
                            variant="secondary"
                            className={cn(
                              "text-xs",
                              inv.status === "success"
                                ? "bg-success/10 text-success border-0"
                                : "bg-destructive/10 text-destructive border-0"
                            )}
                          >
                            {inv.status}
                          </Badge>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground whitespace-nowrap">
                        {formatTimestamp(inv.timestamp)}
                      </td>
                      <td className="px-4 py-3">
                        <Link
                          href={`/functions/${inv.functionName}`}
                          className="text-sm font-medium text-primary hover:underline"
                        >
                          {inv.functionName}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <code className="text-xs text-muted-foreground bg-muted px-2 py-1 rounded">
                          {inv.id.slice(0, 8)}...
                        </code>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground whitespace-nowrap">
                        {inv.duration}ms
                      </td>
                      <td className="px-4 py-3">
                        {inv.coldStart ? (
                          <Badge variant="secondary" className="text-xs bg-blue-500/10 text-blue-500 border-0">
                            <Snowflake className="h-3 w-3 mr-1" />
                            {th("cold")}
                          </Badge>
                        ) : (
                          <Badge variant="secondary" className="text-xs bg-orange-500/10 text-orange-500 border-0">
                            <Flame className="h-3 w-3 mr-1" />
                            {th("warm")}
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className="block max-w-[220px] truncate font-mono text-xs text-muted-foreground"
                          title={inv.inputTitle}
                        >
                          {inv.input || "-"}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className="block max-w-[220px] truncate font-mono text-xs text-muted-foreground"
                          title={inv.outputTitle}
                        >
                          {inv.output || "-"}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleReplay(inv)}
                            disabled={replayingId === inv.id}
                            title="Replay invocation"
                          >
                            {replayingId === inv.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <RotateCcw className="h-4 w-4" />
                            )}
                          </Button>
                          {replayResult[inv.id] && (
                            <span className="text-xs text-muted-foreground max-w-[100px] truncate">
                              {replayResult[inv.id]}
                            </span>
                          )}
                          <Button variant="ghost" size="sm" asChild>
                            <Link href={`/functions/${inv.functionName}`}>
                              <ExternalLink className="h-4 w-4" />
                            </Link>
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {!loading && filteredInvocations.length > 0 && (
            <div className="border-t border-border p-4">
              <Pagination
                totalItems={filteredInvocations.length}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onPageSizeChange={(size) => {
                  setPageSize(size)
                  setPage(1)
                }}
                itemLabel="invocations"
              />
            </div>
          )}
        </div>
        )}
      </div>
    </DashboardLayout>
  )
}

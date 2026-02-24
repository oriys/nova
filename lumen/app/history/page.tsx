"use client"

import { Fragment, useEffect, useState, useCallback } from "react"
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
import { functionsApi, invocationsApi, InvocationListSummary } from "@/lib/api"
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
  ChevronDown,
  ChevronUp,
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
  inputDetail?: string
  outputDetail?: string
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
  const [totalInvocations, setTotalInvocations] = useState(0)
  const [invocationSummary, setInvocationSummary] = useState<InvocationListSummary>({
    total_invocations: 0,
    successes: 0,
    failures: 0,
    cold_starts: 0,
    avg_duration_ms: 0,
  })
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [replayingId, setReplayingId] = useState<string | null>(null)
  const [replayResult, setReplayResult] = useState<Record<string, string>>({})
  const [expandedInvocationId, setExpandedInvocationId] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      const offset = (page - 1) * pageSize
      const [funcPage, logsPage] = await Promise.all([
        functionsApi.listPage(undefined, 500, 0),
        invocationsApi.listPage(pageSize, offset, {
          search: searchQuery || undefined,
          functionName: functionFilter === "all" ? undefined : functionFilter,
          status: statusFilter === "all" ? undefined : (statusFilter as "success" | "failed"),
        }),
      ])

      // Transform functions
      const transformedFuncs = funcPage.items.map((fn) => transformFunction(fn))
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

      const records: InvocationRecord[] = logsPage.items.map((log) => {
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
          inputDetail: input ? prettyPayload(log.input) : th("noInputCaptured"),
          outputDetail: output ? prettyPayload(log.output) : th("noOutputCaptured"),
        }
      })

      setInvocations(records)
      setTotalInvocations(logsPage.total)
      setInvocationSummary(logsPage.summary)
    } catch (err) {
      console.error("Failed to fetch history:", err)
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [functionFilter, page, pageSize, searchQuery, statusFilter, th])

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
        [inv.id]: th("replayOk", { duration: result.duration_ms }),
      }))
      fetchData()
    } catch (err) {
      setReplayResult((prev) => ({
        ...prev,
        [inv.id]: err instanceof Error ? err.message : th("replayFailed"),
      }))
    } finally {
      setReplayingId(null)
    }
  }

  useEffect(() => {
    setPage(1)
  }, [searchQuery, statusFilter, functionFilter])

  const totalPages = Math.max(1, Math.ceil(totalInvocations / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  useEffect(() => {
    if (!expandedInvocationId) return
    const exists = invocations.some((inv) => inv.id === expandedInvocationId)
    if (!exists) {
      setExpandedInvocationId(null)
    }
  }, [expandedInvocationId, invocations])

  const formatTimestamp = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleString()
  }

  const handleRowToggle = useCallback((invocationId: string) => {
    setExpandedInvocationId((current) => (current === invocationId ? null : invocationId))
  }, [])

  const successCount = invocationSummary.successes
  const failedCount = invocationSummary.failures
  const coldStartCount = invocationSummary.cold_starts
  const avgDuration = invocationSummary.avg_duration_ms

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
                <SelectValue placeholder={th("statusPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{th("allStatus")}</SelectItem>
                <SelectItem value="success">{th("success")}</SelectItem>
                <SelectItem value="failed">{th("failed")}</SelectItem>
              </SelectContent>
            </Select>

            <Select value={functionFilter} onValueChange={setFunctionFilter}>
              <SelectTrigger className="w-40">
                <SelectValue placeholder={th("functionPlaceholder")} />
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
              {loading ? "..." : invocationSummary.total_invocations}
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
        {!loading && totalInvocations === 0 ? (
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
                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">
                    {th("colActions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td colSpan={7} className="px-4 py-3">
                        <div className="h-4 bg-muted rounded animate-pulse" />
                      </td>
                    </tr>
                  ))
                ) : invocations.length === 0 ? (
                  <tr>
                    <td
                      colSpan={7}
                      className="px-4 py-8 text-center text-muted-foreground"
                    >
                      {th("noInvocations")}
                    </td>
                  </tr>
                ) : (
                  invocations.map((inv) => {
                    const expanded = expandedInvocationId === inv.id

                    return (
                      <Fragment key={inv.id}>
                        <tr
                          className={cn("border-b border-border hover:bg-muted/50", expanded && "bg-muted/30")}
                          onClick={() => handleRowToggle(inv.id)}
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
                                {inv.status === "success" ? th("success") : th("failed")}
                              </Badge>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-muted-foreground whitespace-nowrap">
                            {formatTimestamp(inv.timestamp)}
                          </td>
                          <td className="px-4 py-3">
                            <Link
                              href={`/functions/${encodeURIComponent(inv.functionName)}`}
                              className="text-sm font-medium text-primary hover:underline"
                              onClick={(e) => e.stopPropagation()}
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
                            {th("durationMs", { duration: inv.duration })}
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
                          <td className="px-4 py-3 text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  handleRowToggle(inv.id)
                                }}
                                title={th("view")}
                                aria-label={th("view")}
                              >
                                {expanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  void handleReplay(inv)
                                }}
                                disabled={replayingId === inv.id}
                                title={th("replayInvocation")}
                              >
                                {replayingId === inv.id ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <RotateCcw className="h-4 w-4" />
                                )}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                asChild
                              >
                                <Link
                                  href={`/functions/${encodeURIComponent(inv.functionName)}`}
                                  onClick={(e) => e.stopPropagation()}
                                >
                                  <ExternalLink className="h-4 w-4" />
                                </Link>
                              </Button>
                            </div>
                          </td>
                        </tr>
                        {expanded && (
                          <tr className="border-b border-border bg-muted/20">
                            <td colSpan={7} className="px-4 pb-4">
                              <div className="rounded-lg border border-border bg-card p-4">
                                <div className="flex flex-wrap items-center justify-between gap-2">
                                  <div className="flex items-center gap-2">
                                    <h3 className="text-base font-semibold text-foreground">
                                      {th("invocationDetail")}
                                    </h3>
                                    <Badge
                                      variant="secondary"
                                      className={cn(
                                        "text-xs",
                                        inv.status === "success"
                                          ? "bg-success/10 text-success border-0"
                                          : "bg-destructive/10 text-destructive border-0"
                                      )}
                                    >
                                      {inv.status === "success" ? th("success") : th("failed")}
                                    </Badge>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    {replayResult[inv.id] && (
                                      <span className="text-xs text-muted-foreground">{replayResult[inv.id]}</span>
                                    )}
                                    <Button variant="outline" size="sm" asChild>
                                      <Link href={`/functions/${encodeURIComponent(inv.functionName)}`}>
                                        <ExternalLink className="mr-2 h-4 w-4" />
                                        {th("openFunction")}
                                      </Link>
                                    </Button>
                                  </div>
                                </div>

                                <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colRequestId")}
                                    </div>
                                    <div className="mt-1 break-all font-mono text-xs text-foreground">{inv.id}</div>
                                  </div>
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colFunction")}
                                    </div>
                                    <div className="mt-1 text-sm text-foreground">{inv.functionName}</div>
                                  </div>
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colTimestamp")}
                                    </div>
                                    <div className="mt-1 text-sm text-foreground">{formatTimestamp(inv.timestamp)}</div>
                                  </div>
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colDuration")}
                                    </div>
                                    <div className="mt-1 text-sm text-foreground">
                                      {th("durationMs", { duration: inv.duration })}
                                    </div>
                                  </div>
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colColdStart")}
                                    </div>
                                    <div className="mt-1 text-sm text-foreground">
                                      {inv.coldStart ? th("cold") : th("warm")}
                                    </div>
                                  </div>
                                  <div>
                                    <div className="text-xs uppercase tracking-wide text-muted-foreground">
                                      {th("colStatus")}
                                    </div>
                                    <div className="mt-1 text-sm text-foreground">
                                      {inv.status === "success" ? th("success") : th("failed")}
                                    </div>
                                  </div>
                                </div>

                                <div className="mt-4 grid gap-3 lg:grid-cols-2">
                                  <div>
                                    <div className="mb-1 text-sm font-medium text-foreground">{th("colInput")}</div>
                                    <textarea
                                      className="h-52 w-full resize-y overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                                      defaultValue={inv.inputDetail}
                                      spellCheck={false}
                                      wrap="off"
                                    />
                                  </div>
                                  <div>
                                    <div className="mb-1 text-sm font-medium text-foreground">{th("colOutput")}</div>
                                    <textarea
                                      className="h-52 w-full resize-y overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                                      defaultValue={inv.outputDetail}
                                      spellCheck={false}
                                      wrap="off"
                                    />
                                  </div>
                                </div>
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    )
                  })
                )}
              </tbody>
            </table>
          </div>

          {!loading && totalInvocations > 0 && (
            <div className="border-t border-border p-4">
              <Pagination
                totalItems={totalInvocations}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onPageSizeChange={(size) => {
                  setPageSize(size)
                  setPage(1)
                }}
                itemLabel={th("invocationsLabel")}
              />
            </div>
          )}
        </div>
        )}
      </div>
    </DashboardLayout>
  )
}

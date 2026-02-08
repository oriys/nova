"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { asyncInvocationsApi, type AsyncInvocationJob, type AsyncInvocationStatus } from "@/lib/api"
import { cn } from "@/lib/utils"
import { RefreshCw, Search, RotateCcw, ExternalLink, Loader2 } from "lucide-react"

function toErrorMessage(err: unknown): string {
  if (err instanceof Error && err.message.trim()) return err.message.trim()
  return "Unexpected error."
}

function formatDate(ts?: string): string {
  if (!ts) return "-"
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return ts
  return date.toLocaleString()
}

function stringifyValue(value: unknown): string {
  if (value === undefined) return "-"
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function getStatusBadge(status: AsyncInvocationStatus) {
  switch (status) {
    case "queued":
      return <Badge variant="outline">queued</Badge>
    case "running":
      return <Badge variant="outline" className="border-blue-600 text-blue-600">running</Badge>
    case "succeeded":
      return <Badge variant="outline" className="border-success text-success">succeeded</Badge>
    case "dlq":
      return <Badge variant="destructive">dlq</Badge>
    default:
      return <Badge variant="secondary">{status}</Badge>
  }
}

export default function AsyncInvocationsPage() {
  const [jobID, setJobID] = useState("")
  const [statusFilter, setStatusFilter] = useState<"all" | AsyncInvocationStatus>("all")
  const [jobs, setJobs] = useState<AsyncInvocationJob[]>([])
  const [selectedJob, setSelectedJob] = useState<AsyncInvocationJob | null>(null)
  const [loadingList, setLoadingList] = useState(true)
  const [loadingLookup, setLoadingLookup] = useState(false)
  const [retryingID, setRetryingID] = useState("")
  const [error, setError] = useState<string | null>(null)

  const loadList = useCallback(async () => {
    setLoadingList(true)
    setError(null)
    try {
      const result = await asyncInvocationsApi.list(100, statusFilter === "all" ? undefined : statusFilter)
      setJobs(result || [])
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setLoadingList(false)
    }
  }, [statusFilter])

  useEffect(() => {
    void loadList()
  }, [loadList])

  const handleLookup = async (id?: string) => {
    const target = (id ?? jobID).trim()
    if (!target) {
      setError("Please enter a job ID.")
      return
    }
    setLoadingLookup(true)
    setError(null)
    try {
      const result = await asyncInvocationsApi.get(target)
      setSelectedJob(result)
      setJobID(result.id)
    } catch (err) {
      setSelectedJob(null)
      setError(toErrorMessage(err))
    } finally {
      setLoadingLookup(false)
    }
  }

  const handleRetry = async (job: AsyncInvocationJob) => {
    setRetryingID(job.id)
    setError(null)
    try {
      const result = await asyncInvocationsApi.retry(job.id)
      setSelectedJob(result)
      await loadList()
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setRetryingID("")
    }
  }

  const selectedPayload = useMemo(() => stringifyValue(selectedJob?.payload), [selectedJob])
  const selectedOutput = useMemo(() => stringifyValue(selectedJob?.output), [selectedJob])

  return (
    <DashboardLayout>
      <Header title="Async Invocations" description="Lookup async jobs by job ID and inspect queue execution state" />

      <div className="space-y-6 p-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
            {error}
          </div>
        )}

        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex min-w-[320px] flex-1 items-center gap-2">
              <Input
                placeholder="Paste async job ID..."
                value={jobID}
                onChange={(e) => setJobID(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault()
                    void handleLookup()
                  }
                }}
              />
              <Button onClick={() => void handleLookup()} disabled={loadingLookup || !jobID.trim()}>
                {loadingLookup ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Search className="mr-2 h-4 w-4" />
                )}
                Query
              </Button>
            </div>

            <Select value={statusFilter} onValueChange={(v: "all" | AsyncInvocationStatus) => setStatusFilter(v)}>
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">all statuses</SelectItem>
                <SelectItem value="queued">queued</SelectItem>
                <SelectItem value="running">running</SelectItem>
                <SelectItem value="succeeded">succeeded</SelectItem>
                <SelectItem value="dlq">dlq</SelectItem>
              </SelectContent>
            </Select>

            <Button variant="outline" onClick={() => void loadList()} disabled={loadingList}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loadingList && "animate-spin")} />
              Refresh List
            </Button>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Query is tenant/namespace scoped. If job ID exists in another scope, this page returns not found.
          </p>
        </div>

        {selectedJob && (
          <div className="rounded-xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <h3 className="text-base font-semibold text-foreground">Job Detail</h3>
                {getStatusBadge(selectedJob.status)}
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" asChild>
                  <Link href={`/functions/${encodeURIComponent(selectedJob.function_name)}`}>
                    <ExternalLink className="mr-2 h-4 w-4" />
                    Open Function
                  </Link>
                </Button>
                {selectedJob.status === "dlq" && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void handleRetry(selectedJob)}
                    disabled={retryingID === selectedJob.id}
                  >
                    {retryingID === selectedJob.id ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <RotateCcw className="mr-2 h-4 w-4" />
                    )}
                    Retry
                  </Button>
                )}
              </div>
            </div>

            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Job ID</div>
                <div className="mt-1 break-all font-mono text-xs text-foreground">{selectedJob.id}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Function</div>
                <div className="mt-1 text-sm text-foreground">{selectedJob.function_name}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Attempts</div>
                <div className="mt-1 text-sm text-foreground">{selectedJob.attempt}/{selectedJob.max_attempts}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Next Run</div>
                <div className="mt-1 text-sm text-foreground">{formatDate(selectedJob.next_run_at)}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Request ID</div>
                <div className="mt-1 break-all font-mono text-xs text-foreground">{selectedJob.request_id || "-"}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">Updated</div>
                <div className="mt-1 text-sm text-foreground">{formatDate(selectedJob.updated_at)}</div>
              </div>
            </div>

            <div className="mt-4 grid gap-3 lg:grid-cols-2">
              <div>
                <div className="mb-1 text-sm font-medium text-foreground">Payload</div>
                <pre className="min-h-[140px] overflow-x-auto rounded-md border border-border bg-muted/30 p-3 text-xs text-foreground">
                  {selectedPayload}
                </pre>
              </div>
              <div>
                <div className="mb-1 text-sm font-medium text-foreground">Output</div>
                <pre className="min-h-[140px] overflow-x-auto rounded-md border border-border bg-muted/30 p-3 text-xs text-foreground">
                  {selectedOutput}
                </pre>
              </div>
            </div>

            {selectedJob.last_error && (
              <div className="mt-3 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
                {selectedJob.last_error}
              </div>
            )}
          </div>
        )}

        <div className="overflow-x-auto rounded-xl border border-border bg-card">
          <table className="w-full min-w-[980px]">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Job ID</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Function</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Attempts</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Created</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Updated</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {loadingList ? (
                Array.from({ length: 4 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td className="px-4 py-3" colSpan={7}>
                      <div className="h-4 animate-pulse rounded bg-muted" />
                    </td>
                  </tr>
                ))
              ) : jobs.length === 0 ? (
                <tr>
                  <td className="px-4 py-8 text-center text-sm text-muted-foreground" colSpan={7}>
                    No async jobs found.
                  </td>
                </tr>
              ) : (
                jobs.map((job) => (
                  <tr key={job.id} className="border-b border-border hover:bg-muted/40">
                    <td className="px-4 py-3 font-mono text-xs">{job.id}</td>
                    <td className="px-4 py-3 text-sm">{job.function_name}</td>
                    <td className="px-4 py-3">{getStatusBadge(job.status)}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{job.attempt}/{job.max_attempts}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">{formatDate(job.created_at)}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">{formatDate(job.updated_at)}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void handleLookup(job.id)}
                          disabled={loadingLookup}
                        >
                          View
                        </Button>
                        {job.status === "dlq" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void handleRetry(job)}
                            disabled={retryingID === job.id}
                          >
                            {retryingID === job.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <RotateCcw className="h-4 w-4" />
                            )}
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </DashboardLayout>
  )
}

"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Pagination } from "@/components/pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { asyncInvocationsApi, type AsyncInvocationJob, type AsyncInvocationStatus } from "@/lib/api"
import { cn } from "@/lib/utils"
import { RefreshCw, Search, RotateCcw, ExternalLink, Loader2, Pause, Play, Trash2 } from "lucide-react"

function toErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message.trim()) return err.message.trim()
  return fallback
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

function getStatusBadge(status: AsyncInvocationStatus, t: (key: string) => string) {
  switch (status) {
    case "queued":
      return <Badge variant="outline">{t("status.queued")}</Badge>
    case "running":
      return <Badge variant="outline" className="border-blue-600 text-blue-600">{t("status.running")}</Badge>
    case "succeeded":
      return <Badge variant="outline" className="border-success text-success">{t("status.succeeded")}</Badge>
    case "dlq":
      return <Badge variant="destructive">{t("status.dlq")}</Badge>
    case "paused":
      return <Badge variant="outline" className="border-yellow-600 text-yellow-600">{t("status.paused")}</Badge>
    default:
      return <Badge variant="secondary">{status}</Badge>
  }
}

export default function AsyncInvocationsPage() {
  const t = useTranslations("pages")
  const ta = useTranslations("asyncInvocationsPage")
  const [jobID, setJobID] = useState("")
  const [statusFilter, setStatusFilter] = useState<"all" | AsyncInvocationStatus>("all")
  const [jobs, setJobs] = useState<AsyncInvocationJob[]>([])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [totalJobs, setTotalJobs] = useState(0)
  const [selectedJob, setSelectedJob] = useState<AsyncInvocationJob | null>(null)
  const [loadingList, setLoadingList] = useState(true)
  const [loadingLookup, setLoadingLookup] = useState(false)
  const [retryingID, setRetryingID] = useState("")
  const [pausingID, setPausingID] = useState("")
  const [resumingID, setResumingID] = useState("")
  const [deletingID, setDeletingID] = useState("")
  const [error, setError] = useState<string | null>(null)

  const loadList = useCallback(async () => {
    setLoadingList(true)
    setError(null)
    try {
      const offset = (page - 1) * pageSize
      const result = await asyncInvocationsApi.listPage(
        pageSize,
        statusFilter === "all" ? undefined : statusFilter,
        offset
      )
      setJobs(result.items || [])
      setTotalJobs(result.total || 0)
    } catch (err) {
      setJobs([])
      setTotalJobs(0)
      setError(toErrorMessage(err, ta("unexpectedError")))
    } finally {
      setLoadingList(false)
    }
  }, [page, pageSize, statusFilter, ta])

  useEffect(() => {
    void loadList()
  }, [loadList])

  useEffect(() => {
    setPage(1)
  }, [statusFilter])

  const totalPages = Math.max(1, Math.ceil(totalJobs / pageSize))
  useEffect(() => {
    if (page > totalPages) {
      setPage(totalPages)
    }
  }, [page, totalPages])

  const handleLookup = async (id?: string) => {
    const target = (id ?? jobID).trim()
    if (!target) {
      setError(ta("enterJobId"))
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
      setError(toErrorMessage(err, ta("unexpectedError")))
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
      setError(toErrorMessage(err, ta("unexpectedError")))
    } finally {
      setRetryingID("")
    }
  }

  const handlePause = async (job: AsyncInvocationJob) => {
    setPausingID(job.id)
    setError(null)
    try {
      const result = await asyncInvocationsApi.pause(job.id)
      setSelectedJob(result)
      await loadList()
    } catch (err) {
      setError(toErrorMessage(err, ta("unexpectedError")))
    } finally {
      setPausingID("")
    }
  }

  const handleResume = async (job: AsyncInvocationJob) => {
    setResumingID(job.id)
    setError(null)
    try {
      const result = await asyncInvocationsApi.resume(job.id)
      setSelectedJob(result)
      await loadList()
    } catch (err) {
      setError(toErrorMessage(err, ta("unexpectedError")))
    } finally {
      setResumingID("")
    }
  }

  const handleDelete = async (job: AsyncInvocationJob) => {
    setDeletingID(job.id)
    setError(null)
    try {
      await asyncInvocationsApi.delete(job.id)
      if (selectedJob?.id === job.id) {
        setSelectedJob(null)
      }
      await loadList()
    } catch (err) {
      setError(toErrorMessage(err, ta("unexpectedError")))
    } finally {
      setDeletingID("")
    }
  }

  const selectedPayload = useMemo(() => stringifyValue(selectedJob?.payload), [selectedJob])
  const selectedOutput = useMemo(() => stringifyValue(selectedJob?.output), [selectedJob])

  return (
    <DashboardLayout>
      <Header title={t("asyncInvocations.title")} description={t("asyncInvocations.description")} />

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
                placeholder={ta("pasteJobId")}
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
                {ta("query")}
              </Button>
            </div>

            <Select value={statusFilter} onValueChange={(v: "all" | AsyncInvocationStatus) => setStatusFilter(v)}>
              <SelectTrigger className="w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{ta("allStatuses")}</SelectItem>
                <SelectItem value="queued">{ta("status.queued")}</SelectItem>
                <SelectItem value="running">{ta("status.running")}</SelectItem>
                <SelectItem value="succeeded">{ta("status.succeeded")}</SelectItem>
                <SelectItem value="paused">{ta("status.paused")}</SelectItem>
                <SelectItem value="dlq">{ta("status.dlq")}</SelectItem>
              </SelectContent>
            </Select>

            <Button variant="outline" onClick={() => void loadList()} disabled={loadingList}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loadingList && "animate-spin")} />
              {ta("refreshList")}
            </Button>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {ta("scopeNote")}
          </p>
        </div>

        {selectedJob && (
          <div className="rounded-xl border border-border bg-card p-5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <h3 className="text-base font-semibold text-foreground">{ta("jobDetail")}</h3>
                {getStatusBadge(selectedJob.status, ta)}
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" asChild>
                  <Link href={`/functions/${encodeURIComponent(selectedJob.function_name)}`}>
                    <ExternalLink className="mr-2 h-4 w-4" />
                    {ta("openFunction")}
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
                    {ta("retry")}
                  </Button>
                )}
                {selectedJob.status === "queued" && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void handlePause(selectedJob)}
                    disabled={pausingID === selectedJob.id}
                  >
                    {pausingID === selectedJob.id ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Pause className="mr-2 h-4 w-4" />
                    )}
                    {ta("pause")}
                  </Button>
                )}
                {selectedJob.status === "paused" && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void handleResume(selectedJob)}
                    disabled={resumingID === selectedJob.id}
                  >
                    {resumingID === selectedJob.id ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Play className="mr-2 h-4 w-4" />
                    )}
                    {ta("resume")}
                  </Button>
                )}
                {(selectedJob.status === "queued" || selectedJob.status === "paused") && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="text-destructive hover:text-destructive"
                    onClick={() => void handleDelete(selectedJob)}
                    disabled={deletingID === selectedJob.id}
                  >
                    {deletingID === selectedJob.id ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="mr-2 h-4 w-4" />
                    )}
                    {ta("delete")}
                  </Button>
                )}
              </div>
            </div>

            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("jobId")}</div>
                <div className="mt-1 break-all font-mono text-xs text-foreground">{selectedJob.id}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("function")}</div>
                <div className="mt-1 text-sm text-foreground">{selectedJob.function_name}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("attempts")}</div>
                <div className="mt-1 text-sm text-foreground">{selectedJob.attempt}/{selectedJob.max_attempts}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("nextRun")}</div>
                <div className="mt-1 text-sm text-foreground">{formatDate(selectedJob.next_run_at)}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("requestId")}</div>
                <div className="mt-1 break-all font-mono text-xs text-foreground">{selectedJob.request_id || "-"}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground">{ta("updated")}</div>
                <div className="mt-1 text-sm text-foreground">{formatDate(selectedJob.updated_at)}</div>
              </div>
            </div>

            <div className="mt-4 grid gap-3 lg:grid-cols-2">
              <div>
                <div className="mb-1 text-sm font-medium text-foreground">{ta("payload")}</div>
                <pre className="min-h-[140px] overflow-x-auto rounded-md border border-border bg-muted/30 p-3 text-xs text-foreground">
                  {selectedPayload}
                </pre>
              </div>
              <div>
                <div className="mb-1 text-sm font-medium text-foreground">{ta("output")}</div>
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
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colJobId")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colFunction")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colStatus")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colAttempts")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colCreated")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{ta("colUpdated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{ta("colActions")}</th>
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
                    {ta("noJobs")}
                  </td>
                </tr>
              ) : (
                jobs.map((job) => (
                  <tr key={job.id} className="border-b border-border hover:bg-muted/40">
                    <td className="px-4 py-3 font-mono text-xs">{job.id}</td>
                    <td className="px-4 py-3 text-sm">{job.function_name}</td>
                    <td className="px-4 py-3">{getStatusBadge(job.status, ta)}</td>
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
                          {ta("view")}
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
                        {job.status === "queued" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void handlePause(job)}
                            disabled={pausingID === job.id}
                          >
                            {pausingID === job.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Pause className="h-4 w-4" />
                            )}
                          </Button>
                        )}
                        {job.status === "paused" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void handleResume(job)}
                            disabled={resumingID === job.id}
                          >
                            {resumingID === job.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Play className="h-4 w-4" />
                            )}
                          </Button>
                        )}
                        {(job.status === "queued" || job.status === "paused") && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => void handleDelete(job)}
                            disabled={deletingID === job.id}
                          >
                            {deletingID === job.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Trash2 className="h-4 w-4" />
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

        {!loadingList && totalJobs > 0 && (
          <div className="rounded-xl border border-border bg-card p-4">
            <Pagination
              totalItems={totalJobs}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(nextSize) => {
                setPageSize(nextSize)
                setPage(1)
              }}
              pageSizeOptions={[20, 50, 100, 200]}
            />
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}

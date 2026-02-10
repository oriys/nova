"use client"

import { useEffect, useState, useCallback } from "react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Pagination } from "@/components/pagination"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { LogEntry } from "@/lib/types"
import { Search, RefreshCw, Download, Info, AlertTriangle, XCircle, Bug, Loader2 } from "lucide-react"

interface FunctionLogsProps {
  logs: LogEntry[]
  onRefresh?: () => void
  loading?: boolean
  highlightedRequestId?: string
}

const levelConfig = {
  info: { icon: Info, color: "text-primary", bg: "bg-primary/10", label: "INFO" },
  warn: { icon: AlertTriangle, color: "text-warning", bg: "bg-warning/10", label: "WARN" },
  error: { icon: XCircle, color: "text-destructive", bg: "bg-destructive/10", label: "ERROR" },
  debug: { icon: Bug, color: "text-muted-foreground", bg: "bg-muted", label: "DEBUG" },
}

export function FunctionLogs({ logs, onRefresh, loading, highlightedRequestId }: FunctionLogsProps) {
  const [searchQuery, setSearchQuery] = useState("")
  const [levelFilter, setLevelFilter] = useState("all")
  const [isLive, setIsLive] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)

  const filteredLogs = logs.filter((log) => {
    const matchesSearch = log.message.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesLevel = levelFilter === "all" || log.level === levelFilter
    return matchesSearch && matchesLevel
  })

  useEffect(() => {
    setPage(1)
  }, [searchQuery, levelFilter])

  const totalPages = Math.max(1, Math.ceil(filteredLogs.length / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const pagedLogs = filteredLogs.slice((page - 1) * pageSize, page * pageSize)

  // Auto-refresh when live mode is enabled
  useEffect(() => {
    if (!isLive || !onRefresh) return
    const interval = setInterval(onRefresh, 3000)
    return () => clearInterval(interval)
  }, [isLive, onRefresh])

  const handleExport = useCallback(() => {
    const exportData = filteredLogs.map(log => ({
      id: log.id,
      timestamp: log.timestamp,
      level: log.level,
      message: log.message,
      requestId: log.requestId,
      functionName: log.functionName,
      duration: log.duration,
    }))

    const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `logs-${new Date().toISOString().split('T')[0]}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }, [filteredLogs])

  return (
    <div className="space-y-4">
      {highlightedRequestId && (
        <div className="rounded-lg border border-primary/30 bg-primary/10 px-3 py-2 text-xs text-primary">
          Highlighted request_id: <code>{highlightedRequestId}</code>
        </div>
      )}

      {/* Toolbar */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <div className="relative flex-1 sm:w-64">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              placeholder="Search logs..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9"
            />
          </div>
          <Select value={levelFilter} onValueChange={setLevelFilter}>
            <SelectTrigger className="w-32">
              <SelectValue placeholder="Level" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Levels</SelectItem>
              <SelectItem value="info">Info</SelectItem>
              <SelectItem value="warn">Warning</SelectItem>
              <SelectItem value="error">Error</SelectItem>
              <SelectItem value="debug">Debug</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant={isLive ? "default" : "outline"}
            size="sm"
            onClick={() => setIsLive(!isLive)}
          >
            <span className={cn(
              "mr-2 h-2 w-2 rounded-full",
              isLive ? "bg-success animate-pulse" : "bg-muted-foreground"
            )} />
            {isLive ? "Live" : "Paused"}
          </Button>
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={loading}>
            {loading ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={handleExport} disabled={filteredLogs.length === 0}>
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        </div>
      </div>

      {/* Logs List */}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <div className="divide-y divide-border">
          {filteredLogs.length === 0 ? (
            <div className="p-8 text-center">
              <p className="text-sm text-muted-foreground">
                {logs.length === 0 ? "No logs available" : "No logs found matching your filter"}
              </p>
            </div>
          ) : (
            pagedLogs.map((log) => {
              const config = levelConfig[log.level]
              const timestamp = new Date(log.timestamp)
              const formattedTime = timestamp.toLocaleTimeString("en-US", {
                hour: "2-digit",
                minute: "2-digit",
                second: "2-digit",
                fractionalSecondDigits: 3,
              })
              const formattedDate = timestamp.toLocaleDateString("en-US", {
                month: "short",
                day: "numeric",
              })

              return (
                <div
                  key={log.id}
                  className={cn(
                    "flex items-start gap-4 px-4 py-3 hover:bg-muted/20 transition-colors font-mono text-sm",
                    highlightedRequestId && log.requestId === highlightedRequestId && "bg-primary/10 ring-1 ring-primary/40"
                  )}
                >
                  <div className="flex items-center gap-2 shrink-0">
                    <div className={cn("rounded px-1.5 py-0.5 text-xs font-medium", config.bg, config.color)}>
                      {config.label}
                    </div>
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {formattedDate} {formattedTime}
                    </span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-foreground break-all">{log.message}</p>
                    <div className="flex items-center gap-3 mt-1">
                      <span className="text-xs text-muted-foreground">
                        Request ID: {log.requestId}
                      </span>
                      {log.duration && (
                        <span className="text-xs text-muted-foreground">
                          Duration: {log.duration}ms
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>

        {filteredLogs.length > 0 && (
          <div className="border-t border-border p-4">
            <Pagination
              totalItems={filteredLogs.length}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                setPageSize(size)
                setPage(1)
              }}
              itemLabel="logs"
            />
          </div>
        )}
      </div>
    </div>
  )
}

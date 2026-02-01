"use client"

import { useEffect, useState, useCallback } from "react"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { functionsApi } from "@/lib/api"
import { transformLog, LogEntry } from "@/lib/types"
import {
  RefreshCw,
  Search,
  Filter,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Info,
  ExternalLink,
} from "lucide-react"
import { cn } from "@/lib/utils"

export default function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [functions, setFunctions] = useState<{ id: string; name: string }[]>([])
  const [searchQuery, setSearchQuery] = useState("")
  const [levelFilter, setLevelFilter] = useState<string>("all")
  const [functionFilter, setFunctionFilter] = useState<string>("all")
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      // Get all functions
      const funcs = await functionsApi.list()
      setFunctions(funcs.map((f) => ({ id: f.id, name: f.name })))

      // Fetch logs from all functions
      const allLogs: LogEntry[] = []
      for (const fn of funcs.slice(0, 10)) {
        try {
          const fnLogs = await functionsApi.logs(fn.name, 20)
          allLogs.push(...fnLogs.map(transformLog))
        } catch {
          // Skip functions without logs
        }
      }

      // Sort by timestamp descending
      allLogs.sort(
        (a, b) =>
          new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
      )
      setLogs(allLogs)
    } catch (err) {
      console.error("Failed to fetch logs:", err)
      setError(err instanceof Error ? err.message : "Failed to load logs")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredLogs = logs.filter((log) => {
    const matchesSearch =
      log.message.toLowerCase().includes(searchQuery.toLowerCase()) ||
      log.functionName.toLowerCase().includes(searchQuery.toLowerCase()) ||
      log.requestId.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesLevel = levelFilter === "all" || log.level === levelFilter
    const matchesFunction =
      functionFilter === "all" || log.functionName === functionFilter
    return matchesSearch && matchesLevel && matchesFunction
  })

  const getLevelIcon = (level: string) => {
    switch (level) {
      case "error":
        return <XCircle className="h-4 w-4 text-destructive" />
      case "warn":
        return <AlertTriangle className="h-4 w-4 text-warning" />
      case "info":
        return <Info className="h-4 w-4 text-primary" />
      case "debug":
        return <CheckCircle className="h-4 w-4 text-muted-foreground" />
      default:
        return <Info className="h-4 w-4" />
    }
  }

  const formatTimestamp = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleString()
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title="Logs" description="Function execution logs" />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load logs</p>
            <p className="text-sm mt-1">{error}</p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title="Logs" description="Function execution logs" />

      <div className="p-6 space-y-6">
        {/* Filters */}
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-1 items-center gap-3">
            <div className="relative flex-1 max-w-sm">
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
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Level" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Levels</SelectItem>
                <SelectItem value="error">Error</SelectItem>
                <SelectItem value="warn">Warning</SelectItem>
                <SelectItem value="info">Info</SelectItem>
                <SelectItem value="debug">Debug</SelectItem>
              </SelectContent>
            </Select>

            <Select value={functionFilter} onValueChange={setFunctionFilter}>
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Function" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Functions</SelectItem>
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
            Refresh
          </Button>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Total Logs</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : filteredLogs.length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Errors</p>
            <p className="text-2xl font-semibold text-destructive">
              {loading ? "..." : filteredLogs.filter((l) => l.level === "error").length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Warnings</p>
            <p className="text-2xl font-semibold text-warning">
              {loading ? "..." : filteredLogs.filter((l) => l.level === "warn").length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Functions</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : new Set(filteredLogs.map((l) => l.functionName)).size}
            </p>
          </div>
        </div>

        {/* Logs Table */}
        <div className="rounded-xl border border-border bg-card">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    Level
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    Timestamp
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    Function
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    Message
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    Duration
                  </th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td colSpan={6} className="px-4 py-3">
                        <div className="h-4 bg-muted rounded animate-pulse" />
                      </td>
                    </tr>
                  ))
                ) : filteredLogs.length === 0 ? (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-8 text-center text-muted-foreground"
                    >
                      No logs found
                    </td>
                  </tr>
                ) : (
                  filteredLogs.slice(0, 50).map((log) => (
                    <tr
                      key={log.id}
                      className="border-b border-border hover:bg-muted/50"
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          {getLevelIcon(log.level)}
                          <Badge
                            variant="secondary"
                            className={cn(
                              "text-xs",
                              log.level === "error" &&
                                "bg-destructive/10 text-destructive border-0",
                              log.level === "warn" &&
                                "bg-warning/10 text-warning border-0",
                              log.level === "info" &&
                                "bg-primary/10 text-primary border-0",
                              log.level === "debug" &&
                                "bg-muted text-muted-foreground border-0"
                            )}
                          >
                            {log.level}
                          </Badge>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground whitespace-nowrap">
                        {formatTimestamp(log.timestamp)}
                      </td>
                      <td className="px-4 py-3">
                        <Link
                          href={`/functions/${log.functionName}`}
                          className="text-sm font-medium text-primary hover:underline"
                        >
                          {log.functionName}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm text-foreground truncate max-w-md">
                          {log.message}
                        </p>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground whitespace-nowrap">
                        {log.duration ? `${log.duration}ms` : "-"}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button variant="ghost" size="sm" asChild>
                          <Link href={`/functions/${log.functionName}`}>
                            <ExternalLink className="h-4 w-4" />
                          </Link>
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </DashboardLayout>
  )
}

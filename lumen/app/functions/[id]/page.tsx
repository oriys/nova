"use client"

import { use, useEffect, useState, useCallback } from "react"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { FunctionMetrics } from "@/components/function-metrics"
import { FunctionCode } from "@/components/function-code"
import { FunctionLogs } from "@/components/function-logs"
import { FunctionConfig } from "@/components/function-config"
import { cn } from "@/lib/utils"
import { functionsApi } from "@/lib/api"
import { transformFunction, transformLog, FunctionData, LogEntry } from "@/lib/types"
import type { FunctionMetrics as FunctionMetricsType } from "@/lib/api"
import {
  ArrowLeft,
  Play,
  RefreshCw,
  Loader2,
} from "lucide-react"

export default function FunctionDetailPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const [activeTab, setActiveTab] = useState("overview")
  const [func, setFunc] = useState<FunctionData | null>(null)
  const [metrics, setMetrics] = useState<FunctionMetricsType | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [invoking, setInvoking] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      // id could be function ID or name, try to get by name first
      const fn = await functionsApi.get(id)
      const [fnMetrics, fnLogs] = await Promise.all([
        functionsApi.metrics(fn.name).catch(() => null),
        functionsApi.logs(fn.name, 20).catch(() => []),
      ])

      setMetrics(fnMetrics)
      setFunc(transformFunction(fn, fnMetrics ?? undefined))
      setLogs(fnLogs.map(transformLog))
    } catch (err) {
      console.error("Failed to fetch function:", err)
      setError(err instanceof Error ? err.message : "Failed to load function")
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleInvoke = async () => {
    if (!func) return

    try {
      setInvoking(true)
      await functionsApi.invoke(func.name, {})
      // Refresh data after invocation
      fetchData()
    } catch (err) {
      console.error("Failed to invoke function:", err)
    } finally {
      setInvoking(false)
    }
  }

  if (loading) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center h-[60vh]">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      </DashboardLayout>
    )
  }

  if (error || !func) {
    return (
      <DashboardLayout>
        <div className="flex flex-col items-center justify-center h-[60vh]">
          <p className="text-muted-foreground mb-4">
            {error || "Function not found"}
          </p>
          <Button asChild variant="outline">
            <Link href="/functions">Back to Functions</Link>
          </Button>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      {/* Header */}
      <header className="sticky top-0 z-30 border-b border-border bg-card/80 backdrop-blur-sm">
        <div className="flex items-center justify-between px-6 py-4">
          <div className="flex items-center gap-4">
            <Button variant="ghost" size="icon" asChild>
              <Link href="/functions">
                <ArrowLeft className="h-4 w-4" />
              </Link>
            </Button>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-xl font-semibold text-foreground">
                  {func.name}
                </h1>
                <Badge
                  variant="secondary"
                  className={cn(
                    "text-xs font-medium",
                    func.status === "active" && "bg-success/10 text-success border-0",
                    func.status === "error" && "bg-destructive/10 text-destructive border-0",
                    func.status === "inactive" && "bg-muted text-muted-foreground border-0"
                  )}
                >
                  {func.status}
                </Badge>
              </div>
              <p className="text-sm text-muted-foreground mt-0.5">
                {func.runtime} · {func.region}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
              Refresh
            </Button>
            <Button
              size="sm"
              onClick={handleInvoke}
              disabled={invoking}
            >
              {invoking ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Play className="mr-2 h-4 w-4" />
              )}
              Invoke
            </Button>
          </div>
        </div>

        {/* Tabs */}
        <Tabs value={activeTab} onValueChange={setActiveTab} className="px-6">
          <TabsList className="h-12 w-full justify-start rounded-none border-0 bg-transparent p-0">
            <TabsTrigger
              value="overview"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Overview
            </TabsTrigger>
            <TabsTrigger
              value="code"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Code
            </TabsTrigger>
            <TabsTrigger
              value="logs"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Logs
            </TabsTrigger>
            <TabsTrigger
              value="config"
              className="relative h-12 rounded-none border-0 bg-transparent px-4 font-medium text-muted-foreground data-[state=active]:text-foreground data-[state=active]:shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 data-[state=active]:after:bg-primary"
            >
              Configuration
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </header>

      {/* Content */}
      <div className="p-6">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsContent value="overview" className="mt-0 space-y-6">
            <FunctionMetrics func={func} metrics={metrics} />
          </TabsContent>

          <TabsContent value="code" className="mt-0">
            <FunctionCode func={func} />
          </TabsContent>

          <TabsContent value="logs" className="mt-0">
            <FunctionLogs logs={logs} onRefresh={fetchData} />
          </TabsContent>

          <TabsContent value="config" className="mt-0">
            <FunctionConfig func={func} onUpdate={fetchData} />
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  )
}

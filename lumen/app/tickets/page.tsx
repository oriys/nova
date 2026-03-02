"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { ticketsApi } from "@/lib/api"
import type { Ticket, TicketComment } from "@/lib/types"
import { ClipboardList, Plus, RefreshCw, MessageSquare, X } from "lucide-react"
import { cn } from "@/lib/utils"
import { getTenantScope } from "@/lib/tenant-scope"

const statusColors: Record<string, string> = {
  open: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  in_progress: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  resolved: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  closed: "bg-muted text-muted-foreground",
}

const priorityColors: Record<string, string> = {
  low: "bg-muted text-muted-foreground",
  medium: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  high: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
  critical: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
}

export default function TicketsPage() {
  const t = useTranslations("pages")
  const tt = useTranslations("ticketsPage")
  const tc = useTranslations("common")
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState("")
  const [newDescription, setNewDescription] = useState("")
  const [newPriority, setNewPriority] = useState<string>("medium")
  const [newCategory, setNewCategory] = useState<string>("general")
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const limit = 20

  // Detail panel state
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null)
  const [comments, setComments] = useState<TicketComment[]>([])
  const [commentText, setCommentText] = useState("")
  const [commentInternal, setCommentInternal] = useState(false)
  const [sendingComment, setSendingComment] = useState(false)

  const isDefaultTenant = getTenantScope().tenantId === "default"

  const fetchTickets = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const result = await ticketsApi.list(limit, offset)
      setTickets(result.items || [])
      setTotal(result.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [offset, tt])

  useEffect(() => {
    fetchTickets()
  }, [fetchTickets])

  const handleCreate = async () => {
    if (!newTitle.trim()) return
    try {
      setCreating(true)
      await ticketsApi.create({
        title: newTitle.trim(),
        description: newDescription.trim() || undefined,
        priority: newPriority as "low" | "medium" | "high" | "critical",
        category: newCategory as "general" | "quota_request" | "incident" | "feature_request" | "bug_report",
      })
      setDialogOpen(false)
      setNewTitle("")
      setNewDescription("")
      setNewPriority("medium")
      setNewCategory("general")
      fetchTickets()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleStatusChange = async (ticket: Ticket, status: string) => {
    try {
      const updated = await ticketsApi.update(ticket.id, { status: status as Ticket["status"] })
      setTickets((prev) => prev.map((t) => (t.id === ticket.id ? updated : t)))
      if (selectedTicket?.id === ticket.id) setSelectedTicket(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToUpdate"))
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await ticketsApi.delete(id)
      setTickets((prev) => prev.filter((t) => t.id !== id))
      if (selectedTicket?.id === id) setSelectedTicket(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToDelete"))
    }
  }

  const openDetail = async (ticket: Ticket) => {
    setSelectedTicket(ticket)
    try {
      const result = await ticketsApi.listComments(ticket.id)
      setComments(result.items || [])
    } catch {
      setComments([])
    }
  }

  const handleSendComment = async () => {
    if (!commentText.trim() || !selectedTicket) return
    try {
      setSendingComment(true)
      const c = await ticketsApi.createComment(selectedTicket.id, {
        content: commentText.trim(),
        internal: commentInternal,
      })
      setComments((prev) => [...prev, c])
      setCommentText("")
      setCommentInternal(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToComment"))
    } finally {
      setSendingComment(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("tickets.title")} description={t("tickets.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
            <button className="ml-2 underline" onClick={() => setError(null)}>✕</button>
          </div>
        )}

        <div className="flex items-center justify-between">
          <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                {tt("createTicket")}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{tt("createTicket")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("title")}</label>
                  <Input
                    value={newTitle}
                    onChange={(e) => setNewTitle(e.target.value)}
                    placeholder={tt("titlePlaceholder")}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("description")}</label>
                  <Textarea
                    value={newDescription}
                    onChange={(e) => setNewDescription(e.target.value)}
                    placeholder={tt("descriptionPlaceholder")}
                    className="min-h-[100px] text-sm"
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tt("priority")}</label>
                    <select
                      value={newPriority}
                      onChange={(e) => setNewPriority(e.target.value)}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    >
                      <option value="low">{tt("priorityLow")}</option>
                      <option value="medium">{tt("priorityMedium")}</option>
                      <option value="high">{tt("priorityHigh")}</option>
                      <option value="critical">{tt("priorityCritical")}</option>
                    </select>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tt("category")}</label>
                    <select
                      value={newCategory}
                      onChange={(e) => setNewCategory(e.target.value)}
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                    >
                      <option value="general">{tt("categoryGeneral")}</option>
                      <option value="quota_request">{tt("categoryQuotaRequest")}</option>
                      <option value="incident">{tt("categoryIncident")}</option>
                      <option value="feature_request">{tt("categoryFeatureRequest")}</option>
                      <option value="bug_report">{tt("categoryBugReport")}</option>
                    </select>
                  </div>
                </div>
                <Button
                  className="w-full"
                  onClick={handleCreate}
                  disabled={creating || !newTitle.trim()}
                >
                  {creating ? tt("creating") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchTickets} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="flex gap-6">
          {/* Ticket list */}
          <div className={cn("flex-1 rounded-xl border border-border bg-card overflow-hidden", selectedTicket && "max-w-[60%]")}>
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colTitle")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colStatus")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colPriority")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colCategory")}</th>
                  {isDefaultTenant && (
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colTenant")}</th>
                  )}
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colCreated")}</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td colSpan={isDefaultTenant ? 6 : 5} className="px-4 py-3">
                        <div className="h-4 bg-muted rounded animate-pulse" />
                      </td>
                    </tr>
                  ))
                ) : tickets.length === 0 ? (
                  <tr>
                    <td colSpan={isDefaultTenant ? 6 : 5} className="px-4 py-8 text-center text-muted-foreground">
                      <ClipboardList className="mx-auto h-8 w-8 mb-2 opacity-50" />
                      {tt("noTickets")}
                    </td>
                  </tr>
                ) : (
                  tickets.map((ticket) => (
                    <tr
                      key={ticket.id}
                      className={cn(
                        "border-b border-border hover:bg-muted/50 cursor-pointer",
                        selectedTicket?.id === ticket.id && "bg-muted/50"
                      )}
                      onClick={() => openDetail(ticket)}
                    >
                      <td className="px-4 py-3">
                        <span className="font-medium text-sm">{ticket.title}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium", statusColors[ticket.status] || "")}>
                          {tt(`status_${ticket.status}`)}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium", priorityColors[ticket.priority] || "")}>
                          {tt(`priority_${ticket.priority}`)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {tt(`category_${ticket.category}`)}
                      </td>
                      {isDefaultTenant && (
                        <td className="px-4 py-3 text-sm text-muted-foreground font-mono">
                          {ticket.creator_tenant_id}
                        </td>
                      )}
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(ticket.created_at).toLocaleDateString()}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>

            {total > limit && (
              <div className="flex items-center justify-between px-4 py-3 border-t border-border">
                <span className="text-sm text-muted-foreground">
                  {tt("showing", { from: offset + 1, to: Math.min(offset + limit, total), total })}
                </span>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - limit))}>
                    {tc("previous")}
                  </Button>
                  <Button variant="outline" size="sm" disabled={offset + limit >= total} onClick={() => setOffset(offset + limit)}>
                    {tc("next")}
                  </Button>
                </div>
              </div>
            )}
          </div>

          {/* Detail panel */}
          {selectedTicket && (
            <div className="w-[40%] rounded-xl border border-border bg-card p-4 space-y-4 overflow-y-auto max-h-[calc(100vh-220px)]">
              <div className="flex items-start justify-between">
                <h3 className="text-lg font-semibold">{selectedTicket.title}</h3>
                <Button variant="ghost" size="sm" onClick={() => setSelectedTicket(null)}>
                  <X className="h-4 w-4" />
                </Button>
              </div>

              {selectedTicket.description && (
                <p className="text-sm text-muted-foreground whitespace-pre-wrap">{selectedTicket.description}</p>
              )}

              <div className="grid grid-cols-2 gap-2 text-sm">
                <div>
                  <span className="text-muted-foreground">{tt("colStatus")}:</span>{" "}
                  {isDefaultTenant ? (
                    <select
                      value={selectedTicket.status}
                      onChange={(e) => handleStatusChange(selectedTicket, e.target.value)}
                      className="rounded border border-input bg-background px-2 py-0.5 text-xs"
                    >
                      <option value="open">{tt("status_open")}</option>
                      <option value="in_progress">{tt("status_in_progress")}</option>
                      <option value="resolved">{tt("status_resolved")}</option>
                      <option value="closed">{tt("status_closed")}</option>
                    </select>
                  ) : (
                    <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium", statusColors[selectedTicket.status] || "")}>
                      {tt(`status_${selectedTicket.status}`)}
                    </span>
                  )}
                </div>
                <div>
                  <span className="text-muted-foreground">{tt("colPriority")}:</span>{" "}
                  <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium", priorityColors[selectedTicket.priority] || "")}>
                    {tt(`priority_${selectedTicket.priority}`)}
                  </span>
                </div>
                <div>
                  <span className="text-muted-foreground">{tt("colCategory")}:</span>{" "}
                  <span className="text-xs">{tt(`category_${selectedTicket.category}`)}</span>
                </div>
                {isDefaultTenant && (
                  <div>
                    <span className="text-muted-foreground">{tt("colTenant")}:</span>{" "}
                    <span className="text-xs font-mono">{selectedTicket.creator_tenant_id}</span>
                  </div>
                )}
              </div>

              {isDefaultTenant && (
                <Button variant="destructive" size="sm" onClick={() => handleDelete(selectedTicket.id)}>
                  {tt("deleteTicket")}
                </Button>
              )}

              {/* Comments */}
              <div className="border-t border-border pt-4 space-y-3">
                <h4 className="text-sm font-medium flex items-center gap-2">
                  <MessageSquare className="h-4 w-4" />
                  {tt("comments")} ({comments.length})
                </h4>

                {comments.map((c) => (
                  <div
                    key={c.id}
                    className={cn(
                      "rounded-lg p-3 text-sm",
                      c.internal
                        ? "bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-800"
                        : "bg-muted"
                    )}
                  >
                    <div className="flex items-center gap-2 mb-1 text-xs text-muted-foreground">
                      <span className="font-mono">{c.author_tenant_id}</span>
                      <span>·</span>
                      <span>{new Date(c.created_at).toLocaleString()}</span>
                      {c.internal && <span className="text-yellow-600 dark:text-yellow-400 font-medium">{tt("internalNote")}</span>}
                    </div>
                    <p className="whitespace-pre-wrap">{c.content}</p>
                  </div>
                ))}

                <div className="space-y-2">
                  <Textarea
                    value={commentText}
                    onChange={(e) => setCommentText(e.target.value)}
                    placeholder={tt("commentPlaceholder")}
                    className="min-h-[60px] text-sm"
                  />
                  <div className="flex items-center justify-between">
                    {isDefaultTenant && (
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="internal"
                          checked={commentInternal}
                          onChange={(e) => setCommentInternal(e.target.checked)}
                          className="h-4 w-4 rounded border-border"
                        />
                        <label htmlFor="internal" className="text-xs text-muted-foreground">{tt("internalNote")}</label>
                      </div>
                    )}
                    <Button
                      size="sm"
                      onClick={handleSendComment}
                      disabled={sendingComment || !commentText.trim()}
                      className={isDefaultTenant ? "" : "ml-auto"}
                    >
                      {sendingComment ? tt("sending") : tt("sendComment")}
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}

"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { ErrorBanner } from "@/components/ui/error-banner"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  eventsApi,
  functionsApi,
  workflowsApi,
  type EventDelivery,
  type EventDeliveryStatus,
  type EventMessage,
  type EventOutboxJob,
  type EventSubscription,
  type EventTopic,
  type NovaFunction,
  type Workflow,
} from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { Plus, RefreshCw, Send, Trash2, RotateCcw } from "lucide-react"

function formatDate(ts?: string) {
  if (!ts) return "-"
  return new Date(ts).toLocaleString()
}

function statusBadgeVariant(status: EventDeliveryStatus): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "succeeded":
      return "default"
    case "dlq":
      return "destructive"
    case "running":
      return "outline"
    default:
      return "secondary"
  }
}

type Notice = {
  kind: "success" | "error" | "info"
  text: string
}

export default function EventsPage() {
  const t = useTranslations("pages.events")
  const [topics, setTopics] = useState<EventTopic[]>([])
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [selectedTopicName, setSelectedTopicName] = useState("")
  const [subscriptions, setSubscriptions] = useState<EventSubscription[]>([])
  const [messages, setMessages] = useState<EventMessage[]>([])
  const [outboxJobs, setOutboxJobs] = useState<EventOutboxJob[]>([])
  const [selectedSubscriptionID, setSelectedSubscriptionID] = useState("")
  const [deliveries, setDeliveries] = useState<EventDelivery[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [pendingTopicDelete, setPendingTopicDelete] = useState<string | null>(null)
  const [pendingSubscriptionDelete, setPendingSubscriptionDelete] = useState<string | null>(null)

  const [createTopicName, setCreateTopicName] = useState("")
  const [createTopicDesc, setCreateTopicDesc] = useState("")
  const [createTopicRetentionHours, setCreateTopicRetentionHours] = useState("168")

  const [newSubName, setNewSubName] = useState("")
  const [newSubGroup, setNewSubGroup] = useState("")
  const [newSubType, setNewSubType] = useState<"function" | "workflow">("function")
  const [newSubFunction, setNewSubFunction] = useState("")
  const [newSubWorkflow, setNewSubWorkflow] = useState("")
  const [newSubMaxAttempts, setNewSubMaxAttempts] = useState("3")
  const [newSubBackoffBase, setNewSubBackoffBase] = useState("1000")
  const [newSubBackoffMax, setNewSubBackoffMax] = useState("60000")
  const [newSubMaxInflight, setNewSubMaxInflight] = useState("0")
  const [newSubRateLimitPerS, setNewSubRateLimitPerS] = useState("0")
  // Webhook fields
  const [newSubWebhookURL, setNewSubWebhookURL] = useState("")
  const [newSubWebhookMethod, setNewSubWebhookMethod] = useState("POST")
  const [newSubWebhookHeaders, setNewSubWebhookHeaders] = useState("{}")
  const [newSubWebhookSecret, setNewSubWebhookSecret] = useState("")
  const [newSubWebhookTimeout, setNewSubWebhookTimeout] = useState("30000")

  const [publishPayload, setPublishPayload] = useState("{}")
  const [publishHeaders, setPublishHeaders] = useState("{}")
  const [publishOrderingKey, setPublishOrderingKey] = useState("")
  const [outboxMaxAttempts, setOutboxMaxAttempts] = useState("5")
  const [outboxBackoffBase, setOutboxBackoffBase] = useState("1000")
  const [outboxBackoffMax, setOutboxBackoffMax] = useState("60000")

  const [replayFromSequence, setReplayFromSequence] = useState("1")
  const [replayLimit, setReplayLimit] = useState("100")
  const [replayFromTime, setReplayFromTime] = useState("")
  const [replayResetCursor, setReplayResetCursor] = useState("true")
  const [seekFromSequence, setSeekFromSequence] = useState("1")
  const [seekFromTime, setSeekFromTime] = useState("")

  const [editSubMaxInflight, setEditSubMaxInflight] = useState("0")
  const [editSubRateLimitPerS, setEditSubRateLimitPerS] = useState("0")

  const [busy, setBusy] = useState(false)

  const selectedTopic = useMemo(() => topics.find((t) => t.name === selectedTopicName) || null, [topics, selectedTopicName])
  const selectedSubscription = useMemo(
    () => subscriptions.find((s) => s.id === selectedSubscriptionID) || null,
    [subscriptions, selectedSubscriptionID]
  )

  const fetchBaseData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [topicData, functionData, workflowData] = await Promise.all([
        eventsApi.listTopics(200),
        functionsApi.list(),
        workflowsApi.list(),
      ])
      setTopics(topicData)
      setFunctions(functionData)
      setWorkflows(workflowData)

      if (topicData.length > 0) {
        setSelectedTopicName((prev) => {
          if (prev && topicData.some((t) => t.name === prev)) {
            return prev
          }
          return topicData[0].name
        })
      } else {
        setSelectedTopicName("")
        setSubscriptions([])
        setMessages([])
        setOutboxJobs([])
        setSelectedSubscriptionID("")
        setDeliveries([])
      }

      if (functionData.length > 0 && !newSubFunction) {
        setNewSubFunction(functionData[0].name)
      }
      if (workflowData.length > 0 && !newSubWorkflow) {
        setNewSubWorkflow(workflowData[0].name)
      }
    } catch (err) {
      console.error("Failed to load event bus data:", err)
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [newSubFunction, newSubWorkflow])

  const fetchTopicDetails = useCallback(async (topicName: string) => {
    if (!topicName) {
      setSubscriptions([])
      setMessages([])
      setOutboxJobs([])
      setSelectedSubscriptionID("")
      setDeliveries([])
      return
    }

    try {
      const [subData, messageData, outboxData] = await Promise.all([
        eventsApi.listSubscriptions(topicName),
        eventsApi.listMessages(topicName, 100),
        eventsApi.listOutbox(topicName, 100),
      ])
      setSubscriptions(subData)
      setMessages(messageData)
      setOutboxJobs(outboxData)

      if (subData.length > 0) {
        setSelectedSubscriptionID((prev) => {
          if (prev && subData.some((s) => s.id === prev)) {
            return prev
          }
          return subData[0].id
        })
      } else {
        setSelectedSubscriptionID("")
        setDeliveries([])
      }
    } catch (err) {
      console.error("Failed to load topic details:", err)
      setError(toUserErrorMessage(err))
    }
  }, [])

  const fetchDeliveries = useCallback(async (subscriptionID: string) => {
    if (!subscriptionID) {
      setDeliveries([])
      return
    }
    try {
      const data = await eventsApi.listDeliveries(subscriptionID, 100)
      setDeliveries(data)
    } catch (err) {
      console.error("Failed to load deliveries:", err)
      setError(toUserErrorMessage(err))
    }
  }, [])

  useEffect(() => {
    fetchBaseData()
  }, [fetchBaseData])

  useEffect(() => {
    fetchTopicDetails(selectedTopicName)
  }, [selectedTopicName, fetchTopicDetails])

  useEffect(() => {
    setPendingSubscriptionDelete(null)
  }, [selectedTopicName])

  useEffect(() => {
    fetchDeliveries(selectedSubscriptionID)
  }, [selectedSubscriptionID, fetchDeliveries])

  useEffect(() => {
    if (!selectedSubscription) {
      return
    }
    setEditSubMaxInflight(String(selectedSubscription.max_inflight ?? 0))
    setEditSubRateLimitPerS(String(selectedSubscription.rate_limit_per_sec ?? 0))
    const nextSeq = Math.max(1, (selectedSubscription.last_acked_sequence || 0) + 1)
    setReplayFromSequence(String(nextSeq))
    setSeekFromSequence(String(nextSeq))
  }, [selectedSubscription])

  const parseJSONText = (raw: string, fieldName: string): unknown => {
    const text = raw.trim()
    if (!text) {
      return {}
    }
    try {
      return JSON.parse(text)
    } catch {
      throw new Error(t("errors.mustBeValidJson", { fieldName }))
    }
  }

  const handleCreateTopic = async () => {
    if (!createTopicName.trim()) {
      setNotice({ kind: "error", text: t("notices.topicNameRequired") })
      return
    }
    try {
      setBusy(true)
      await eventsApi.createTopic({
        name: createTopicName.trim(),
        description: createTopicDesc.trim() || undefined,
        retention_hours: Math.max(1, Number(createTopicRetentionHours) || 168),
      })
      setCreateTopicName("")
      setCreateTopicDesc("")
      setCreateTopicRetentionHours("168")
      await fetchBaseData()
      setSelectedTopicName(createTopicName.trim())
      setNotice({ kind: "success", text: t("notices.topicCreated", { name: createTopicName.trim() }) })
    } catch (err) {
      console.error("Failed to create topic:", err)
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
    } finally {
      setBusy(false)
    }
  }

  const handleDeleteTopic = async (topicName: string) => {
    if (pendingTopicDelete !== topicName) {
      setPendingTopicDelete(topicName)
      setNotice({ kind: "info", text: t("notices.confirmTopicDelete", { name: topicName }) })
      return
    }
    try {
      setBusy(true)
      await eventsApi.deleteTopic(topicName)
      await fetchBaseData()
      setPendingTopicDelete(null)
      setNotice({ kind: "success", text: t("notices.topicDeleted", { name: topicName }) })
    } catch (err) {
      console.error("Failed to delete topic:", err)
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
    } finally {
      setBusy(false)
    }
  }

  const handleCreateSubscription = async () => {
    if (!selectedTopicName) {
      setNotice({ kind: "error", text: t("notices.selectTopicFirst") })
      return
    }
    if (!newSubName.trim()) {
      setNotice({ kind: "error", text: t("notices.subscriptionNameRequired") })
      return
    }
    if (newSubType === "function" && !newSubFunction) {
      setNotice({ kind: "error", text: t("notices.selectFunction") })
      return
    }
    if (newSubType === "workflow" && !newSubWorkflow) {
      setNotice({ kind: "error", text: t("notices.selectWorkflow") })
      return
    }

    try {
      setBusy(true)
      const base = {
        name: newSubName.trim(),
        consumer_group: newSubGroup.trim() || undefined,
        type: newSubType as "function" | "workflow",
        max_attempts: Math.max(1, Number(newSubMaxAttempts) || 3),
        backoff_base_ms: Math.max(1, Number(newSubBackoffBase) || 1000),
        backoff_max_ms: Math.max(1, Number(newSubBackoffMax) || 60000),
        max_inflight: Math.max(0, Number(newSubMaxInflight) || 0),
        rate_limit_per_sec: Math.max(0, Number(newSubRateLimitPerS) || 0),
      }

      if (newSubType === "function") {
        await eventsApi.createSubscription(selectedTopicName, {
          ...base,
          function_name: newSubFunction,
        })
      } else {
        let webhookHeaders: Record<string, string> | undefined
        try {
          const parsed = JSON.parse(newSubWebhookHeaders)
          if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
            webhookHeaders = parsed
          }
        } catch { /* ignore invalid JSON */ }

        await eventsApi.createSubscription(selectedTopicName, {
          ...base,
          workflow_name: newSubWorkflow,
          webhook_url: newSubWebhookURL.trim() || undefined,
          webhook_method: newSubWebhookMethod || undefined,
          webhook_headers: webhookHeaders,
          webhook_signing_secret: newSubWebhookSecret || undefined,
          webhook_timeout_ms: newSubWebhookURL.trim() ? Math.max(1000, Number(newSubWebhookTimeout) || 30000) : undefined,
        })
      }

      setNewSubName("")
      setNewSubGroup("")
      setNewSubWorkflow(workflows[0]?.name || "")
      setNewSubWebhookURL("")
      setNewSubWebhookSecret("")
      setNewSubMaxInflight("0")
      setNewSubRateLimitPerS("0")
      await fetchTopicDetails(selectedTopicName)
      setNotice({ kind: "success", text: t("notices.subscriptionCreated", { name: newSubName.trim() }) })
    } catch (err) {
      console.error("Failed to create subscription:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.createSubscriptionFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleToggleSubscription = async (sub: EventSubscription) => {
    try {
      setBusy(true)
      await eventsApi.updateSubscription(sub.id, { enabled: !sub.enabled })
      await fetchTopicDetails(selectedTopicName)
      setNotice({
        kind: "success",
        text: t("notices.subscriptionToggled", {
          name: sub.name,
          status: sub.enabled ? t("labels.disabled") : t("labels.enabled"),
        }),
      })
    } catch (err) {
      console.error("Failed to update subscription:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.updateSubscriptionFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleDeleteSubscription = async (sub: EventSubscription) => {
    if (pendingSubscriptionDelete !== sub.id) {
      setPendingSubscriptionDelete(sub.id)
      setNotice({ kind: "info", text: t("notices.confirmSubscriptionDelete", { name: sub.name }) })
      return
    }
    try {
      setBusy(true)
      await eventsApi.deleteSubscription(sub.id)
      await fetchTopicDetails(selectedTopicName)
      setPendingSubscriptionDelete(null)
      setNotice({ kind: "success", text: t("notices.subscriptionDeleted", { name: sub.name }) })
    } catch (err) {
      console.error("Failed to delete subscription:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.deleteSubscriptionFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleSaveSubscriptionFlow = async () => {
    if (!selectedSubscriptionID) {
      setNotice({ kind: "error", text: t("notices.selectSubscriptionFirst") })
      return
    }
    try {
      setBusy(true)
      await eventsApi.updateSubscription(selectedSubscriptionID, {
        max_inflight: Math.max(0, Number(editSubMaxInflight) || 0),
        rate_limit_per_sec: Math.max(0, Number(editSubRateLimitPerS) || 0),
      })
      await fetchTopicDetails(selectedTopicName)
      setNotice({ kind: "success", text: t("notices.subscriptionFlowUpdated") })
    } catch (err) {
      console.error("Failed to update subscription flow controls:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.updateSubscriptionFlowFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handlePublish = async () => {
    if (!selectedTopicName) {
      setNotice({ kind: "error", text: t("notices.selectTopicFirst") })
      return
    }
    try {
      setBusy(true)
      const payload = parseJSONText(publishPayload, t("fields.payloadJson"))
      const headers = parseJSONText(publishHeaders, t("fields.headersJson"))
      const result = await eventsApi.publish(selectedTopicName, {
        payload,
        headers,
        ordering_key: publishOrderingKey.trim() || undefined,
      })
      await fetchTopicDetails(selectedTopicName)
      if (selectedSubscriptionID) {
        await fetchDeliveries(selectedSubscriptionID)
      }
      setNotice({
        kind: "success",
        text: t("notices.publishSuccess", { sequence: result.message.sequence, deliveries: result.deliveries }),
      })
    } catch (err) {
      console.error("Failed to publish event:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.publishFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleEnqueueOutbox = async () => {
    if (!selectedTopicName) {
      setNotice({ kind: "error", text: t("notices.selectTopicFirst") })
      return
    }
    try {
      setBusy(true)
      const payload = parseJSONText(publishPayload, t("fields.payloadJson"))
      const headers = parseJSONText(publishHeaders, t("fields.headersJson"))
      const job = await eventsApi.enqueueOutbox(selectedTopicName, {
        payload,
        headers,
        ordering_key: publishOrderingKey.trim() || undefined,
        max_attempts: Math.max(1, Number(outboxMaxAttempts) || 5),
        backoff_base_ms: Math.max(1, Number(outboxBackoffBase) || 1000),
        backoff_max_ms: Math.max(1, Number(outboxBackoffMax) || 60000),
      })
      await fetchTopicDetails(selectedTopicName)
      setNotice({ kind: "success", text: t("notices.outboxEnqueued", { id: job.id }) })
    } catch (err) {
      console.error("Failed to enqueue outbox event:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.enqueueOutboxFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleReplay = async () => {
    if (!selectedSubscriptionID) {
      setNotice({ kind: "error", text: t("notices.selectSubscriptionFirst") })
      return
    }

    try {
      setBusy(true)
      const response = await eventsApi.replaySubscription(
        selectedSubscriptionID,
        Math.max(1, Number(replayFromSequence) || 1),
        Math.max(1, Number(replayLimit) || 100),
        {
          from_time: replayFromTime.trim() || undefined,
          reset_cursor: replayResetCursor === "true",
        }
      )
      await fetchTopicDetails(selectedTopicName)
      await fetchDeliveries(selectedSubscriptionID)
      setNotice({ kind: "success", text: t("notices.replayQueued", { queued: response.queued }) })
    } catch (err) {
      console.error("Failed to replay:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.replayFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleSeek = async () => {
    if (!selectedSubscriptionID) {
      setNotice({ kind: "error", text: t("notices.selectSubscriptionFirst") })
      return
    }
    try {
      setBusy(true)
      const result = await eventsApi.seekSubscription(
        selectedSubscriptionID,
        Math.max(1, Number(seekFromSequence) || 1),
        seekFromTime.trim() || undefined
      )
      await fetchTopicDetails(selectedTopicName)
      await fetchDeliveries(selectedSubscriptionID)
      setNotice({ kind: "success", text: t("notices.cursorMoved", { sequence: result.from_sequence }) })
    } catch (err) {
      console.error("Failed to seek subscription cursor:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.seekCursorFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleRetryDelivery = async (deliveryID: string) => {
    try {
      setBusy(true)
      await eventsApi.retryDelivery(deliveryID)
      await fetchDeliveries(selectedSubscriptionID)
      setNotice({ kind: "success", text: t("notices.deliveryRetryQueued") })
    } catch (err) {
      console.error("Failed to retry delivery:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.retryDeliveryFailed") })
    } finally {
      setBusy(false)
    }
  }

  const handleRetryOutbox = async (outboxID: string) => {
    try {
      setBusy(true)
      await eventsApi.retryOutbox(outboxID)
      await fetchTopicDetails(selectedTopicName)
      setNotice({ kind: "success", text: t("notices.outboxRetryQueued") })
    } catch (err) {
      console.error("Failed to retry outbox:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : t("errors.retryOutboxFailed") })
    } finally {
      setBusy(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("title")} description={t("description")} />

      <div className="p-6 space-y-6">
        {error && (
          <ErrorBanner error={error} title={t("titles.loadError")} onRetry={fetchBaseData} />
        )}

        {notice && (
          <div
            className={`rounded-lg border p-4 text-sm ${
              notice.kind === "success"
                ? "border-success/50 bg-success/10 text-success"
                : notice.kind === "error"
                  ? "border-destructive/50 bg-destructive/10 text-destructive"
                  : "border-primary/40 bg-primary/10 text-primary"
            }`}
          >
            <div className="flex items-center justify-between gap-3">
              <p>{notice.text}</p>
              <Button variant="ghost" size="sm" onClick={() => setNotice(null)}>
                {t("buttons.dismiss")}
              </Button>
            </div>
          </div>
        )}

        <div className="flex items-center justify-between">
          <Button variant="outline" onClick={fetchBaseData} disabled={loading || busy}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            {t("buttons.refresh")}
          </Button>
        </div>

        <div className="rounded-lg border border-border bg-card p-4">
          <p className="text-sm font-medium text-foreground mb-3">{t("titles.createTopic")}</p>
          <div className="grid gap-3 md:grid-cols-4">
            <div className="space-y-1">
              <Label>{t("fields.topicName")}</Label>
              <Input
                placeholder={t("placeholders.topicName")}
                value={createTopicName}
                onChange={(e) => setCreateTopicName(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>{t("fields.description")}</Label>
              <Input
                value={createTopicDesc}
                onChange={(e) => setCreateTopicDesc(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>{t("fields.retentionHours")}</Label>
              <Input
                type="number"
                min={1}
                value={createTopicRetentionHours}
                onChange={(e) => setCreateTopicRetentionHours(e.target.value)}
              />
            </div>
            <div className="flex items-end">
              <Button onClick={handleCreateTopic} disabled={busy || !createTopicName.trim()}>
                <Plus className="mr-2 h-4 w-4" />
                {t("buttons.createTopic")}
              </Button>
            </div>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-3">
          <div className="rounded-lg border border-border bg-card">
            <div className="border-b border-border px-4 py-3">
              <p className="text-sm font-medium text-foreground">{t("titles.topics")}</p>
            </div>
            <div className="max-h-[520px] overflow-auto">
              {topics.length === 0 ? (
                <div className="p-4">
                  <EmptyState
                    compact
                    title={t("empty.noTopicsTitle")}
                    description={t("empty.noTopicsDescription")}
                  />
                </div>
              ) : (
                <div className="divide-y divide-border">
                  {topics.map((topic) => {
                    const active = topic.name === selectedTopicName
                    return (
                      <div
                        key={topic.id}
                        className={`p-4 cursor-pointer transition-colors ${active ? "bg-muted/60" : "hover:bg-muted/30"}`}
                        onClick={() => setSelectedTopicName(topic.name)}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div>
                            <p className="font-medium text-foreground">{topic.name}</p>
                            <p className="text-xs text-muted-foreground mt-1">{topic.description || "-"}</p>
                            <p className="text-xs text-muted-foreground mt-1">
                              {t("texts.retention", { hours: topic.retention_hours })}
                            </p>
                          </div>
                          {pendingTopicDelete === topic.name ? (
                            <div className="flex items-center gap-1">
                              <Button
                                size="sm"
                                variant="destructive"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  handleDeleteTopic(topic.name)
                                }}
                                disabled={busy}
                              >
                                {t("buttons.confirm")}
                              </Button>
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  setPendingTopicDelete(null)
                                  setNotice(null)
                                }}
                                disabled={busy}
                              >
                                {t("buttons.cancel")}
                              </Button>
                            </div>
                          ) : (
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={(e) => {
                                e.stopPropagation()
                                handleDeleteTopic(topic.name)
                              }}
                              disabled={busy}
                            >
                              <Trash2 className="h-4 w-4 text-destructive" />
                            </Button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </div>

          <div className="space-y-6 lg:col-span-2">
            {!selectedTopic ? (
              <EmptyState
                title={t("empty.selectTopicTitle")}
                description={t("empty.selectTopicDescription")}
                compact
              />
            ) : (
              <>
                <div className="rounded-lg border border-border bg-card p-4 space-y-3">
                  <p className="text-sm font-medium text-foreground">{t("titles.publishTo", { topic: selectedTopic.name })}</p>
                  <div className="grid gap-3 md:grid-cols-2">
                    <div className="space-y-2 md:col-span-2">
                      <Label>{t("fields.payloadJson")}</Label>
                      <Textarea
                        rows={6}
                        value={publishPayload}
                        onChange={(e) => setPublishPayload(e.target.value)}
                        placeholder={t("placeholders.payloadJson")}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("fields.headersJson")}</Label>
                      <Textarea
                        rows={3}
                        value={publishHeaders}
                        onChange={(e) => setPublishHeaders(e.target.value)}
                        placeholder={t("placeholders.headersJson")}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("fields.orderingKey")}</Label>
                      <Input
                        value={publishOrderingKey}
                        onChange={(e) => setPublishOrderingKey(e.target.value)}
                        placeholder={t("placeholders.orderingKey")}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("fields.outboxMaxAttempts")}</Label>
                      <Input
                        type="number"
                        min={1}
                        value={outboxMaxAttempts}
                        onChange={(e) => setOutboxMaxAttempts(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("fields.outboxBackoffBaseMs")}</Label>
                      <Input
                        type="number"
                        min={1}
                        value={outboxBackoffBase}
                        onChange={(e) => setOutboxBackoffBase(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t("fields.outboxBackoffMaxMs")}</Label>
                      <Input
                        type="number"
                        min={1}
                        value={outboxBackoffMax}
                        onChange={(e) => setOutboxBackoffMax(e.target.value)}
                      />
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button onClick={handlePublish} disabled={busy}>
                      <Send className="mr-2 h-4 w-4" />
                      {t("buttons.publishEvent")}
                    </Button>
                    <Button variant="outline" onClick={handleEnqueueOutbox} disabled={busy}>
                      {t("buttons.enqueueOutbox")}
                    </Button>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-card p-4 space-y-4">
                  <p className="text-sm font-medium text-foreground">{t("titles.subscriptions")}</p>

                  <div className="space-y-3">
                    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-12">
                      <div className="space-y-1 xl:col-span-4">
                        <Label>{t("fields.name")}</Label>
                        <Input
                          value={newSubName}
                          onChange={(e) => setNewSubName(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-4">
                        <Label>{t("fields.consumerGroup")}</Label>
                        <Input
                          value={newSubGroup}
                          onChange={(e) => setNewSubGroup(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-2">
                        <Label>{t("fields.type")}</Label>
                        <Select value={newSubType} onValueChange={(v) => setNewSubType(v as "function" | "workflow")}>
                          <SelectTrigger className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="function">{t("fields.function")}</SelectItem>
                            <SelectItem value="workflow">{t("fields.workflow")}</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      {newSubType === "function" ? (
                        <div className="space-y-1 xl:col-span-2">
                          <Label>{t("fields.function")}</Label>
                          <Select value={newSubFunction} onValueChange={setNewSubFunction}>
                            <SelectTrigger className="w-full">
                              <SelectValue placeholder={t("placeholders.selectFunction")} />
                            </SelectTrigger>
                            <SelectContent>
                              {functions.map((fn) => (
                                <SelectItem key={fn.id} value={fn.name}>
                                  {fn.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      ) : (
                        <div className="space-y-1 xl:col-span-2">
                          <Label>{t("fields.workflow")}</Label>
                          <Select value={newSubWorkflow} onValueChange={setNewSubWorkflow}>
                            <SelectTrigger className="w-full">
                              <SelectValue placeholder={t("placeholders.selectWorkflow")} />
                            </SelectTrigger>
                            <SelectContent>
                              {workflows.map((wf) => (
                                <SelectItem key={wf.id} value={wf.name}>
                                  {wf.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      )}
                    </div>

                    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-12">
                      <div className="space-y-1 xl:col-span-2">
                        <Label className="flex min-h-[2.5rem] items-end leading-tight">{t("fields.maxAttempts")}</Label>
                        <Input
                          type="number"
                          min={1}
                          value={newSubMaxAttempts}
                          onChange={(e) => setNewSubMaxAttempts(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-3">
                        <Label className="flex min-h-[2.5rem] items-end leading-tight">{t("fields.backoffBaseMs")}</Label>
                        <Input
                          type="number"
                          min={1}
                          value={newSubBackoffBase}
                          onChange={(e) => setNewSubBackoffBase(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-3">
                        <Label className="flex min-h-[2.5rem] items-end leading-tight">{t("fields.backoffMaxMs")}</Label>
                        <Input
                          type="number"
                          min={1}
                          value={newSubBackoffMax}
                          onChange={(e) => setNewSubBackoffMax(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-2">
                        <Label className="flex min-h-[2.5rem] items-end leading-tight">{t("fields.maxInflightUnlimited")}</Label>
                        <Input
                          type="number"
                          min={0}
                          value={newSubMaxInflight}
                          onChange={(e) => setNewSubMaxInflight(e.target.value)}
                        />
                      </div>
                      <div className="space-y-1 xl:col-span-2">
                        <Label className="flex min-h-[2.5rem] items-end leading-tight">{t("fields.rateLimitPerSecUnlimited")}</Label>
                        <Input
                          type="number"
                          min={0}
                          value={newSubRateLimitPerS}
                          onChange={(e) => setNewSubRateLimitPerS(e.target.value)}
                        />
                      </div>
                    </div>

                    {newSubType === "workflow" && (
                      <div className="space-y-3 rounded-md border border-dashed border-border bg-muted/20 p-3">
                        <p className="text-xs font-medium text-muted-foreground">{t("fields.webhookOptional")}</p>
                        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-12">
                          <div className="space-y-1 xl:col-span-5">
                            <Label>{t("fields.webhookUrl")}</Label>
                            <Input
                              placeholder={t("placeholders.webhookUrl")}
                              value={newSubWebhookURL}
                              onChange={(e) => setNewSubWebhookURL(e.target.value)}
                            />
                          </div>
                          <div className="space-y-1 xl:col-span-2">
                            <Label>{t("fields.method")}</Label>
                            <Select value={newSubWebhookMethod} onValueChange={setNewSubWebhookMethod}>
                              <SelectTrigger className="w-full">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="POST">POST</SelectItem>
                                <SelectItem value="PUT">PUT</SelectItem>
                                <SelectItem value="PATCH">PATCH</SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                          <div className="space-y-1 xl:col-span-2">
                            <Label>{t("fields.timeoutMs")}</Label>
                            <Input
                              type="number"
                              min={1000}
                              value={newSubWebhookTimeout}
                              onChange={(e) => setNewSubWebhookTimeout(e.target.value)}
                            />
                          </div>
                          <div className="space-y-1 xl:col-span-3">
                            <Label>{t("fields.signingSecret")}</Label>
                            <Input
                              value={newSubWebhookSecret}
                              onChange={(e) => setNewSubWebhookSecret(e.target.value)}
                            />
                          </div>
                          <div className="space-y-1 xl:col-span-12">
                            <Label>{t("fields.headersJson")}</Label>
                            <Textarea
                              rows={3}
                              value={newSubWebhookHeaders}
                              onChange={(e) => setNewSubWebhookHeaders(e.target.value)}
                              placeholder={t("placeholders.webhookHeadersJson")}
                            />
                          </div>
                        </div>
                      </div>
                    )}

                    <div className="flex justify-end">
                      <Button onClick={handleCreateSubscription} disabled={busy || !newSubName.trim() || (newSubType === "function" ? !newSubFunction : !newSubWorkflow)}>
                        <Plus className="mr-2 h-4 w-4" />
                        {t("buttons.addSubscription")}
                      </Button>
                    </div>
                  </div>

                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.name")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.type")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.target")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.cursor")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.lag")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.backlog")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.flow")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.status")}</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">{t("fields.actions")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {subscriptions.length === 0 ? (
                          <tr>
                            <td colSpan={9} className="px-3 py-4 text-center text-muted-foreground">{t("empty.noSubscriptions")}</td>
                          </tr>
                        ) : (
                          subscriptions.map((sub) => {
                            const isSelected = sub.id === selectedSubscriptionID
                            return (
                              <tr
                                key={sub.id}
                                className={`border-b border-border last:border-0 ${isSelected ? "bg-muted/50" : "hover:bg-muted/20"}`}
                              >
                                <td className="px-3 py-2">
                                  <button
                                    className="font-medium text-foreground hover:text-primary"
                                    onClick={() => setSelectedSubscriptionID(sub.id)}
                                  >
                                    {sub.name}
                                  </button>
                                </td>
                                <td className="px-3 py-2">
                                  <Badge variant="outline">{sub.type ? t(`types.${sub.type}`) : t("types.function")}</Badge>
                                </td>
                                <td className="px-3 py-2 text-muted-foreground text-xs max-w-[220px] truncate" title={sub.type === "workflow" ? (sub.webhook_url || sub.workflow_name) : sub.function_name}>
                                  {sub.type === "workflow" ? (sub.workflow_name || "-") : sub.function_name}
                                  {sub.type === "workflow" && sub.webhook_url ? ` -> ${sub.webhook_url}` : ""}
                                </td>
                                <td className="px-3 py-2 text-muted-foreground">{sub.last_acked_sequence}</td>
                                <td className="px-3 py-2 text-muted-foreground">{sub.lag}</td>
                                <td className="px-3 py-2 text-muted-foreground">
                                  {t("texts.subscriptionBacklog", { inflight: sub.inflight, queued: sub.queued, dlq: sub.dlq })}
                                </td>
                                <td className="px-3 py-2 text-muted-foreground">
                                  {t("texts.subscriptionFlow", {
                                    inflight: sub.max_inflight || t("labels.unlimited"),
                                    rate: sub.rate_limit_per_sec || t("labels.unlimited"),
                                  })}
                                </td>
                                <td className="px-3 py-2">
                                  <Badge variant={sub.enabled ? "default" : "secondary"}>
                                    {sub.enabled ? t("labels.enabled") : t("labels.disabled")}
                                  </Badge>
                                </td>
                                <td className="px-3 py-2">
                                  <div className="flex justify-end gap-2">
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleToggleSubscription(sub)}
                                      disabled={busy}
                                    >
                                      {sub.enabled ? t("buttons.disable") : t("buttons.enable")}
                                    </Button>
                                    {pendingSubscriptionDelete === sub.id ? (
                                      <div className="flex items-center gap-1">
                                        <Button
                                          size="sm"
                                          variant="destructive"
                                          onClick={() => handleDeleteSubscription(sub)}
                                          disabled={busy}
                                        >
                                          {t("buttons.confirm")}
                                        </Button>
                                        <Button
                                          size="sm"
                                          variant="outline"
                                          onClick={() => {
                                            setPendingSubscriptionDelete(null)
                                            setNotice(null)
                                          }}
                                          disabled={busy}
                                        >
                                          {t("buttons.cancel")}
                                        </Button>
                                      </div>
                                    ) : (
                                      <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() => handleDeleteSubscription(sub)}
                                        disabled={busy}
                                      >
                                        <Trash2 className="h-4 w-4 text-destructive" />
                                      </Button>
                                    )}
                                  </div>
                                </td>
                              </tr>
                            )
                          })
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-card p-4 space-y-4">
                  <div className="space-y-3">
                    <div>
                      <p className="text-sm font-medium text-foreground">{t("titles.deliveries")}</p>
                      <p className="text-xs text-muted-foreground">
                        {selectedSubscription
                          ? t("texts.selectedSubscription", { name: selectedSubscription.name })
                          : t("empty.selectSubscription")}
                      </p>
                      {selectedSubscription && (
                        <p className="text-xs text-muted-foreground mt-1">
                          {t("texts.deliveryCursorSummary", {
                            cursor: selectedSubscription.last_acked_sequence,
                            lag: selectedSubscription.lag,
                            oldest: selectedSubscription.oldest_unacked_age_s ?? 0,
                          })}
                        </p>
                      )}
                    </div>

                    <div className="grid gap-3 xl:grid-cols-3">
                      <div className="space-y-2 rounded-md border border-border p-3">
                        <p className="text-xs font-medium text-muted-foreground">{t("titles.flowControls")}</p>
                        <div className="grid gap-2 sm:grid-cols-2">
                          <div className="space-y-1">
                            <Label>{t("fields.maxInflight")}</Label>
                            <Input
                              type="number"
                              min={0}
                              value={editSubMaxInflight}
                              onChange={(e) => setEditSubMaxInflight(e.target.value)}
                              disabled={!selectedSubscriptionID}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label>{t("fields.ratePerSecond")}</Label>
                            <Input
                              type="number"
                              min={0}
                              value={editSubRateLimitPerS}
                              onChange={(e) => setEditSubRateLimitPerS(e.target.value)}
                              disabled={!selectedSubscriptionID}
                            />
                          </div>
                        </div>
                        <Button className="w-full" variant="outline" onClick={handleSaveSubscriptionFlow} disabled={busy || !selectedSubscriptionID}>
                          {t("buttons.saveFlow")}
                        </Button>
                      </div>

                      <div className="space-y-2 rounded-md border border-border p-3">
                        <p className="text-xs font-medium text-muted-foreground">{t("titles.seekCursor")}</p>
                        <div className="space-y-1">
                          <Label>{t("fields.seekSequence")}</Label>
                          <Input
                            type="number"
                            min={1}
                            value={seekFromSequence}
                            onChange={(e) => setSeekFromSequence(e.target.value)}
                            disabled={!selectedSubscriptionID}
                          />
                        </div>
                        <div className="space-y-1">
                          <Label>{t("fields.seekTimeRfc3339")}</Label>
                          <Input
                            type="text"
                            value={seekFromTime}
                            onChange={(e) => setSeekFromTime(e.target.value)}
                            disabled={!selectedSubscriptionID}
                          />
                        </div>
                        <Button className="w-full" variant="outline" onClick={handleSeek} disabled={busy || !selectedSubscriptionID}>
                          {t("buttons.seekCursor")}
                        </Button>
                      </div>

                      <div className="space-y-2 rounded-md border border-border p-3">
                        <p className="text-xs font-medium text-muted-foreground">{t("titles.replay")}</p>
                        <div className="grid gap-2 sm:grid-cols-2">
                          <div className="space-y-1">
                            <Label>{t("fields.fromSequence")}</Label>
                            <Input
                              type="number"
                              min={1}
                              value={replayFromSequence}
                              onChange={(e) => setReplayFromSequence(e.target.value)}
                              disabled={!selectedSubscriptionID}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label>{t("fields.limit")}</Label>
                            <Input
                              type="number"
                              min={1}
                              value={replayLimit}
                              onChange={(e) => setReplayLimit(e.target.value)}
                              disabled={!selectedSubscriptionID}
                            />
                          </div>
                        </div>
                        <div className="space-y-1">
                          <Label>{t("fields.fromTimeRfc3339")}</Label>
                          <Input
                            type="text"
                            value={replayFromTime}
                            onChange={(e) => setReplayFromTime(e.target.value)}
                            disabled={!selectedSubscriptionID}
                          />
                        </div>
                        <div className="space-y-1">
                          <Label>{t("fields.cursorReset")}</Label>
                          <Select value={replayResetCursor} onValueChange={setReplayResetCursor}>
                            <SelectTrigger className="w-full" disabled={!selectedSubscriptionID}>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="true">{t("options.replayResetCursor")}</SelectItem>
                              <SelectItem value="false">{t("options.replayOnly")}</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                        <Button className="w-full" variant="outline" onClick={handleReplay} disabled={busy || !selectedSubscriptionID}>
                          <RotateCcw className="mr-2 h-4 w-4" />
                          {t("buttons.replay")}
                        </Button>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.seq")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.key")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.status")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.attempt")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.updated")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.error")}</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">{t("fields.actions")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {deliveries.length === 0 ? (
                          <tr>
                            <td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">{t("empty.noDeliveries")}</td>
                          </tr>
                        ) : (
                          deliveries.map((delivery) => (
                            <tr key={delivery.id} className="border-b border-border last:border-0">
                              <td className="px-3 py-2 text-muted-foreground">{delivery.message_sequence}</td>
                              <td className="px-3 py-2 text-muted-foreground">{delivery.ordering_key || "-"}</td>
                              <td className="px-3 py-2">
                                <Badge variant={statusBadgeVariant(delivery.status)}>{t(`deliveryStatus.${delivery.status}`)}</Badge>
                              </td>
                              <td className="px-3 py-2 text-muted-foreground">
                                {delivery.attempt}/{delivery.max_attempts}
                              </td>
                              <td className="px-3 py-2 text-muted-foreground">{formatDate(delivery.updated_at)}</td>
                              <td className="px-3 py-2 text-muted-foreground max-w-[280px] truncate">
                                {delivery.last_error || "-"}
                              </td>
                              <td className="px-3 py-2 text-right">
                                {delivery.status === "dlq" ? (
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => handleRetryDelivery(delivery.id)}
                                    disabled={busy}
                                  >
                                    {t("buttons.retry")}
                                  </Button>
                                ) : (
                                  <span className="text-muted-foreground">-</span>
                                )}
                              </td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-card p-4">
                  <p className="text-sm font-medium text-foreground mb-2">{t("titles.recentMessages")}</p>
                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.sequence")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.orderingKey")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.published")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {messages.length === 0 ? (
                          <tr>
                            <td colSpan={3} className="px-3 py-4 text-center text-muted-foreground">{t("empty.noMessages")}</td>
                          </tr>
                        ) : (
                          messages.map((message) => (
                            <tr key={message.id} className="border-b border-border last:border-0">
                              <td className="px-3 py-2 text-muted-foreground">{message.sequence}</td>
                              <td className="px-3 py-2 text-muted-foreground">{message.ordering_key || "-"}</td>
                              <td className="px-3 py-2 text-muted-foreground">{formatDate(message.published_at)}</td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-card p-4">
                  <p className="text-sm font-medium text-foreground mb-2">{t("titles.outboxJobs")}</p>
                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.id")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.status")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.attempt")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.message")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.nextAttempt")}</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("fields.error")}</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">{t("fields.actions")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {outboxJobs.length === 0 ? (
                          <tr>
                            <td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">{t("empty.noOutboxJobs")}</td>
                          </tr>
                        ) : (
                          outboxJobs.map((job) => (
                            <tr key={job.id} className="border-b border-border last:border-0">
                              <td className="px-3 py-2 text-xs text-muted-foreground">{job.id.slice(0, 8)}...</td>
                              <td className="px-3 py-2">
                                <Badge variant={
                                  job.status === "published"
                                    ? "default"
                                    : job.status === "failed"
                                      ? "destructive"
                                      : "secondary"
                                }>
                                  {t(`outboxStatus.${job.status}`)}
                                </Badge>
                              </td>
                              <td className="px-3 py-2 text-muted-foreground">{job.attempt}/{job.max_attempts}</td>
                              <td className="px-3 py-2 text-muted-foreground">{job.message_id || "-"}</td>
                              <td className="px-3 py-2 text-muted-foreground">{formatDate(job.next_attempt_at)}</td>
                              <td className="px-3 py-2 text-muted-foreground max-w-[280px] truncate">{job.last_error || "-"}</td>
                              <td className="px-3 py-2 text-right">
                                {job.status === "failed" ? (
                                  <Button variant="outline" size="sm" onClick={() => handleRetryOutbox(job.id)} disabled={busy}>
                                    {t("buttons.retry")}
                                  </Button>
                                ) : (
                                  <span className="text-muted-foreground">-</span>
                                )}
                              </td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </DashboardLayout>
  )
}

"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
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
  type EventDelivery,
  type EventDeliveryStatus,
  type EventMessage,
  type EventOutboxJob,
  type EventSubscription,
  type EventTopic,
  type NovaFunction,
} from "@/lib/api"
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

export default function EventsPage() {
  const [topics, setTopics] = useState<EventTopic[]>([])
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [selectedTopicName, setSelectedTopicName] = useState("")
  const [subscriptions, setSubscriptions] = useState<EventSubscription[]>([])
  const [messages, setMessages] = useState<EventMessage[]>([])
  const [outboxJobs, setOutboxJobs] = useState<EventOutboxJob[]>([])
  const [selectedSubscriptionID, setSelectedSubscriptionID] = useState("")
  const [deliveries, setDeliveries] = useState<EventDelivery[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createTopicName, setCreateTopicName] = useState("")
  const [createTopicDesc, setCreateTopicDesc] = useState("")
  const [createTopicRetentionHours, setCreateTopicRetentionHours] = useState("168")

  const [newSubName, setNewSubName] = useState("")
  const [newSubGroup, setNewSubGroup] = useState("")
  const [newSubType, setNewSubType] = useState<"function" | "webhook">("function")
  const [newSubFunction, setNewSubFunction] = useState("")
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
  const [newSubTransformFn, setNewSubTransformFn] = useState("__none__")

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
      const [topicData, functionData] = await Promise.all([
        eventsApi.listTopics(200),
        functionsApi.list(),
      ])
      setTopics(topicData)
      setFunctions(functionData)

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
    } catch (err) {
      console.error("Failed to load event bus data:", err)
      setError(err instanceof Error ? err.message : "Failed to load event bus data")
    } finally {
      setLoading(false)
    }
  }, [newSubFunction])

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
      setError(err instanceof Error ? err.message : "Failed to load topic details")
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
      setError(err instanceof Error ? err.message : "Failed to load deliveries")
    }
  }, [])

  useEffect(() => {
    fetchBaseData()
  }, [fetchBaseData])

  useEffect(() => {
    fetchTopicDetails(selectedTopicName)
  }, [selectedTopicName, fetchTopicDetails])

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
      throw new Error(`${fieldName} must be valid JSON`)
    }
  }

  const handleCreateTopic = async () => {
    if (!createTopicName.trim()) {
      alert("Topic name is required")
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
    } catch (err) {
      console.error("Failed to create topic:", err)
      alert(err instanceof Error ? err.message : "Failed to create topic")
    } finally {
      setBusy(false)
    }
  }

  const handleDeleteTopic = async (topicName: string) => {
    if (!confirm(`Delete topic "${topicName}" and all subscriptions/messages?`)) return
    try {
      setBusy(true)
      await eventsApi.deleteTopic(topicName)
      await fetchBaseData()
    } catch (err) {
      console.error("Failed to delete topic:", err)
      alert(err instanceof Error ? err.message : "Failed to delete topic")
    } finally {
      setBusy(false)
    }
  }

  const handleCreateSubscription = async () => {
    if (!selectedTopicName) {
      alert("Select a topic first")
      return
    }
    if (!newSubName.trim()) {
      alert("Subscription name is required")
      return
    }
    if (newSubType === "function" && !newSubFunction) {
      alert("Select a function")
      return
    }
    if (newSubType === "webhook" && !newSubWebhookURL.trim()) {
      alert("Webhook URL is required")
      return
    }

    try {
      setBusy(true)
      const base = {
        name: newSubName.trim(),
        consumer_group: newSubGroup.trim() || undefined,
        type: newSubType as "function" | "webhook",
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
          webhook_url: newSubWebhookURL.trim(),
          webhook_method: newSubWebhookMethod || "POST",
          webhook_headers: webhookHeaders,
          webhook_signing_secret: newSubWebhookSecret || undefined,
          webhook_timeout_ms: Math.max(1000, Number(newSubWebhookTimeout) || 30000),
          transform_function_name: newSubTransformFn && newSubTransformFn !== "__none__" ? newSubTransformFn : undefined,
        })
      }

      setNewSubName("")
      setNewSubGroup("")
      setNewSubWebhookURL("")
      setNewSubWebhookSecret("")
      setNewSubTransformFn("__none__")
      setNewSubMaxInflight("0")
      setNewSubRateLimitPerS("0")
      await fetchTopicDetails(selectedTopicName)
    } catch (err) {
      console.error("Failed to create subscription:", err)
      alert(err instanceof Error ? err.message : "Failed to create subscription")
    } finally {
      setBusy(false)
    }
  }

  const handleToggleSubscription = async (sub: EventSubscription) => {
    try {
      setBusy(true)
      await eventsApi.updateSubscription(sub.id, { enabled: !sub.enabled })
      await fetchTopicDetails(selectedTopicName)
    } catch (err) {
      console.error("Failed to update subscription:", err)
      alert(err instanceof Error ? err.message : "Failed to update subscription")
    } finally {
      setBusy(false)
    }
  }

  const handleDeleteSubscription = async (sub: EventSubscription) => {
    if (!confirm(`Delete subscription "${sub.name}"?`)) return
    try {
      setBusy(true)
      await eventsApi.deleteSubscription(sub.id)
      await fetchTopicDetails(selectedTopicName)
    } catch (err) {
      console.error("Failed to delete subscription:", err)
      alert(err instanceof Error ? err.message : "Failed to delete subscription")
    } finally {
      setBusy(false)
    }
  }

  const handleSaveSubscriptionFlow = async () => {
    if (!selectedSubscriptionID) {
      alert("Select a subscription first")
      return
    }
    try {
      setBusy(true)
      await eventsApi.updateSubscription(selectedSubscriptionID, {
        max_inflight: Math.max(0, Number(editSubMaxInflight) || 0),
        rate_limit_per_sec: Math.max(0, Number(editSubRateLimitPerS) || 0),
      })
      await fetchTopicDetails(selectedTopicName)
    } catch (err) {
      console.error("Failed to update subscription flow controls:", err)
      alert(err instanceof Error ? err.message : "Failed to update subscription flow controls")
    } finally {
      setBusy(false)
    }
  }

  const handlePublish = async () => {
    if (!selectedTopicName) {
      alert("Select a topic first")
      return
    }
    try {
      setBusy(true)
      const payload = parseJSONText(publishPayload, "Payload")
      const headers = parseJSONText(publishHeaders, "Headers")
      const result = await eventsApi.publish(selectedTopicName, {
        payload,
        headers,
        ordering_key: publishOrderingKey.trim() || undefined,
      })
      await fetchTopicDetails(selectedTopicName)
      if (selectedSubscriptionID) {
        await fetchDeliveries(selectedSubscriptionID)
      }
      alert(`Published message #${result.message.sequence} with ${result.deliveries} delivery fanout`) 
    } catch (err) {
      console.error("Failed to publish event:", err)
      alert(err instanceof Error ? err.message : "Failed to publish event")
    } finally {
      setBusy(false)
    }
  }

  const handleEnqueueOutbox = async () => {
    if (!selectedTopicName) {
      alert("Select a topic first")
      return
    }
    try {
      setBusy(true)
      const payload = parseJSONText(publishPayload, "Payload")
      const headers = parseJSONText(publishHeaders, "Headers")
      const job = await eventsApi.enqueueOutbox(selectedTopicName, {
        payload,
        headers,
        ordering_key: publishOrderingKey.trim() || undefined,
        max_attempts: Math.max(1, Number(outboxMaxAttempts) || 5),
        backoff_base_ms: Math.max(1, Number(outboxBackoffBase) || 1000),
        backoff_max_ms: Math.max(1, Number(outboxBackoffMax) || 60000),
      })
      await fetchTopicDetails(selectedTopicName)
      alert(`Outbox enqueued: ${job.id}`)
    } catch (err) {
      console.error("Failed to enqueue outbox event:", err)
      alert(err instanceof Error ? err.message : "Failed to enqueue outbox event")
    } finally {
      setBusy(false)
    }
  }

  const handleReplay = async () => {
    if (!selectedSubscriptionID) {
      alert("Select a subscription first")
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
      alert(`Replay queued ${response.queued} deliveries`)
    } catch (err) {
      console.error("Failed to replay:", err)
      alert(err instanceof Error ? err.message : "Failed to replay")
    } finally {
      setBusy(false)
    }
  }

  const handleSeek = async () => {
    if (!selectedSubscriptionID) {
      alert("Select a subscription first")
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
      alert(`Cursor moved. Next replay/invoke starts from sequence ${result.from_sequence}`)
    } catch (err) {
      console.error("Failed to seek subscription cursor:", err)
      alert(err instanceof Error ? err.message : "Failed to seek subscription cursor")
    } finally {
      setBusy(false)
    }
  }

  const handleRetryDelivery = async (deliveryID: string) => {
    try {
      setBusy(true)
      await eventsApi.retryDelivery(deliveryID)
      await fetchDeliveries(selectedSubscriptionID)
    } catch (err) {
      console.error("Failed to retry delivery:", err)
      alert(err instanceof Error ? err.message : "Failed to retry delivery")
    } finally {
      setBusy(false)
    }
  }

  const handleRetryOutbox = async (outboxID: string) => {
    try {
      setBusy(true)
      await eventsApi.retryOutbox(outboxID)
      await fetchTopicDetails(selectedTopicName)
    } catch (err) {
      console.error("Failed to retry outbox:", err)
      alert(err instanceof Error ? err.message : "Failed to retry outbox")
    } finally {
      setBusy(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title="Events" description="Topic / Subscription / Consumer Group event bus" />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load event bus</p>
            <p className="text-sm mt-1">{error}</p>
          </div>
        )}

        <div className="flex items-center justify-between">
          <Button variant="outline" onClick={fetchBaseData} disabled={loading || busy}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>

        <div className="rounded-lg border border-border bg-card p-4">
          <p className="text-sm font-medium text-foreground mb-3">Create Topic</p>
          <div className="grid gap-3 md:grid-cols-4">
            <div className="space-y-1">
              <Label>Topic Name</Label>
              <Input
                placeholder="orders.created"
                value={createTopicName}
                onChange={(e) => setCreateTopicName(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>Description</Label>
              <Input
                value={createTopicDesc}
                onChange={(e) => setCreateTopicDesc(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label>Retention (hours)</Label>
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
                Create Topic
              </Button>
            </div>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-3">
          <div className="rounded-lg border border-border bg-card">
            <div className="border-b border-border px-4 py-3">
              <p className="text-sm font-medium text-foreground">Topics</p>
            </div>
            <div className="max-h-[520px] overflow-auto">
              {topics.length === 0 ? (
                <div className="p-4 text-sm text-muted-foreground">No topics yet.</div>
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
                            <p className="text-xs text-muted-foreground mt-1">Retention: {topic.retention_hours}h</p>
                          </div>
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
              <div className="rounded-lg border border-border bg-card p-6 text-sm text-muted-foreground">
                Select a topic to manage subscriptions and publish events.
              </div>
            ) : (
              <>
                <div className="rounded-lg border border-border bg-card p-4 space-y-3">
                  <p className="text-sm font-medium text-foreground">Publish to {selectedTopic.name}</p>
                  <div className="grid gap-3 md:grid-cols-2">
                    <div className="space-y-2 md:col-span-2">
                      <Label>Payload JSON</Label>
                      <Textarea
                        rows={6}
                        value={publishPayload}
                        onChange={(e) => setPublishPayload(e.target.value)}
                        placeholder='{"order_id": "123", "amount": 10.5}'
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Headers JSON</Label>
                      <Textarea
                        rows={3}
                        value={publishHeaders}
                        onChange={(e) => setPublishHeaders(e.target.value)}
                        placeholder='{"source": "api"}'
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Ordering Key</Label>
                      <Input
                        value={publishOrderingKey}
                        onChange={(e) => setPublishOrderingKey(e.target.value)}
                        placeholder="customer-42"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Outbox Max Attempts</Label>
                      <Input
                        type="number"
                        min={1}
                        value={outboxMaxAttempts}
                        onChange={(e) => setOutboxMaxAttempts(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Outbox Backoff Base (ms)</Label>
                      <Input
                        type="number"
                        min={1}
                        value={outboxBackoffBase}
                        onChange={(e) => setOutboxBackoffBase(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Outbox Backoff Max (ms)</Label>
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
                      Publish Event
                    </Button>
                    <Button variant="outline" onClick={handleEnqueueOutbox} disabled={busy}>
                      Enqueue Outbox
                    </Button>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-card p-4 space-y-4">
                  <p className="text-sm font-medium text-foreground">Subscriptions</p>

                  <div className="grid gap-3 md:grid-cols-4">
                    <div className="space-y-1">
                      <Label>Name</Label>
                      <Input
                        value={newSubName}
                        onChange={(e) => setNewSubName(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Consumer Group</Label>
                      <Input
                        value={newSubGroup}
                        onChange={(e) => setNewSubGroup(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Type</Label>
                      <Select value={newSubType} onValueChange={(v) => setNewSubType(v as "function" | "webhook")}>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="function">Function</SelectItem>
                          <SelectItem value="webhook">Webhook</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {newSubType === "function" ? (
                      <div className="space-y-1">
                        <Label>Function</Label>
                        <Select value={newSubFunction} onValueChange={setNewSubFunction}>
                          <SelectTrigger>
                            <SelectValue placeholder="Select function" />
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
                      <div className="space-y-1">
                        <Label>Webhook URL</Label>
                        <Input
                          placeholder="https://..."
                          value={newSubWebhookURL}
                          onChange={(e) => setNewSubWebhookURL(e.target.value)}
                        />
                      </div>
                    )}
                    {newSubType === "webhook" && (
                      <>
                        <div className="space-y-1">
                          <Label>Method</Label>
                          <Select value={newSubWebhookMethod} onValueChange={setNewSubWebhookMethod}>
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="POST">POST</SelectItem>
                              <SelectItem value="PUT">PUT</SelectItem>
                              <SelectItem value="PATCH">PATCH</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                        <div className="space-y-1">
                          <Label>Signing Secret</Label>
                          <Input
                            value={newSubWebhookSecret}
                            onChange={(e) => setNewSubWebhookSecret(e.target.value)}
                          />
                        </div>
                        <div className="space-y-1">
                          <Label>Timeout (ms)</Label>
                          <Input
                            type="number"
                            min={1000}
                            value={newSubWebhookTimeout}
                            onChange={(e) => setNewSubWebhookTimeout(e.target.value)}
                          />
                        </div>
                        <div className="space-y-1">
                          <Label>Transform Function</Label>
                          <Select value={newSubTransformFn} onValueChange={setNewSubTransformFn}>
                            <SelectTrigger>
                              <SelectValue placeholder="None" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="__none__">None</SelectItem>
                              {functions.map((fn) => (
                                <SelectItem key={fn.id} value={fn.name}>
                                  {fn.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      </>
                    )}
                    <div className="space-y-1">
                      <Label>Max Attempts</Label>
                      <Input
                        type="number"
                        min={1}
                        value={newSubMaxAttempts}
                        onChange={(e) => setNewSubMaxAttempts(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Backoff Base (ms)</Label>
                      <Input
                        type="number"
                        min={1}
                        value={newSubBackoffBase}
                        onChange={(e) => setNewSubBackoffBase(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Backoff Max (ms)</Label>
                      <Input
                        type="number"
                        min={1}
                        value={newSubBackoffMax}
                        onChange={(e) => setNewSubBackoffMax(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Max Inflight (0=unlimited)</Label>
                      <Input
                        type="number"
                        min={0}
                        value={newSubMaxInflight}
                        onChange={(e) => setNewSubMaxInflight(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Rate Limit/s (0=unlimited)</Label>
                      <Input
                        type="number"
                        min={0}
                        value={newSubRateLimitPerS}
                        onChange={(e) => setNewSubRateLimitPerS(e.target.value)}
                      />
                    </div>
                  </div>

                  <Button onClick={handleCreateSubscription} disabled={busy || !newSubName.trim() || (newSubType === "function" ? !newSubFunction : !newSubWebhookURL.trim())}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add Subscription
                  </Button>

                  <div className="rounded-md border border-border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Name</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Type</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Target</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Cursor</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Lag</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Backlog</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Flow</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {subscriptions.length === 0 ? (
                          <tr>
                            <td colSpan={9} className="px-3 py-4 text-center text-muted-foreground">No subscriptions.</td>
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
                                  <Badge variant="outline">{sub.type || "function"}</Badge>
                                </td>
                                <td className="px-3 py-2 text-muted-foreground text-xs max-w-[200px] truncate" title={sub.type === "webhook" ? sub.webhook_url : sub.function_name}>
                                  {sub.type === "webhook" ? sub.webhook_url : sub.function_name}
                                  {sub.type === "webhook" && sub.transform_function_name ? ` (transform: ${sub.transform_function_name})` : ""}
                                </td>
                                <td className="px-3 py-2 text-muted-foreground">{sub.last_acked_sequence}</td>
                                <td className="px-3 py-2 text-muted-foreground">{sub.lag}</td>
                                <td className="px-3 py-2 text-muted-foreground">
                                  r{sub.inflight} / q{sub.queued} / dlq{sub.dlq}
                                </td>
                                <td className="px-3 py-2 text-muted-foreground">
                                  inflight {sub.max_inflight || "unlimited"} · rate {sub.rate_limit_per_sec || "unlimited"}/s
                                </td>
                                <td className="px-3 py-2">
                                  <Badge variant={sub.enabled ? "default" : "secondary"}>
                                    {sub.enabled ? "enabled" : "disabled"}
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
                                      {sub.enabled ? "Disable" : "Enable"}
                                    </Button>
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      onClick={() => handleDeleteSubscription(sub)}
                                      disabled={busy}
                                    >
                                      <Trash2 className="h-4 w-4 text-destructive" />
                                    </Button>
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
                      <p className="text-sm font-medium text-foreground">Deliveries</p>
                      <p className="text-xs text-muted-foreground">
                        {selectedSubscription ? `Subscription: ${selectedSubscription.name}` : "Select a subscription"}
                      </p>
                      {selectedSubscription && (
                        <p className="text-xs text-muted-foreground mt-1">
                          Cursor {selectedSubscription.last_acked_sequence} · lag {selectedSubscription.lag} · oldest unacked {selectedSubscription.oldest_unacked_age_s ?? 0}s
                        </p>
                      )}
                    </div>

                    <div className="grid gap-2 md:grid-cols-6">
                      <div className="space-y-1">
                        <Label>Max Inflight</Label>
                        <Input
                          type="number"
                          min={0}
                          value={editSubMaxInflight}
                          onChange={(e) => setEditSubMaxInflight(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>Rate/s</Label>
                        <Input
                          type="number"
                          min={0}
                          value={editSubRateLimitPerS}
                          onChange={(e) => setEditSubRateLimitPerS(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="flex items-end">
                        <Button variant="outline" onClick={handleSaveSubscriptionFlow} disabled={busy || !selectedSubscriptionID}>
                          Save Flow
                        </Button>
                      </div>
                      <div className="space-y-1">
                        <Label>Seek Sequence</Label>
                        <Input
                          type="number"
                          min={1}
                          value={seekFromSequence}
                          onChange={(e) => setSeekFromSequence(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>Seek Time (RFC3339)</Label>
                        <Input
                          type="text"
                          value={seekFromTime}
                          onChange={(e) => setSeekFromTime(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="flex items-end">
                        <Button variant="outline" onClick={handleSeek} disabled={busy || !selectedSubscriptionID}>
                          Seek Cursor
                        </Button>
                      </div>
                    </div>

                    <div className="grid gap-2 md:grid-cols-6">
                      <div className="space-y-1">
                        <Label>From Sequence</Label>
                        <Input
                          type="number"
                          min={1}
                          value={replayFromSequence}
                          onChange={(e) => setReplayFromSequence(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>Limit</Label>
                        <Input
                          type="number"
                          min={1}
                          value={replayLimit}
                          onChange={(e) => setReplayLimit(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>From Time (RFC3339)</Label>
                        <Input
                          type="text"
                          value={replayFromTime}
                          onChange={(e) => setReplayFromTime(e.target.value)}
                          disabled={!selectedSubscriptionID}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>Cursor Reset</Label>
                        <Select value={replayResetCursor} onValueChange={setReplayResetCursor}>
                          <SelectTrigger disabled={!selectedSubscriptionID}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="true">Replay + reset cursor</SelectItem>
                            <SelectItem value="false">Replay only</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="md:col-span-2 flex items-end">
                        <Button className="w-full" variant="outline" onClick={handleReplay} disabled={busy || !selectedSubscriptionID}>
                          <RotateCcw className="mr-2 h-4 w-4" />
                          Replay
                        </Button>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Seq</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Key</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Attempt</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Updated</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Error</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {deliveries.length === 0 ? (
                          <tr>
                            <td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">No deliveries yet.</td>
                          </tr>
                        ) : (
                          deliveries.map((delivery) => (
                            <tr key={delivery.id} className="border-b border-border last:border-0">
                              <td className="px-3 py-2 text-muted-foreground">{delivery.message_sequence}</td>
                              <td className="px-3 py-2 text-muted-foreground">{delivery.ordering_key || "-"}</td>
                              <td className="px-3 py-2">
                                <Badge variant={statusBadgeVariant(delivery.status)}>{delivery.status}</Badge>
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
                                    Retry
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
                  <p className="text-sm font-medium text-foreground mb-2">Recent Messages</p>
                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Sequence</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Ordering Key</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Published</th>
                        </tr>
                      </thead>
                      <tbody>
                        {messages.length === 0 ? (
                          <tr>
                            <td colSpan={3} className="px-3 py-4 text-center text-muted-foreground">No messages.</td>
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
                  <p className="text-sm font-medium text-foreground mb-2">Outbox Jobs</p>
                  <div className="rounded-md border border-border overflow-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">ID</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Attempt</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Message</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Next Attempt</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Error</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {outboxJobs.length === 0 ? (
                          <tr>
                            <td colSpan={7} className="px-3 py-4 text-center text-muted-foreground">No outbox jobs.</td>
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
                                  {job.status}
                                </Badge>
                              </td>
                              <td className="px-3 py-2 text-muted-foreground">{job.attempt}/{job.max_attempts}</td>
                              <td className="px-3 py-2 text-muted-foreground">{job.message_id || "-"}</td>
                              <td className="px-3 py-2 text-muted-foreground">{formatDate(job.next_attempt_at)}</td>
                              <td className="px-3 py-2 text-muted-foreground max-w-[280px] truncate">{job.last_error || "-"}</td>
                              <td className="px-3 py-2 text-right">
                                {job.status === "failed" ? (
                                  <Button variant="outline" size="sm" onClick={() => handleRetryOutbox(job.id)} disabled={busy}>
                                    Retry
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

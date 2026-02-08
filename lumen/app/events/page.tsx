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
  const [selectedSubscriptionID, setSelectedSubscriptionID] = useState("")
  const [deliveries, setDeliveries] = useState<EventDelivery[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createTopicName, setCreateTopicName] = useState("")
  const [createTopicDesc, setCreateTopicDesc] = useState("")
  const [createTopicRetentionHours, setCreateTopicRetentionHours] = useState("168")

  const [newSubName, setNewSubName] = useState("")
  const [newSubGroup, setNewSubGroup] = useState("")
  const [newSubFunction, setNewSubFunction] = useState("")
  const [newSubMaxAttempts, setNewSubMaxAttempts] = useState("3")
  const [newSubBackoffBase, setNewSubBackoffBase] = useState("1000")
  const [newSubBackoffMax, setNewSubBackoffMax] = useState("60000")

  const [publishPayload, setPublishPayload] = useState("{}")
  const [publishHeaders, setPublishHeaders] = useState("{}")
  const [publishOrderingKey, setPublishOrderingKey] = useState("")

  const [replayFromSequence, setReplayFromSequence] = useState("1")
  const [replayLimit, setReplayLimit] = useState("100")

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
      setSelectedSubscriptionID("")
      setDeliveries([])
      return
    }

    try {
      const [subData, messageData] = await Promise.all([
        eventsApi.listSubscriptions(topicName),
        eventsApi.listMessages(topicName, 100),
      ])
      setSubscriptions(subData)
      setMessages(messageData)

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
    if (!newSubFunction) {
      alert("Select a function")
      return
    }

    try {
      setBusy(true)
      await eventsApi.createSubscription(selectedTopicName, {
        name: newSubName.trim(),
        consumer_group: newSubGroup.trim() || undefined,
        function_name: newSubFunction,
        max_attempts: Math.max(1, Number(newSubMaxAttempts) || 3),
        backoff_base_ms: Math.max(1, Number(newSubBackoffBase) || 1000),
        backoff_max_ms: Math.max(1, Number(newSubBackoffMax) || 60000),
      })
      setNewSubName("")
      setNewSubGroup("")
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
        Math.max(1, Number(replayLimit) || 100)
      )
      await fetchDeliveries(selectedSubscriptionID)
      alert(`Replay queued ${response.queued} deliveries`)
    } catch (err) {
      console.error("Failed to replay:", err)
      alert(err instanceof Error ? err.message : "Failed to replay")
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
            <Input
              placeholder="topic name (orders.created)"
              value={createTopicName}
              onChange={(e) => setCreateTopicName(e.target.value)}
            />
            <Input
              placeholder="description"
              value={createTopicDesc}
              onChange={(e) => setCreateTopicDesc(e.target.value)}
            />
            <Input
              type="number"
              min={1}
              value={createTopicRetentionHours}
              onChange={(e) => setCreateTopicRetentionHours(e.target.value)}
              placeholder="retention hours"
            />
            <Button onClick={handleCreateTopic} disabled={busy || !createTopicName.trim()}>
              <Plus className="mr-2 h-4 w-4" />
              Create Topic
            </Button>
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
                  </div>
                  <Button onClick={handlePublish} disabled={busy}>
                    <Send className="mr-2 h-4 w-4" />
                    Publish Event
                  </Button>
                </div>

                <div className="rounded-lg border border-border bg-card p-4 space-y-4">
                  <p className="text-sm font-medium text-foreground">Subscriptions</p>

                  <div className="grid gap-3 md:grid-cols-3">
                    <Input
                      placeholder="subscription name"
                      value={newSubName}
                      onChange={(e) => setNewSubName(e.target.value)}
                    />
                    <Input
                      placeholder="consumer group (optional)"
                      value={newSubGroup}
                      onChange={(e) => setNewSubGroup(e.target.value)}
                    />
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
                    <Input
                      type="number"
                      min={1}
                      placeholder="max attempts"
                      value={newSubMaxAttempts}
                      onChange={(e) => setNewSubMaxAttempts(e.target.value)}
                    />
                    <Input
                      type="number"
                      min={1}
                      placeholder="backoff base ms"
                      value={newSubBackoffBase}
                      onChange={(e) => setNewSubBackoffBase(e.target.value)}
                    />
                    <Input
                      type="number"
                      min={1}
                      placeholder="backoff max ms"
                      value={newSubBackoffMax}
                      onChange={(e) => setNewSubBackoffMax(e.target.value)}
                    />
                  </div>

                  <Button onClick={handleCreateSubscription} disabled={busy || !newSubName.trim() || !newSubFunction}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add Subscription
                  </Button>

                  <div className="rounded-md border border-border">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Name</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Consumer Group</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Function</th>
                          <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
                          <th className="px-3 py-2 text-right font-medium text-muted-foreground">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {subscriptions.length === 0 ? (
                          <tr>
                            <td colSpan={5} className="px-3 py-4 text-center text-muted-foreground">No subscriptions.</td>
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
                                <td className="px-3 py-2 text-muted-foreground">{sub.consumer_group}</td>
                                <td className="px-3 py-2 text-muted-foreground">{sub.function_name}</td>
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
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <p className="text-sm font-medium text-foreground">Deliveries</p>
                      <p className="text-xs text-muted-foreground">
                        {selectedSubscription ? `Subscription: ${selectedSubscription.name}` : "Select a subscription"}
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <Input
                        className="w-28"
                        type="number"
                        min={1}
                        value={replayFromSequence}
                        onChange={(e) => setReplayFromSequence(e.target.value)}
                        placeholder="from seq"
                      />
                      <Input
                        className="w-24"
                        type="number"
                        min={1}
                        value={replayLimit}
                        onChange={(e) => setReplayLimit(e.target.value)}
                        placeholder="limit"
                      />
                      <Button variant="outline" onClick={handleReplay} disabled={busy || !selectedSubscriptionID}>
                        <RotateCcw className="mr-2 h-4 w-4" />
                        Replay
                      </Button>
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
              </>
            )}
          </div>
        </div>
      </div>
    </DashboardLayout>
  )
}

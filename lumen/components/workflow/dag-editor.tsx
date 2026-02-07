"use client"

import { useState, useCallback } from "react"
import {
  ReactFlow,
  Controls,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  addEdge,
  type Node,
  type Edge,
  type Connection,
  type NodeMouseHandler,
  type OnNodesDelete,
  type OnEdgesDelete,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { WorkflowNode, type WorkflowNodeData } from "./workflow-node"
import { WorkflowEdge } from "./workflow-edge"
import { NodeConfigPanel, type NodeConfig } from "./node-config-panel"
import { computeLayout, validateDAG, NODE_WIDTH, NODE_HEIGHT, type LayoutMap } from "./dag-layout"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { PublishVersionRequest, NodeDefinition, EdgeDefinition } from "@/lib/api"
import { Plus, LayoutGrid, Code2, Eye } from "lucide-react"

const nodeTypes = { workflow: WorkflowNode }
const edgeTypes = { workflow: WorkflowEdge }

interface DagEditorProps {
  functions: string[]
  title?: string
  initialDefinition?: PublishVersionRequest & { layout?: LayoutMap }
  onSave: (def: PublishVersionRequest & { layout?: LayoutMap }) => void
  onCancel: () => void
  onFunctionClick?: (functionName: string) => void
  saving?: boolean
}

interface EditorNodeData {
  nodeKey: string
  functionName: string
  timeoutS: number
  retryMax: number
  retryBaseMs: number
  retryMaxBackoffMs: number
}

let nodeCounter = 0

function makeNodeId() {
  return `node-${Date.now()}-${++nodeCounter}`
}

function editorToFlowNodes(editorNodes: EditorNodeData[], layout: LayoutMap, selectedId?: string, onFunctionClick?: (fn: string) => void): Node<WorkflowNodeData>[] {
  return editorNodes.map((n) => ({
    id: n.nodeKey,
    type: "workflow",
    position: layout[n.nodeKey] || { x: 0, y: 0 },
    data: {
      nodeKey: n.nodeKey,
      functionName: n.functionName,
      timeoutS: n.timeoutS,
      retryMax: n.retryMax > 1 ? n.retryMax : undefined,
      mode: "editor" as const,
      selected: n.nodeKey === selectedId,
      onFunctionClick,
    },
  }))
}

export function DagEditor({ functions, title, initialDefinition, onSave, onCancel, onFunctionClick, saving }: DagEditorProps) {
  const [jsonMode, setJsonMode] = useState(false)
  const [jsonText, setJsonText] = useState("")
  const [jsonError, setJsonError] = useState<string | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [selectedNodeKey, setSelectedNodeKey] = useState<string | null>(null)

  // Internal node data store (source of truth for config)
  const [editorNodes, setEditorNodes] = useState<EditorNodeData[]>(() => {
    if (!initialDefinition?.nodes?.length) return []
    return initialDefinition.nodes.map((n) => ({
      nodeKey: n.node_key,
      functionName: n.function_name,
      timeoutS: n.timeout_s || 30,
      retryMax: n.retry_policy?.max_attempts || 1,
      retryBaseMs: n.retry_policy?.base_ms || 100,
      retryMaxBackoffMs: n.retry_policy?.max_backoff_ms || 5000,
    }))
  })

  // Compute initial layout
  const [initialLayout] = useState<LayoutMap>(() => {
    const layoutNodes = editorNodes.map((n) => ({ key: n.nodeKey, width: NODE_WIDTH, height: NODE_HEIGHT }))
    const edgeList = initialDefinition?.edges?.map((e) => ({ from: e.from, to: e.to })) || []
    return computeLayout(layoutNodes, edgeList, initialDefinition?.layout)
  })

  const [nodes, setNodes, onNodesChange] = useNodesState(
    editorToFlowNodes(editorNodes, initialLayout, undefined, onFunctionClick)
  )

  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(
    (initialDefinition?.edges || []).map((e, i) => ({
      id: `e-${i}`,
      source: e.from,
      target: e.to,
      type: "workflow",
    }))
  )

  const onConnect = useCallback((connection: Connection) => {
    if (!connection.source || !connection.target) return
    if (connection.source === connection.target) return

    // Check for duplicate
    const exists = edges.some(
      (e) => e.source === connection.source && e.target === connection.target
    )
    if (exists) return

    // Check for cycles
    const allNodeKeys = editorNodes.map((n) => n.nodeKey)
    const newEdge = { from: connection.source, to: connection.target }
    const allEdges = [
      ...edges.map((e) => ({ from: e.source, to: e.target })),
      newEdge,
    ]
    const result = validateDAG(allNodeKeys, allEdges)
    if (!result.valid) {
      setValidationError(result.message)
      setTimeout(() => setValidationError(null), 3000)
      return
    }

    setEdges((eds) => addEdge({ ...connection, type: "workflow" }, eds))
    setValidationError(null)
  }, [edges, editorNodes, setEdges])

  const onNodeClick: NodeMouseHandler = useCallback((_event, node) => {
    setSelectedNodeKey(node.id)
    setNodes((nds) =>
      nds.map((n) => ({
        ...n,
        data: { ...n.data, selected: n.id === node.id },
      }))
    )
  }, [setNodes])

  const onPaneClick = useCallback(() => {
    setSelectedNodeKey(null)
    setNodes((nds) =>
      nds.map((n) => ({ ...n, data: { ...n.data, selected: false } }))
    )
  }, [setNodes])

  const onNodesDelete: OnNodesDelete = useCallback((deletedNodes) => {
    const deletedIds = new Set(deletedNodes.map((n) => n.id))
    setEditorNodes((prev) => prev.filter((n) => !deletedIds.has(n.nodeKey)))
    if (selectedNodeKey && deletedIds.has(selectedNodeKey)) {
      setSelectedNodeKey(null)
    }
  }, [selectedNodeKey])

  const onEdgesDelete: OnEdgesDelete = useCallback(() => {
    // edges are removed by xyflow automatically
  }, [])

  const handleAddNode = useCallback(() => {
    const id = makeNodeId()
    const key = `step${editorNodes.length + 1}`
    const newNode: EditorNodeData = {
      nodeKey: key,
      functionName: "",
      timeoutS: 30,
      retryMax: 1,
      retryBaseMs: 100,
      retryMaxBackoffMs: 5000,
    }

    // Place new node below existing ones
    let maxY = 0
    for (const n of nodes) {
      if (n.position.y + NODE_HEIGHT > maxY) maxY = n.position.y + NODE_HEIGHT
    }

    setEditorNodes((prev) => [...prev, newNode])
    setNodes((nds) => [
      ...nds.map((n) => ({ ...n, data: { ...n.data, selected: false } })),
      {
        id: key,
        type: "workflow",
        position: { x: 200, y: maxY + 40 },
        data: {
          nodeKey: key,
          functionName: "",
          timeoutS: 30,
          mode: "editor" as const,
          selected: true,
          onFunctionClick,
        },
      },
    ])
    setSelectedNodeKey(key)
  }, [editorNodes, nodes, setNodes, onFunctionClick])

  const handleAutoLayout = useCallback(() => {
    const layoutNodes = editorNodes.map((n) => ({ key: n.nodeKey, width: NODE_WIDTH, height: NODE_HEIGHT }))
    const layoutEdges = edges.map((e) => ({ from: e.source, to: e.target }))
    const layout = computeLayout(layoutNodes, layoutEdges)

    setNodes((nds) =>
      nds.map((n) => ({
        ...n,
        position: layout[n.id] || n.position,
      }))
    )
  }, [editorNodes, edges, setNodes])

  const handleNodeConfigChange = useCallback((updated: NodeConfig) => {
    const oldKey = selectedNodeKey
    if (!oldKey) return

    const keyChanged = updated.nodeKey !== oldKey

    setEditorNodes((prev) =>
      prev.map((n) => (n.nodeKey === oldKey ? {
        nodeKey: updated.nodeKey,
        functionName: updated.functionName,
        timeoutS: updated.timeoutS,
        retryMax: updated.retryMax,
        retryBaseMs: updated.retryBaseMs,
        retryMaxBackoffMs: updated.retryMaxBackoffMs,
      } : n))
    )

    setNodes((nds) =>
      nds.map((n) => {
        if (n.id !== oldKey) return n
        return {
          ...n,
          id: updated.nodeKey,
          data: {
            ...n.data,
            nodeKey: updated.nodeKey,
            functionName: updated.functionName,
            timeoutS: updated.timeoutS,
            retryMax: updated.retryMax > 1 ? updated.retryMax : undefined,
          },
        }
      })
    )

    if (keyChanged) {
      setEdges((eds) =>
        eds.map((e) => ({
          ...e,
          source: e.source === oldKey ? updated.nodeKey : e.source,
          target: e.target === oldKey ? updated.nodeKey : e.target,
        }))
      )
      setSelectedNodeKey(updated.nodeKey)
    }
  }, [selectedNodeKey, setNodes, setEdges])

  const handleNodeDelete = useCallback(() => {
    if (!selectedNodeKey) return
    setEditorNodes((prev) => prev.filter((n) => n.nodeKey !== selectedNodeKey))
    setNodes((nds) => nds.filter((n) => n.id !== selectedNodeKey))
    setEdges((eds) => eds.filter((e) => e.source !== selectedNodeKey && e.target !== selectedNodeKey))
    setSelectedNodeKey(null)
  }, [selectedNodeKey, setNodes, setEdges])

  const buildDefinition = useCallback((): (PublishVersionRequest & { layout?: LayoutMap }) | null => {
    // Validate all nodes have keys and function names
    for (const n of editorNodes) {
      if (!n.nodeKey.trim()) {
        setValidationError("All nodes must have a node key")
        return null
      }
      if (!n.functionName.trim()) {
        setValidationError(`Node "${n.nodeKey}" must have a function name`)
        return null
      }
    }

    // Check for duplicate keys
    const keys = editorNodes.map((n) => n.nodeKey)
    const uniqueKeys = new Set(keys)
    if (uniqueKeys.size !== keys.length) {
      setValidationError("Duplicate node keys found")
      return null
    }

    const edgeList = edges.map((e) => ({ from: e.source, to: e.target }))
    const dagResult = validateDAG(keys, edgeList)
    if (!dagResult.valid) {
      setValidationError(dagResult.message)
      return null
    }

    const layout: LayoutMap = {}
    for (const n of nodes) {
      layout[n.id] = { x: n.position.x, y: n.position.y }
    }

    const nodeDefs: NodeDefinition[] = editorNodes.map((n) => ({
      node_key: n.nodeKey,
      function_name: n.functionName,
      timeout_s: n.timeoutS,
      ...(n.retryMax > 1 ? {
        retry_policy: {
          max_attempts: n.retryMax,
          base_ms: n.retryBaseMs,
          max_backoff_ms: n.retryMaxBackoffMs,
        },
      } : {}),
    }))

    const edgeDefs: EdgeDefinition[] = edgeList

    return { nodes: nodeDefs, edges: edgeDefs, layout }
  }, [editorNodes, edges, nodes])

  const handleSave = useCallback(() => {
    setValidationError(null)
    if (jsonMode) {
      try {
        const parsed = JSON.parse(jsonText)
        onSave(parsed)
      } catch {
        setJsonError("Invalid JSON")
      }
      return
    }

    const def = buildDefinition()
    if (def) {
      onSave(def)
    }
  }, [jsonMode, jsonText, buildDefinition, onSave])

  const handleSwitchToJson = useCallback(() => {
    const def = buildDefinition()
    if (def) {
      setJsonText(JSON.stringify(def, null, 2))
    } else {
      // Build best-effort JSON even if validation fails
      const layout: LayoutMap = {}
      for (const n of nodes) layout[n.id] = { x: n.position.x, y: n.position.y }
      setJsonText(JSON.stringify({
        nodes: editorNodes.map((n) => ({
          node_key: n.nodeKey,
          function_name: n.functionName,
          timeout_s: n.timeoutS,
        })),
        edges: edges.map((e) => ({ from: e.source, to: e.target })),
        layout,
      }, null, 2))
    }
    setValidationError(null)
    setJsonMode(true)
  }, [buildDefinition, editorNodes, edges, nodes])

  const handleSwitchToVisual = useCallback(() => {
    setJsonMode(false)
    setJsonError(null)
  }, [])

  const selectedEditorNode = selectedNodeKey
    ? editorNodes.find((n) => n.nodeKey === selectedNodeKey)
    : null

  const selectedConfig: NodeConfig | null = selectedEditorNode
    ? {
        nodeKey: selectedEditorNode.nodeKey,
        functionName: selectedEditorNode.functionName,
        timeoutS: selectedEditorNode.timeoutS,
        retryMax: selectedEditorNode.retryMax,
        retryBaseMs: selectedEditorNode.retryBaseMs,
        retryMaxBackoffMs: selectedEditorNode.retryMaxBackoffMs,
      }
    : null

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2 border-b border-border bg-card shrink-0">
        <div className="flex items-center gap-3">
          {title && (
            <h2 className="text-sm font-semibold text-foreground whitespace-nowrap">{title}</h2>
          )}
          <div className="flex items-center gap-2">
            {!jsonMode && (
              <>
                <Button variant="outline" size="sm" onClick={handleAddNode}>
                  <Plus className="mr-1.5 h-3.5 w-3.5" />
                  Add Node
                </Button>
                <Button variant="outline" size="sm" onClick={handleAutoLayout}>
                  <LayoutGrid className="mr-1.5 h-3.5 w-3.5" />
                  Auto Layout
                </Button>
              </>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={jsonMode ? handleSwitchToVisual : handleSwitchToJson}
            >
              {jsonMode ? <Eye className="mr-1.5 h-3.5 w-3.5" /> : <Code2 className="mr-1.5 h-3.5 w-3.5" />}
              {jsonMode ? "Visual" : "JSON"}
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel}>Cancel</Button>
          <Button size="sm" onClick={handleSave} disabled={saving}>
            {saving ? "Publishing..." : "Publish"}
          </Button>
        </div>
      </div>

      {/* Validation error */}
      {(validationError || jsonError) && (
        <div className="px-4 py-2 bg-destructive/10 text-destructive text-sm border-b border-destructive/20">
          {validationError || jsonError}
        </div>
      )}

      {/* Editor area */}
      <div className="flex-1 flex min-h-0">
        {jsonMode ? (
          <div className="flex-1 p-4">
            <Textarea
              className="font-mono text-sm h-full min-h-[400px] resize-none"
              value={jsonText}
              onChange={(e) => {
                setJsonText(e.target.value)
                setJsonError(null)
              }}
            />
          </div>
        ) : (
          <>
            <div className="flex-1 relative">
              <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={nodeTypes}
                edgeTypes={edgeTypes}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onConnect={onConnect}
                onNodeClick={onNodeClick}
                onPaneClick={onPaneClick}
                onNodesDelete={onNodesDelete}
                onEdgesDelete={onEdgesDelete}
                fitView
                snapToGrid
                snapGrid={[10, 10]}
                deleteKeyCode="Delete"
                minZoom={0.3}
                maxZoom={2}
                proOptions={{ hideAttribution: true }}
              >
                <Background variant={BackgroundVariant.Dots} gap={16} size={1} className="!bg-background" />
                <Controls showInteractive={false} />
              </ReactFlow>
            </div>

            {selectedConfig && (
              <NodeConfigPanel
                node={selectedConfig}
                functions={functions}
                onChange={handleNodeConfigChange}
                onDelete={handleNodeDelete}
                onClose={() => {
                  setSelectedNodeKey(null)
                  setNodes((nds) =>
                    nds.map((n) => ({ ...n, data: { ...n.data, selected: false } }))
                  )
                }}
              />
            )}
          </>
        )}
      </div>
    </div>
  )
}

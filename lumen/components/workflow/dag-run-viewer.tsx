"use client"

import { useMemo, useState, useCallback } from "react"
import {
  ReactFlow,
  Controls,
  MiniMap,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeMouseHandler,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { WorkflowNode, type WorkflowNodeData } from "./workflow-node"
import { WorkflowEdge, type WorkflowEdgeData } from "./workflow-edge"
import { NodeDetailPanel } from "./node-detail-panel"
import { computeLayout, NODE_WIDTH, NODE_HEIGHT, type LayoutMap } from "./dag-layout"
import type { WorkflowVersion, WorkflowRun, RunNode, WorkflowNode as WFNode, WorkflowEdge as WFEdge } from "@/lib/api"

const nodeTypes = { workflow: WorkflowNode }
const edgeTypes = { workflow: WorkflowEdge }

interface DagRunViewerProps {
  version: WorkflowVersion
  run: WorkflowRun
  className?: string
  onFunctionClick?: (functionName: string) => void
}

const activeStatuses = new Set(["running", "ready"])

export function DagRunViewer({ version, run, className, onFunctionClick }: DagRunViewerProps) {
  const [selectedNodeKey, setSelectedNodeKey] = useState<string | null>(null)

  const vNodes = version.nodes || []
  const vEdges = version.edges || []
  const runNodes = run.nodes || []

  // Build status map from run nodes
  const statusMap = useMemo(() => {
    const map: Record<string, RunNode> = {}
    for (const rn of runNodes) {
      map[rn.node_key] = rn
    }
    return map
  }, [runNodes])

  const { flowNodes, flowEdges } = useMemo(() => {
    const nodeIdMap: Record<string, string> = {}
    for (const n of vNodes) {
      nodeIdMap[n.id] = n.node_key
    }

    const layoutEdges = vEdges.map((e: WFEdge) => ({
      from: nodeIdMap[e.from_node_id] || e.from_node_id,
      to: nodeIdMap[e.to_node_id] || e.to_node_id,
    }))

    const layoutNodes = vNodes.map((n: WFNode) => ({
      key: n.node_key,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    }))

    const storedLayout = (version.definition as { layout?: LayoutMap })?.layout
    const layout = computeLayout(layoutNodes, layoutEdges, storedLayout)

    const flowNodes: Node<WorkflowNodeData>[] = vNodes.map((n: WFNode) => {
      const runNode = statusMap[n.node_key]
      return {
        id: n.node_key,
        type: "workflow",
        position: layout[n.node_key] || { x: 0, y: 0 },
        data: {
          nodeKey: n.node_key,
          functionName: n.function_name,
          timeoutS: n.timeout_s,
          retryMax: n.retry_policy?.max_attempts,
          status: runNode?.status || "pending",
          mode: "run" as const,
          onFunctionClick,
        },
        draggable: false,
      }
    })

    const flowEdges: Edge<WorkflowEdgeData>[] = vEdges.map((e: WFEdge, i: number) => {
      const sourceKey = nodeIdMap[e.from_node_id] || e.from_node_id
      const targetKey = nodeIdMap[e.to_node_id] || e.to_node_id
      const sourceStatus = statusMap[sourceKey]?.status
      const targetStatus = statusMap[targetKey]?.status
      const isActive = (sourceStatus && activeStatuses.has(sourceStatus)) ||
                       (targetStatus && activeStatuses.has(targetStatus))

      return {
        id: `e-${i}`,
        source: sourceKey,
        target: targetKey,
        type: "workflow",
        data: { animated: !!isActive },
        style: sourceStatus === "succeeded" && targetStatus
          ? { stroke: "var(--color-success)" }
          : undefined,
      }
    })

    return { flowNodes, flowEdges }
  }, [vNodes, vEdges, version.definition, statusMap, onFunctionClick])

  const [nodes] = useNodesState(flowNodes)
  const [edges] = useEdgesState(flowEdges)

  const selectedRunNode = selectedNodeKey ? statusMap[selectedNodeKey] : null

  const onNodeClick: NodeMouseHandler = useCallback((_event, node) => {
    setSelectedNodeKey(node.id)
  }, [])

  const onInit = useCallback((instance: { fitView: () => void }) => {
    setTimeout(() => instance.fitView(), 50)
  }, [])

  if (vNodes.length === 0) {
    return null
  }

  return (
    <div className={`rounded-lg border border-border bg-card overflow-hidden flex ${className || ""}`} style={{ height: 420 }}>
      <div className="flex-1 relative">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onInit={onInit}
          onNodeClick={onNodeClick}
          fitView
          nodesDraggable={false}
          nodesConnectable={false}
          panOnDrag
          zoomOnScroll
          minZoom={0.3}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
        >
          <Background variant={BackgroundVariant.Dots} gap={16} size={1} className="!bg-background" />
          <Controls showInteractive={false} />
          <MiniMap pannable zoomable />
        </ReactFlow>
      </div>

      {selectedRunNode && (
        <NodeDetailPanel node={selectedRunNode} onClose={() => setSelectedNodeKey(null)} />
      )}
    </div>
  )
}

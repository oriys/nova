"use client"

import { useMemo, useCallback } from "react"
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
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { WorkflowNode, type WorkflowNodeData } from "./workflow-node"
import { WorkflowEdge } from "./workflow-edge"
import { computeLayout, NODE_WIDTH, NODE_HEIGHT, type LayoutMap } from "./dag-layout"
import type { WorkflowVersion, WorkflowNode as WFNode, WorkflowEdge as WFEdge } from "@/lib/api"

const nodeTypes = { workflow: WorkflowNode }
const edgeTypes = { workflow: WorkflowEdge }

interface DagViewerProps {
  version: WorkflowVersion
  className?: string
  onFunctionClick?: (functionName: string) => void
}

function buildNodeIdMap(versionNodes: WFNode[]): Record<string, string> {
  const map: Record<string, string> = {}
  for (const n of versionNodes) {
    map[n.id] = n.node_key
  }
  return map
}

export function DagViewer({ version, className, onFunctionClick }: DagViewerProps) {
  const vNodes = version.nodes || []
  const vEdges = version.edges || []

  const { nodes: flowNodes, edges: flowEdges } = useMemo(() => {
    const nodeIdMap = buildNodeIdMap(vNodes)

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

    const nodes: Node<WorkflowNodeData>[] = vNodes.map((n: WFNode) => ({
      id: n.node_key,
      type: "workflow",
      position: layout[n.node_key] || { x: 0, y: 0 },
      data: {
        nodeKey: n.node_key,
        nodeType: n.node_type || "function",
        functionName: n.function_name,
        workflowName: n.workflow_name,
        timeoutS: n.timeout_s,
        retryMax: n.retry_policy?.max_attempts,
        mode: "viewer" as const,
        onFunctionClick,
      },
      draggable: false,
    }))

    const edges: Edge[] = vEdges.map((e: WFEdge, i: number) => ({
      id: `e-${i}`,
      source: nodeIdMap[e.from_node_id] || e.from_node_id,
      target: nodeIdMap[e.to_node_id] || e.to_node_id,
      type: "workflow",
    }))

    return { nodes, edges }
  }, [vNodes, vEdges, version.definition, onFunctionClick])

  const [nodes] = useNodesState(flowNodes)
  const [edges] = useEdgesState(flowEdges)

  const onInit = useCallback((instance: { fitView: () => void }) => {
    setTimeout(() => instance.fitView(), 50)
  }, [])

  if (vNodes.length === 0) {
    return (
      <div className={`flex items-center justify-center rounded-lg border border-border bg-card text-muted-foreground ${className || ""}`} style={{ height: 400 }}>
        No nodes in this version.
      </div>
    )
  }

  return (
    <div className={`rounded-lg border border-border bg-card overflow-hidden ${className || ""}`} style={{ height: 500 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onInit={onInit}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
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
  )
}

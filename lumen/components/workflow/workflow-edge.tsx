"use client"

import { memo } from "react"
import { BaseEdge, getSmoothStepPath, type EdgeProps } from "@xyflow/react"

export interface WorkflowEdgeData {
  [key: string]: unknown
  animated?: boolean
}

function WorkflowEdgeComponent({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  style,
}: EdgeProps & { data?: WorkflowEdgeData }) {
  const [edgePath] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 8,
  })

  return (
    <BaseEdge
      id={id}
      path={edgePath}
      style={style}
      className={data?.animated ? "wf-edge-animated" : ""}
    />
  )
}

export const WorkflowEdge = memo(WorkflowEdgeComponent)

"use client"

import { memo, useCallback } from "react"
import { Handle, Position, type NodeProps } from "@xyflow/react"
import type { NodeStatus } from "@/lib/api"

export interface WorkflowNodeData {
  [key: string]: unknown
  nodeKey: string
  functionName: string
  timeoutS?: number
  retryMax?: number
  status?: NodeStatus
  mode: "viewer" | "editor" | "run"
  selected?: boolean
  onFunctionClick?: (functionName: string) => void
}

type WFNodeProps = NodeProps & { data: WorkflowNodeData }

const statusStyles: Record<string, string> = {
  pending: "border-muted bg-muted/20",
  ready: "border-blue-500 border-dashed bg-blue-50 dark:bg-blue-950/30",
  running: "border-blue-500 bg-blue-50 dark:bg-blue-950/30 wf-node-pulse",
  succeeded: "border-emerald-500 bg-emerald-50 dark:bg-emerald-950/30",
  failed: "border-destructive bg-destructive/10",
  skipped: "border-muted bg-muted/10 wf-node-skipped",
}

const statusDotColors: Record<string, string> = {
  pending: "bg-muted-foreground",
  ready: "bg-blue-500",
  running: "bg-blue-500 animate-pulse",
  succeeded: "bg-emerald-500",
  failed: "bg-destructive",
  skipped: "bg-muted-foreground",
}

function WorkflowNodeComponent({ data }: WFNodeProps) {
  const status = data.status || "pending"
  const borderStyle = data.mode === "run" ? (statusStyles[status] || statusStyles.pending) : "border-border bg-card"
  const isEditor = data.mode === "editor"

  const handleFnClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    if (data.functionName && data.onFunctionClick) {
      data.onFunctionClick(data.functionName)
    }
  }, [data.functionName, data.onFunctionClick])

  return (
    <div
      className={`w-[220px] rounded-lg border-2 px-3 py-2.5 shadow-sm transition-colors ${borderStyle} ${
        data.selected ? "ring-2 ring-ring ring-offset-1 ring-offset-background" : ""
      }`}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!w-3 !h-3 !bg-muted-foreground/60 !border-2 !border-background hover:!bg-primary !-top-1.5"
        isConnectable={isEditor}
      />

      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            {data.mode === "run" && (
              <span className={`inline-block h-2 w-2 rounded-full shrink-0 ${statusDotColors[status] || statusDotColors.pending}`} />
            )}
            <p className="font-mono text-sm font-bold truncate text-foreground">{data.nodeKey}</p>
          </div>
          {data.functionName ? (
            <button
              type="button"
              onClick={handleFnClick}
              className="text-xs text-primary truncate mt-0.5 block max-w-full hover:underline cursor-pointer text-left"
              title={`View source: ${data.functionName}`}
            >
              {data.functionName}
            </button>
          ) : (
            <p className="text-xs text-muted-foreground truncate mt-0.5">No function</p>
          )}
        </div>
      </div>

      {(data.timeoutS || data.retryMax) && (
        <div className="flex items-center gap-1.5 mt-1.5">
          {data.timeoutS && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono">
              {data.timeoutS}s
            </span>
          )}
          {data.retryMax && data.retryMax > 1 && (
            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono">
              {data.retryMax}x
            </span>
          )}
        </div>
      )}

      <Handle
        type="source"
        position={Position.Bottom}
        className="!w-3 !h-3 !bg-muted-foreground/60 !border-2 !border-background hover:!bg-primary !-bottom-1.5"
        isConnectable={isEditor}
      />
    </div>
  )
}

export const WorkflowNode = memo(WorkflowNodeComponent)

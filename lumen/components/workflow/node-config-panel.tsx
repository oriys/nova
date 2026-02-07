"use client"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { FunctionSelector } from "./function-selector"
import { X, Trash2 } from "lucide-react"

export interface NodeConfig {
  nodeKey: string
  functionName: string
  timeoutS: number
  retryMax: number
  retryBaseMs: number
  retryMaxBackoffMs: number
}

interface NodeConfigPanelProps {
  node: NodeConfig
  functions: string[]
  onChange: (updated: NodeConfig) => void
  onDelete: () => void
  onClose: () => void
}

export function NodeConfigPanel({ node, functions, onChange, onDelete, onClose }: NodeConfigPanelProps) {
  const update = (patch: Partial<NodeConfig>) => {
    onChange({ ...node, ...patch })
  }

  return (
    <div className="w-72 border-l border-border bg-card h-full overflow-y-auto shrink-0">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h3 className="font-medium text-sm text-foreground">Node Config</h3>
        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="p-4 space-y-4">
        <div>
          <Label className="text-xs">Node Key</Label>
          <Input
            value={node.nodeKey}
            onChange={(e) => update({ nodeKey: e.target.value })}
            className="font-mono text-sm mt-1"
            placeholder="e.g. step1"
          />
        </div>

        <div>
          <Label className="text-xs">Function Name</Label>
          <div className="mt-1">
            <FunctionSelector
              functions={functions}
              value={node.functionName}
              onChange={(v) => update({ functionName: v })}
            />
          </div>
        </div>

        <div>
          <Label className="text-xs">Timeout (seconds)</Label>
          <Input
            type="number"
            value={node.timeoutS}
            onChange={(e) => update({ timeoutS: parseInt(e.target.value) || 30 })}
            className="mt-1"
            min={1}
          />
        </div>

        <div>
          <Label className="text-xs">Max Retries</Label>
          <Input
            type="number"
            value={node.retryMax}
            onChange={(e) => update({ retryMax: parseInt(e.target.value) || 1 })}
            className="mt-1"
            min={1}
          />
        </div>

        {node.retryMax > 1 && (
          <>
            <div>
              <Label className="text-xs">Retry Base (ms)</Label>
              <Input
                type="number"
                value={node.retryBaseMs}
                onChange={(e) => update({ retryBaseMs: parseInt(e.target.value) || 100 })}
                className="mt-1"
                min={0}
              />
            </div>
            <div>
              <Label className="text-xs">Max Backoff (ms)</Label>
              <Input
                type="number"
                value={node.retryMaxBackoffMs}
                onChange={(e) => update({ retryMaxBackoffMs: parseInt(e.target.value) || 5000 })}
                className="mt-1"
                min={0}
              />
            </div>
          </>
        )}

        <div className="pt-2 border-t border-border">
          <Button variant="destructive" size="sm" className="w-full" onClick={onDelete}>
            <Trash2 className="mr-2 h-3 w-3" />
            Delete Node
          </Button>
        </div>
      </div>
    </div>
  )
}

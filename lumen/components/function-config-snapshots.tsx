"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { SectionHeader } from "@/components/section-header"
import { snapshotsApi } from "@/lib/api"
import { Camera, Loader2 } from "lucide-react"

interface FunctionConfigSnapshotsProps {
  functionName: string
}

export function FunctionConfigSnapshots({ functionName }: FunctionConfigSnapshotsProps) {
  const [creating, setCreating] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null)

  const handleCreate = async () => {
    try {
      setCreating(true)
      setMessage(null)
      await snapshotsApi.create(functionName)
      setMessage({ type: 'success', text: 'Snapshot created successfully' })
    } catch (err) {
      console.error("Failed to create snapshot:", err)
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to create snapshot' })
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <SectionHeader
        className="mb-4"
        title="VM Snapshot"
        description="Create a snapshot for faster cold starts"
        titleClassName="text-lg font-semibold text-card-foreground"
        descriptionClassName="text-sm"
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={handleCreate}
            disabled={creating}
          >
            {creating ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Camera className="mr-2 h-4 w-4" />
            )}
            Create Snapshot
          </Button>
        }
      />
      {message && (
        <div className={`text-sm p-3 rounded-lg ${
          message.type === 'success'
            ? 'bg-success/10 text-success'
            : 'bg-destructive/10 text-destructive'
        }`}>
          {message.text}
        </div>
      )}
    </div>
  )
}

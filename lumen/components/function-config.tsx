"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { FunctionData } from "@/lib/types"
import { functionsApi } from "@/lib/api"
import { Save, Plus, Trash2, Eye, EyeOff, Key, Loader2 } from "lucide-react"

interface FunctionConfigProps {
  func: FunctionData
  onUpdate?: () => void
}

export function FunctionConfig({ func, onUpdate }: FunctionConfigProps) {
  const [memory, setMemory] = useState(func.memory.toString())
  const [timeout, setTimeout] = useState(func.timeout.toString())
  const [handler, setHandler] = useState(func.handler)
  const [saving, setSaving] = useState(false)
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({})

  // Use function's env vars
  const envVars = Object.entries(func.envVars || {}).map(([key, value], idx) => ({
    id: `env-${idx}`,
    key,
    value,
    type: key.toLowerCase().includes("secret") || key.toLowerCase().includes("key") || key.toLowerCase().includes("password")
      ? "secret" as const
      : "string" as const,
  }))

  const toggleSecret = (id: string) => {
    setShowSecrets((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      await functionsApi.update(func.name, {
        handler,
        memory_mb: parseInt(memory),
        timeout_s: parseInt(timeout),
      })
      onUpdate?.()
    } catch (err) {
      console.error("Failed to save configuration:", err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      {/* General Settings */}
      <div className="rounded-xl border border-border bg-card p-6">
        <h3 className="text-lg font-semibold text-card-foreground mb-4">
          General Settings
        </h3>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="handler">Handler</Label>
            <Input
              id="handler"
              value={handler}
              onChange={(e) => setHandler(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              The entry point for your function
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="memory">Memory</Label>
            <Select value={memory} onValueChange={setMemory}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="128">128 MB</SelectItem>
                <SelectItem value="256">256 MB</SelectItem>
                <SelectItem value="512">512 MB</SelectItem>
                <SelectItem value="1024">1024 MB</SelectItem>
                <SelectItem value="2048">2048 MB</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Allocated memory for execution
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="timeout">Timeout (seconds)</Label>
            <Input
              id="timeout"
              type="number"
              min="1"
              max="900"
              value={timeout}
              onChange={(e) => setTimeout(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Maximum execution time
            </p>
          </div>
        </div>

        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>Code Hash</Label>
            <Input value={func.codeHash || ""} readOnly className="bg-muted/50 font-mono text-sm" />
            <p className="text-xs text-muted-foreground">
              SHA256 hash of the function source code
            </p>
          </div>

          <div className="space-y-2">
            <Label>Execution Mode</Label>
            <Input value={func.mode || "process"} readOnly className="bg-muted/50" />
            <p className="text-xs text-muted-foreground">
              Function execution mode
            </p>
          </div>
        </div>

        <div className="mt-6 flex justify-end">
          <Button onClick={handleSave} disabled={saving}>
            {saving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            Save Changes
          </Button>
        </div>
      </div>

      {/* Environment Variables */}
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">
              Environment Variables
            </h3>
            <p className="text-sm text-muted-foreground">
              Manage secrets and configuration values
            </p>
          </div>
          <Button variant="outline" size="sm" disabled>
            <Plus className="mr-2 h-4 w-4" />
            Add Variable
          </Button>
        </div>

        <div className="space-y-3">
          {envVars.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              No environment variables configured
            </p>
          ) : (
            envVars.map((config) => (
              <div
                key={config.id}
                className="flex items-center gap-4 rounded-lg border border-border bg-muted/30 p-4"
              >
                <div className="flex items-center gap-2 shrink-0">
                  <Key className="h-4 w-4 text-muted-foreground" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground font-mono">
                    {config.key}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {config.type === "secret" ? (
                    <>
                      <Input
                        type={showSecrets[config.id] ? "text" : "password"}
                        value={showSecrets[config.id] ? config.value : "••••••••"}
                        readOnly
                        className="w-40 font-mono text-sm"
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => toggleSecret(config.id)}
                      >
                        {showSecrets[config.id] ? (
                          <EyeOff className="h-4 w-4" />
                        ) : (
                          <Eye className="h-4 w-4" />
                        )}
                      </Button>
                    </>
                  ) : (
                    <Input
                      value={config.value}
                      readOnly
                      className="w-40 font-mono text-sm"
                    />
                  )}
                  <Button variant="ghost" size="icon" className="text-destructive hover:text-destructive" disabled>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {/* Danger Zone */}
      <div className="rounded-xl border border-destructive/50 bg-card p-6">
        <h3 className="text-lg font-semibold text-destructive mb-2">
          Danger Zone
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          These actions are irreversible. Please proceed with caution.
        </p>
        <div className="flex gap-3">
          <Button variant="outline" className="border-destructive text-destructive hover:bg-destructive hover:text-destructive-foreground bg-transparent" disabled>
            Delete Function
          </Button>
        </div>
      </div>
    </div>
  )
}

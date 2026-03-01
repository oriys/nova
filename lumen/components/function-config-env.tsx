"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { SectionHeader } from "@/components/section-header"
import { FunctionData } from "@/lib/types"
import { functionsApi } from "@/lib/api"
import { Plus, Trash2, Eye, EyeOff, Key, Loader2 } from "lucide-react"

interface FunctionConfigEnvProps {
  func: FunctionData
  onUpdate?: () => void
}

export function FunctionConfigEnv({ func, onUpdate }: FunctionConfigEnvProps) {
  const [envVarsState, setEnvVarsState] = useState<Record<string, string>>(func.envVars || {})
  const [newEnvKey, setNewEnvKey] = useState("")
  const [newEnvValue, setNewEnvValue] = useState("")
  const [showAddEnvDialog, setShowAddEnvDialog] = useState(false)
  const [savingEnv, setSavingEnv] = useState(false)
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({})

  const envVars = Object.entries(envVarsState).map(([key, value], idx) => ({
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

  const handleAddEnvVar = async () => {
    if (!newEnvKey.trim()) return
    try {
      setSavingEnv(true)
      const updatedEnvVars = { ...envVarsState, [newEnvKey]: newEnvValue }
      await functionsApi.update(func.name, { env_vars: updatedEnvVars })
      setEnvVarsState(updatedEnvVars)
      setNewEnvKey("")
      setNewEnvValue("")
      setShowAddEnvDialog(false)
      onUpdate?.()
    } catch (err) {
      console.error("Failed to add environment variable:", err)
    } finally {
      setSavingEnv(false)
    }
  }

  const handleDeleteEnvVar = async (key: string) => {
    try {
      const updatedEnvVars = { ...envVarsState }
      delete updatedEnvVars[key]
      await functionsApi.update(func.name, { env_vars: updatedEnvVars })
      setEnvVarsState(updatedEnvVars)
      onUpdate?.()
    } catch (err) {
      console.error("Failed to delete environment variable:", err)
    }
  }

  return (
    <>
      <div className="rounded-xl border border-border bg-card p-6">
        <SectionHeader
          className="mb-4"
          title="Environment Variables"
          description="Manage secrets and configuration values"
          titleClassName="text-lg font-semibold text-card-foreground"
          descriptionClassName="text-sm"
          action={
            <Button variant="outline" size="sm" onClick={() => setShowAddEnvDialog(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Add Variable
            </Button>
          }
        />

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
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-destructive hover:text-destructive"
                    onClick={() => handleDeleteEnvVar(config.key)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      <Dialog open={showAddEnvDialog} onOpenChange={setShowAddEnvDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Environment Variable</DialogTitle>
            <DialogDescription>
              Add a new environment variable to this function.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="envKey">Key</Label>
              <Input
                id="envKey"
                placeholder="MY_VARIABLE"
                value={newEnvKey}
                onChange={(e) => setNewEnvKey(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, '_'))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="envValue">Value</Label>
              <Input
                id="envValue"
                placeholder="value"
                value={newEnvValue}
                onChange={(e) => setNewEnvValue(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowAddEnvDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleAddEnvVar} disabled={!newEnvKey.trim() || savingEnv}>
              {savingEnv ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Plus className="mr-2 h-4 w-4" />
              )}
              Add Variable
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

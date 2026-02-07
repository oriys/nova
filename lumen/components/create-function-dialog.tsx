"use client"

import React from "react"
import { useState, useEffect } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { CodeEditor } from "@/components/code-editor"
import { RuntimeInfo } from "@/lib/types"
import { functionsApi, CompileStatus, type ResourceLimits } from "@/lib/api"
import { Loader2, Check, AlertCircle } from "lucide-react"

// Code templates for each runtime (handler-only style)
const CODE_TEMPLATES: Record<string, string> = {
  python: `def handler(event, context):
    name = event.get("name", "World")
    return {"message": f"Hello, {name}!"}
`,
  node: `function handler(event, context) {
  const name = event.name || 'World';
  return { message: \`Hello, \${name}!\` };
}

module.exports = { handler };
`,
  go: `package main

import (
	"encoding/json"
	"fmt"
)

func Handler(event json.RawMessage, ctx Context) (interface{}, error) {
	var input struct {
		Name string \`json:"name"\`
	}
	json.Unmarshal(event, &input)
	name := input.Name
	if name == "" {
		name = "World"
	}
	return map[string]string{"message": fmt.Sprintf("Hello, %s!", name)}, nil
}
`,
  rust: `use serde_json::Value;

pub fn handler(event: Value, ctx: crate::context::Context) -> Result<Value, String> {
    let name = event.get("name")
        .and_then(|v| v.as_str())
        .unwrap_or("World");
    Ok(serde_json::json!({ "message": format!("Hello, {}!", name) }))
}
`,
  java: `import java.util.*;

public class Handler {
    public static Object handler(String event, Map<String, Object> context) {
        // Parse event JSON manually or use a JSON library
        String name = event.contains("name") ? "User" : "World";
        return "{\\"message\\": \\"Hello, " + name + "!\\"}";
    }
}
`,
  ruby: `def handler(event, context)
  name = event['name'] || 'World'
  { message: "Hello, #{name}!" }
end
`,
  php: `<?php
function handler($event, $context) {
    $name = $event['name'] ?? 'World';
    return ['message' => "Hello, $name!"];
}
`,
  dotnet: `using System.Text.Json;
using System.Collections.Generic;

public static class Handler
{
    public static object Handle(string eventJson, Dictionary<string, object> context)
    {
        var evt = JsonSerializer.Deserialize<Dictionary<string, string>>(eventJson);
        var name = evt?.GetValueOrDefault("name", "World") ?? "World";
        return new { message = $"Hello, {name}!" };
    }
}
`,
  deno: `export function handler(event, context) {
  const name = event.name || 'World';
  return { message: \`Hello, \${name}!\` };
}
`,
  bun: `function handler(event, context) {
  const name = event.name || 'World';
  return { message: \`Hello, \${name}!\` };
}

module.exports = { handler };
`,
}

// Runtimes that require compilation
const COMPILED_RUNTIMES = ['go', 'rust', 'java', 'kotlin', 'swift', 'zig', 'dotnet', 'scala']

// Get base runtime from versioned ID (e.g., "python3.11" -> "python")
function getBaseRuntime(runtimeId: string): string {
  const prefixes = ['python', 'node', 'go', 'rust', 'java', 'ruby', 'php', 'dotnet', 'deno', 'bun']
  for (const prefix of prefixes) {
    if (runtimeId.startsWith(prefix)) return prefix
  }
  return runtimeId
}

function needsCompilation(runtimeId: string): boolean {
  const base = getBaseRuntime(runtimeId)
  return COMPILED_RUNTIMES.includes(base)
}

interface CreateFunctionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (
    name: string,
    runtime: string,
    handler: string,
    memory: number,
    timeout: number,
    code: string,
    limits?: ResourceLimits
  ) => Promise<void>
  runtimes?: RuntimeInfo[]
}

export function CreateFunctionDialog({
  open,
  onOpenChange,
  onCreate,
  runtimes = [],
}: CreateFunctionDialogProps) {
  const [name, setName] = useState("")
  const [runtime, setRuntime] = useState("python")
  const [memory, setMemory] = useState("128")
  const [timeout, setTimeout] = useState("30")
  const [handler, setHandler] = useState("main.handler")
  const [code, setCode] = useState(CODE_TEMPLATES.python)
  const [vcpus, setVcpus] = useState("1")
  const [diskIops, setDiskIops] = useState("0")
  const [diskBandwidth, setDiskBandwidth] = useState("0")
  const [netRx, setNetRx] = useState("0")
  const [netTx, setNetTx] = useState("0")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Compile status tracking after creation
  const [createdFunctionName, setCreatedFunctionName] = useState<string | null>(null)
  const [compileStatus, setCompileStatus] = useState<CompileStatus | undefined>()
  const [compileError, setCompileError] = useState<string | undefined>()

  // Update code template when runtime changes
  useEffect(() => {
    const baseRuntime = getBaseRuntime(runtime)
    setCode(CODE_TEMPLATES[baseRuntime] || CODE_TEMPLATES.python)
  }, [runtime])

  // Poll for compile status after creation
  useEffect(() => {
    if (!createdFunctionName || compileStatus !== 'compiling') return

    const interval = setInterval(async () => {
      try {
        const response = await functionsApi.getCode(createdFunctionName)
        setCompileStatus(response.compile_status)
        setCompileError(response.compile_error)
      } catch {
        // Ignore polling errors
      }
    }, 2000)

    return () => clearInterval(interval)
  }, [createdFunctionName, compileStatus])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setCreatedFunctionName(null)
    setCompileStatus(undefined)
    setCompileError(undefined)

    const codeValue = code
    if (!codeValue.trim()) {
      setError("Code is required")
      return
    }

    try {
      setSubmitting(true)
      const limits: ResourceLimits = {
        vcpus: parseInt(vcpus) || 1,
        disk_iops: parseInt(diskIops) || 0,
        disk_bandwidth: parseInt(diskBandwidth) || 0,
        net_rx_bandwidth: parseInt(netRx) || 0,
        net_tx_bandwidth: parseInt(netTx) || 0,
      }
      await onCreate(name, runtime, handler, parseInt(memory), parseInt(timeout), codeValue, limits)

      // If it's a compiled language, track compile status
      if (needsCompilation(runtime)) {
        setCreatedFunctionName(name)
        setCompileStatus('compiling')
      } else {
        // Reset form and close dialog for interpreted languages
        resetForm()
        onOpenChange(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create function")
    } finally {
      setSubmitting(false)
    }
  }

  const resetForm = () => {
    setName("")
    setRuntime("python")
    setMemory("128")
    setTimeout("30")
    setHandler("main.handler")
    setCode(CODE_TEMPLATES.python)
    setVcpus("1")
    setDiskIops("0")
    setDiskBandwidth("0")
    setNetRx("0")
    setNetTx("0")
    setCreatedFunctionName(null)
    setCompileStatus(undefined)
    setCompileError(undefined)
  }

  const handleClose = () => {
    resetForm()
    onOpenChange(false)
  }

  // Group runtimes by language for better UX
  const groupedRuntimes = runtimes.length > 0 ? runtimes : [
    { id: "python", name: "Python", version: "3.12.12", status: "available" as const, functionsCount: 0, icon: "python" },
    { id: "node", name: "Node.js", version: "24.13.0", status: "available" as const, functionsCount: 0, icon: "nodejs" },
    { id: "go", name: "Go", version: "1.25.6", status: "available" as const, functionsCount: 0, icon: "go" },
    { id: "rust", name: "Rust", version: "1.93.0", status: "available" as const, functionsCount: 0, icon: "rust" },
    { id: "java", name: "Java", version: "21.0.10", status: "available" as const, functionsCount: 0, icon: "java" },
    { id: "ruby", name: "Ruby", version: "3.4.8", status: "available" as const, functionsCount: 0, icon: "ruby" },
    { id: "php", name: "PHP", version: "8.4.17", status: "available" as const, functionsCount: 0, icon: "php" },
    { id: "dotnet", name: ".NET", version: "8.0.23", status: "available" as const, functionsCount: 0, icon: "dotnet" },
    { id: "deno", name: "Deno", version: "2.6.7", status: "available" as const, functionsCount: 0, icon: "deno" },
    { id: "bun", name: "Bun", version: "1.3.8", status: "available" as const, functionsCount: 0, icon: "bun" },
  ]

  // Render compile status view after creation
  if (createdFunctionName && compileStatus) {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Function Created</DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{createdFunctionName}</span>
              {compileStatus === 'compiling' && (
                <Badge variant="outline" className="text-yellow-600 border-yellow-600">
                  <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  Compiling
                </Badge>
              )}
              {compileStatus === 'success' && (
                <Badge variant="outline" className="text-green-600 border-green-600">
                  <Check className="mr-1 h-3 w-3" />
                  Compiled
                </Badge>
              )}
              {compileStatus === 'failed' && (
                <Badge variant="destructive">
                  <AlertCircle className="mr-1 h-3 w-3" />
                  Failed
                </Badge>
              )}
            </div>

            {compileStatus === 'compiling' && (
              <div className="text-sm text-muted-foreground">
                Your function is being compiled. This may take a moment...
              </div>
            )}

            {compileStatus === 'success' && (
              <div className="rounded-md bg-green-50 dark:bg-green-950 p-3 text-sm text-green-700 dark:text-green-300">
                Compilation successful! Your function is ready to use.
              </div>
            )}

            {compileStatus === 'failed' && compileError && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                <div className="font-medium mb-1">Compilation Failed</div>
                <pre className="whitespace-pre-wrap text-xs font-mono">{compileError}</pre>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button onClick={handleClose}>
              {compileStatus === 'compiling' ? 'Close (Compiling in Background)' : 'Done'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create New Function</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">Function Name</Label>
              <Input
                id="name"
                placeholder="my-function"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="runtime">Runtime</Label>
              <Select value={runtime} onValueChange={setRuntime}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="max-h-64">
                  {groupedRuntimes.map((rt) => (
                    <SelectItem key={rt.id} value={rt.id}>
                      {rt.name} {rt.version}
                      {needsCompilation(rt.id) && (
                        <span className="ml-2 text-xs text-muted-foreground">(compiled)</span>
                      )}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label>Code</Label>
            <CodeEditor
              code={code}
              onChange={setCode}
              runtime={runtime}
              minHeight="256px"
            />
            <p className="text-xs text-muted-foreground">
              Template loaded for {getBaseRuntime(runtime)}.
              {needsCompilation(runtime) && " This runtime requires compilation."}
            </p>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="memory">Memory (MB)</Label>
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
            </div>

            <div className="space-y-2">
              <Label htmlFor="timeout">Timeout (s)</Label>
              <Input
                id="timeout"
                type="number"
                min="1"
                max="900"
                value={timeout}
                onChange={(e) => setTimeout(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="handler">Handler</Label>
              <Input
                id="handler"
                placeholder="main.handler"
                value={handler}
                onChange={(e) => setHandler(e.target.value)}
              />
            </div>
          </div>

          {/* Resource Limits */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">Resource Limits</Label>
            <p className="text-xs text-muted-foreground mb-2">
              Configure CPU, disk I/O, and network bandwidth. Set to 0 for unlimited.
            </p>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
              <div className="space-y-1">
                <Label htmlFor="vcpus" className="text-xs text-muted-foreground">vCPUs</Label>
                <Select value={vcpus} onValueChange={setVcpus}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[1, 2, 4, 8, 16, 32].map((v) => (
                      <SelectItem key={v} value={v.toString()}>{v} vCPU{v > 1 ? "s" : ""}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="diskIops" className="text-xs text-muted-foreground">Disk IOPS</Label>
                <Input
                  id="diskIops"
                  type="number"
                  min="0"
                  className="h-9"
                  value={diskIops}
                  onChange={(e) => setDiskIops(e.target.value)}
                  placeholder="0"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="diskBw" className="text-xs text-muted-foreground">Disk BW (B/s)</Label>
                <Input
                  id="diskBw"
                  type="number"
                  min="0"
                  className="h-9"
                  value={diskBandwidth}
                  onChange={(e) => setDiskBandwidth(e.target.value)}
                  placeholder="0"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="netRx" className="text-xs text-muted-foreground">Net RX (B/s)</Label>
                <Input
                  id="netRx"
                  type="number"
                  min="0"
                  className="h-9"
                  value={netRx}
                  onChange={(e) => setNetRx(e.target.value)}
                  placeholder="0"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="netTx" className="text-xs text-muted-foreground">Net TX (B/s)</Label>
                <Input
                  id="netTx"
                  type="number"
                  min="0"
                  className="h-9"
                  value={netTx}
                  onChange={(e) => setNetTx(e.target.value)}
                  placeholder="0"
                />
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || !code.trim() || submitting}
            >
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Function
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

"use client"

import React from "react"
import { useState, useEffect } from "react"
import { useTranslations } from "next-intl"
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
import { functionsApi, aiApi, CompileStatus, type NetworkPolicy, type ResourceLimits } from "@/lib/api"
import { Loader2, Check, AlertCircle, Trash2, Sparkles } from "lucide-react"

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
const COMPILED_RUNTIMES = ['go', 'rust', 'java', 'kotlin', 'swift', 'zig', 'scala']

const AWS_FUNCTION_NAME_PATTERN = /^[A-Za-z0-9_-]{1,64}$/
const AWS_MODULE_HANDLER_PATTERN = /^[A-Za-z0-9_./-]+\.[A-Za-z0-9_$][A-Za-z0-9_$.]*$/
const AWS_JAVA_HANDLER_PATTERN = /^[A-Za-z0-9_$.]+::[A-Za-z0-9_$]+$/
const AWS_EXECUTABLE_HANDLER_PATTERN = /^[A-Za-z0-9_/-]{1,128}$/

// Get base runtime from versioned ID (e.g., "python3.11" -> "python")
function getBaseRuntime(runtimeId: string): string {
  const prefixes = ['python', 'node', 'go', 'rust', 'java', 'ruby', 'php', 'deno', 'bun']
  for (const prefix of prefixes) {
    if (runtimeId.startsWith(prefix)) return prefix
  }
  return runtimeId
}

function needsCompilation(runtimeId: string): boolean {
  const base = getBaseRuntime(runtimeId)
  return COMPILED_RUNTIMES.includes(base)
}

function getDefaultHandler(runtimeId: string): string {
  const base = getBaseRuntime(runtimeId)
  if (base === "java" || base === "kotlin" || base === "scala") {
    return "example.Handler::handleRequest"
  }
  if (base === "go" || base === "rust" || base === "swift" || base === "zig" || base === "wasm") {
    return "handler"
  }
  return "main.handler"
}

type ValidationKey =
  | "validationNameFormat"
  | "validationMemoryRange"
  | "validationTimeoutRange"
  | "validationJavaHandler"
  | "validationCompiledHandler"
  | "validationDefaultHandler"

function validateAwsCreateInput(params: {
  name: string
  runtime: string
  handler: string
  memory: number
  timeout: number
}): ValidationKey | null {
  const { name, runtime, handler, memory, timeout } = params

  if (!AWS_FUNCTION_NAME_PATTERN.test(name)) {
    return "validationNameFormat"
  }
  if (!Number.isFinite(memory) || memory < 128 || memory > 10240) {
    return "validationMemoryRange"
  }
  if (!Number.isFinite(timeout) || timeout < 1 || timeout > 900) {
    return "validationTimeoutRange"
  }

  const base = getBaseRuntime(runtime)
  if (base === "java" || base === "kotlin" || base === "scala") {
    if (!AWS_JAVA_HANDLER_PATTERN.test(handler)) {
      return "validationJavaHandler"
    }
    return null
  }
  if (base === "go" || base === "rust" || base === "swift" || base === "zig" || base === "wasm") {
    if (!AWS_EXECUTABLE_HANDLER_PATTERN.test(handler)) {
      return "validationCompiledHandler"
    }
    return null
  }
  if (!AWS_MODULE_HANDLER_PATTERN.test(handler)) {
    return "validationDefaultHandler"
  }
  return null
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
    limits?: ResourceLimits,
    networkPolicy?: NetworkPolicy
  ) => Promise<void>
  runtimes?: RuntimeInfo[]
}

type EditableEgressRule = {
  host: string
  port: string
  protocol: string
}

type EditableIngressRule = {
  source: string
  port: string
  protocol: string
}

export function CreateFunctionDialog({
  open,
  onOpenChange,
  onCreate,
  runtimes = [],
}: CreateFunctionDialogProps) {
  const t = useTranslations("createFunction")
  const tc = useTranslations("common")
  const [name, setName] = useState("")
  const [runtime, setRuntime] = useState("python")
  const [memory, setMemory] = useState("128")
  const [timeout, setTimeout] = useState("30")
  const [handler, setHandler] = useState(getDefaultHandler("python"))
  const [code, setCode] = useState(CODE_TEMPLATES.python)
  const [vcpus, setVcpus] = useState("1")
  const [diskIops, setDiskIops] = useState("0")
  const [diskBandwidth, setDiskBandwidth] = useState("0")
  const [netRx, setNetRx] = useState("0")
  const [netTx, setNetTx] = useState("0")
  const [isolationMode, setIsolationMode] = useState("egress-only")
  const [denyExternalAccess, setDenyExternalAccess] = useState("false")
  const [ingressRules, setIngressRules] = useState<EditableIngressRule[]>([])
  const [egressRules, setEgressRules] = useState<EditableEgressRule[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Compile status tracking after creation
  const [createdFunctionName, setCreatedFunctionName] = useState<string | null>(null)
  const [compileStatus, setCompileStatus] = useState<CompileStatus | undefined>()
  const [compileError, setCompileError] = useState<string | undefined>()

  // AI generation state
  const [aiEnabled, setAiEnabled] = useState(false)
  const [aiDescription, setAiDescription] = useState("")
  const [aiGenerating, setAiGenerating] = useState(false)

  // Update code template when runtime changes
  useEffect(() => {
    const baseRuntime = getBaseRuntime(runtime)
    setCode(CODE_TEMPLATES[baseRuntime] || CODE_TEMPLATES.python)
    setHandler(getDefaultHandler(runtime))
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

  // Check AI status on mount
  useEffect(() => {
    aiApi.status().then((res) => setAiEnabled(res.enabled)).catch(() => {})
  }, [])

  const handleAiGenerate = async () => {
    if (!aiDescription.trim()) return
    try {
      setAiGenerating(true)
      setError(null)
      const baseRuntime = getBaseRuntime(runtime)
      const response = await aiApi.generate({ description: aiDescription, runtime: baseRuntime })
      setCode(response.code)
      if (response.function_name && !name.trim()) {
        setName(response.function_name)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "AI generation failed")
    } finally {
      setAiGenerating(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setCreatedFunctionName(null)
    setCompileStatus(undefined)
    setCompileError(undefined)

    const codeValue = code
    if (!codeValue.trim()) {
      setError(t("codeRequired"))
      return
    }

    const trimmedName = name.trim()
    const resolvedHandler = handler.trim() || getDefaultHandler(runtime)
    const parsedMemory = Number.parseInt(memory, 10)
    const parsedTimeout = Number.parseInt(timeout, 10)
    const validationError = validateAwsCreateInput({
      name: trimmedName,
      runtime,
      handler: resolvedHandler,
      memory: parsedMemory,
      timeout: parsedTimeout,
    })
    if (validationError) {
      setError(t(validationError))
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
      const parsedIngressRules: NonNullable<NetworkPolicy["ingress_rules"]> = []
      for (const rule of ingressRules) {
        const source = rule.source.trim()
        if (!source) {
          continue
        }
        const port = Number.parseInt(rule.port, 10)
        const protocol = rule.protocol.trim().toLowerCase()
        parsedIngressRules.push({
          source,
          port: Number.isFinite(port) && port > 0 ? port : undefined,
          protocol: protocol === "udp" ? "udp" : "tcp",
        })
      }
      const parsedEgressRules: NonNullable<NetworkPolicy["egress_rules"]> = []
      for (const rule of egressRules) {
        const host = rule.host.trim()
        if (!host) {
          continue
        }
        const port = Number.parseInt(rule.port, 10)
        const protocol = rule.protocol.trim().toLowerCase()
        parsedEgressRules.push({
          host,
          port: Number.isFinite(port) && port > 0 ? port : undefined,
          protocol: protocol === "udp" ? "udp" : "tcp",
        })
      }
      const networkPolicy: NetworkPolicy = {
        isolation_mode: isolationMode,
        deny_external_access: denyExternalAccess === "true",
        ingress_rules: parsedIngressRules,
        egress_rules: parsedEgressRules,
      }
      await onCreate(trimmedName, runtime, resolvedHandler, parsedMemory, parsedTimeout, codeValue, limits, networkPolicy)

      // If it's a compiled language, track compile status
      if (needsCompilation(runtime)) {
        setCreatedFunctionName(trimmedName)
        setCompileStatus('compiling')
      } else {
        // Reset form and close dialog for interpreted languages
        resetForm()
        onOpenChange(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : tc("error"))
    } finally {
      setSubmitting(false)
    }
  }

  const resetForm = () => {
    setName("")
    setRuntime("python")
    setMemory("128")
    setTimeout("30")
    setHandler(getDefaultHandler("python"))
    setCode(CODE_TEMPLATES.python)
    setVcpus("1")
    setDiskIops("0")
    setDiskBandwidth("0")
    setNetRx("0")
    setNetTx("0")
    setIsolationMode("egress-only")
    setDenyExternalAccess("false")
    setIngressRules([])
    setEgressRules([])
    setCreatedFunctionName(null)
    setCompileStatus(undefined)
    setCompileError(undefined)
  }

  const addEgressRule = () => {
    setEgressRules((prev) => [...prev, { host: "", port: "", protocol: "tcp" }])
  }

  const addIngressRule = () => {
    setIngressRules((prev) => [...prev, { source: "", port: "", protocol: "tcp" }])
  }

  const updateEgressRule = (index: number, field: keyof EditableEgressRule, value: string) => {
    setEgressRules((prev) => prev.map((rule, i) => (i === index ? { ...rule, [field]: value } : rule)))
  }

  const removeEgressRule = (index: number) => {
    setEgressRules((prev) => prev.filter((_, i) => i !== index))
  }

  const updateIngressRule = (index: number, field: keyof EditableIngressRule, value: string) => {
    setIngressRules((prev) => prev.map((rule, i) => (i === index ? { ...rule, [field]: value } : rule)))
  }

  const removeIngressRule = (index: number) => {
    setIngressRules((prev) => prev.filter((_, i) => i !== index))
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
    { id: "deno", name: "Deno", version: "2.6.7", status: "available" as const, functionsCount: 0, icon: "deno" },
    { id: "bun", name: "Bun", version: "1.3.8", status: "available" as const, functionsCount: 0, icon: "bun" },
  ]

  // Render compile status view after creation
  if (createdFunctionName && compileStatus) {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("functionCreated")}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{createdFunctionName}</span>
              {compileStatus === 'compiling' && (
                <Badge variant="outline" className="text-yellow-600 border-yellow-600">
                  <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                  {t("compiling")}
                </Badge>
              )}
              {compileStatus === 'success' && (
                <Badge variant="outline" className="text-green-600 border-green-600">
                  <Check className="mr-1 h-3 w-3" />
                  {t("compiledStatus")}
                </Badge>
              )}
              {compileStatus === 'failed' && (
                <Badge variant="destructive">
                  <AlertCircle className="mr-1 h-3 w-3" />
                  {t("failed")}
                </Badge>
              )}
            </div>

            {compileStatus === 'compiling' && (
              <div className="text-sm text-muted-foreground">
                {t("compilingMessage")}
              </div>
            )}

            {compileStatus === 'success' && (
              <div className="rounded-md bg-green-50 dark:bg-green-950 p-3 text-sm text-green-700 dark:text-green-300">
                {t("compiledMessage")}
              </div>
            )}

            {compileStatus === 'failed' && compileError && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                <div className="font-medium mb-1">{t("compilationFailed")}</div>
                <pre className="whitespace-pre-wrap text-xs font-mono">{compileError}</pre>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button onClick={handleClose}>
              {compileStatus === 'compiling' ? t("closeCompiling") : t("done")}
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
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">{t("functionName")}</Label>
              <Input
                id="name"
                placeholder={t("functionNamePlaceholder")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={64}
                required
              />
              <p className="text-xs text-muted-foreground">
                {t("functionNameHelp")}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="runtime">{t("runtime")}</Label>
              <Select value={runtime} onValueChange={setRuntime}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="max-h-64">
                  {groupedRuntimes.map((rt) => (
                    <SelectItem key={rt.id} value={rt.id}>
                      {rt.name} {rt.version}
                      {needsCompilation(rt.id) && (
                        <span className="ml-2 text-xs text-muted-foreground">{t("compiled")}</span>
                      )}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t("code")}</Label>
            {aiEnabled && (
              <div className="flex items-center gap-2 mb-2">
                <div className="relative flex-1">
                  <Input
                    placeholder="Describe your function... (e.g., 'Calculate Fibonacci numbers')"
                    value={aiDescription}
                    onChange={(e) => setAiDescription(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && aiDescription.trim()) {
                        e.preventDefault()
                        handleAiGenerate()
                      }
                    }}
                  />
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleAiGenerate}
                  disabled={aiGenerating || !aiDescription.trim()}
                >
                  {aiGenerating ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Sparkles className="mr-2 h-4 w-4" />
                  )}
                  Generate with AI
                </Button>
              </div>
            )}
            <CodeEditor
              code={code}
              onChange={setCode}
              runtime={runtime}
              minHeight="256px"
            />
            <p className="text-xs text-muted-foreground">
              {t("templateLoaded", { runtime: getBaseRuntime(runtime) })}
              {needsCompilation(runtime) && t("requiresCompilation")}
            </p>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="memory">{t("memoryMb")}</Label>
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
              <Label htmlFor="timeout">{t("timeoutS")}</Label>
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
              <Label htmlFor="handler">{t("handler")}</Label>
              <Input
                id="handler"
                placeholder={getDefaultHandler(runtime)}
                value={handler}
                onChange={(e) => setHandler(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {t("handlerHelp")}
              </p>
            </div>
          </div>

          {/* Resource Limits */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">{t("resourceLimits")}</Label>
            <p className="text-xs text-muted-foreground mb-2">
              {t("resourceLimitsHelp")}
            </p>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
              <div className="space-y-1">
                <Label htmlFor="vcpus" className="text-xs text-muted-foreground">{t("vcpus")}</Label>
                <Select value={vcpus} onValueChange={setVcpus}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[1, 2, 4, 8, 16, 32].map((v) => (
                      <SelectItem key={v} value={v.toString()}>{v} {t("vcpu")}{v > 1 ? "s" : ""}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor="diskIops" className="text-xs text-muted-foreground">{t("diskIops")}</Label>
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
                <Label htmlFor="diskBw" className="text-xs text-muted-foreground">{t("diskBw")}</Label>
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
                <Label htmlFor="netRx" className="text-xs text-muted-foreground">{t("netRx")}</Label>
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
                <Label htmlFor="netTx" className="text-xs text-muted-foreground">{t("netTx")}</Label>
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

          {/* Network Policy */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">{t("networkPolicy")}</Label>
            <p className="text-xs text-muted-foreground mb-2">
              {t("networkPolicyHelp")}
            </p>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">{t("isolationMode")}</Label>
                <Select value={isolationMode} onValueChange={setIsolationMode}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">none</SelectItem>
                    <SelectItem value="egress-only">egress-only</SelectItem>
                    <SelectItem value="strict">strict</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">{t("denyExternalAccess")}</Label>
                <Select value={denyExternalAccess} onValueChange={setDenyExternalAccess}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="false">false</SelectItem>
                    <SelectItem value="true">true</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="pt-2 space-y-2">
              <div className="flex items-center justify-between">
                <Label className="text-xs text-muted-foreground">{t("ingressRules")}</Label>
                <Button type="button" variant="outline" size="sm" onClick={addIngressRule}>
                  {t("addRule")}
                </Button>
              </div>
              {ingressRules.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t("noIngressRules")}</p>
              ) : (
                ingressRules.map((rule, index) => (
                  <div key={`create-ingress-${index}`} className="grid grid-cols-[1fr_100px_110px_auto] gap-2 items-end">
                    <Input
                      className="h-9"
                      value={rule.source}
                      onChange={(e) => updateIngressRule(index, "source", e.target.value)}
                      placeholder="caller-func or 10.0.0.0/8"
                    />
                    <Input
                      className="h-9"
                      type="number"
                      min="0"
                      max="65535"
                      value={rule.port}
                      onChange={(e) => updateIngressRule(index, "port", e.target.value)}
                      placeholder="0"
                    />
                    <Select value={rule.protocol} onValueChange={(value) => updateIngressRule(index, "protocol", value)}>
                      <SelectTrigger className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="tcp">tcp</SelectItem>
                        <SelectItem value="udp">udp</SelectItem>
                      </SelectContent>
                    </Select>
                    <Button type="button" variant="ghost" size="icon" onClick={() => removeIngressRule(index)}>
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                ))
              )}
            </div>

            <div className="pt-2 space-y-2">
              <div className="flex items-center justify-between">
                <Label className="text-xs text-muted-foreground">{t("egressRules")}</Label>
                <Button type="button" variant="outline" size="sm" onClick={addEgressRule}>
                  {t("addRule")}
                </Button>
              </div>
              {egressRules.length === 0 ? (
                <p className="text-xs text-muted-foreground">{t("noEgressRules")}</p>
              ) : (
                egressRules.map((rule, index) => (
                  <div key={`create-egress-${index}`} className="grid grid-cols-[1fr_100px_110px_auto] gap-2 items-end">
                    <Input
                      className="h-9"
                      value={rule.host}
                      onChange={(e) => updateEgressRule(index, "host", e.target.value)}
                      placeholder="example.com or 10.0.0.0/8"
                    />
                    <Input
                      className="h-9"
                      type="number"
                      min="0"
                      max="65535"
                      value={rule.port}
                      onChange={(e) => updateEgressRule(index, "port", e.target.value)}
                      placeholder="0"
                    />
                    <Select value={rule.protocol} onValueChange={(value) => updateEgressRule(index, "protocol", value)}>
                      <SelectTrigger className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="tcp">tcp</SelectItem>
                        <SelectItem value="udp">udp</SelectItem>
                      </SelectContent>
                    </Select>
                    <Button type="button" variant="ghost" size="icon" onClick={() => removeEgressRule(index)}>
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                ))
              )}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose}>
              {tc("cancel")}
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || !code.trim() || submitting}
            >
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {tc("create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

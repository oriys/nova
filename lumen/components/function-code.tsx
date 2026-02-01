"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { CodeDisplay } from "@/components/code-editor"
import { FunctionData } from "@/lib/types"
import { Copy, Check, Download, Upload } from "lucide-react"

interface FunctionCodeProps {
  func: FunctionData
}

const defaultCode = `export async function handler(event, context) {
  console.log('Event:', JSON.stringify(event, null, 2));

  try {
    // Your function logic here
    const response = {
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        message: 'Hello from ${'{func.name}'}!',
        timestamp: new Date().toISOString(),
      }),
    };

    return response;
  } catch (error) {
    console.error('Error:', error);
    return {
      statusCode: 500,
      body: JSON.stringify({ error: 'Internal Server Error' }),
    };
  }
}`

// Map display runtime names back to runtime IDs for highlighting
function getRuntimeId(displayName: string): string {
  const lower = displayName.toLowerCase()
  if (lower.includes("python")) return "python"
  if (lower.includes("go ")) return "go"
  if (lower.includes("rust")) return "rust"
  if (lower.includes("node")) return "node"
  if (lower.includes("ruby")) return "ruby"
  if (lower.includes("java") && !lower.includes("javascript")) return "java"
  if (lower.includes("deno")) return "deno"
  if (lower.includes("bun")) return "bun"
  if (lower.includes("php")) return "php"
  if (lower.includes(".net")) return "dotnet"
  if (lower.includes("elixir")) return "elixir"
  if (lower.includes("kotlin")) return "kotlin"
  if (lower.includes("swift")) return "swift"
  if (lower.includes("zig")) return "zig"
  if (lower.includes("lua")) return "lua"
  if (lower.includes("perl")) return "perl"
  if (lower.includes("scala")) return "scala"
  if (lower.includes("wasm") || lower.includes("webassembly")) return "wasm"
  return "javascript"
}

export function FunctionCode({ func }: FunctionCodeProps) {
  const [copied, setCopied] = useState(false)
  const code = func.code || defaultCode.replace('${func.name}', func.name)
  const runtimeId = getRuntimeId(func.runtime)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-foreground">
            {func.handler}
          </span>
          <span className="text-xs text-muted-foreground">
            {func.runtime}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <Check className="mr-2 h-4 w-4" />
            ) : (
              <Copy className="mr-2 h-4 w-4" />
            )}
            {copied ? "Copied" : "Copy"}
          </Button>
          <Button variant="outline" size="sm">
            <Download className="mr-2 h-4 w-4" />
            Download
          </Button>
          <Button variant="outline" size="sm">
            <Upload className="mr-2 h-4 w-4" />
            Upload
          </Button>
        </div>
      </div>

      {/* Code Display with Syntax Highlighting */}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-2">
          <div className="flex items-center gap-2">
            <div className="h-3 w-3 rounded-full bg-destructive/50" />
            <div className="h-3 w-3 rounded-full bg-warning/50" />
            <div className="h-3 w-3 rounded-full bg-success/50" />
          </div>
          <span className="text-xs text-muted-foreground">{func.handler}</span>
        </div>
        <CodeDisplay
          code={code}
          runtime={runtimeId}
          className="border-0 rounded-none"
          maxHeight="600px"
        />
      </div>

      {/* Info */}
      <div className="rounded-lg border border-border bg-muted/30 p-4">
        <p className="text-sm text-muted-foreground">
          This is a read-only view of your function code. To edit, download the code,
          make changes locally, and upload the updated version.
        </p>
      </div>
    </div>
  )
}

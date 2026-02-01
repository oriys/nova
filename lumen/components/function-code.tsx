"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
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

export function FunctionCode({ func }: FunctionCodeProps) {
  const [copied, setCopied] = useState(false)
  const code = func.code || defaultCode.replace('${func.name}', func.name)

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

      {/* Code Editor */}
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-2">
          <div className="flex items-center gap-2">
            <div className="h-3 w-3 rounded-full bg-destructive/50" />
            <div className="h-3 w-3 rounded-full bg-warning/50" />
            <div className="h-3 w-3 rounded-full bg-success/50" />
          </div>
          <span className="text-xs text-muted-foreground">index.js</span>
        </div>
        <div className="relative">
          <pre className="overflow-x-auto p-4 text-sm">
            <code className="font-mono text-foreground">
              {code.split('\n').map((line, i) => (
                <div key={i} className="flex">
                  <span className="w-8 shrink-0 select-none text-right text-muted-foreground/50 pr-4">
                    {i + 1}
                  </span>
                  <span className="flex-1">{line || ' '}</span>
                </div>
              ))}
            </code>
          </pre>
        </div>
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

"use client"

import { useState, useRef, useEffect, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Loader2, Trash2 } from "lucide-react"
import { sandboxApi } from "@/lib/api"

interface TerminalLine {
  type: "input" | "stdout" | "stderr" | "info"
  text: string
}

interface SandboxTerminalProps {
  sandboxId: string
}

export function SandboxTerminal({ sandboxId }: SandboxTerminalProps) {
  const t = useTranslations("pages.sandboxes.terminal")
  const [lines, setLines] = useState<TerminalLine[]>([
    { type: "info", text: `Connected to sandbox ${sandboxId.slice(0, 12)}…` },
  ])
  const [input, setInput] = useState("")
  const [running, setRunning] = useState(false)
  const [historyIndex, setHistoryIndex] = useState(-1)
  const historyRef = useRef<string[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [lines])

  const execCommand = useCallback(async (cmd: string) => {
    if (!cmd.trim()) return

    historyRef.current = [cmd, ...historyRef.current.slice(0, 99)]
    setHistoryIndex(-1)
    setLines((prev) => [...prev, { type: "input", text: `$ ${cmd}` }])
    setInput("")
    setRunning(true)

    try {
      const resp = await sandboxApi.exec(sandboxId, { command: cmd })
      if (resp.stdout) {
        setLines((prev) => [...prev, { type: "stdout", text: resp.stdout }])
      }
      if (resp.stderr) {
        setLines((prev) => [...prev, { type: "stderr", text: resp.stderr }])
      }
      if (resp.exit_code !== 0) {
        setLines((prev) => [
          ...prev,
          { type: "info", text: `exit code: ${resp.exit_code}` },
        ])
      }
    } catch (err) {
      setLines((prev) => [
        ...prev,
        { type: "stderr", text: `Error: ${err instanceof Error ? err.message : String(err)}` },
      ])
    } finally {
      setRunning(false)
      inputRef.current?.focus()
    }
  }, [sandboxId])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && !running) {
      execCommand(input)
      return
    }
    if (e.key === "ArrowUp") {
      e.preventDefault()
      const next = Math.min(historyIndex + 1, historyRef.current.length - 1)
      if (next >= 0 && historyRef.current[next]) {
        setHistoryIndex(next)
        setInput(historyRef.current[next])
      }
      return
    }
    if (e.key === "ArrowDown") {
      e.preventDefault()
      const next = historyIndex - 1
      if (next < 0) {
        setHistoryIndex(-1)
        setInput("")
      } else {
        setHistoryIndex(next)
        setInput(historyRef.current[next] ?? "")
      }
    }
  }

  const lineColor: Record<TerminalLine["type"], string> = {
    input: "text-blue-400",
    stdout: "text-foreground",
    stderr: "text-red-400",
    info: "text-muted-foreground",
  }

  return (
    <div className="flex h-full flex-col rounded-lg border border-border bg-black/95">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="text-xs font-medium text-muted-foreground">{t("title")}</span>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => setLines([{ type: "info", text: t("cleared") }])}
          title={t("clear")}
        >
          <Trash2 className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto p-3 font-mono text-sm">
        {lines.map((line, i) => (
          <pre key={i} className={`whitespace-pre-wrap break-all ${lineColor[line.type]}`}>
            {line.text}
          </pre>
        ))}
        <div ref={bottomRef} />
      </div>

      <div className="flex items-center border-t border-border px-3 py-2">
        <span className="mr-2 font-mono text-sm text-green-400">$</span>
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={running}
          placeholder={running ? t("running") : t("placeholder")}
          className="flex-1 bg-transparent font-mono text-sm text-foreground outline-none placeholder:text-muted-foreground"
          autoFocus
        />
        {running && <Loader2 className="ml-2 h-4 w-4 animate-spin text-muted-foreground" />}
      </div>
    </div>
  )
}

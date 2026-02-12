"use client"

import { useRef, useCallback } from "react"
import { useTheme } from "next-themes"
import Editor, { OnMount } from "@monaco-editor/react"
import type { editor } from "monaco-editor"
import { cn } from "@/lib/utils"

function useMonacoTheme() {
  const { resolvedTheme } = useTheme()
  return resolvedTheme === "dark" ? "vs-dark" : "light"
}

// Map runtime IDs to Monaco language identifiers
const LANGUAGE_MAP: Record<string, string> = {
  python: "python",
  node: "javascript",
  go: "go",
  rust: "rust",
  java: "java",
  ruby: "ruby",
  php: "php",
  deno: "typescript",
  bun: "typescript",
  javascript: "javascript",
  typescript: "typescript",
  csharp: "csharp",
  kotlin: "kotlin",
  swift: "swift",
  scala: "scala",
  shell: "shell",
  bash: "shell",
  lua: "lua",
  perl: "perl",
  r: "r",
  sql: "sql",
  yaml: "yaml",
  json: "json",
  xml: "xml",
  html: "html",
  css: "css",
  markdown: "markdown",
  dockerfile: "dockerfile",
  c: "c",
  cpp: "cpp",
}

interface CodeEditorProps {
  code: string
  onChange?: (code: string) => void
  language?: string
  runtime?: string
  readOnly?: boolean
  className?: string
  showLineNumbers?: boolean
  minHeight?: string
  fontSize?: number
  minimap?: boolean
}

export function CodeEditor({
  code,
  onChange,
  language,
  runtime,
  readOnly = false,
  className,
  showLineNumbers = true,
  minHeight = "200px",
  fontSize = 14,
  minimap = false,
}: CodeEditorProps) {
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const monacoTheme = useMonacoTheme()

  const monacoLanguage = language || LANGUAGE_MAP[runtime || ""] || "plaintext"

  const handleMount: OnMount = useCallback((editor) => {
    editorRef.current = editor
    editor.focus()
  }, [])

  const handleChange = useCallback(
    (value: string | undefined) => {
      onChange?.(value ?? "")
    },
    [onChange]
  )

  return (
    <div
      className={cn("rounded-lg border border-border overflow-hidden", className)}
      style={{ minHeight }}
    >
      <Editor
        height={minHeight}
        language={monacoLanguage}
        value={code}
        onChange={handleChange}
        onMount={handleMount}
        theme={monacoTheme}
        options={{
          readOnly,
          fontSize,
          lineNumbers: showLineNumbers ? "on" : "off",
          minimap: { enabled: minimap },
          scrollBeyondLastLine: false,
          folding: true,
          wordWrap: "on",
          automaticLayout: true,
          tabSize: 2,
          insertSpaces: true,
          renderWhitespace: "selection",
          bracketPairColorization: { enabled: true },
          guides: {
            indentation: true,
            bracketPairs: true,
          },
          padding: { top: 12, bottom: 12 },
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          overviewRulerBorder: false,
          scrollbar: {
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8,
          },
          contextmenu: true,
          quickSuggestions: !readOnly,
          suggestOnTriggerCharacters: !readOnly,
          find: {
            addExtraSpaceOnTop: false,
          },
        }}
        loading={
          <div className="flex items-center justify-center h-full bg-background text-muted-foreground text-sm">
            Loading editor...
          </div>
        }
      />
    </div>
  )
}

// Read-only code display component
interface CodeDisplayProps {
  code: string
  language?: string
  runtime?: string
  className?: string
  showLineNumbers?: boolean
  maxHeight?: string
  fontSize?: number
}

export function CodeDisplay({
  code,
  language,
  runtime,
  className,
  showLineNumbers = true,
  maxHeight = "400px",
  fontSize = 13,
}: CodeDisplayProps) {
  const monacoLanguage = language || LANGUAGE_MAP[runtime || ""] || "plaintext"
  const monacoTheme = useMonacoTheme()

  return (
    <div
      className={cn("rounded-lg border border-border overflow-hidden", className)}
      style={{ maxHeight }}
    >
      <Editor
        height={maxHeight}
        language={monacoLanguage}
        value={code}
        theme={monacoTheme}
        options={{
          readOnly: true,
          fontSize,
          lineNumbers: showLineNumbers ? "on" : "off",
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          folding: true,
          wordWrap: "on",
          automaticLayout: true,
          padding: { top: 12, bottom: 12 },
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          overviewRulerBorder: false,
          scrollbar: {
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8,
          },
          domReadOnly: true,
          renderLineHighlight: "none",
          contextmenu: false,
        }}
        loading={
          <div className="flex items-center justify-center h-full bg-background text-muted-foreground text-sm">
            Loading...
          </div>
        }
      />
    </div>
  )
}

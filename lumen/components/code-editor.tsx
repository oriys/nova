"use client"

import { useState, useRef, useCallback } from "react"
import { Highlight, themes } from "prism-react-renderer"
import { cn } from "@/lib/utils"

// Map runtime IDs to Prism language names
const LANGUAGE_MAP: Record<string, string> = {
  python: "python",
  node: "javascript",
  go: "go",
  rust: "rust",
  java: "java",
  ruby: "ruby",
  php: "markup",
  dotnet: "csharp",
  deno: "typescript",
  bun: "typescript",
  javascript: "javascript",
  typescript: "typescript",
  csharp: "csharp",
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
}: CodeEditorProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [isFocused, setIsFocused] = useState(false)

  // Determine language from runtime or explicit language prop
  const prismLanguage = language || LANGUAGE_MAP[runtime || ""] || "javascript"

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (readOnly) return

      // Handle Tab key for indentation
      if (e.key === "Tab") {
        e.preventDefault()
        const textarea = e.currentTarget
        const start = textarea.selectionStart
        const end = textarea.selectionEnd
        const value = textarea.value

        // Insert 2 spaces for tab
        const newValue = value.substring(0, start) + "  " + value.substring(end)
        onChange?.(newValue)

        // Move cursor after the inserted spaces
        requestAnimationFrame(() => {
          textarea.selectionStart = textarea.selectionEnd = start + 2
        })
      }
    },
    [onChange, readOnly]
  )

  const handleScroll = useCallback((e: React.UIEvent<HTMLTextAreaElement>) => {
    // Sync scroll between textarea and highlighted code
    const textarea = e.currentTarget
    const pre = textarea.parentElement?.querySelector("pre")
    if (pre) {
      pre.scrollTop = textarea.scrollTop
      pre.scrollLeft = textarea.scrollLeft
    }
  }, [])

  return (
    <div
      className={cn(
        "relative rounded-lg border border-border bg-[#1e1e1e] overflow-hidden font-mono text-sm",
        isFocused && "ring-2 ring-ring ring-offset-2 ring-offset-background",
        className
      )}
      style={{ minHeight }}
    >
      {/* Syntax highlighted code (visual layer) */}
      <Highlight theme={themes.vsDark} code={code} language={prismLanguage}>
        {({ className: highlightClass, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={cn(highlightClass, "m-0 p-4 overflow-auto pointer-events-none")}
            style={{
              ...style,
              background: "transparent",
              minHeight,
              margin: 0,
            }}
          >
            {tokens.map((line, i) => (
              <div key={i} {...getLineProps({ line })} className="table-row">
                {showLineNumbers && (
                  <span className="table-cell pr-4 text-right select-none text-muted-foreground/50 w-8">
                    {i + 1}
                  </span>
                )}
                <span className="table-cell">
                  {line.map((token, key) => (
                    <span key={key} {...getTokenProps({ token })} />
                  ))}
                </span>
              </div>
            ))}
          </pre>
        )}
      </Highlight>

      {/* Editable textarea (input layer) */}
      {!readOnly && (
        <textarea
          ref={textareaRef}
          value={code}
          onChange={(e) => onChange?.(e.target.value)}
          onKeyDown={handleKeyDown}
          onScroll={handleScroll}
          onFocus={() => setIsFocused(true)}
          onBlur={() => setIsFocused(false)}
          spellCheck={false}
          className={cn(
            "absolute inset-0 w-full h-full p-4 resize-none bg-transparent text-transparent caret-white",
            "focus:outline-none font-mono text-sm leading-relaxed",
            showLineNumbers && "pl-12"
          )}
          style={{
            minHeight,
            lineHeight: "1.5rem",
          }}
        />
      )}
    </div>
  )
}

// Read-only code display component (simpler version)
interface CodeDisplayProps {
  code: string
  language?: string
  runtime?: string
  className?: string
  showLineNumbers?: boolean
  maxHeight?: string
}

export function CodeDisplay({
  code,
  language,
  runtime,
  className,
  showLineNumbers = true,
  maxHeight,
}: CodeDisplayProps) {
  const prismLanguage = language || LANGUAGE_MAP[runtime || ""] || "javascript"

  return (
    <div
      className={cn(
        "rounded-lg border border-border bg-[#1e1e1e] overflow-auto font-mono text-sm",
        className
      )}
      style={{ maxHeight }}
    >
      <Highlight theme={themes.vsDark} code={code} language={prismLanguage}>
        {({ className: highlightClass, style, tokens, getLineProps, getTokenProps }) => (
          <pre
            className={cn(highlightClass, "m-0 p-4")}
            style={{
              ...style,
              background: "transparent",
              margin: 0,
            }}
          >
            {tokens.map((line, i) => (
              <div key={i} {...getLineProps({ line })} className="table-row">
                {showLineNumbers && (
                  <span className="table-cell pr-4 text-right select-none text-muted-foreground/50 w-8">
                    {i + 1}
                  </span>
                )}
                <span className="table-cell whitespace-pre">
                  {line.map((token, key) => (
                    <span key={key} {...getTokenProps({ token })} />
                  ))}
                </span>
              </div>
            ))}
          </pre>
        )}
      </Highlight>
    </div>
  )
}

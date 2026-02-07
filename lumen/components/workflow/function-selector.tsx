"use client"

import { useState, useRef, useEffect } from "react"
import { Input } from "@/components/ui/input"

interface FunctionSelectorProps {
  functions: string[]
  value: string
  onChange: (value: string) => void
}

export function FunctionSelector({ functions, value, onChange }: FunctionSelectorProps) {
  const [search, setSearch] = useState(value)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const filtered = functions.filter((f) =>
    f.toLowerCase().includes(search.toLowerCase())
  )

  useEffect(() => {
    setSearch(value)
  }, [value])

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClick)
    return () => document.removeEventListener("mousedown", handleClick)
  }, [])

  return (
    <div ref={ref} className="relative">
      <Input
        value={search}
        onChange={(e) => {
          setSearch(e.target.value)
          onChange(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        placeholder="Function name..."
        className="font-mono text-sm"
      />
      {open && filtered.length > 0 && (
        <div className="absolute z-50 mt-1 w-full max-h-48 overflow-y-auto rounded-md border border-border bg-popover shadow-md">
          {filtered.map((fn) => (
            <button
              key={fn}
              type="button"
              className="w-full px-3 py-2 text-left text-sm font-mono hover:bg-accent truncate"
              onMouseDown={(e) => {
                e.preventDefault()
                onChange(fn)
                setSearch(fn)
                setOpen(false)
              }}
            >
              {fn}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

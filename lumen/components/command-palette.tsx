"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { useRouter } from "next/navigation"
import { functionsApi, type NovaFunction } from "@/lib/api"
import { cn } from "@/lib/utils"
import { Command, Search } from "lucide-react"

type CommandItem = {
  id: string
  title: string
  subtitle?: string
  keywords: string[]
  run: () => void
}

export function CommandPalette() {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [mounted, setMounted] = useState(false)
  const [query, setQuery] = useState("")
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [activeIndex, setActiveIndex] = useState(0)

  const staticItems: CommandItem[] = useMemo(
    () => [
      {
        id: "goto-dashboard",
        title: "Open Dashboard",
        subtitle: "View global metrics",
        keywords: ["dashboard", "home", "overview"],
        run: () => router.push("/dashboard"),
      },
      {
        id: "goto-functions",
        title: "Open Functions",
        subtitle: "Manage functions",
        keywords: ["functions", "lambda", "func"],
        run: () => router.push("/functions"),
      },
      {
        id: "create-function",
        title: "Create Function",
        subtitle: "Open the functions page and create dialog",
        keywords: ["create", "new", "function"],
        run: () => router.push("/functions?create=1"),
      },
      {
        id: "goto-gateway",
        title: "Open Gateway",
        subtitle: "Manage routes",
        keywords: ["gateway", "route", "http"],
        run: () => router.push("/gateway"),
      },
      {
        id: "create-route",
        title: "Create Gateway Route",
        subtitle: "Open the gateway page and create dialog",
        keywords: ["create", "route", "gateway"],
        run: () => router.push("/gateway?create=1"),
      },
      {
        id: "goto-events",
        title: "Open Events",
        subtitle: "Event bus",
        keywords: ["events", "topic", "subscription"],
        run: () => router.push("/events"),
      },
      {
        id: "goto-history",
        title: "Open History",
        subtitle: "Invocation history",
        keywords: ["history", "invocation", "logs"],
        run: () => router.push("/history"),
      },
      {
        id: "goto-tenancy",
        title: "Open Tenancy",
        subtitle: "Tenants and namespaces",
        keywords: ["tenant", "namespace", "scope"],
        run: () => router.push("/tenancy"),
      },
      {
        id: "goto-docs",
        title: "Open Docs",
        subtitle: "View installation and API docs",
        keywords: ["docs", "guide", "help"],
        run: () => router.push("/docs"),
      },
    ],
    [router]
  )

  useEffect(() => {
    setMounted(true)
    return () => setMounted(false)
  }, [])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setOpen((prev) => !prev)
        return
      }
      if (event.key === "Escape") {
        setOpen(false)
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  useEffect(() => {
    if (!open) return
    setQuery("")
    setActiveIndex(0)
    functionsApi
      .list(undefined, 200)
      .then((data) => setFunctions(data || []))
      .catch(() => setFunctions([]))
  }, [open])

  const items = useMemo(() => {
    const functionItems: CommandItem[] = functions.map((fn) => ({
      id: `fn-${fn.id}`,
      title: `Open Function: ${fn.name}`,
      subtitle: `${fn.runtime} · ${fn.memory_mb}MB`,
      keywords: [fn.name, fn.runtime, "function", "open", "invoke"],
      run: () => router.push(`/functions/${encodeURIComponent(fn.name)}`),
    }))

    const all = [...staticItems, ...functionItems]
    const next = query.trim().toLowerCase()
    if (!next) {
      return all.slice(0, 30)
    }
    return all
      .filter((item) => {
        const full = `${item.title} ${item.subtitle || ""} ${item.keywords.join(" ")}`.toLowerCase()
        return full.includes(next)
      })
      .slice(0, 30)
  }, [functions, query, router, staticItems])

  useEffect(() => {
    if (activeIndex >= items.length) {
      setActiveIndex(items.length > 0 ? items.length - 1 : 0)
    }
  }, [activeIndex, items.length])

  const execute = useCallback(
    (index: number) => {
      const target = items[index]
      if (!target) return
      target.run()
      setOpen(false)
    },
    [items]
  )

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowDown") {
        event.preventDefault()
        setActiveIndex((prev) => (prev + 1 >= items.length ? 0 : prev + 1))
      } else if (event.key === "ArrowUp") {
        event.preventDefault()
        setActiveIndex((prev) => (prev - 1 < 0 ? Math.max(0, items.length - 1) : prev - 1))
      } else if (event.key === "Enter") {
        event.preventDefault()
        execute(activeIndex)
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [activeIndex, execute, items.length, open])

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="hidden items-center gap-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-muted-foreground lg:inline-flex"
      >
        <Command className="h-3.5 w-3.5" />
        <span>K</span>
      </button>

      {mounted && open
        ? createPortal(
            <div
              className="fixed inset-0 z-[120] bg-black/35 p-4 backdrop-blur-[1px]"
              onClick={() => setOpen(false)}
            >
              <div
                className="mx-auto mt-20 w-full max-w-2xl rounded-xl border border-border bg-card shadow-2xl"
                onClick={(event) => event.stopPropagation()}
              >
                <div className="flex items-center gap-2 border-b border-border px-3 py-2">
                  <Search className="h-4 w-4 text-muted-foreground" />
                  <input
                    autoFocus
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="Type a command or function name..."
                    className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                  />
                </div>

                <div className="max-h-[420px] overflow-y-auto p-2">
                  {items.length === 0 ? (
                    <div className="rounded-md px-3 py-6 text-center text-sm text-muted-foreground">
                      No matching commands
                    </div>
                  ) : (
                    items.map((item, index) => (
                      <button
                        key={item.id}
                        type="button"
                        onMouseEnter={() => setActiveIndex(index)}
                        onClick={() => execute(index)}
                        className={cn(
                          "w-full rounded-md px-3 py-2 text-left transition-colors",
                          index === activeIndex ? "bg-accent text-accent-foreground" : "hover:bg-accent/60"
                        )}
                      >
                        <p className="text-sm font-medium">{item.title}</p>
                        {item.subtitle && (
                          <p className="mt-0.5 text-xs text-muted-foreground">{item.subtitle}</p>
                        )}
                      </button>
                    ))
                  )}
                </div>
              </div>
            </div>,
            document.body
          )
        : null}
    </>
  )
}

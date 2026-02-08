"use client"

import { useEffect, useRef, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { Bell, Search, User } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { functionsApi, type NovaFunction } from "@/lib/api"
import {
  FUNCTION_SEARCH_EVENT,
  type FunctionSearchDetail,
  dispatchFunctionSearch,
  readFunctionSearchFromLocation,
} from "@/lib/function-search"
import { ThemeToggle } from "./theme-toggle"
import { GlobalScopeSwitcher } from "./global-scope-switcher"

interface HeaderProps {
  title: string
  description?: string
}

const GLOBAL_SEARCH_LIMIT = 10

export function Header({ title, description }: HeaderProps) {
  const pathname = usePathname()
  const router = useRouter()
  const [query, setQuery] = useState("")
  const [searchResults, setSearchResults] = useState<NovaFunction[]>([])
  const [searchLoading, setSearchLoading] = useState(false)
  const [searchFocused, setSearchFocused] = useState(false)
  const [activeResultIndex, setActiveResultIndex] = useState(-1)
  const searchContainerRef = useRef<HTMLDivElement | null>(null)
  const searchRequestIDRef = useRef(0)
  const isFunctionsPage = pathname === "/functions"

  useEffect(() => {
    setQuery(readFunctionSearchFromLocation())
    setSearchResults([])
    setActiveResultIndex(-1)
  }, [pathname])

  useEffect(() => {
    const handleFunctionSearch = (event: Event) => {
      const custom = event as CustomEvent<FunctionSearchDetail>
      const next = custom.detail?.query ?? ""
      setQuery((prev) => (prev === next ? prev : next))
    }

    window.addEventListener(FUNCTION_SEARCH_EVENT, handleFunctionSearch)
    return () => {
      window.removeEventListener(FUNCTION_SEARCH_EVENT, handleFunctionSearch)
    }
  }, [])

  useEffect(() => {
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node
      if (searchContainerRef.current?.contains(target)) {
        return
      }
      setSearchFocused(false)
      setActiveResultIndex(-1)
    }

    document.addEventListener("mousedown", handlePointerDown)
    return () => {
      document.removeEventListener("mousedown", handlePointerDown)
    }
  }, [])

  useEffect(() => {
    const next = query.trim()
    if (!next) {
      setSearchLoading(false)
      setSearchResults([])
      setActiveResultIndex(-1)
      return
    }

    const requestID = searchRequestIDRef.current + 1
    searchRequestIDRef.current = requestID
    const timer = window.setTimeout(async () => {
      try {
        setSearchLoading(true)
        const funcs = await functionsApi.list(next, GLOBAL_SEARCH_LIMIT)
        if (searchRequestIDRef.current !== requestID) {
          return
        }
        setSearchResults(funcs)
        setActiveResultIndex((prev) => {
          if (funcs.length === 0) {
            return -1
          }
          if (prev < 0 || prev >= funcs.length) {
            return 0
          }
          return prev
        })
      } catch {
        if (searchRequestIDRef.current !== requestID) {
          return
        }
        setSearchResults([])
        setActiveResultIndex(-1)
      } finally {
        if (searchRequestIDRef.current === requestID) {
          setSearchLoading(false)
        }
      }
    }, 220)

    return () => clearTimeout(timer)
  }, [query])

  const openFunctionDetail = (name: string) => {
    setSearchFocused(false)
    setSearchResults([])
    setActiveResultIndex(-1)
    router.push(`/functions/${encodeURIComponent(name)}`)
  }

  useEffect(() => {
    if (!isFunctionsPage) {
      return
    }
    const timer = setTimeout(() => {
      const next = query.trim()
      const params = new URLSearchParams(window.location.search)
      const current = readFunctionSearchFromLocation()
      if (next === current) {
        return
      }

      if (next) {
        params.set("q", next)
      } else {
        params.delete("q")
      }
      const qs = params.toString()
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false })
      dispatchFunctionSearch(next)
    }, 300)

    return () => clearTimeout(timer)
  }, [isFunctionsPage, pathname, query, router])

  const handleSearchSubmit = () => {
    const next = query.trim()
    if (isFunctionsPage) {
      const params = new URLSearchParams(window.location.search)
      if (next) {
        params.set("q", next)
      } else {
        params.delete("q")
      }
      const qs = params.toString()
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false })
      dispatchFunctionSearch(next)
      return
    }
    router.push(next ? `/functions?q=${encodeURIComponent(next)}` : "/functions")
  }

  const showSearchDropdown = searchFocused && query.trim().length > 0

  return (
    <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-border bg-card/80 px-6 backdrop-blur-sm">
      <div>
        <h1 className="text-xl font-semibold text-foreground">{title}</h1>
        {description && (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>

      <div className="flex items-center gap-4">
        <div ref={searchContainerRef} className="relative hidden md:block">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            placeholder="Search functions..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onFocus={() => setSearchFocused(true)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" && showSearchDropdown && searchResults.length > 0) {
                e.preventDefault()
                setActiveResultIndex((prev) => (
                  prev >= searchResults.length - 1 ? 0 : prev + 1
                ))
                return
              }
              if (e.key === "ArrowUp" && showSearchDropdown && searchResults.length > 0) {
                e.preventDefault()
                setActiveResultIndex((prev) => (
                  prev <= 0 ? searchResults.length - 1 : prev - 1
                ))
                return
              }
              if (e.key === "Escape") {
                setSearchFocused(false)
                setActiveResultIndex(-1)
                return
              }
              if (e.key === "Enter") {
                e.preventDefault()
                if (
                  showSearchDropdown &&
                  activeResultIndex >= 0 &&
                  activeResultIndex < searchResults.length
                ) {
                  openFunctionDetail(searchResults[activeResultIndex].name)
                  return
                }
                handleSearchSubmit()
              }
            }}
            className="w-64 pl-9 bg-muted/50 border-0 focus-visible:ring-1"
          />

          {showSearchDropdown && (
            <div className="absolute left-0 right-0 top-[calc(100%+0.5rem)] z-50 overflow-hidden rounded-lg border border-border bg-popover shadow-lg">
              {searchLoading ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">Searching...</div>
              ) : searchResults.length === 0 ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">No matching functions</div>
              ) : (
                <ul className="max-h-80 overflow-y-auto py-1">
                  {searchResults.map((fn, index) => (
                    <li key={fn.id}>
                      <button
                        type="button"
                        onMouseEnter={() => setActiveResultIndex(index)}
                        onMouseDown={(event) => {
                          event.preventDefault()
                          openFunctionDetail(fn.name)
                        }}
                        className={cn(
                          "flex w-full flex-col items-start px-3 py-2 text-left transition-colors",
                          index === activeResultIndex
                            ? "bg-accent text-accent-foreground"
                            : "hover:bg-accent/60"
                        )}
                      >
                        <span className="text-sm font-medium">{fn.name}</span>
                        <span className="text-xs text-muted-foreground">
                          {fn.runtime} • {fn.memory_mb} MB
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>

        <Button variant="ghost" size="icon" className="relative">
          <Bell className="h-5 w-5 text-muted-foreground" />
          <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-primary" />
        </Button>

        <GlobalScopeSwitcher />

        <ThemeToggle />

        <Button variant="ghost" size="icon">
          <User className="h-5 w-5 text-muted-foreground" />
        </Button>
      </div>
    </header>
  )
}

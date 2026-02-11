"use client"

import Link from "next/link"
import { useEffect, useRef, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useTranslations } from "next-intl"
import { Bell, Search, User, BookOpenText, Server } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { functionsApi, notificationsApi, type NotificationEntry, type NovaFunction } from "@/lib/api"
import { useHealth, type HealthLevel } from "@/lib/hooks/use-health"
import {
  FUNCTION_SEARCH_EVENT,
  type FunctionSearchDetail,
  dispatchFunctionSearch,
  readFunctionSearchFromLocation,
} from "@/lib/function-search"
import { ThemeToggle } from "./theme-toggle"
import { GlobalScopeSwitcher } from "./global-scope-switcher"
import { CommandPalette } from "./command-palette"
import { LanguageSwitcher } from "./language-switcher"

interface HeaderProps {
  title: string
  description?: string
}

const GLOBAL_SEARCH_LIMIT = 10

export function Header({ title, description }: HeaderProps) {
  const pathname = usePathname()
  const router = useRouter()
  const health = useHealth(15000)
  const t = useTranslations("header")
  const tc = useTranslations("common")
  const [query, setQuery] = useState("")
  const [searchResults, setSearchResults] = useState<NovaFunction[]>([])
  const [searchLoading, setSearchLoading] = useState(false)
  const [searchFocused, setSearchFocused] = useState(false)
  const [activeResultIndex, setActiveResultIndex] = useState(-1)
  const [healthOpen, setHealthOpen] = useState(false)
  const [notificationOpen, setNotificationOpen] = useState(false)
  const [notifications, setNotifications] = useState<NotificationEntry[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [notificationLoading, setNotificationLoading] = useState(false)
  const searchContainerRef = useRef<HTMLDivElement | null>(null)
  const healthContainerRef = useRef<HTMLDivElement | null>(null)
  const notificationContainerRef = useRef<HTMLDivElement | null>(null)
  const searchRequestIDRef = useRef(0)
  const isFunctionsPage = pathname === "/functions"
  const isDocsPage = pathname.startsWith("/docs")

  const healthClassName: Record<HealthLevel, string> = {
    healthy: "bg-success",
    degraded: "bg-warning",
    down: "bg-destructive",
    unknown: "bg-muted-foreground",
  }

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

      if (healthContainerRef.current?.contains(target)) {
        return
      }
      setHealthOpen(false)

      if (notificationContainerRef.current?.contains(target)) {
        return
      }
      setNotificationOpen(false)
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

  useEffect(() => {
    let active = true

    const loadUnreadCount = async () => {
      try {
        const res = await notificationsApi.unreadCount()
        if (!active) {
          return
        }
        setUnreadCount(Math.max(0, Number(res.unread) || 0))
      } catch {
        if (!active) {
          return
        }
        setUnreadCount(0)
      }
    }

    loadUnreadCount()
    const timer = window.setInterval(loadUnreadCount, 15000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    if (!notificationOpen) {
      return
    }

    let cancelled = false
    const loadNotifications = async () => {
      setNotificationLoading(true)
      try {
        const items = await notificationsApi.list("all", 20)
        if (cancelled) {
          return
        }
        setNotifications(items)
      } catch {
        if (cancelled) {
          return
        }
        setNotifications([])
      } finally {
        if (!cancelled) {
          setNotificationLoading(false)
        }
      }
    }

    loadNotifications()
    return () => {
      cancelled = true
    }
  }, [notificationOpen])

  const handleNotificationClick = async (item: NotificationEntry) => {
    if (item.status === "unread") {
      try {
        await notificationsApi.markRead(item.id)
      } catch {
        // Keep optimistic UI fallback below.
      }
      setNotifications((prev) => prev.map((n) => (n.id === item.id ? { ...n, status: "read", read_at: new Date().toISOString() } : n)))
      setUnreadCount((prev) => Math.max(0, prev - 1))
    }

    if (item.function_name) {
      setNotificationOpen(false)
      router.push(`/functions/${encodeURIComponent(item.function_name)}`)
    }
  }

  const handleMarkAllRead = async () => {
    try {
      await notificationsApi.markAllRead()
    } catch {
      // Keep UI update best-effort.
    }
    setNotifications((prev) => prev.map((n) => (n.status === "unread" ? { ...n, status: "read", read_at: new Date().toISOString() } : n)))
    setUnreadCount(0)
  }

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
            placeholder={t("searchPlaceholder")}
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
                <div className="px-3 py-2 text-sm text-muted-foreground">{t("searching")}</div>
              ) : searchResults.length === 0 ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">{t("noMatchingFunctions")}</div>
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

        <div ref={healthContainerRef} className="relative">
          <Button
            variant="outline"
            size="icon-sm"
            className="relative hidden lg:inline-flex"
            onClick={() => setHealthOpen((prev) => !prev)}
            aria-label={t("systemHealth")}
            title={t("systemHealth")}
          >
            <Server className="h-4 w-4" />
            <span className={cn("absolute right-1.5 top-1.5 h-2 w-2 rounded-full", healthClassName[health.status])} />
          </Button>

          {healthOpen && (
            <div className="absolute right-0 top-[calc(100%+0.5rem)] z-50 w-72 rounded-lg border border-border bg-popover p-3 shadow-lg">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-popover-foreground">{t("systemHealth")}</p>
                <Button variant="ghost" size="sm" onClick={() => health.refresh()}>
                  {tc("refresh")}
                </Button>
              </div>

              <div className="mt-2 rounded-md border border-border bg-muted/20 p-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">{t("overall")}</span>
                  <span className="flex items-center gap-2 text-xs capitalize text-popover-foreground">
                    <span className={cn("h-2 w-2 rounded-full", healthClassName[health.status])} />
                    {health.status}
                  </span>
                </div>
              </div>

              <div className="mt-2 space-y-1">
                {Object.entries(health.components).length === 0 ? (
                  <p className="text-xs text-muted-foreground">{t("noComponentDetails")}</p>
                ) : (
                  Object.entries(health.components).map(([name, level]) => (
                    <div key={name} className="flex items-center justify-between rounded border border-border px-2 py-1">
                      <span className="text-xs text-popover-foreground">{name}</span>
                      <span className="flex items-center gap-2 text-xs capitalize text-muted-foreground">
                        <span className={cn("h-2 w-2 rounded-full", healthClassName[level])} />
                        {level}
                      </span>
                    </div>
                  ))
                )}
              </div>

              {health.error && (
                <p className="mt-2 text-xs text-destructive">{health.error}</p>
              )}
              {health.updatedAt && (
                <p className="mt-2 text-[11px] text-muted-foreground">
                  {t("updated")} {health.updatedAt.toLocaleTimeString()}
                </p>
              )}
            </div>
          )}
        </div>

        <CommandPalette />

        <div ref={notificationContainerRef} className="relative">
          <Button
            variant="ghost"
            size="icon"
            className="relative"
            aria-label={t("notifications")}
            title={t("notifications")}
            onClick={() => setNotificationOpen((prev) => !prev)}
          >
            <Bell className="h-5 w-5 text-muted-foreground" />
            {unreadCount > 0 && (
              <span className="absolute right-1 top-1 inline-flex min-h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-semibold text-primary-foreground">
                {unreadCount > 99 ? "99+" : unreadCount}
              </span>
            )}
          </Button>

          {notificationOpen && (
            <div className="absolute right-0 top-[calc(100%+0.5rem)] z-50 w-96 overflow-hidden rounded-lg border border-border bg-popover shadow-lg">
              <div className="flex items-center justify-between border-b border-border px-3 py-2">
                <p className="text-sm font-medium text-popover-foreground">{t("notifications")}</p>
                <Button variant="ghost" size="sm" onClick={handleMarkAllRead}>
                  {t("markAllRead")}
                </Button>
              </div>

              <div className="max-h-80 overflow-y-auto">
                {notificationLoading ? (
                  <p className="px-3 py-3 text-sm text-muted-foreground">{t("loadingNotifications")}</p>
                ) : notifications.length === 0 ? (
                  <p className="px-3 py-3 text-sm text-muted-foreground">{t("noNotifications")}</p>
                ) : (
                  <ul className="divide-y divide-border">
                    {notifications.map((item) => (
                      <li key={item.id}>
                        <button
                          type="button"
                          className={cn(
                            "w-full px-3 py-2 text-left transition-colors hover:bg-accent/60",
                            item.status === "unread" ? "bg-accent/20" : ""
                          )}
                          onClick={() => handleNotificationClick(item)}
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium text-popover-foreground">{item.title}</p>
                              <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{item.message}</p>
                              <p className="mt-1 text-[11px] text-muted-foreground">
                                {new Date(item.created_at).toLocaleString()}
                              </p>
                            </div>
                            {item.status === "unread" && (
                              <span className="mt-1 h-2.5 w-2.5 rounded-full bg-primary" />
                            )}
                          </div>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          )}
        </div>

        <Button asChild variant={isDocsPage ? "secondary" : "ghost"} size="icon">
          <Link href="/docs" aria-label={t("openDocs")} title={t("openDocs")}>
            <BookOpenText className="h-5 w-5 text-muted-foreground" />
          </Link>
        </Button>

        <GlobalScopeSwitcher />

        <LanguageSwitcher />

        <ThemeToggle />

        <Button variant="ghost" size="icon">
          <User className="h-5 w-5 text-muted-foreground" />
        </Button>
      </div>
    </header>
  )
}

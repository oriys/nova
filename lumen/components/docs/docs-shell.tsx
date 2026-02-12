"use client"

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { type KeyboardEvent, useEffect, useMemo, useRef, useState } from "react"
import { useTranslations } from "next-intl"
import { Search } from "lucide-react"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

export type DocsSection = "introduction" | "architecture" | "installation" | "api" | "cli" | "mcp"

interface TocItem {
  id: string
  label: string
}

export interface DocsNavItem {
  id?: DocsSection | string
  label: string
  href: string
  children?: Array<{
    label: string
    href: string
  }>
}

export interface DocsNavGroup {
  title: string
  items: DocsNavItem[]
}

interface DocsSearchItem {
  label: string
  href: string
  context: string
  keywords: string
}

interface DocsShellProps {
  current: DocsSection
  activeHref?: string
  title: string
  description?: string
  toc: TocItem[]
  navGroups?: DocsNavGroup[]
  children: React.ReactNode
}

function isNavItemActive(item: DocsNavItem, activeHref?: string, current?: DocsSection): boolean {
  if (activeHref) {
    if (activeHref === item.href) return true
    if (item.href === "/docs" || item.href === "/docs/api") return false
    return activeHref.startsWith(`${item.href}/`)
  }
  return item.id ? current === item.id : false
}

function isNavChildActive(href: string, activeHref?: string): boolean {
  if (!activeHref) return false
  return activeHref === href || activeHref.startsWith(`${href}/`)
}

function normalizeSearchText(value: string): string {
  return value.trim().toLowerCase()
}

function buildSearchItems(
  navGroups: DocsNavGroup[],
  toc: TocItem[],
  pathname: string,
  onThisPageLabel: string
): DocsSearchItem[] {
  const baseItems: DocsSearchItem[] = []

  for (const group of navGroups) {
    for (const item of group.items) {
      baseItems.push({
        label: item.label,
        href: item.href,
        context: group.title,
        keywords: `${group.title} ${item.label} ${item.href}`,
      })

      for (const child of item.children ?? []) {
        baseItems.push({
          label: child.label,
          href: child.href,
          context: `${group.title} / ${item.label}`,
          keywords: `${group.title} ${item.label} ${child.label} ${child.href}`,
        })
      }
    }
  }

  for (const item of toc) {
    baseItems.push({
      label: item.label,
      href: `${pathname}#${item.id}`,
      context: onThisPageLabel,
      keywords: `section ${item.label} ${item.id}`,
    })
  }

  const deduped = new Map<string, DocsSearchItem>()
  for (const item of baseItems) {
    const key = `${item.href}::${item.label}`
    if (!deduped.has(key)) {
      deduped.set(key, item)
    }
  }

  return Array.from(deduped.values())
}

function scoreSearchItem(item: DocsSearchItem, query: string): number {
  const label = item.label.toLowerCase()
  const keywords = item.keywords.toLowerCase()

  if (label === query) return 500
  if (label.startsWith(query)) return 400
  if (label.includes(query)) return 300

  const parts = query.split(/\s+/).filter(Boolean)
  if (parts.length > 1 && parts.every((part) => keywords.includes(part))) {
    return 250
  }

  if (keywords.includes(query)) return 200
  return 0
}

export function DocsShell({ current, activeHref, title, description, toc, navGroups, children }: DocsShellProps) {
  const t = useTranslations("docsShell")
  const [activeTocId, setActiveTocId] = useState<string>(toc[0]?.id ?? "")
  const [searchValue, setSearchValue] = useState("")
  const [isSearchOpen, setIsSearchOpen] = useState(false)
  const [activeSearchIndex, setActiveSearchIndex] = useState(0)
  const searchContainerRef = useRef<HTMLDivElement>(null)
  const pathname = usePathname() || "/docs"
  const router = useRouter()
  const defaultNavGroups = useMemo<DocsNavGroup[]>(
    () => [
      {
        title: t("nav.guides"),
        items: [
          { id: "introduction", label: t("nav.introduction"), href: "/docs" },
          { id: "architecture", label: t("nav.architecture"), href: "/docs/architecture" },
          { id: "installation", label: t("nav.installation"), href: "/docs/installation" },
        ],
      },
      {
        title: t("nav.reference"),
        items: [
          { id: "api", label: t("nav.apiOverview"), href: "/docs/api" },
          { id: "api", label: t("nav.functionsApi"), href: "/docs/api/functions" },
          { id: "api", label: t("nav.workflowsApi"), href: "/docs/api/workflows" },
          { id: "api", label: t("nav.eventsApi"), href: "/docs/api/events" },
          { id: "api", label: t("nav.operationsApi"), href: "/docs/api/operations" },
          { id: "cli", label: t("nav.orbitCli"), href: "/docs/cli" },
          { id: "mcp", label: t("nav.atlasMcpServer"), href: "/docs/mcp-server" },
        ],
      },
    ],
    [t]
  )
  const resolvedNavGroups = navGroups ?? defaultNavGroups

  const normalizedSearch = normalizeSearchText(searchValue)

  const searchItems = useMemo(
    () => buildSearchItems(resolvedNavGroups, toc, pathname, t("onThisPage")),
    [resolvedNavGroups, toc, pathname, t]
  )

  const searchResults = useMemo(() => {
    if (!normalizedSearch) {
      return []
    }

    return searchItems
      .map((item) => ({
        item,
        score: scoreSearchItem(item, normalizedSearch),
      }))
      .filter((entry) => entry.score > 0)
      .sort((left, right) => {
        if (left.score !== right.score) {
          return right.score - left.score
        }
        return left.item.label.localeCompare(right.item.label)
      })
      .slice(0, 8)
      .map((entry) => entry.item)
  }, [normalizedSearch, searchItems])

  const showSearchMenu = isSearchOpen && normalizedSearch.length > 0

  const jumpToSearchResult = (href: string) => {
    setSearchValue("")
    setIsSearchOpen(false)
    setActiveSearchIndex(0)
    router.push(href)
  }

  const handleSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      setIsSearchOpen(false)
      return
    }

    if (!showSearchMenu || searchResults.length === 0) {
      return
    }

    if (event.key === "ArrowDown") {
      event.preventDefault()
      setActiveSearchIndex((prev) => (prev + 1) % searchResults.length)
      return
    }

    if (event.key === "ArrowUp") {
      event.preventDefault()
      setActiveSearchIndex((prev) => (prev - 1 + searchResults.length) % searchResults.length)
      return
    }

    if (event.key === "Enter") {
      event.preventDefault()
      const target = searchResults[activeSearchIndex] ?? searchResults[0]
      if (target) {
        jumpToSearchResult(target.href)
      }
    }
  }

  useEffect(() => {
    setActiveTocId(toc[0]?.id ?? "")
  }, [toc])

  useEffect(() => {
    setActiveSearchIndex(0)
  }, [normalizedSearch])

  useEffect(() => {
    if (activeSearchIndex < searchResults.length) {
      return
    }
    setActiveSearchIndex(0)
  }, [activeSearchIndex, searchResults.length])

  useEffect(() => {
    if (!isSearchOpen) {
      return
    }

    const onMouseDown = (event: MouseEvent) => {
      if (!(event.target instanceof Node)) {
        return
      }
      if (!searchContainerRef.current?.contains(event.target)) {
        setIsSearchOpen(false)
      }
    }

    document.addEventListener("mousedown", onMouseDown)
    return () => {
      document.removeEventListener("mousedown", onMouseDown)
    }
  }, [isSearchOpen])

  useEffect(() => {
    setIsSearchOpen(false)
    setSearchValue("")
    setActiveSearchIndex(0)
  }, [pathname])

  useEffect(() => {
    if (typeof window === "undefined" || toc.length === 0) {
      return
    }

    const sections = toc
      .map((item) => document.getElementById(item.id))
      .filter((section): section is HTMLElement => section instanceof HTMLElement)

    if (sections.length === 0) {
      return
    }

    let ticking = false
    const offset = 128

    const syncActiveSection = () => {
      const currentY = window.scrollY + offset
      let nextActive = sections[0].id

      for (const section of sections) {
        if (section.offsetTop <= currentY) {
          nextActive = section.id
        } else {
          break
        }
      }

      setActiveTocId((prev) => (prev === nextActive ? prev : nextActive))
      ticking = false
    }

    const onScroll = () => {
      if (!ticking) {
        ticking = true
        window.requestAnimationFrame(syncActiveSection)
      }
    }

    const syncFromHash = () => {
      const hash = decodeURIComponent(window.location.hash.replace(/^#/, ""))
      if (hash && toc.some((item) => item.id === hash)) {
        setActiveTocId(hash)
      }
    }

    syncFromHash()
    syncActiveSection()

    window.addEventListener("scroll", onScroll, { passive: true })
    window.addEventListener("resize", onScroll)
    window.addEventListener("hashchange", syncFromHash)

    return () => {
      window.removeEventListener("scroll", onScroll)
      window.removeEventListener("resize", onScroll)
      window.removeEventListener("hashchange", syncFromHash)
    }
  }, [toc])

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-40 bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-16 w-full max-w-[1600px] items-center justify-between gap-4 px-4 md:px-6">
          <div className="flex min-w-0 items-center gap-6">
            <Link href="/docs" className="text-base font-semibold tracking-tight">
              {t("title")}
            </Link>
          </div>

          <div ref={searchContainerRef} className="relative hidden lg:block">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t("search.placeholder")}
              className="h-9 w-72 border-border bg-background pl-9"
              value={searchValue}
              onChange={(event) => {
                setSearchValue(event.currentTarget.value)
                setIsSearchOpen(true)
              }}
              onFocus={() => setIsSearchOpen(true)}
              onKeyDown={handleSearchKeyDown}
              aria-label={t("search.ariaLabel")}
              aria-expanded={showSearchMenu}
              aria-controls="docs-search-results"
            />
            {showSearchMenu && (
              <div className="absolute right-0 top-11 z-50 w-[26rem] overflow-hidden rounded-md border border-border bg-background shadow-lg">
                {searchResults.length > 0 ? (
                  <ul id="docs-search-results" role="listbox" className="max-h-80 overflow-y-auto py-1">
                    {searchResults.map((result, index) => (
                      <li key={`${result.href}-${result.label}`}>
                        <button
                          type="button"
                          className={cn(
                            "w-full px-3 py-2 text-left transition-colors",
                            index === activeSearchIndex ? "bg-muted" : "hover:bg-muted/60"
                          )}
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => jumpToSearchResult(result.href)}
                        >
                          <p className="truncate text-sm font-medium text-foreground">{result.label}</p>
                          <p className="truncate text-xs text-muted-foreground">{result.context}</p>
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="px-3 py-2 text-sm text-muted-foreground">
                    {t("search.noMatches", { query: searchValue.trim() })}
                  </p>
                )}
              </div>
            )}
          </div>
        </div>
        <div className="h-px w-full bg-gradient-to-r from-border/15 via-border to-border/15" />
      </header>

      <div className="mx-auto grid w-full max-w-[1600px] grid-cols-1 xl:grid-cols-[260px_minmax(0,1fr)_260px]">
        <aside className="relative hidden h-[calc(100vh-4rem)] px-6 py-8 xl:sticky xl:top-16 xl:block xl:self-start">
          <p className="mb-3 text-sm text-muted-foreground">{t("documentation")}</p>
          <div className="space-y-5">
            {resolvedNavGroups.map((group) => (
              <div key={group.title} className="space-y-1">
                <p className="px-3 text-xs uppercase tracking-wide text-muted-foreground/80">{group.title}</p>
                {group.items.map((item) => (
                  <div key={`${group.title}-${item.href}`}>
                    <Link
                      href={item.href}
                      className={cn(
                        "block rounded-md px-3 py-2 text-sm transition-colors",
                        isNavItemActive(item, activeHref, current)
                          ? "bg-muted font-medium text-foreground"
                          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                      )}
                    >
                      {item.label}
                    </Link>
                    {item.children && item.children.length > 0 && (
                      <div className="mt-1 space-y-1 pl-4">
                        {item.children.map((child) => (
                          <Link
                            key={`${item.href}-${child.href}`}
                            href={child.href}
                            className={cn(
                              "block rounded-md px-3 py-1.5 text-xs transition-colors",
                              isNavChildActive(child.href, activeHref)
                                ? "bg-muted font-medium text-foreground"
                                : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                            )}
                          >
                            {child.label}
                          </Link>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ))}
          </div>
          <div className="pointer-events-none absolute inset-y-0 right-0 w-px bg-gradient-to-b from-border/15 via-border to-border/15" />
        </aside>

        <main className="min-w-0 px-5 py-8 sm:px-8 lg:px-14 xl:px-16">
          <article className="mx-auto max-w-3xl">
            <h1 className="text-4xl font-semibold tracking-tight">{title}</h1>
            {description && (
              <p className="mt-4 text-lg leading-8 text-muted-foreground">{description}</p>
            )}
            <div className="mt-8 space-y-8">{children}</div>
          </article>
        </main>

        <aside className="relative hidden h-[calc(100vh-4rem)] px-6 py-8 xl:sticky xl:top-16 xl:block xl:self-start">
          <div className="pointer-events-none absolute inset-y-0 left-0 w-px bg-gradient-to-b from-border/15 via-border to-border/15" />
          <p className="mb-3 text-sm text-muted-foreground">{t("onThisPage")}</p>
          <div className="space-y-2">
            {toc.map((item) => (
              <a
                key={item.id}
                href={`#${item.id}`}
                className={cn(
                  "block text-sm transition-colors",
                  activeTocId === item.id
                    ? "font-medium text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {item.label}
              </a>
            ))}
          </div>
        </aside>
      </div>
    </div>
  )
}

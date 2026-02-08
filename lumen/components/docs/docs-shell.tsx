"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
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

interface DocsShellProps {
  current: DocsSection
  activeHref?: string
  title: string
  description?: string
  toc: TocItem[]
  navGroups?: DocsNavGroup[]
  children: React.ReactNode
}

const defaultDocsNavGroups: DocsNavGroup[] = [
  {
    title: "Guides",
    items: [
      { id: "introduction", label: "Introduction", href: "/docs" },
      { id: "architecture", label: "Architecture", href: "/docs/architecture" },
      { id: "installation", label: "Installation", href: "/docs/installation" },
    ],
  },
  {
    title: "Reference",
    items: [
      { id: "api", label: "API Overview", href: "/docs/api" },
      { id: "api", label: "Functions API", href: "/docs/api/functions" },
      { id: "api", label: "Workflows API", href: "/docs/api/workflows" },
      { id: "api", label: "Events API", href: "/docs/api/events" },
      { id: "api", label: "Operations API", href: "/docs/api/operations" },
      { id: "cli", label: "Orbit CLI", href: "/docs/cli" },
      { id: "mcp", label: "Atlas MCP Server", href: "/docs/mcp-server" },
    ],
  },
]

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

export function DocsShell({ current, activeHref, title, description, toc, navGroups, children }: DocsShellProps) {
  const [activeTocId, setActiveTocId] = useState<string>(toc[0]?.id ?? "")
  const resolvedNavGroups = navGroups ?? defaultDocsNavGroups

  useEffect(() => {
    setActiveTocId(toc[0]?.id ?? "")
  }, [toc])

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
              Nova Docs
            </Link>
          </div>

          <div className="relative hidden lg:block">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search documentation..."
              className="h-9 w-72 border-border bg-background pl-9"
              readOnly
            />
          </div>
        </div>
        <div className="h-px w-full bg-gradient-to-r from-border/15 via-border to-border/15" />
      </header>

      <div className="mx-auto grid w-full max-w-[1600px] grid-cols-1 xl:grid-cols-[260px_minmax(0,1fr)_260px]">
        <aside className="relative hidden h-[calc(100vh-4rem)] px-6 py-8 xl:sticky xl:top-16 xl:block xl:self-start">
          <p className="mb-3 text-sm text-muted-foreground">Documentation</p>
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
          <p className="mb-3 text-sm text-muted-foreground">On This Page</p>
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

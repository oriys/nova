"use client"

import { useEffect, useState, useCallback } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { useTranslations } from "next-intl"
import { cn } from "@/lib/utils"
import { useSidebar } from "./sidebar-context"
import { getTenantScope } from "@/lib/tenant-scope"
import { tenantsApi, type MenuPermission } from "@/lib/api"
import {
  LayoutDashboard,
  Code2,
  Play,
  Settings,
  History,
  GitBranch,
  RadioTower,
  Building2,
  Network,

  ShieldCheck,
  HardDrive,
  Layers,
  Camera,
  Zap,
  Server,
  Activity,
} from "lucide-react"

type NavKey = "dashboard" | "functions" | "events" | "workflows" | "tenancy" | "invocations" | "runtimes" | "gateway" | "triggers" | "volumes" | "layers" | "cluster" | "snapshots" | "accessControl" | "alerting" | "settings" | "docs"

// Extra paths that should highlight a merged nav item
const extraActivePaths: Record<string, string[]> = {
  "/history": ["/async-invocations"],
  "/rbac": ["/audit-logs"],
  "/alerts": ["/notifications"],
  "/configurations": ["/secrets", "/api-keys"],
}

const navigation: { key: NavKey; href: string; icon: typeof LayoutDashboard }[] = [
  { key: "dashboard", href: "/dashboard", icon: LayoutDashboard },
  { key: "functions", href: "/functions", icon: Code2 },
  { key: "gateway", href: "/gateway", icon: Network },
  { key: "events", href: "/events", icon: RadioTower },
  { key: "triggers", href: "/triggers", icon: Zap },
  { key: "workflows", href: "/workflows", icon: GitBranch },
  { key: "tenancy", href: "/tenancy", icon: Building2 },
  { key: "invocations", href: "/history", icon: History },
  { key: "runtimes", href: "/runtimes", icon: Play },
  { key: "layers", href: "/layers", icon: Layers },
  { key: "volumes", href: "/volumes", icon: HardDrive },
  { key: "cluster", href: "/cluster", icon: Server },
  { key: "snapshots", href: "/snapshots", icon: Camera },

  { key: "accessControl", href: "/rbac", icon: ShieldCheck },
  { key: "alerting", href: "/alerts", icon: Activity },
  { key: "settings", href: "/configurations", icon: Settings },
]

function LumenLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      <path
        d="M10 6V22H22"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="22" cy="10" r="4" fill="currentColor" opacity="0.9" />
    </svg>
  )
}

function useMenuPermissions(): Set<string> | null {
  const [enabledKeys, setEnabledKeys] = useState<Set<string> | null>(null)

  const fetchPermissions = useCallback(() => {
    const { tenantId } = getTenantScope()
    tenantsApi
      .listMenuPermissions(tenantId)
      .then((perms: MenuPermission[]) => {
        const keys = new Set<string>()
        for (const p of perms) {
          if (p.enabled) keys.add(p.menu_key)
        }
        setEnabledKeys(keys)
      })
      .catch(() => {
        // On error, fall back to a safe minimal set (dashboard only)
        setEnabledKeys(new Set(["dashboard"]))
      })
  }, [])

  useEffect(() => {
    fetchPermissions()
    const handler = () => fetchPermissions()
    window.addEventListener("nova:tenant-scope-changed", handler)
    return () => window.removeEventListener("nova:tenant-scope-changed", handler)
  }, [fetchPermissions])

  return enabledKeys
}

export function Sidebar() {
  const pathname = usePathname()
  const { collapsed, toggle } = useSidebar()
  const t = useTranslations("nav")
  const enabledMenuKeys = useMenuPermissions()

  // Show empty menu while loading permissions to prevent flash
  const visibleNavigation =
    enabledMenuKeys === null
      ? []
      : navigation.filter((item) => enabledMenuKeys.has(item.key))

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 z-40 flex h-screen flex-col border-r border-border bg-sidebar transition-all duration-300",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <button
        onClick={toggle}
        className={cn(
          "flex h-16 items-center border-b border-border transition-colors hover:bg-muted/50",
          collapsed ? "justify-center px-0" : "gap-2.5 px-6"
        )}
      >
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-foreground">
          <LumenLogo className="h-5 w-5 text-background" />
        </div>
        {!collapsed && (
          <span className="text-lg font-semibold tracking-tight text-foreground">
            {t("brandName")}
          </span>
        )}
      </button>

      <nav className={cn("flex-1 overflow-y-auto space-y-1", collapsed ? "px-2 py-4" : "px-3 py-4")}>
        {visibleNavigation.map((item) => {
          const label = t(item.key)
          const isActive =
            pathname === item.href ||
            (item.href !== "/dashboard" && pathname.startsWith(item.href)) ||
            (extraActivePaths[item.href]?.some(p => pathname === p || pathname.startsWith(p + "/")))

          return (
            <Link
              key={item.key}
              href={item.href}
              title={collapsed ? label : undefined}
              className={cn(
                "flex items-center rounded-lg text-sm font-medium transition-colors",
                collapsed ? "justify-center px-0 py-2.5" : "gap-3 px-3 py-2.5",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              <item.icon
                className={cn("h-5 w-5 flex-shrink-0", isActive && "text-primary")}
              />
              {!collapsed && label}
            </Link>
          )
        })}
      </nav>
    </aside>
  )
}

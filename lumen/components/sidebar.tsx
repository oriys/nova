"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { useTranslations } from "next-intl"
import { cn } from "@/lib/utils"
import { useSidebar } from "./sidebar-context"
import {
  LayoutDashboard,
  Code2,
  Play,
  Settings,
  History,
  GitBranch,
  KeyRound,
  Lock,
  RadioTower,
  Building2,
  Clock3,
} from "lucide-react"

type NavKey = "dashboard" | "functions" | "events" | "workflows" | "tenancy" | "asyncJobs" | "history" | "runtimes" | "configurations" | "secrets" | "apiKeys"

const navigation: { key: NavKey; href: string; icon: typeof LayoutDashboard }[] = [
  { key: "dashboard", href: "/dashboard", icon: LayoutDashboard },
  { key: "functions", href: "/functions", icon: Code2 },
  { key: "events", href: "/events", icon: RadioTower },
  { key: "workflows", href: "/workflows", icon: GitBranch },
  { key: "tenancy", href: "/tenancy", icon: Building2 },
  { key: "asyncJobs", href: "/async-invocations", icon: Clock3 },
  { key: "history", href: "/history", icon: History },
  { key: "runtimes", href: "/runtimes", icon: Play },
  { key: "configurations", href: "/configurations", icon: Settings },
  { key: "secrets", href: "/secrets", icon: Lock },
  { key: "apiKeys", href: "/api-keys", icon: KeyRound },
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

export function Sidebar() {
  const pathname = usePathname()
  const { collapsed, toggle } = useSidebar()
  const t = useTranslations("nav")

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
            Lumen
          </span>
        )}
      </button>

      <nav className={cn("flex-1 space-y-1", collapsed ? "px-2 py-4" : "px-3 py-4")}>
        {navigation.map((item) => {
          const label = t(item.key)
          const isActive =
            pathname === item.href ||
            (item.href !== "/dashboard" && pathname.startsWith(item.href))

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

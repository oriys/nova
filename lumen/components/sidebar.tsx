"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"
import { useSidebar } from "./sidebar-context"
import {
  LayoutDashboard,
  Code2,
  Play,
  Settings,
  ScrollText,
  History,
} from "lucide-react"

const navigation = [
  { name: "Dashboard", href: "/", icon: LayoutDashboard },
  { name: "Functions", href: "/functions", icon: Code2 },
  { name: "Runtimes", href: "/runtimes", icon: Play },
  { name: "Configurations", href: "/configurations", icon: Settings },
  { name: "Logs", href: "/logs", icon: ScrollText },
  { name: "History", href: "/history", icon: History },
]

// Lumen logo - minimalist "L" with a subtle glow accent
function LumenLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      {/* L shape */}
      <path
        d="M10 6V22H22"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {/* Glow dot accent */}
      <circle
        cx="22"
        cy="10"
        r="4"
        fill="currentColor"
        opacity="0.9"
      />
    </svg>
  )
}

export function Sidebar() {
  const pathname = usePathname()
  const { collapsed, toggle } = useSidebar()

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
          "flex h-16 items-center border-b border-border hover:bg-muted/50 transition-colors",
          collapsed ? "justify-center px-0" : "gap-2.5 px-6"
        )}
      >
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-foreground flex-shrink-0">
          <LumenLogo className="h-5 w-5 text-background" />
        </div>
        {!collapsed && (
          <span className="text-lg font-semibold tracking-tight text-foreground">
            Lumen
          </span>
        )}
      </button>

      <nav className={cn("flex-1 space-y-1 py-4", collapsed ? "px-2" : "px-3")}>
        {navigation.map((item) => {
          const isActive =
            pathname === item.href ||
            (item.href !== "/" && pathname.startsWith(item.href))

          return (
            <Link
              key={item.name}
              href={item.href}
              title={collapsed ? item.name : undefined}
              className={cn(
                "flex items-center rounded-lg text-sm font-medium transition-colors",
                collapsed
                  ? "justify-center px-0 py-2.5"
                  : "gap-3 px-3 py-2.5",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              <item.icon
                className={cn("h-5 w-5 flex-shrink-0", isActive && "text-primary")}
              />
              {!collapsed && item.name}
            </Link>
          )
        })}
      </nav>
    </aside>
  )
}

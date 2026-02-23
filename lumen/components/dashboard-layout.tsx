"use client"

import React from "react"

import { Sidebar } from "./sidebar"
import { useSidebar } from "./sidebar-context"
import { cn } from "@/lib/utils"

interface DashboardLayoutProps {
  children: React.ReactNode
}

export function DashboardLayout({ children }: DashboardLayoutProps) {
  const { collapsed } = useSidebar()

  return (
    <div className="min-h-screen bg-background">
      <Sidebar />
      <main
        className={cn(
          "min-h-screen transition-all duration-300",
          collapsed ? "ml-0 md:ml-16" : "ml-0 md:ml-64"
        )}
      >
        {children}
      </main>
    </div>
  )
}

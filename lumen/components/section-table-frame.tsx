"use client"

import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

interface SectionTableFrameProps {
  children: ReactNode
  className?: string
}

export function SectionTableFrame({
  children,
  className,
}: SectionTableFrameProps) {
  return (
    <div className={cn("rounded-lg border border-border bg-card overflow-hidden", className)}>
      {children}
    </div>
  )
}

"use client"

import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

interface SectionEmptyHintProps {
  children: ReactNode
  className?: string
}

export function SectionEmptyHint({
  children,
  className,
}: SectionEmptyHintProps) {
  return (
    <div className={cn("rounded-lg border border-dashed border-border/80 bg-muted/20 px-4 py-5", className)}>
      <p className="text-sm text-muted-foreground">{children}</p>
    </div>
  )
}

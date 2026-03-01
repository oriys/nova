"use client"

import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

interface SectionHeaderProps {
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  className?: string
  titleClassName?: string
  descriptionClassName?: string
}

export function SectionHeader({
  title,
  description,
  action,
  className,
  titleClassName,
  descriptionClassName,
}: SectionHeaderProps) {
  return (
    <div className={cn("flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between", className)}>
      <div className="space-y-0.5">
        <h3 className={cn("text-sm font-medium text-foreground", titleClassName)}>{title}</h3>
        {description ? (
          <p className={cn("text-xs text-muted-foreground", descriptionClassName)}>{description}</p>
        ) : null}
      </div>
      {action ? <div className="shrink-0 self-start">{action}</div> : null}
    </div>
  )
}

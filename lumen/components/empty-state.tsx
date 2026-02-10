"use client"

import Link from "next/link"
import { Button } from "@/components/ui/button"
import { CircleDashed } from "lucide-react"
import type { LucideIcon } from "lucide-react"

interface EmptyStateAction {
  label: string
  href?: string
  onClick?: () => void
}

interface EmptyStateProps {
  title: string
  description?: string
  icon?: LucideIcon
  primaryAction?: EmptyStateAction
  secondaryAction?: EmptyStateAction
  compact?: boolean
}

function ActionButton({
  action,
  variant,
}: {
  action: EmptyStateAction
  variant: "default" | "outline"
}) {
  if (action.href) {
    return (
      <Button asChild variant={variant} size="sm">
        <Link href={action.href}>{action.label}</Link>
      </Button>
    )
  }
  return (
    <Button variant={variant} size="sm" onClick={action.onClick}>
      {action.label}
    </Button>
  )
}

export function EmptyState({
  title,
  description,
  icon: Icon = CircleDashed,
  primaryAction,
  secondaryAction,
  compact = false,
}: EmptyStateProps) {
  return (
    <div
      className={`rounded-xl border border-border bg-card text-center ${
        compact ? "p-6" : "p-12"
      }`}
    >
      <Icon className="mx-auto h-8 w-8 text-muted-foreground" />
      <p className="mt-3 text-base font-medium text-foreground">{title}</p>
      {description && (
        <p className="mx-auto mt-1 max-w-xl text-sm text-muted-foreground">
          {description}
        </p>
      )}

      {(primaryAction || secondaryAction) && (
        <div className="mt-4 flex items-center justify-center gap-2">
          {primaryAction && <ActionButton action={primaryAction} variant="default" />}
          {secondaryAction && <ActionButton action={secondaryAction} variant="outline" />}
        </div>
      )}
    </div>
  )
}

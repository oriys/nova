"use client"

import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { resolveUserError } from "@/lib/error-map"

interface ErrorBannerProps {
  error: unknown
  title?: string
  onRetry?: () => void
}

export function ErrorBanner({ error, title, onRetry }: ErrorBannerProps) {
  const t = useTranslations("errorBanner")
  const resolved = resolveUserError(error, title || t("defaultTitle"))

  return (
    <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
      <p className="font-medium">{resolved.title}</p>
      <p className="mt-1 text-sm">{resolved.message}</p>
      {resolved.hint && (
        <p className="mt-2 text-sm text-destructive/90">{t("hint")} {resolved.hint}</p>
      )}
      {onRetry && (
        <div className="mt-3">
          <Button variant="outline" size="sm" onClick={onRetry}>
            {t("retry")}
          </Button>
        </div>
      )}
    </div>
  )
}

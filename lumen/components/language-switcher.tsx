"use client"

import { useLocale, useTranslations } from "next-intl"
import { useRouter } from "next/navigation"
import { useTransition } from "react"
import { Globe } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  locales,
  localeNames,
  type Locale,
} from "@/i18n/request"

export function LanguageSwitcher() {
  const t = useTranslations("common")
  const locale = useLocale()
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  function onSelectChange(nextLocale: Locale) {
    document.cookie = `NEXT_LOCALE=${nextLocale};path=/;max-age=31536000`
    startTransition(() => {
      router.refresh()
    })
  }

  return (
    <div className="relative group">
      <Button
        variant="ghost"
        size="icon"
        className="h-9 w-9"
        disabled={isPending}
        aria-label={t("language")}
      >
        <Globe className="h-4 w-4" />
      </Button>
      <div className="absolute right-0 top-full z-50 mt-1 hidden min-w-[140px] rounded-md border border-border bg-popover p-1 shadow-md group-hover:block">
        {locales.map((l) => (
          <button
            key={l}
            onClick={() => onSelectChange(l)}
            className={`flex w-full items-center rounded-sm px-3 py-1.5 text-sm transition-colors hover:bg-accent hover:text-accent-foreground ${
              locale === l ? "bg-accent text-accent-foreground font-medium" : ""
            }`}
          >
            {localeNames[l]}
          </button>
        ))}
      </div>
    </div>
  )
}

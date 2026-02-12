"use client"

import { useEffect, useRef, useState } from "react"
import { useLocale, useTranslations } from "next-intl"
import { useRouter } from "next/navigation"
import { useTransition } from "react"
import { Languages } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  locales,
  localeNames,
  type Locale,
} from "@/i18n/config"

export function LanguageSwitcher() {
  const t = useTranslations("common")
  const locale = useLocale()
  const router = useRouter()
  const [isPending, startTransition] = useTransition()
  const [open, setOpen] = useState(false)
  const closeTimerRef = useRef<number | null>(null)

  const clearCloseTimer = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current)
      closeTimerRef.current = null
    }
  }

  const openMenu = () => {
    clearCloseTimer()
    setOpen(true)
  }

  const closeMenuWithDelay = () => {
    clearCloseTimer()
    closeTimerRef.current = window.setTimeout(() => {
      setOpen(false)
      closeTimerRef.current = null
    }, 180)
  }

  function onSelectChange(nextLocale: Locale) {
    document.cookie = `NEXT_LOCALE=${nextLocale};path=/;max-age=31536000`
    setOpen(false)
    startTransition(() => {
      router.refresh()
    })
  }

  useEffect(() => {
    return () => {
      clearCloseTimer()
    }
  }, [])

  return (
    <div
      className="relative"
      onMouseEnter={openMenu}
      onMouseLeave={closeMenuWithDelay}
    >
      <Button
        variant="ghost"
        size="icon"
        className="h-9 w-9"
        disabled={isPending}
        aria-label={t("language")}
        aria-expanded={open}
        aria-haspopup="menu"
        onFocus={openMenu}
      >
        <Languages className="h-4 w-4" />
      </Button>
      <div
        className={`absolute right-0 top-full z-50 mt-1 min-w-[140px] rounded-md border border-border bg-popover p-1 shadow-md ${
          open ? "block" : "hidden"
        }`}
        role="menu"
      >
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

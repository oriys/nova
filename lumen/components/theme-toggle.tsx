"use client"

import { useTheme } from "next-themes"
import { useEffect, useState, useRef } from "react"
import { Sun, Moon } from "lucide-react"
import { Button } from "@/components/ui/button"

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)
  const btnRef = useRef<HTMLButtonElement>(null)

  useEffect(() => setMounted(true), [])

  const toggleTheme = async () => {
    const isDark = resolvedTheme === "dark"
    const nextTheme = isDark ? "light" : "dark"

    if (!document.startViewTransition) {
      setTheme(nextTheme)
      return
    }

    const btn = btnRef.current!
    const rect = btn.getBoundingClientRect()
    const x = rect.left + rect.width / 2
    const y = rect.top + rect.height / 2

    const maxRadius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y)
    )

    const transition = document.startViewTransition(() => {
      document.documentElement.classList.remove("light", "dark")
      document.documentElement.classList.add(nextTheme)
      document.documentElement.style.colorScheme = nextTheme
      setTheme(nextTheme)
    })

    await transition.ready

    document.documentElement.animate(
      {
        clipPath: [
          `circle(0px at ${x}px ${y}px)`,
          `circle(${maxRadius}px at ${x}px ${y}px)`,
        ],
      },
      {
        duration: 500,
        easing: "cubic-bezier(0.4, 0, 0.2, 1)",
        pseudoElement: "::view-transition-new(root)",
      }
    )
  }

  if (!mounted) return null

  const isDark = resolvedTheme === "dark"

  return (
    <Button
      ref={btnRef}
      variant="ghost"
      size="icon"
      onClick={toggleTheme}
      title={isDark ? "Switch to light mode" : "Switch to dark mode"}
    >
      {isDark ? (
        <Sun className="h-5 w-5 text-muted-foreground" />
      ) : (
        <Moon className="h-5 w-5 text-muted-foreground" />
      )}
    </Button>
  )
}

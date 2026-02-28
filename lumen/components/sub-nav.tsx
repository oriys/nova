"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"

interface SubNavItem {
  label: string
  href: string
}

export function SubNav({ items }: { items: SubNavItem[] }) {
  const pathname = usePathname()

  return (
    <div className="inline-flex gap-1 rounded-lg bg-muted p-1">
      {items.map((item) => (
        <Link
          key={item.href}
          href={item.href}
          className={cn(
            "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
            pathname === item.href
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          )}
        >
          {item.label}
        </Link>
      ))}
    </div>
  )
}

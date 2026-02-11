"use client"

import { useMemo } from "react"
import { useTranslations } from "next-intl"
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

type PaginationItem = number | "ellipsis"

function getPaginationItems(totalPages: number, currentPage: number): PaginationItem[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }

  const showLeftEllipsis = currentPage > 4
  const showRightEllipsis = currentPage < totalPages - 3

  if (!showLeftEllipsis) {
    return [1, 2, 3, 4, 5, "ellipsis", totalPages]
  }

  if (!showRightEllipsis) {
    return [
      1,
      "ellipsis",
      totalPages - 4,
      totalPages - 3,
      totalPages - 2,
      totalPages - 1,
      totalPages,
    ]
  }

  return [
    1,
    "ellipsis",
    currentPage - 1,
    currentPage,
    currentPage + 1,
    "ellipsis",
    totalPages,
  ]
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}

export interface PaginationProps {
  className?: string
  totalItems: number
  page: number
  pageSize: number
  onPageChange: (page: number) => void
  onPageSizeChange?: (pageSize: number) => void
  pageSizeOptions?: number[]
  itemLabel?: string
}

export function Pagination({
  className,
  totalItems,
  page,
  pageSize,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50, 100],
  itemLabel = "items",
}: PaginationProps) {
  const t = useTranslations("pagination")
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize))
  const safePage = clamp(page, 1, totalPages)

  const start = totalItems === 0 ? 0 : (safePage - 1) * pageSize + 1
  const end = Math.min(safePage * pageSize, totalItems)

  const items = useMemo(() => getPaginationItems(totalPages, safePage), [totalPages, safePage])

  return (
    <div className={cn("flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between", className)}>
      <div className="text-sm text-muted-foreground">
        {totalItems === 0 ? t("zeroResults") : t("showing", { start, end, total: totalItems, label: itemLabel })}
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        {onPageSizeChange && (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground whitespace-nowrap">{t("rows")}</span>
            <Select
              value={String(pageSize)}
              onValueChange={(v) => onPageSizeChange(Number(v))}
            >
              <SelectTrigger className="h-8 w-[92px]">
                <SelectValue placeholder={t("size")} />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((opt) => (
                  <SelectItem key={opt} value={String(opt)}>
                    {opt}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        <div className="flex items-center justify-between gap-2 sm:justify-start">
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={() => onPageChange(1)}
              disabled={safePage <= 1}
              aria-label={t("firstPage")}
            >
              <ChevronsLeft className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={() => onPageChange(safePage - 1)}
              disabled={safePage <= 1}
              aria-label={t("previousPage")}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>

            <div className="hidden sm:flex items-center gap-1">
              {items.map((it, idx) =>
                it === "ellipsis" ? (
                  <span key={`e-${idx}`} className="px-2 text-muted-foreground">
                    …
                  </span>
                ) : (
                  <Button
                    key={it}
                    type="button"
                    variant={it === safePage ? "default" : "outline"}
                    size="sm"
                    className="h-8 min-w-8 px-2"
                    onClick={() => onPageChange(clamp(it, 1, totalPages))}
                    aria-label={t("page", { number: it })}
                  >
                    {it}
                  </Button>
                )
              )}
            </div>

            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={() => onPageChange(safePage + 1)}
              disabled={safePage >= totalPages}
              aria-label={t("nextPage")}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={() => onPageChange(totalPages)}
              disabled={safePage >= totalPages}
              aria-label={t("lastPage")}
            >
              <ChevronsRight className="h-4 w-4" />
            </Button>
          </div>

          <div className="sm:hidden text-sm text-muted-foreground whitespace-nowrap">
            {safePage}/{totalPages}
          </div>
        </div>
      </div>
    </div>
  )
}


"use client"

import { useEffect, useState, useMemo, useRef } from "react"
import { metricsApi, type HeatmapPoint } from "@/lib/api"

const CELL_SIZE = 13
const CELL_GAP = 3
const CELL_ROUND = 2
const LABEL_OFFSET = 36
const DAY_LABELS = ["Mon", "", "Wed", "", "Fri", "", "Sun"]
const MONTH_LABELS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

function getColor(count: number, max: number): string {
  if (count === 0) return "var(--muted)"
  const ratio = count / max
  if (ratio <= 0.25) return "color-mix(in oklch, var(--foreground) 20%, transparent)"
  if (ratio <= 0.50) return "color-mix(in oklch, var(--foreground) 40%, transparent)"
  if (ratio <= 0.75) return "color-mix(in oklch, var(--foreground) 65%, transparent)"
  return "color-mix(in oklch, var(--foreground) 90%, transparent)"
}

export function GlobalHeatmap() {
  const containerRef = useRef<HTMLDivElement>(null)
  const [containerWidth, setContainerWidth] = useState(0)
  const [data, setData] = useState<HeatmapPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [tooltip, setTooltip] = useState<{ x: number; y: number; date: string; count: number } | null>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const numWeeks = useMemo(() => {
    if (containerWidth === 0) return 0
    const available = containerWidth - LABEL_OFFSET
    const weeks = Math.floor(available / (CELL_SIZE + CELL_GAP))
    return Math.max(20, Math.min(104, weeks))
  }, [containerWidth])

  useEffect(() => {
    if (numWeeks === 0) return
    let cancelled = false
    setLoading(true)
    metricsApi.heatmap(numWeeks).then((res) => {
      if (!cancelled) {
        setData(res || [])
        setLoading(false)
      }
    }).catch(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [numWeeks])

  const { grid, weeks, maxCount, monthMarkers, totalInvocations } = useMemo(() => {
    if (numWeeks === 0) return { grid: [], weeks: 0, maxCount: 0, monthMarkers: [], totalInvocations: 0 }

    const lookup = new Map<string, number>()
    let total = 0
    for (const d of data) {
      lookup.set(d.date, d.invocations)
      total += d.invocations
    }

    const today = new Date()
    today.setHours(0, 0, 0, 0)
    const dayOfWeek = (today.getDay() + 6) % 7

    const start = new Date(today)
    start.setDate(start.getDate() - ((numWeeks - 1) * 7 + dayOfWeek))

    const cells: { date: string; count: number; col: number; row: number }[] = []
    let maxC = 0
    const current = new Date(start)

    const markers: { label: string; col: number }[] = []
    let lastMonth = -1

    for (let week = 0; week < numWeeks; week++) {
      for (let day = 0; day < 7; day++) {
        if (current > today) break
        const dateStr = current.toISOString().slice(0, 10)
        const count = lookup.get(dateStr) || 0
        if (count > maxC) maxC = count
        cells.push({ date: dateStr, count, col: week, row: day })

        const month = current.getMonth()
        if (day === 0 && month !== lastMonth) {
          markers.push({ label: MONTH_LABELS[month], col: week })
          lastMonth = month
        }

        current.setDate(current.getDate() + 1)
      }
    }

    return { grid: cells, weeks: numWeeks, maxCount: maxC, monthMarkers: markers, totalInvocations: total }
  }, [data, numWeeks])

  const svgWidth = weeks * (CELL_SIZE + CELL_GAP) + LABEL_OFFSET
  const svgHeight = 7 * (CELL_SIZE + CELL_GAP) + 24

  const periodLabel = numWeeks <= 13 ? `last ${numWeeks} weeks`
    : numWeeks <= 52 ? "last year"
    : `last ${Math.round(numWeeks / 52 * 10) / 10} years`

  return (
    <div ref={containerRef} className="rounded-xl border border-border bg-card p-6">
      {(loading || numWeeks === 0) ? (
        <div className="h-32 flex items-center justify-center">
          <span className="text-sm text-muted-foreground">Loading heatmap...</span>
        </div>
      ) : (
        <>
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-card-foreground">Invocation Activity</h3>
              <p className="text-xs text-muted-foreground">
                {totalInvocations.toLocaleString()} total invocations across all functions in the {periodLabel}
              </p>
            </div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>Less</span>
              <svg width={CELL_SIZE} height={CELL_SIZE}>
                <rect width={CELL_SIZE} height={CELL_SIZE} rx={CELL_ROUND} fill="var(--muted)" />
              </svg>
              {[0.25, 0.5, 0.75, 1].map((r) => (
                <svg key={r} width={CELL_SIZE} height={CELL_SIZE}>
                  <rect
                    width={CELL_SIZE}
                    height={CELL_SIZE}
                    rx={CELL_ROUND}
                    fill={getColor(r * (maxCount || 1), maxCount || 1)}
                  />
                </svg>
              ))}
              <span>More</span>
            </div>
          </div>

          <svg
            width={svgWidth}
            height={svgHeight}
            className="block"
            onMouseLeave={() => setTooltip(null)}
          >
            {monthMarkers.map((m, i) => (
              <text
                key={i}
                x={LABEL_OFFSET + m.col * (CELL_SIZE + CELL_GAP)}
                y={10}
                className="fill-muted-foreground"
                fontSize={10}
              >
                {m.label}
              </text>
            ))}

            {DAY_LABELS.map((label, i) => (
              label ? (
                <text
                  key={i}
                  x={0}
                  y={24 + i * (CELL_SIZE + CELL_GAP) + CELL_SIZE * 0.75}
                  className="fill-muted-foreground"
                  fontSize={10}
                >
                  {label}
                </text>
              ) : null
            ))}

            {grid.map((cell) => (
              <rect
                key={cell.date}
                x={LABEL_OFFSET + cell.col * (CELL_SIZE + CELL_GAP)}
                y={18 + cell.row * (CELL_SIZE + CELL_GAP)}
                width={CELL_SIZE}
                height={CELL_SIZE}
                rx={CELL_ROUND}
                fill={getColor(cell.count, maxCount || 1)}
                className="cursor-pointer transition-opacity hover:opacity-80"
                onMouseEnter={(e) => {
                  const rect = e.currentTarget.getBoundingClientRect()
                  const container = e.currentTarget.closest("svg")?.getBoundingClientRect()
                  if (container) {
                    setTooltip({
                      x: rect.left - container.left + CELL_SIZE / 2,
                      y: rect.top - container.top - 8,
                      date: cell.date,
                      count: cell.count,
                    })
                  }
                }}
                onMouseLeave={() => setTooltip(null)}
              />
            ))}

            {tooltip && (
              <g>
                <rect
                  x={tooltip.x - 60}
                  y={tooltip.y - 32}
                  width={120}
                  height={28}
                  rx={6}
                  className="fill-popover stroke-border"
                  strokeWidth={1}
                />
                <text
                  x={tooltip.x}
                  y={tooltip.y - 14}
                  textAnchor="middle"
                  className="fill-popover-foreground"
                  fontSize={11}
                >
                  {tooltip.count} invocation{tooltip.count !== 1 ? "s" : ""} on {tooltip.date}
                </text>
              </g>
            )}
          </svg>
        </>
      )}
    </div>
  )
}

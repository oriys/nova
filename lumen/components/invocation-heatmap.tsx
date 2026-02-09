"use client"

import { useEffect, useState, useMemo, useRef } from "react"
import { functionsApi, type HeatmapPoint } from "@/lib/api"

interface InvocationHeatmapProps {
  functionName: string
}

const CELL_SIZE = 13
const CELL_GAP = 3
const CELL_ROUND = 2
const LABEL_OFFSET = 36
const GRID_TOP_OFFSET = 18
const DAY_LABELS = ["Mon", "", "Wed", "", "Fri", "", "Sun"]
const MONTH_LABELS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
const TOOLTIP_MIN_WIDTH = 120
const TOOLTIP_PADDING_X = 10
const TOOLTIP_CHAR_WIDTH = 6.4
const TOOLTIP_HEIGHT = 28
const TOOLTIP_MARGIN = 4
const TOOLTIP_OFFSET = 8

function getColor(count: number, max: number): string {
  if (count === 0) return "var(--muted)"
  const ratio = count / max
  if (ratio <= 0.25) return "color-mix(in oklch, var(--foreground) 20%, transparent)"
  if (ratio <= 0.50) return "color-mix(in oklch, var(--foreground) 40%, transparent)"
  if (ratio <= 0.75) return "color-mix(in oklch, var(--foreground) 65%, transparent)"
  return "color-mix(in oklch, var(--foreground) 90%, transparent)"
}

export function InvocationHeatmap({ functionName }: InvocationHeatmapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [containerWidth, setContainerWidth] = useState(0)
  const [data, setData] = useState<HeatmapPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [tooltip, setTooltip] = useState<{ x: number; y: number; date: string; count: number } | null>(null)

  // Measure container width
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    let timeoutId: ReturnType<typeof setTimeout> | undefined
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (timeoutId) {
          clearTimeout(timeoutId)
        }
        timeoutId = setTimeout(() => {
          setContainerWidth(entry.contentRect.width)
        }, 100)
      }
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
      if (timeoutId) {
        clearTimeout(timeoutId)
      }
    }
  }, [])

  // Calculate how many weeks fit in the container (min 20, max 104)
  const numWeeks = useMemo(() => {
    if (containerWidth === 0) return 0
    const available = containerWidth - LABEL_OFFSET
    const weeks = Math.floor(available / (CELL_SIZE + CELL_GAP))
    return Math.max(20, Math.min(104, weeks))
  }, [containerWidth])

  // Fetch data when weeks are determined
  useEffect(() => {
    if (numWeeks === 0) return
    let cancelled = false
    setLoading(true)
    functionsApi.heatmap(functionName, numWeeks).then((res) => {
      if (!cancelled) {
        setData(res || [])
        setLoading(false)
      }
    }).catch(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [functionName, numWeeks])

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
    const dayOfWeek = (today.getDay() + 6) % 7 // 0=Mon, 6=Sun

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

  const isInitialLoading = data.length === 0 && (loading || numWeeks === 0)

  return (
    <div ref={containerRef} className="rounded-xl border border-border bg-card p-6 min-h-[226px]">
      {isInitialLoading ? (
        <div className="h-[178px] flex items-center justify-center">
          <span className="text-sm text-muted-foreground animate-pulse">Loading heatmap...</span>
        </div>
      ) : (
        <div className={loading ? "opacity-50 pointer-events-none transition-opacity" : "transition-opacity"}>
          <div className="flex items-center justify-between mb-4">
            <div>
              <h3 className="text-sm font-semibold text-card-foreground">Invocation Activity</h3>
              <p className="text-xs text-muted-foreground">
                {totalInvocations.toLocaleString()} invocations in the {periodLabel}
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

          <div className="overflow-hidden">
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
                  y={GRID_TOP_OFFSET + cell.row * (CELL_SIZE + CELL_GAP)}
                  width={CELL_SIZE}
                  height={CELL_SIZE}
                  rx={CELL_ROUND}
                  fill={getColor(cell.count, maxCount || 1)}
                  className="cursor-pointer transition-opacity hover:opacity-80"
                  onMouseEnter={() => {
                    setTooltip({
                      x: LABEL_OFFSET + cell.col * (CELL_SIZE + CELL_GAP) + CELL_SIZE / 2,
                      y: GRID_TOP_OFFSET + cell.row * (CELL_SIZE + CELL_GAP),
                      date: cell.date,
                      count: cell.count,
                    })
                  }}
                  onMouseLeave={() => setTooltip(null)}
                />
              ))}

              {tooltip && (() => {
                const label = `${tooltip.count} invocation${tooltip.count !== 1 ? "s" : ""} on ${tooltip.date}`
                const tooltipWidth = Math.max(
                  TOOLTIP_MIN_WIDTH,
                  label.length * TOOLTIP_CHAR_WIDTH + TOOLTIP_PADDING_X * 2
                )
                const tooltipCenterX = Math.max(
                  tooltipWidth / 2 + TOOLTIP_MARGIN,
                  Math.min(svgWidth - tooltipWidth / 2 - TOOLTIP_MARGIN, tooltip.x)
                )
                const preferredY = tooltip.y - TOOLTIP_HEIGHT - TOOLTIP_OFFSET
                const tooltipY = preferredY < TOOLTIP_MARGIN
                  ? tooltip.y + CELL_SIZE + TOOLTIP_OFFSET
                  : preferredY

                return (
                  <g>
                    <rect
                      x={tooltipCenterX - tooltipWidth / 2}
                      y={tooltipY}
                      width={tooltipWidth}
                      height={TOOLTIP_HEIGHT}
                      rx={6}
                      className="fill-popover stroke-border"
                      strokeWidth={1}
                    />
                    <text
                      x={tooltipCenterX}
                      y={tooltipY + 18}
                      textAnchor="middle"
                      className="fill-popover-foreground"
                      fontSize={11}
                    >
                      {label}
                    </text>
                  </g>
                )
              })()}
            </svg>
          </div>
        </div>
      )}
    </div>
  )
}

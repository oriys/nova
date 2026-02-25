"use client"

import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { ParamMapping, ParamSource, ParamTransform, ParamType } from "@/lib/types"
import { Plus, Trash2, ChevronDown, ChevronRight } from "lucide-react"
import { useState } from "react"

interface ParamMappingEditorProps {
  value: ParamMapping[]
  onChange: (mappings: ParamMapping[]) => void
  disabled?: boolean
}

const SOURCES: ParamSource[] = ["query", "path", "body", "header"]
const TRANSFORMS: ParamTransform[] = ["", "camel_case", "snake_case", "kebab_case", "upper_case", "lower_case", "upper_first"]
const TYPES: ParamType[] = ["", "integer", "float", "boolean", "json"]

function emptyMapping(): ParamMapping {
  return { source: "query", name: "" }
}

export function ParamMappingEditor({ value, onChange, disabled }: ParamMappingEditorProps) {
  const t = useTranslations("paramMapping")
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())

  const toggleExpand = (idx: number) => {
    setExpandedRows((prev) => {
      const next = new Set(prev)
      if (next.has(idx)) next.delete(idx)
      else next.add(idx)
      return next
    })
  }

  const addMapping = () => {
    const next = [...value, emptyMapping()]
    onChange(next)
    setExpandedRows((prev) => new Set(prev).add(next.length - 1))
  }

  const removeMapping = (idx: number) => {
    const next = value.filter((_, i) => i !== idx)
    onChange(next)
    setExpandedRows((prev) => {
      const s = new Set<number>()
      prev.forEach((i) => {
        if (i < idx) s.add(i)
        else if (i > idx) s.add(i - 1)
      })
      return s
    })
  }

  const updateMapping = (idx: number, patch: Partial<ParamMapping>) => {
    const next = value.map((m, i) => (i === idx ? { ...m, ...patch } : m))
    onChange(next)
  }

  const sourceLabel = (s: ParamSource) => t(`sources.${s}`)
  const transformLabel = (tr: ParamTransform) => (tr ? t(`transforms.${tr}`) : t("transforms.none"))
  const typeLabel = (ty: ParamType) => (ty ? t(`types.${ty}`) : t("types.string"))

  const summaryText = (m: ParamMapping) => {
    const parts: string[] = []
    parts.push(`${sourceLabel(m.source)}.${m.name || "?"}`)
    parts.push("→")
    parts.push(m.target || m.name || "?")
    if (m.transform) parts.push(`[${transformLabel(m.transform)}]`)
    if (m.type) parts.push(`(${typeLabel(m.type)})`)
    return parts.join(" ")
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">{t("title")}</Label>
        <Button type="button" variant="outline" size="sm" onClick={addMapping} disabled={disabled}>
          <Plus className="mr-1 h-3 w-3" />
          {t("add")}
        </Button>
      </div>

      {value.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("empty")}</p>
      )}

      <div className="space-y-1">
        {value.map((mapping, idx) => {
          const expanded = expandedRows.has(idx)
          return (
            <div
              key={idx}
              className="rounded-md border border-border bg-muted/30"
            >
              {/* Collapsed summary row */}
              <div
                className="flex cursor-pointer items-center gap-2 px-3 py-2"
                onClick={() => toggleExpand(idx)}
              >
                {expanded ? (
                  <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                )}
                <span className="flex-1 truncate text-xs font-mono">
                  {summaryText(mapping)}
                </span>
                {mapping.required && (
                  <span className="rounded bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive">
                    {t("required")}
                  </span>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
                  onClick={(e) => { e.stopPropagation(); removeMapping(idx) }}
                  disabled={disabled}
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>

              {/* Expanded detail form */}
              {expanded && (
                <div className="border-t border-border px-3 pb-3 pt-2">
                  <div className="grid gap-3 sm:grid-cols-2">
                    {/* Source */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.source")}</Label>
                      <Select
                        value={mapping.source}
                        onValueChange={(v: ParamSource) => updateMapping(idx, { source: v })}
                        disabled={disabled}
                      >
                        <SelectTrigger className="h-8 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {SOURCES.map((s) => (
                            <SelectItem key={s} value={s}>
                              {sourceLabel(s)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    {/* Name */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.name")}</Label>
                      <Input
                        className="h-8 text-xs"
                        value={mapping.name}
                        onChange={(e) => updateMapping(idx, { name: e.target.value })}
                        placeholder={t("placeholders.name")}
                        disabled={disabled}
                      />
                    </div>

                    {/* Target */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.target")}</Label>
                      <Input
                        className="h-8 text-xs"
                        value={mapping.target || ""}
                        onChange={(e) => updateMapping(idx, { target: e.target.value || undefined })}
                        placeholder={mapping.name || t("placeholders.target")}
                        disabled={disabled}
                      />
                    </div>

                    {/* Transform */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.transform")}</Label>
                      <Select
                        value={mapping.transform || "__none__"}
                        onValueChange={(v) => updateMapping(idx, { transform: (v === "__none__" ? "" : v) as ParamTransform })}
                        disabled={disabled}
                      >
                        <SelectTrigger className="h-8 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {TRANSFORMS.map((tr) => (
                            <SelectItem key={tr || "__none__"} value={tr || "__none__"}>
                              {transformLabel(tr)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    {/* Type */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.type")}</Label>
                      <Select
                        value={mapping.type || "__none__"}
                        onValueChange={(v) => updateMapping(idx, { type: (v === "__none__" ? "" : v) as ParamType })}
                        disabled={disabled}
                      >
                        <SelectTrigger className="h-8 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {TYPES.map((ty) => (
                            <SelectItem key={ty || "__none__"} value={ty || "__none__"}>
                              {typeLabel(ty)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    {/* Required toggle */}
                    <div className="space-y-1">
                      <Label className="text-xs">{t("fields.required")}</Label>
                      <Select
                        value={mapping.required ? "true" : "false"}
                        onValueChange={(v) => updateMapping(idx, { required: v === "true" })}
                        disabled={disabled}
                      >
                        <SelectTrigger className="h-8 text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="false">{t("no")}</SelectItem>
                          <SelectItem value="true">{t("yes")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    {/* Default value */}
                    <div className="space-y-1 sm:col-span-2">
                      <Label className="text-xs">{t("fields.default")}</Label>
                      <Input
                        className="h-8 text-xs"
                        value={mapping.default !== undefined && mapping.default !== null ? String(mapping.default) : ""}
                        onChange={(e) => {
                          const raw = e.target.value
                          if (raw === "") {
                            updateMapping(idx, { default: undefined })
                          } else {
                            // Try to parse as JSON for complex defaults
                            try {
                              const parsed = JSON.parse(raw)
                              updateMapping(idx, { default: parsed })
                            } catch {
                              updateMapping(idx, { default: raw })
                            }
                          }
                        }}
                        placeholder={t("placeholders.default")}
                        disabled={disabled}
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

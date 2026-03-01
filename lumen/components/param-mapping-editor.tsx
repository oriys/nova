"use client"

import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { ParamMapping, ParamSource, ParamTransform, ParamType } from "@/lib/types"
import { Plus, Trash2, ChevronDown, ChevronRight, Code, ListTree } from "lucide-react"
import { useCallback, useEffect, useState } from "react"

interface ParamMappingEditorProps {
  value: ParamMapping[]
  onChange: (mappings: ParamMapping[]) => void
  disabled?: boolean
  allowedSources?: ParamSource[]
}

const SOURCES: ParamSource[] = ["query", "path", "body", "header"]
const TRANSFORMS: ParamTransform[] = ["", "camel_case", "snake_case", "kebab_case", "upper_case", "lower_case", "upper_first"]
const TYPES: ParamType[] = ["", "integer", "float", "boolean", "json"]

const VALID_SOURCES = new Set<string>(SOURCES)
const VALID_TRANSFORMS = new Set<string>(TRANSFORMS)
const VALID_TYPES = new Set<string>(TYPES)

function emptyMapping(source: ParamSource): ParamMapping {
  return { source, name: "" }
}

// ── DSL serializer ──────────────────────────────────────────
// Syntax per line:
//   source.name -> target | transform | type = default !
//
// Examples:
//   query.user_id -> userId | camel_case | integer
//   path.id -> recordId | integer !
//   header.X-Request-ID -> requestId
//   query.page -> page | integer = 1
//   body.email -> Email | upper_first

function mappingToDsl(m: ParamMapping): string {
  const parts: string[] = [`${m.source}.${m.name}`]
  const target = m.target && m.target !== m.name ? m.target : ""
  if (target) parts.push(`-> ${target}`)

  const pipes: string[] = []
  if (m.transform) pipes.push(m.transform)
  if (m.type) pipes.push(m.type)
  if (pipes.length > 0) {
    if (!target) parts.push(`-> ${m.name}`)
    parts.push("| " + pipes.join(" | "))
  }

  if (m.default !== undefined && m.default !== null) {
    const def = typeof m.default === "string" ? m.default : JSON.stringify(m.default)
    parts.push(`= ${def}`)
  }
  if (m.required) parts.push("!")
  return parts.join(" ")
}

function mappingsToDsl(mappings: ParamMapping[]): string {
  return mappings.map(mappingToDsl).join("\n")
}

// ── DSL parser ──────────────────────────────────────────────

interface DslParseResult {
  mappings: ParamMapping[]
  errors: { line: number; message: string }[]
}

function parseDsl(text: string): DslParseResult {
  const lines = text.split("\n")
  const mappings: ParamMapping[] = []
  const errors: { line: number; message: string }[] = []

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i].trim()
    if (!raw || raw.startsWith("#") || raw.startsWith("//")) continue

    const m = parseDslLine(raw)
    if (m.error) {
      errors.push({ line: i + 1, message: m.error })
    } else if (m.mapping) {
      mappings.push(m.mapping)
    }
  }
  return { mappings, errors }
}

function parseDslLine(line: string): { mapping?: ParamMapping; error?: string } {
  // Check for required flag
  const required = line.endsWith("!")
  if (required) line = line.slice(0, -1).trim()

  // Split off default value: ... = defaultVal
  let defaultVal: unknown = undefined
  const eqIdx = line.lastIndexOf("=")
  // Only treat = as default separator if it's not inside the source.name part
  if (eqIdx > 0 && !line.substring(0, eqIdx).includes("|") ? line.indexOf("->") < eqIdx || line.indexOf("->") === -1 ? eqIdx > line.indexOf(".") : true : true) {
    const afterEq = line.substring(eqIdx + 1).trim()
    const beforeEq = line.substring(0, eqIdx).trim()
    // Only parse as default if there's actually content after =
    if (afterEq) {
      line = beforeEq
      try {
        defaultVal = JSON.parse(afterEq)
      } catch {
        defaultVal = afterEq
      }
    }
  }

  // Split by pipes for transform/type modifiers
  const pipeParts = line.split("|").map((s) => s.trim())
  const mainPart = pipeParts[0]

  // Parse main part: source.name [-> target]
  const arrowParts = mainPart.split("->").map((s) => s.trim())
  const sourcePart = arrowParts[0]
  const targetPart = arrowParts.length > 1 ? arrowParts[1] : ""

  // Parse source.name
  const dotIdx = sourcePart.indexOf(".")
  if (dotIdx <= 0) return { error: `invalid format, expected "source.name"` }

  const source = sourcePart.substring(0, dotIdx)
  const name = sourcePart.substring(dotIdx + 1)

  if (!VALID_SOURCES.has(source)) return { error: `unknown source "${source}", use: query, path, body, header` }
  if (!name) return { error: `field name is empty` }

  // Parse pipe modifiers (transform and type)
  let transform: ParamTransform = ""
  let type: ParamType = ""

  for (let i = 1; i < pipeParts.length; i++) {
    const mod = pipeParts[i]
    if (VALID_TRANSFORMS.has(mod) && mod !== "") {
      transform = mod as ParamTransform
    } else if (VALID_TYPES.has(mod) && mod !== "") {
      type = mod as ParamType
    } else if (mod) {
      return { error: `unknown modifier "${mod}"` }
    }
  }

  const mapping: ParamMapping = {
    source: source as ParamSource,
    name,
    ...(targetPart && targetPart !== name ? { target: targetPart } : {}),
    ...(transform ? { transform } : {}),
    ...(type ? { type } : {}),
    ...(defaultVal !== undefined ? { default: defaultVal } : {}),
    ...(required ? { required: true } : {}),
  }

  return { mapping }
}

// ── Component ───────────────────────────────────────────────

export function ParamMappingEditor({
  value,
  onChange,
  disabled,
  allowedSources,
}: ParamMappingEditorProps) {
  const t = useTranslations("paramMapping")
  const availableSources = allowedSources && allowedSources.length > 0 ? allowedSources : SOURCES
  const fixedSource = availableSources.length === 1 ? availableSources[0] : undefined
  const [mode, setMode] = useState<"visual" | "dsl">("visual")
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set())
  const [dslText, setDslText] = useState(() => mappingsToDsl(value))
  const [dslErrors, setDslErrors] = useState<{ line: number; message: string }[]>([])

  const normalizeMappings = useCallback((mappings: ParamMapping[]) => {
    return mappings.map((mapping) => {
      if (fixedSource) {
        if (mapping.source === fixedSource) {
          return mapping
        }
        return { ...mapping, source: fixedSource }
      }
      if (availableSources.includes(mapping.source)) {
        return mapping
      }
      return { ...mapping, source: availableSources[0] }
    })
  }, [availableSources, fixedSource])

  useEffect(() => {
    const normalized = normalizeMappings(value)
    const changed = normalized.some((mapping, idx) => mapping.source !== value[idx]?.source)
    if (changed) {
      onChange(normalized)
    }
  }, [normalizeMappings, onChange, value])

  // Sync DSL text when switching to DSL mode or when value changes externally in visual mode
  const syncDslFromValue = useCallback(() => {
    setDslText(mappingsToDsl(value))
    setDslErrors([])
  }, [value])

  const switchMode = (next: "visual" | "dsl") => {
    if (next === mode) return
    if (next === "dsl") {
      syncDslFromValue()
    } else {
      // Switching to visual — apply DSL first
      const result = parseDsl(dslText)
      if (result.errors.length === 0) {
        onChange(normalizeMappings(result.mappings))
        setDslErrors([])
      }
    }
    setMode(next)
  }

  const handleDslChange = (text: string) => {
    setDslText(text)
    const result = parseDsl(text)
    setDslErrors(result.errors)
    if (result.errors.length === 0) {
      onChange(normalizeMappings(result.mappings))
    }
  }

  // Keep DSL in sync when value changes from outside while in DSL mode
  useEffect(() => {
    if (mode === "dsl") {
      // Only sync if the mappings actually differ (avoid cursor jumps)
      const currentDslMappings = parseDsl(dslText)
      if (currentDslMappings.errors.length > 0) return
      const serialized = mappingsToDsl(value)
      const reParsed = mappingsToDsl(currentDslMappings.mappings)
      if (serialized !== reParsed) {
        setDslText(serialized)
      }
    }
  }, [dslText, mode, value])

  // ── Visual mode helpers ─────────────────────────────────

  const toggleExpand = (idx: number) => {
    setExpandedRows((prev) => {
      const next = new Set(prev)
      if (next.has(idx)) next.delete(idx)
      else next.add(idx)
      return next
    })
  }

  const addMapping = () => {
    const next = [...value, emptyMapping(availableSources[0])]
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
      {/* Header with title and mode toggle */}
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">{t("title")}</Label>
        <div className="flex items-center gap-1">
          <div className="flex rounded-md border border-border">
            <button
              type="button"
              onClick={() => switchMode("visual")}
              className={`inline-flex items-center gap-1 rounded-l-md px-2 py-1 text-xs transition-colors ${
                mode === "visual"
                  ? "bg-primary text-primary-foreground"
                  : "bg-transparent text-muted-foreground hover:text-foreground"
              }`}
              disabled={disabled}
            >
              <ListTree className="h-3 w-3" />
              {t("modeVisual")}
            </button>
            <button
              type="button"
              onClick={() => switchMode("dsl")}
              className={`inline-flex items-center gap-1 rounded-r-md px-2 py-1 text-xs transition-colors ${
                mode === "dsl"
                  ? "bg-primary text-primary-foreground"
                  : "bg-transparent text-muted-foreground hover:text-foreground"
              }`}
              disabled={disabled}
            >
              <Code className="h-3 w-3" />
              {t("modeDsl")}
            </button>
          </div>
          {mode === "visual" && (
            <Button type="button" variant="outline" size="sm" onClick={addMapping} disabled={disabled}>
              <Plus className="mr-1 h-3 w-3" />
              {t("add")}
            </Button>
          )}
        </div>
      </div>

      {/* ── DSL mode ── */}
      {mode === "dsl" && (
        <div className="space-y-2">
          <textarea
            className="w-full rounded-md border border-border bg-muted/30 px-3 py-2 font-mono text-xs leading-relaxed placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-50"
            rows={Math.max(5, dslText.split("\n").length + 1)}
            value={dslText}
            onChange={(e) => handleDslChange(e.target.value)}
            placeholder={t("dslPlaceholder")}
            spellCheck={false}
            disabled={disabled}
          />
          {dslErrors.length > 0 && (
            <div className="space-y-0.5">
              {dslErrors.map((err, i) => (
                <p key={i} className="text-xs text-destructive">
                  {t("dslError", { line: err.line, message: err.message })}
                </p>
              ))}
            </div>
          )}
          <p className="text-[11px] leading-relaxed text-muted-foreground whitespace-pre-line">
            {t("dslHelp")}
          </p>
        </div>
      )}

      {/* ── Visual mode ── */}
      {mode === "visual" && (
        <>
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
                          {fixedSource ? (
                            <div className="flex h-8 items-center rounded-md border border-border bg-muted/20 px-3 text-xs text-muted-foreground">
                              {sourceLabel(fixedSource)}
                            </div>
                          ) : (
                            <Select
                              value={mapping.source}
                              onValueChange={(v: ParamSource) => updateMapping(idx, { source: v })}
                              disabled={disabled}
                            >
                              <SelectTrigger className="h-8 text-xs">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {availableSources.map((s) => (
                                  <SelectItem key={s} value={s}>
                                    {sourceLabel(s)}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          )}
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
        </>
      )}
    </div>
  )
}

"use client"

import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  apiDocsApi,
  functionDocsApi,
  functionsApi,
  type GenerateDocsResponse,
} from "@/lib/api"
import type { FunctionData } from "@/lib/types"
import {
  FileText,
  Sparkles,
  Save,
  Trash2,
  Loader2,
  Pencil,
  X,
  Check,
  RotateCcw,
} from "lucide-react"
import { cn } from "@/lib/utils"

// Defines all doc sections that can be toggled/deleted
type DocSection =
  | "meta"
  | "endpoint"
  | "requestFields"
  | "responseFields"
  | "statusCodes"
  | "errorModel"
  | "authIdempotency"
  | "observability"
  | "curlExample"
  | "requestExample"
  | "responseExample"
  | "errorExample"
  | "notes"
  | "changelog"

const ALL_SECTIONS: DocSection[] = [
  "meta",
  "endpoint",
  "requestFields",
  "responseFields",
  "statusCodes",
  "errorModel",
  "authIdempotency",
  "observability",
  "curlExample",
  "requestExample",
  "responseExample",
  "errorExample",
  "notes",
  "changelog",
]

const SECTION_LABELS: Record<DocSection, string> = {
  meta: "Metadata",
  endpoint: "Endpoint",
  requestFields: "Request Fields",
  responseFields: "Response Fields",
  statusCodes: "Status Codes",
  errorModel: "Error Model",
  authIdempotency: "Auth & Idempotency",
  observability: "Observability & Performance",
  curlExample: "cURL Example",
  requestExample: "Request Example",
  responseExample: "Response Example",
  errorExample: "Error Example",
  notes: "Notes",
  changelog: "Changelog",
}

interface FunctionDocsProps {
  func: FunctionData
}

export function FunctionDocs({ func: fn }: FunctionDocsProps) {
  const [docs, setDocs] = useState<GenerateDocsResponse | null>(null)
  const [savedDocs, setSavedDocs] = useState<GenerateDocsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hiddenSections, setHiddenSections] = useState<Set<DocSection>>(new Set())
  const [editingField, setEditingField] = useState<string | null>(null)
  const [editValue, setEditValue] = useState("")
  const [hasChanges, setHasChanges] = useState(false)

  const fetchDocs = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const record = await functionDocsApi.get(fn.name)
      if (record && record.doc_content) {
        setDocs(record.doc_content)
        setSavedDocs(record.doc_content)
        // Restore hidden sections from saved data
        const hidden = new Set<DocSection>()
        const content = record.doc_content as GenerateDocsResponse & { _hidden_sections?: DocSection[] }
        if (content._hidden_sections) {
          content._hidden_sections.forEach((s: DocSection) => hidden.add(s))
        }
        setHiddenSections(hidden)
      }
    } catch {
      // No saved docs - that's fine
      setDocs(null)
      setSavedDocs(null)
    } finally {
      setLoading(false)
    }
  }, [fn.name])

  useEffect(() => {
    fetchDocs()
  }, [fetchDocs])

  const handleGenerate = async () => {
    try {
      setGenerating(true)
      setError(null)

      let code = ""
      try {
        const codeResp = await functionsApi.getCode(fn.name)
        code = codeResp.source_code || ""
      } catch {
        code = "// Source code not available"
      }

      const generated = await apiDocsApi.generateDocs({
        function_name: fn.name,
        runtime: fn.runtime,
        code: code,
        handler: fn.handler || "handler",
      })
      setDocs(generated)
      setHiddenSections(new Set())
      setHasChanges(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to generate docs")
    } finally {
      setGenerating(false)
    }
  }

  const handleSave = async () => {
    if (!docs) return
    try {
      setSaving(true)
      setError(null)
      // Store hidden sections in the doc content for persistence
      const toSave = {
        ...docs,
        _hidden_sections: Array.from(hiddenSections),
      } as GenerateDocsResponse
      await functionDocsApi.save(fn.name, toSave)
      setSavedDocs(toSave)
      setHasChanges(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save docs")
    } finally {
      setSaving(false)
    }
  }

  const handleDeleteAll = async () => {
    try {
      setError(null)
      await functionDocsApi.delete(fn.name)
      setDocs(null)
      setSavedDocs(null)
      setHiddenSections(new Set())
      setHasChanges(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete docs")
    }
  }

  const handleRevert = () => {
    if (savedDocs) {
      setDocs(savedDocs)
      const hidden = new Set<DocSection>()
      const content = savedDocs as GenerateDocsResponse & { _hidden_sections?: DocSection[] }
      if (content._hidden_sections) {
        content._hidden_sections.forEach((s: DocSection) => hidden.add(s))
      }
      setHiddenSections(hidden)
    }
    setHasChanges(false)
  }

  const toggleSection = (section: DocSection) => {
    const next = new Set(hiddenSections)
    if (next.has(section)) {
      next.delete(section)
    } else {
      next.add(section)
    }
    setHiddenSections(next)
    setHasChanges(true)
  }

  const startEdit = (field: string, value: string) => {
    setEditingField(field)
    setEditValue(value)
  }

  const saveEdit = (field: string) => {
    if (!docs) return
    const updated = { ...docs }

    // Handle nested fields
    const parts = field.split(".")
    if (parts.length === 1) {
      ;(updated as Record<string, unknown>)[field] = editValue
    } else if (parts[0] === "error_model") {
      updated.error_model = { ...updated.error_model, [parts[1]]: editValue }
    }

    setDocs(updated)
    setEditingField(null)
    setEditValue("")
    setHasChanges(true)
  }

  const cancelEdit = () => {
    setEditingField(null)
    setEditValue("")
  }

  const isVisible = (section: DocSection) => !hiddenSections.has(section)

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // No docs yet - show generate prompt
  if (!docs) {
    return (
      <div className="rounded-xl border border-border bg-card p-8">
        <div className="flex flex-col items-center justify-center text-center space-y-4">
          <FileText className="h-12 w-12 text-muted-foreground/50" />
          <div>
            <h3 className="text-lg font-semibold">No Documentation Yet</h3>
            <p className="text-sm text-muted-foreground mt-1">
              Generate AI-powered API documentation for this function
            </p>
          </div>
          {error && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-destructive text-sm w-full max-w-md">
              {error}
            </div>
          )}
          <Button onClick={handleGenerate} disabled={generating} size="lg">
            {generating ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Generating...
              </>
            ) : (
              <>
                <Sparkles className="mr-2 h-4 w-4" />
                Generate Documentation
              </>
            )}
          </Button>
        </div>
      </div>
    )
  }

  // Inline editable text component
  const EditableText = ({
    field,
    value,
    className,
    multiline,
  }: {
    field: string
    value: string
    className?: string
    multiline?: boolean
  }) => {
    if (editingField === field) {
      return (
        <div className="flex items-center gap-1">
          {multiline ? (
            <Textarea
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              className="min-h-[60px] text-sm"
              autoFocus
            />
          ) : (
            <Input
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              className="h-7 text-sm"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter") saveEdit(field)
                if (e.key === "Escape") cancelEdit()
              }}
            />
          )}
          <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={() => saveEdit(field)}>
            <Check className="h-3 w-3" />
          </Button>
          <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={cancelEdit}>
            <X className="h-3 w-3" />
          </Button>
        </div>
      )
    }
    return (
      <span
        className={cn("cursor-pointer hover:bg-muted/50 rounded px-1 -mx-1 transition-colors group", className)}
        onClick={() => startEdit(field, value)}
        title="Click to edit"
      >
        {value}
        <Pencil className="inline-block ml-1 h-3 w-3 opacity-0 group-hover:opacity-50 transition-opacity" />
      </span>
    )
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <FileText className="h-5 w-5 text-primary" />
          <h2 className="text-lg font-semibold">{docs.name}</h2>
          {docs.stability && <Badge variant="secondary">{docs.stability}</Badge>}
          {docs.version && <Badge variant="outline">{docs.version}</Badge>}
        </div>
        <div className="flex items-center gap-2">
          {hasChanges && (
            <Button variant="ghost" size="sm" onClick={handleRevert} title="Revert changes">
              <RotateCcw className="mr-2 h-4 w-4" />
              Revert
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={handleGenerate}
            disabled={generating}
            title="Regenerate documentation"
          >
            {generating ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Sparkles className="mr-2 h-4 w-4" />
            )}
            Regenerate
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving || !hasChanges}
          >
            {saving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            Save
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={handleDeleteAll}
            title="Delete all documentation"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-destructive text-sm">
          {error}
        </div>
      )}

      {/* Section visibility toggles */}
      <div className="rounded-lg border border-border bg-muted/20 p-3">
        <p className="text-xs text-muted-foreground mb-2">Toggle sections (click to show/hide):</p>
        <div className="flex flex-wrap gap-1.5">
          {ALL_SECTIONS.map((section) => (
            <button
              key={section}
              onClick={() => toggleSection(section)}
              className={cn(
                "px-2 py-0.5 rounded text-xs border transition-colors",
                isVisible(section)
                  ? "bg-primary text-primary-foreground border-primary"
                  : "border-border text-muted-foreground hover:bg-accent hover:text-accent-foreground line-through"
              )}
            >
              {SECTION_LABELS[section]}
            </button>
          ))}
        </div>
      </div>

      {/* Summary */}
      <div className="rounded-xl border border-border bg-card p-6 space-y-6">
        <p className="text-muted-foreground">
          <EditableText field="summary" value={docs.summary} multiline />
        </p>

        {/* Meta Section */}
        {isVisible("meta") && (
          <div className="space-y-2">
            <SectionHeader section="meta" onDelete={() => toggleSection("meta")} />
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">Protocol</span>
                <p className="text-sm font-medium">
                  <EditableText field="protocol" value={docs.protocol} />
                </p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">Auth</span>
                <p className="text-sm font-medium">
                  <EditableText field="auth_method" value={docs.auth_method} />
                </p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">Rate Limit</span>
                <p className="text-sm font-medium">
                  <EditableText field="rate_limit" value={docs.rate_limit} />
                </p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">Timeout</span>
                <p className="text-sm font-medium">
                  <EditableText field="timeout" value={docs.timeout} />
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Endpoint */}
        {isVisible("endpoint") && (
          <div className="space-y-2">
            <SectionHeader section="endpoint" onDelete={() => toggleSection("endpoint")} />
            <div className="flex items-center gap-2">
              <Badge>{docs.method}</Badge>
              <code className="rounded bg-muted px-2 py-1 text-sm">
                <EditableText field="path" value={docs.path} />
              </code>
            </div>
          </div>
        )}

        {/* Request Fields */}
        {isVisible("requestFields") && docs.request_fields && docs.request_fields.length > 0 && (
          <div className="space-y-2">
            <SectionHeader section="requestFields" onDelete={() => toggleSection("requestFields")} />
            <div className="rounded-lg border overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-3 py-2 text-left font-medium">Field</th>
                    <th className="px-3 py-2 text-left font-medium">Type</th>
                    <th className="px-3 py-2 text-left font-medium">Required</th>
                    <th className="px-3 py-2 text-left font-medium">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {docs.request_fields.map((f, i) => (
                    <tr key={i} className="border-b">
                      <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                      <td className="px-3 py-2">
                        <Badge variant="outline" className="text-xs">{f.type}</Badge>
                      </td>
                      <td className="px-3 py-2">
                        {f.required ? (
                          <Badge variant="default" className="text-xs">Required</Badge>
                        ) : (
                          <span className="text-muted-foreground text-xs">Optional</span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">{f.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Response Fields */}
        {isVisible("responseFields") && docs.response_fields && docs.response_fields.length > 0 && (
          <div className="space-y-2">
            <SectionHeader section="responseFields" onDelete={() => toggleSection("responseFields")} />
            <div className="rounded-lg border overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-3 py-2 text-left font-medium">Field</th>
                    <th className="px-3 py-2 text-left font-medium">Type</th>
                    <th className="px-3 py-2 text-left font-medium">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {docs.response_fields.map((f, i) => (
                    <tr key={i} className="border-b">
                      <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                      <td className="px-3 py-2">
                        <Badge variant="outline" className="text-xs">{f.type}</Badge>
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">{f.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Status Codes */}
        {isVisible("statusCodes") && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {docs.success_codes && docs.success_codes.length > 0 && (
              <div className="space-y-2">
                <SectionHeader section="statusCodes" onDelete={() => toggleSection("statusCodes")} />
                <div className="space-y-1">
                  {docs.success_codes.map((c, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <Badge variant="secondary" className="bg-success/10 text-success border-0">
                        {c.code}
                      </Badge>
                      <span className="text-sm text-muted-foreground">{c.meaning}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
            {docs.error_codes && docs.error_codes.length > 0 && (
              <div className="space-y-2">
                <h3 className="font-semibold">Error Codes</h3>
                <div className="space-y-1">
                  {docs.error_codes.map((c, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <Badge variant="secondary" className="bg-destructive/10 text-destructive border-0">
                        {c.code}
                      </Badge>
                      <span className="text-sm text-muted-foreground">{c.meaning}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Error Model */}
        {isVisible("errorModel") && docs.error_model && (
          <div className="space-y-2">
            <SectionHeader section="errorModel" onDelete={() => toggleSection("errorModel")} />
            <div className="rounded-lg border bg-muted/20 p-4 space-y-2 text-sm">
              <p>
                <span className="font-medium">Format:</span>{" "}
                <EditableText field="error_model.format" value={docs.error_model.format} />
              </p>
              <p>
                <span className="font-medium">Retryable:</span>{" "}
                <EditableText field="error_model.retryable" value={docs.error_model.retryable} />
              </p>
              <p>
                <span className="font-medium">Description:</span>{" "}
                <EditableText field="error_model.description" value={docs.error_model.description} />
              </p>
            </div>
          </div>
        )}

        {/* Security & Idempotency */}
        {isVisible("authIdempotency") && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <SectionHeader section="authIdempotency" onDelete={() => toggleSection("authIdempotency")} />
              <div className="rounded-lg border bg-muted/20 p-4 space-y-1 text-sm">
                <p>
                  <span className="font-medium">Method:</span>{" "}
                  <EditableText field="auth_method" value={docs.auth_method} />
                </p>
                {docs.roles_required && docs.roles_required.length > 0 && (
                  <p>
                    <span className="font-medium">Roles:</span> {docs.roles_required.join(", ")}
                  </p>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <h3 className="font-semibold">Idempotency</h3>
              <div className="rounded-lg border bg-muted/20 p-4 space-y-1 text-sm">
                <p>
                  <span className="font-medium">Idempotent:</span>{" "}
                  {docs.idempotent ? "Yes" : "No"}
                </p>
                {docs.idempotent_key && (
                  <p>
                    <span className="font-medium">Key:</span>{" "}
                    <EditableText field="idempotent_key" value={docs.idempotent_key} />
                  </p>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Observability */}
        {isVisible("observability") && (
          <div className="space-y-2">
            <SectionHeader section="observability" onDelete={() => toggleSection("observability")} />
            <div className="rounded-lg border bg-muted/20 p-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              <div>
                <span className="font-medium">Tracing:</span> {docs.supports_tracing ? "Yes" : "No"}
              </div>
              <div>
                <span className="font-medium">Rate Limit:</span>{" "}
                <EditableText field="rate_limit" value={docs.rate_limit} />
              </div>
              <div>
                <span className="font-medium">Timeout:</span>{" "}
                <EditableText field="timeout" value={docs.timeout} />
              </div>
              <div>
                <span className="font-medium">Pagination:</span>{" "}
                <EditableText field="pagination" value={docs.pagination || "N/A"} />
              </div>
            </div>
          </div>
        )}

        {/* Examples */}
        {isVisible("curlExample") && docs.curl_example && (
          <div className="space-y-2">
            <SectionHeader section="curlExample" onDelete={() => toggleSection("curlExample")} />
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto">
              <code>{docs.curl_example}</code>
            </pre>
          </div>
        )}
        {isVisible("requestExample") && docs.request_example && (
          <div className="space-y-2">
            <SectionHeader section="requestExample" onDelete={() => toggleSection("requestExample")} />
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto">
              <code>{docs.request_example}</code>
            </pre>
          </div>
        )}
        {isVisible("responseExample") && docs.response_example && (
          <div className="space-y-2">
            <SectionHeader section="responseExample" onDelete={() => toggleSection("responseExample")} />
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto">
              <code>{docs.response_example}</code>
            </pre>
          </div>
        )}
        {isVisible("errorExample") && docs.error_example && (
          <div className="space-y-2">
            <SectionHeader section="errorExample" onDelete={() => toggleSection("errorExample")} />
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto">
              <code>{docs.error_example}</code>
            </pre>
          </div>
        )}

        {/* Notes */}
        {isVisible("notes") && docs.notes && docs.notes.length > 0 && (
          <div className="space-y-2">
            <SectionHeader section="notes" onDelete={() => toggleSection("notes")} />
            <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
              {docs.notes.map((note, i) => (
                <li key={i}>{note}</li>
              ))}
            </ul>
          </div>
        )}

        {/* Changelog */}
        {isVisible("changelog") && docs.changelog && docs.changelog.length > 0 && (
          <div className="space-y-2">
            <SectionHeader section="changelog" onDelete={() => toggleSection("changelog")} />
            <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
              {docs.changelog.map((entry, i) => (
                <li key={i}>{entry}</li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  )
}

function SectionHeader({
  section,
  onDelete,
}: {
  section: DocSection
  onDelete: () => void
}) {
  return (
    <div className="flex items-center justify-between group">
      <h3 className="font-semibold">{SECTION_LABELS[section]}</h3>
      <Button
        variant="ghost"
        size="sm"
        className="h-6 w-6 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
        onClick={onDelete}
        title={`Hide ${SECTION_LABELS[section]}`}
      >
        <X className="h-3.5 w-3.5 text-muted-foreground hover:text-destructive" />
      </Button>
    </div>
  )
}

"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  functionsApi,
  apiDocsApi,
  type NovaFunction,
  type GenerateDocsResponse,
  type APIDocShare,
} from "@/lib/api"
import {
  FileText,
  Sparkles,
  Share2,
  Trash2,
  RefreshCw,
  Copy,
  Check,
  ExternalLink,
  Clock,
  Eye,
  Loader2,
} from "lucide-react"
import { cn } from "@/lib/utils"

export default function APIDocsPage() {
  const t = useTranslations("pages")
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [selectedFunction, setSelectedFunction] = useState<string>("")
  const [generatedDocs, setGeneratedDocs] = useState<GenerateDocsResponse | null>(null)
  const [shares, setShares] = useState<APIDocShare[]>([])
  const [generating, setGenerating] = useState(false)
  const [sharesLoading, setSharesLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [shareDialogOpen, setShareDialogOpen] = useState(false)
  const [shareTitle, setShareTitle] = useState("")
  const [shareExpiry, setShareExpiry] = useState("")
  const [creatingShare, setCreatingShare] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)

  const fetchFunctions = useCallback(async () => {
    try {
      const data = await functionsApi.list()
      setFunctions(data || [])
    } catch {
      // ignore
    }
  }, [])

  const fetchShares = useCallback(async () => {
    try {
      setSharesLoading(true)
      const data = await apiDocsApi.listShares()
      setShares(data || [])
    } catch {
      // ignore
    } finally {
      setSharesLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchFunctions()
    fetchShares()
  }, [fetchFunctions, fetchShares])

  const handleGenerate = async () => {
    if (!selectedFunction) return
    const fn = functions.find((f) => f.name === selectedFunction)
    if (!fn) return

    try {
      setGenerating(true)
      setError(null)

      let code = fn.source_code || ""
      if (!code) {
        try {
          const codeResp = await functionsApi.getCode(fn.name)
          code = codeResp.source_code || ""
        } catch {
          code = `// ${t("apiDocs.sourceCodeNotAvailable")}`
        }
      }

      const docs = await apiDocsApi.generateDocs({
        function_name: fn.name,
        runtime: fn.runtime,
        code: code,
        handler: fn.handler,
      })
      setGeneratedDocs(docs)
    } catch (err) {
      setError(err instanceof Error ? err.message : t("apiDocs.generateFailed"))
    } finally {
      setGenerating(false)
    }
  }

  const handleCreateShare = async () => {
    if (!generatedDocs || !shareTitle.trim()) return
    try {
      setCreatingShare(true)
      await apiDocsApi.createShare({
        function_name: selectedFunction,
        title: shareTitle.trim(),
        doc_content: generatedDocs,
        expires_in: shareExpiry || undefined,
      })
      setShareDialogOpen(false)
      setShareTitle("")
      setShareExpiry("")
      fetchShares()
    } catch (err) {
      setError(err instanceof Error ? err.message : t("apiDocs.createShareFailed"))
    } finally {
      setCreatingShare(false)
    }
  }

  const handleDeleteShare = async (id: string) => {
    try {
      await apiDocsApi.deleteShare(id)
      fetchShares()
    } catch (err) {
      setError(err instanceof Error ? err.message : t("apiDocs.deleteShareFailed"))
    }
  }

  const handleCopy = async (text: string, id: string) => {
    await navigator.clipboard.writeText(window.location.origin + "/api-docs/shared/" + text)
    setCopied(id)
    setTimeout(() => setCopied(null), 2000)
  }

  return (
    <DashboardLayout>
      <Header title={t("apiDocs.title")} description={t("apiDocs.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        {/* Generate Docs Section */}
        <div className="rounded-xl border border-border bg-card p-6 space-y-4">
          <div className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-primary" />
            <h2 className="text-lg font-semibold">{t("apiDocs.generateTitle")}</h2>
          </div>
          <p className="text-sm text-muted-foreground">{t("apiDocs.generateDescription")}</p>
          <div className="flex items-end gap-4">
            <div className="flex-1 space-y-2">
              <label className="text-sm font-medium">{t("apiDocs.selectFunction")}</label>
              <Select value={selectedFunction} onValueChange={setSelectedFunction}>
                <SelectTrigger>
                  <SelectValue placeholder={t("apiDocs.selectFunctionPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {functions.map((fn) => (
                    <SelectItem key={fn.name} value={fn.name}>
                      {fn.name} ({fn.runtime})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button onClick={handleGenerate} disabled={!selectedFunction || generating}>
              {generating ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t("apiDocs.generating")}
                </>
              ) : (
                <>
                  <Sparkles className="mr-2 h-4 w-4" />
                  {t("apiDocs.generate")}
                </>
              )}
            </Button>
          </div>
        </div>

        {/* Generated Docs Display */}
        {generatedDocs && (
          <div className="rounded-xl border border-border bg-card p-6 space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <FileText className="h-5 w-5 text-primary" />
                <h2 className="text-lg font-semibold">{generatedDocs.name}</h2>
                <Badge variant="secondary">{generatedDocs.stability}</Badge>
                <Badge variant="outline">{generatedDocs.version}</Badge>
              </div>
              <Dialog open={shareDialogOpen} onOpenChange={setShareDialogOpen}>
                <DialogTrigger asChild>
                  <Button variant="outline" size="sm">
                    <Share2 className="mr-2 h-4 w-4" />
                    {t("apiDocs.share")}
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>{t("apiDocs.createShareLink")}</DialogTitle>
                  </DialogHeader>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{t("apiDocs.shareTitle")}</label>
                      <Input
                        value={shareTitle}
                        onChange={(e) => setShareTitle(e.target.value)}
                        placeholder={t("apiDocs.shareTitlePlaceholder")}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{t("apiDocs.expiry")}</label>
                      <Select value={shareExpiry} onValueChange={setShareExpiry}>
                        <SelectTrigger>
                          <SelectValue placeholder={t("apiDocs.noExpiration")} />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="">{t("apiDocs.noExpiration")}</SelectItem>
                          <SelectItem value="24h">{t("apiDocs.expiry24Hours")}</SelectItem>
                          <SelectItem value="168h">{t("apiDocs.expiry7Days")}</SelectItem>
                          <SelectItem value="720h">{t("apiDocs.expiry30Days")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <Button
                      className="w-full"
                      onClick={handleCreateShare}
                      disabled={creatingShare || !shareTitle.trim()}
                    >
                      {creatingShare ? t("apiDocs.creating") : t("apiDocs.createLink")}
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>
            </div>

            <p className="text-muted-foreground">{generatedDocs.summary}</p>

            {/* Meta Section */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">{t("apiDocs.protocol")}</span>
                <p className="text-sm font-medium">{generatedDocs.protocol}</p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">{t("apiDocs.auth")}</span>
                <p className="text-sm font-medium">{generatedDocs.auth_method}</p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">{t("apiDocs.rateLimit")}</span>
                <p className="text-sm font-medium">{generatedDocs.rate_limit}</p>
              </div>
              <div className="space-y-1">
                <span className="text-xs text-muted-foreground">{t("apiDocs.timeout")}</span>
                <p className="text-sm font-medium">{generatedDocs.timeout}</p>
              </div>
            </div>

            {/* Endpoint */}
            <div className="space-y-2">
              <h3 className="font-semibold">{t("apiDocs.endpoint")}</h3>
              <div className="flex items-center gap-2">
                <Badge>{generatedDocs.method}</Badge>
                <code className="rounded bg-muted px-2 py-1 text-sm">{generatedDocs.path}</code>
              </div>
            </div>

            {/* Request Fields */}
            {generatedDocs.request_fields && generatedDocs.request_fields.length > 0 && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.requestFields")}</h3>
                <div className="rounded-lg border overflow-hidden">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.field")}</th>
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.type")}</th>
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.required")}</th>
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.descriptionLabel")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {generatedDocs.request_fields.map((f, i) => (
                        <tr key={i} className="border-b">
                          <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                          <td className="px-3 py-2"><Badge variant="outline" className="text-xs">{f.type}</Badge></td>
                          <td className="px-3 py-2">
                            {f.required ? (
                              <Badge variant="default" className="text-xs">{t("apiDocs.required")}</Badge>
                            ) : (
                              <span className="text-muted-foreground text-xs">{t("apiDocs.optional")}</span>
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
            {generatedDocs.response_fields && generatedDocs.response_fields.length > 0 && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.responseFields")}</h3>
                <div className="rounded-lg border overflow-hidden">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.field")}</th>
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.type")}</th>
                        <th className="px-3 py-2 text-left font-medium">{t("apiDocs.descriptionLabel")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {generatedDocs.response_fields.map((f, i) => (
                        <tr key={i} className="border-b">
                          <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                          <td className="px-3 py-2"><Badge variant="outline" className="text-xs">{f.type}</Badge></td>
                          <td className="px-3 py-2 text-muted-foreground">{f.description}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {/* Status Codes */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {generatedDocs.success_codes && generatedDocs.success_codes.length > 0 && (
                <div className="space-y-2">
                  <h3 className="font-semibold">{t("apiDocs.successCodes")}</h3>
                  <div className="space-y-1">
                    {generatedDocs.success_codes.map((c, i) => (
                      <div key={i} className="flex items-center gap-2">
                        <Badge variant="secondary" className="bg-success/10 text-success border-0">{c.code}</Badge>
                        <span className="text-sm text-muted-foreground">{c.meaning}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {generatedDocs.error_codes && generatedDocs.error_codes.length > 0 && (
                <div className="space-y-2">
                  <h3 className="font-semibold">{t("apiDocs.errorCodes")}</h3>
                  <div className="space-y-1">
                    {generatedDocs.error_codes.map((c, i) => (
                      <div key={i} className="flex items-center gap-2">
                        <Badge variant="secondary" className="bg-destructive/10 text-destructive border-0">{c.code}</Badge>
                        <span className="text-sm text-muted-foreground">{c.meaning}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Error Model */}
            {generatedDocs.error_model && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.errorModel")}</h3>
                <div className="rounded-lg border bg-muted/20 p-4 space-y-2 text-sm">
                  <p><span className="font-medium">{t("apiDocs.format")}:</span> {generatedDocs.error_model.format}</p>
                  <p><span className="font-medium">{t("apiDocs.retryable")}:</span> {generatedDocs.error_model.retryable}</p>
                  <p><span className="font-medium">{t("apiDocs.descriptionLabel")}:</span> {generatedDocs.error_model.description}</p>
                </div>
              </div>
            )}

            {/* Security & Idempotency */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.authAndAuthorization")}</h3>
                <div className="rounded-lg border bg-muted/20 p-4 space-y-1 text-sm">
                  <p><span className="font-medium">{t("apiDocs.method")}:</span> {generatedDocs.auth_method}</p>
                  {generatedDocs.roles_required && generatedDocs.roles_required.length > 0 && (
                    <p><span className="font-medium">{t("apiDocs.roles")}:</span> {generatedDocs.roles_required.join(", ")}</p>
                  )}
                </div>
              </div>
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.idempotency")}</h3>
                <div className="rounded-lg border bg-muted/20 p-4 space-y-1 text-sm">
                  <p><span className="font-medium">{t("apiDocs.idempotent")}:</span> {generatedDocs.idempotent ? t("apiDocs.yes") : t("apiDocs.no")}</p>
                  {generatedDocs.idempotent_key && (
                    <p><span className="font-medium">{t("apiDocs.key")}:</span> {generatedDocs.idempotent_key}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Observability */}
            <div className="space-y-2">
              <h3 className="font-semibold">{t("apiDocs.observabilityAndPerformance")}</h3>
              <div className="rounded-lg border bg-muted/20 p-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div><span className="font-medium">{t("apiDocs.tracing")}:</span> {generatedDocs.supports_tracing ? t("apiDocs.yes") : t("apiDocs.no")}</div>
                <div><span className="font-medium">{t("apiDocs.rateLimit")}:</span> {generatedDocs.rate_limit}</div>
                <div><span className="font-medium">{t("apiDocs.timeout")}:</span> {generatedDocs.timeout}</div>
                <div><span className="font-medium">{t("apiDocs.pagination")}:</span> {generatedDocs.pagination || t("apiDocs.na")}</div>
              </div>
            </div>

            {/* Examples */}
            {generatedDocs.curl_example && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.curlExample")}</h3>
                <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{generatedDocs.curl_example}</code></pre>
              </div>
            )}
            {generatedDocs.request_example && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.requestExample")}</h3>
                <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{generatedDocs.request_example}</code></pre>
              </div>
            )}
            {generatedDocs.response_example && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.responseExample")}</h3>
                <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{generatedDocs.response_example}</code></pre>
              </div>
            )}
            {generatedDocs.error_example && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.errorExample")}</h3>
                <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{generatedDocs.error_example}</code></pre>
              </div>
            )}

            {/* Notes */}
            {generatedDocs.notes && generatedDocs.notes.length > 0 && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.notes")}</h3>
                <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
                  {generatedDocs.notes.map((note, i) => (
                    <li key={i}>{note}</li>
                  ))}
                </ul>
              </div>
            )}

            {/* Changelog */}
            {generatedDocs.changelog && generatedDocs.changelog.length > 0 && (
              <div className="space-y-2">
                <h3 className="font-semibold">{t("apiDocs.changelog")}</h3>
                <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
                  {generatedDocs.changelog.map((entry, i) => (
                    <li key={i}>{entry}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        )}

        {/* Shared Links Section */}
        <div className="rounded-xl border border-border bg-card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Share2 className="h-5 w-5 text-primary" />
              <h2 className="text-lg font-semibold">{t("apiDocs.sharedLinks")}</h2>
            </div>
            <Button variant="outline" size="sm" onClick={fetchShares} disabled={sharesLoading}>
              <RefreshCw className={cn("mr-2 h-4 w-4", sharesLoading && "animate-spin")} />
              {t("apiDocs.refresh")}
            </Button>
          </div>

          <div className="rounded-lg border overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("apiDocs.colTitle")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("apiDocs.colFunction")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("apiDocs.colExpires")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{t("apiDocs.colAccess")}</th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{t("apiDocs.colActions")}</th>
                </tr>
              </thead>
              <tbody>
                {sharesLoading ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td colSpan={5} className="px-4 py-3">
                        <div className="h-4 bg-muted rounded animate-pulse" />
                      </td>
                    </tr>
                  ))
                ) : shares.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                      <Share2 className="mx-auto h-8 w-8 mb-2 opacity-50" />
                      {t("apiDocs.noShares")}
                    </td>
                  </tr>
                ) : (
                  shares.map((share) => (
                    <tr key={share.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3">
                        <span className="font-medium text-sm">{share.title}</span>
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant="secondary" className="text-xs">{share.function_name}</Badge>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        <div className="flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {share.expires_at
                            ? new Date(share.expires_at).toLocaleDateString()
                            : t("apiDocs.neverExpires")}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        <div className="flex items-center gap-1">
                          <Eye className="h-3 w-3" />
                          {share.access_count}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleCopy(share.token, share.id)}
                            title={t("apiDocs.copyLink")}
                          >
                            {copied === share.id ? (
                              <Check className="h-4 w-4 text-success" />
                            ) : (
                              <Copy className="h-4 w-4" />
                            )}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => window.open("/api-docs/shared/" + share.token, "_blank")}
                            title={t("apiDocs.open")}
                          >
                            <ExternalLink className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDeleteShare(share.id)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </DashboardLayout>
  )
}

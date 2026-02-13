"use client"

import { useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { functionsApi, type InvokeResponse } from "@/lib/api"
import { Plus, Play, Trash2, Loader2, CheckCircle2, XCircle, Clock } from "lucide-react"

interface TestCase {
  id: string
  name: string
  input: string
  expectedOutput: string
  result?: {
    status: "pass" | "fail" | "error"
    output: string
    duration_ms: number
    error?: string
    request_id?: string
  }
  running?: boolean
}

export function FunctionTestSuite({ functionName }: { functionName: string }) {
  const t = useTranslations("testSuite")
  const [testCases, setTestCases] = useState<TestCase[]>([])
  const [runningAll, setRunningAll] = useState(false)

  const addTestCase = () => {
    setTestCases((prev) => [
      ...prev,
      {
        id: crypto.randomUUID(),
        name: `${t("testCase")} ${prev.length + 1}`,
        input: "{}",
        expectedOutput: "",
      },
    ])
  }

  const updateTestCase = (id: string, updates: Partial<TestCase>) => {
    setTestCases((prev) =>
      prev.map((tc) => (tc.id === id ? { ...tc, ...updates } : tc))
    )
  }

  const removeTestCase = (id: string) => {
    setTestCases((prev) => prev.filter((tc) => tc.id !== id))
  }

  const runSingleTest = useCallback(
    async (tc: TestCase) => {
      setTestCases((prev) =>
        prev.map((t) =>
          t.id === tc.id ? { ...t, running: true, result: undefined } : t
        )
      )

      try {
        let payload: unknown = {}
        try {
          payload = JSON.parse(tc.input)
        } catch {
          setTestCases((prev) =>
            prev.map((t) =>
              t.id === tc.id
                ? {
                    ...t,
                    running: false,
                    result: {
                      status: "error" as const,
                      output: "",
                      duration_ms: 0,
                      error: "Invalid JSON input",
                    },
                  }
                : t
            )
          )
          return
        }

        const response: InvokeResponse = await functionsApi.invoke(
          functionName,
          payload
        )
        const outputStr = JSON.stringify(response.output ?? null, null, 2)
        let status: "pass" | "fail" | "error" = "pass"

        if (response.error) {
          status = "error"
        } else if (tc.expectedOutput.trim()) {
          try {
            const expected = JSON.parse(tc.expectedOutput)
            const actual = response.output
            if (JSON.stringify(expected) !== JSON.stringify(actual)) {
              status = "fail"
            }
          } catch {
            if (tc.expectedOutput.trim() !== outputStr.trim()) {
              status = "fail"
            }
          }
        }

        setTestCases((prev) =>
          prev.map((t) =>
            t.id === tc.id
              ? {
                  ...t,
                  running: false,
                  result: {
                    status,
                    output: outputStr,
                    duration_ms: response.duration_ms,
                    error: response.error || undefined,
                    request_id: response.request_id,
                  },
                }
              : t
          )
        )
      } catch (err) {
        setTestCases((prev) =>
          prev.map((t) =>
            t.id === tc.id
              ? {
                  ...t,
                  running: false,
                  result: {
                    status: "error" as const,
                    output: "",
                    duration_ms: 0,
                    error:
                      err instanceof Error ? err.message : "Invocation failed",
                  },
                }
              : t
          )
        )
      }
    },
    [functionName]
  )

  const runAllTests = async () => {
    setRunningAll(true)
    for (const tc of testCases) {
      await runSingleTest(tc)
    }
    setRunningAll(false)
  }

  const passCount = testCases.filter((tc) => tc.result?.status === "pass").length
  const failCount = testCases.filter(
    (tc) => tc.result?.status === "fail" || tc.result?.status === "error"
  ).length
  const totalWithResults = testCases.filter((tc) => tc.result).length

  return (
    <div className="space-y-4">
      {/* Summary Bar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          {totalWithResults > 0 && (
            <>
              <Badge variant="secondary" className="bg-success/10 text-success border-0">
                <CheckCircle2 className="mr-1 h-3 w-3" />
                {passCount} {t("passed")}
              </Badge>
              {failCount > 0 && (
                <Badge variant="secondary" className="bg-destructive/10 text-destructive border-0">
                  <XCircle className="mr-1 h-3 w-3" />
                  {failCount} {t("failed")}
                </Badge>
              )}
              <span className="text-sm text-muted-foreground">
                {totalWithResults}/{testCases.length} {t("executed")}
              </span>
            </>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={addTestCase}>
            <Plus className="mr-2 h-4 w-4" />
            {t("addTestCase")}
          </Button>
          {testCases.length > 0 && (
            <Button
              size="sm"
              onClick={runAllTests}
              disabled={runningAll || testCases.length === 0}
            >
              {runningAll ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Play className="mr-2 h-4 w-4" />
              )}
              {t("runAll")}
            </Button>
          )}
        </div>
      </div>

      {/* Test Cases */}
      {testCases.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border bg-card py-12 text-muted-foreground">
          <p className="text-sm mb-3">{t("emptyDescription")}</p>
          <Button variant="outline" size="sm" onClick={addTestCase}>
            <Plus className="mr-2 h-4 w-4" />
            {t("addFirstTestCase")}
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {testCases.map((tc, index) => (
            <div
              key={tc.id}
              className={cn(
                "rounded-lg border bg-card overflow-hidden",
                tc.result?.status === "pass" && "border-success/30",
                tc.result?.status === "fail" && "border-destructive/30",
                tc.result?.status === "error" && "border-destructive/30",
                !tc.result && "border-border"
              )}
            >
              {/* Test Case Header */}
              <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-muted/30">
                <div className="flex items-center gap-2">
                  {tc.result?.status === "pass" && (
                    <CheckCircle2 className="h-4 w-4 text-success" />
                  )}
                  {(tc.result?.status === "fail" || tc.result?.status === "error") && (
                    <XCircle className="h-4 w-4 text-destructive" />
                  )}
                  {!tc.result && !tc.running && (
                    <div className="h-4 w-4 rounded-full border-2 border-muted-foreground/30" />
                  )}
                  {tc.running && (
                    <Loader2 className="h-4 w-4 animate-spin text-primary" />
                  )}
                  <Input
                    value={tc.name}
                    onChange={(e) =>
                      updateTestCase(tc.id, { name: e.target.value })
                    }
                    className="h-7 w-48 text-sm font-medium border-0 bg-transparent px-1 focus-visible:ring-0"
                    placeholder={`${t("testCase")} ${index + 1}`}
                  />
                  {tc.result && (
                    <span className="text-xs text-muted-foreground flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {tc.result.duration_ms}ms
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => runSingleTest(tc)}
                    disabled={tc.running || runningAll}
                  >
                    {tc.running ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Play className="h-3.5 w-3.5" />
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => removeTestCase(tc.id)}
                    disabled={tc.running}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-destructive" />
                  </Button>
                </div>
              </div>

              {/* Test Case Body */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-0 divide-y md:divide-y-0 md:divide-x divide-border">
                <div className="p-3">
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    {t("input")}
                  </label>
                  <Textarea
                    value={tc.input}
                    onChange={(e) =>
                      updateTestCase(tc.id, { input: e.target.value })
                    }
                    placeholder="{}"
                    className="min-h-[80px] font-mono text-xs resize-none"
                    disabled={tc.running}
                  />
                </div>
                <div className="p-3">
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    {t("expectedOutput")}
                  </label>
                  <Textarea
                    value={tc.expectedOutput}
                    onChange={(e) =>
                      updateTestCase(tc.id, {
                        expectedOutput: e.target.value,
                      })
                    }
                    placeholder={t("expectedOutputPlaceholder")}
                    className="min-h-[80px] font-mono text-xs resize-none"
                    disabled={tc.running}
                  />
                </div>
                <div className="p-3">
                  <label className="text-xs font-medium text-muted-foreground mb-1 block">
                    {t("actualOutput")}
                  </label>
                  <div
                    className={cn(
                      "min-h-[80px] rounded-md border bg-muted/50 p-2 font-mono text-xs whitespace-pre-wrap overflow-auto",
                      tc.result?.error && "text-destructive"
                    )}
                  >
                    {tc.running ? (
                      <span className="text-muted-foreground">{t("running")}</span>
                    ) : tc.result?.error ? (
                      tc.result.error
                    ) : tc.result?.output ? (
                      tc.result.output
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

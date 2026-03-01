"use client"

import { useState, useEffect, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Folder, File, ArrowUp, RefreshCw, Trash2, Save, Plus } from "lucide-react"
import { sandboxApi, type SandboxFileInfo } from "@/lib/api"

interface SandboxFileBrowserProps {
  sandboxId: string
}

export function SandboxFileBrowser({ sandboxId }: SandboxFileBrowserProps) {
  const t = useTranslations("pages.sandboxes.files")
  const [currentPath, setCurrentPath] = useState("/home/sandbox")
  const [entries, setEntries] = useState<SandboxFileInfo[]>([])
  const [fileContent, setFileContent] = useState<string | null>(null)
  const [editingContent, setEditingContent] = useState("")
  const [viewingFile, setViewingFile] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [newFileName, setNewFileName] = useState("")
  const [showNewFile, setShowNewFile] = useState(false)

  const loadDirectory = useCallback(async (path: string) => {
    setLoading(true)
    setError(null)
    setFileContent(null)
    setViewingFile(null)
    try {
      const res = await sandboxApi.fileReadOrList(sandboxId, path)
      if (res.entries) {
        setEntries(res.entries)
        setCurrentPath(path)
      } else if (res.content !== undefined) {
        setFileContent(res.content)
        setEditingContent(res.content)
        setViewingFile(path)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [sandboxId])

  useEffect(() => {
    loadDirectory(currentPath)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const navigateUp = () => {
    const parent = currentPath.split("/").slice(0, -1).join("/") || "/"
    loadDirectory(parent)
  }

  const handleEntry = (entry: SandboxFileInfo) => {
    if (entry.is_dir) {
      loadDirectory(entry.path)
    } else {
      loadDirectory(entry.path)
    }
  }

  const handleSave = async () => {
    if (!viewingFile) return
    try {
      await sandboxApi.fileWrite(sandboxId, viewingFile, editingContent)
      setFileContent(editingContent)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleDelete = async (path: string) => {
    try {
      await sandboxApi.fileDelete(sandboxId, path)
      loadDirectory(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleCreateFile = async () => {
    if (!newFileName.trim()) return
    const path = `${currentPath}/${newFileName}`.replace(/\/+/g, "/")
    try {
      await sandboxApi.fileWrite(sandboxId, path, "")
      setNewFileName("")
      setShowNewFile(false)
      loadDirectory(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const formatSize = (size: number) => {
    if (size < 1024) return `${size} B`
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
    return `${(size / 1024 / 1024).toFixed(1)} MB`
  }

  if (viewingFile && fileContent !== null) {
    return (
      <div className="flex h-full flex-col rounded-lg border border-border bg-card">
        <div className="flex items-center justify-between border-b border-border px-3 py-2">
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => loadDirectory(currentPath)}>
              ← {t("backToList")}
            </Button>
            <span className="text-xs font-mono text-muted-foreground">{viewingFile}</span>
          </div>
          <Button size="sm" onClick={handleSave}>
            <Save className="mr-1 h-3.5 w-3.5" />
            {t("save")}
          </Button>
        </div>
        <textarea
          value={editingContent}
          onChange={(e) => setEditingContent(e.target.value)}
          className="flex-1 resize-none bg-black/95 p-3 font-mono text-sm text-foreground outline-none"
          spellCheck={false}
        />
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col rounded-lg border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon-sm" onClick={navigateUp} title={t("up")} disabled={currentPath === "/"}>
            <ArrowUp className="h-4 w-4" />
          </Button>
          <span className="text-xs font-mono text-muted-foreground">{currentPath}</span>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon-sm" onClick={() => setShowNewFile(!showNewFile)} title={t("newFile")}>
            <Plus className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={() => loadDirectory(currentPath)} title={t("refresh")}>
            <RefreshCw className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {showNewFile && (
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Input
            placeholder={t("newFilePlaceholder")}
            value={newFileName}
            onChange={(e) => setNewFileName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleCreateFile()}
            className="h-8 text-sm"
            autoFocus
          />
          <Button size="sm" onClick={handleCreateFile}>{t("create")}</Button>
        </div>
      )}

      {error && (
        <div className="border-b border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t("loading")}</p>
        ) : entries.length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t("empty")}</p>
        ) : (
          <table className="w-full text-sm">
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.path} className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
                  <td className="px-3 py-2">
                    <button
                      onClick={() => handleEntry(entry)}
                      className="flex items-center gap-2 text-left hover:text-primary"
                    >
                      {entry.is_dir ? (
                        <Folder className="h-4 w-4 text-blue-400" />
                      ) : (
                        <File className="h-4 w-4 text-muted-foreground" />
                      )}
                      <span className="font-mono text-xs">{entry.name}</span>
                    </button>
                  </td>
                  <td className="px-3 py-2 text-right text-xs text-muted-foreground">
                    {entry.is_dir ? <Badge variant="outline">dir</Badge> : formatSize(entry.size)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {!entry.is_dir && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleDelete(entry.path)}
                        className="text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

"use client"

import { useState } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { SectionHeader } from "@/components/section-header"
import { SectionTableFrame } from "@/components/section-table-frame"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { cn } from "@/lib/utils"
import { schedulesApi, type ScheduleEntry } from "@/lib/api"
import { Plus, Pencil, Trash2, ToggleLeft, ToggleRight } from "lucide-react"

interface FunctionSchedulesProps {
  functionName: string
  schedules: ScheduleEntry[]
  onSchedulesChange: (schedules: ScheduleEntry[]) => void
}

export function FunctionSchedules({ functionName, schedules, onSchedulesChange }: FunctionSchedulesProps) {
  const t = useTranslations("functionDetailPage")
  const [schedDialogOpen, setSchedDialogOpen] = useState(false)
  const [newCron, setNewCron] = useState("")
  const [newSchedInput, setNewSchedInput] = useState("")
  const [creatingSchedule, setCreatingSchedule] = useState(false)
  const [editingSchedule, setEditingSchedule] = useState<ScheduleEntry | null>(null)
  const [editCron, setEditCron] = useState("")
  const [editDialogOpen, setEditDialogOpen] = useState(false)

  const cronPresets = [
    { label: t("schedules.presets.every1m"), value: "@every 1m" },
    { label: t("schedules.presets.every5m"), value: "@every 5m" },
    { label: t("schedules.presets.every15m"), value: "*/15 * * * *" },
    { label: t("schedules.presets.every30m"), value: "*/30 * * * *" },
    { label: t("schedules.presets.hourly"), value: "@hourly" },
    { label: t("schedules.presets.daily"), value: "@daily" },
    { label: t("schedules.presets.weekly"), value: "@weekly" },
  ]

  const refreshSchedules = async () => {
    const updated = await schedulesApi.list(functionName)
    onSchedulesChange(updated || [])
  }

  return (
    <div className="space-y-4">
      <Dialog open={schedDialogOpen} onOpenChange={setSchedDialogOpen}>
        <SectionHeader
          title={t("schedules.title")}
          description={t("schedules.description")}
          action={
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-3.5 w-3.5" />
                {t("schedules.createSchedule")}
              </Button>
            </DialogTrigger>
          }
        />
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("schedules.createTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">{t("schedules.cronExpression")}</label>
              <Input
                value={newCron}
                onChange={(e) => setNewCron(e.target.value)}
                placeholder={t("schedules.cronPlaceholder")}
              />
              <CronPresetPicker presets={cronPresets} value={newCron} onChange={setNewCron} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t("schedules.inputOptionalJson")}</label>
              <Textarea
                value={newSchedInput}
                onChange={(e) => setNewSchedInput(e.target.value)}
                placeholder={t("schedules.inputPlaceholder")}
                className="min-h-[80px] font-mono text-xs"
              />
            </div>
            <Button
              className="w-full"
              onClick={async () => {
                if (!newCron.trim()) return
                setCreatingSchedule(true)
                try {
                  let input: unknown = undefined
                  if (newSchedInput.trim()) {
                    input = JSON.parse(newSchedInput)
                  }
                  await schedulesApi.create(functionName, newCron.trim(), input)
                  setSchedDialogOpen(false)
                  setNewCron("")
                  setNewSchedInput("")
                  await refreshSchedules()
                } catch (err) {
                  console.error("Failed to create schedule:", err)
                } finally {
                  setCreatingSchedule(false)
                }
              }}
              disabled={creatingSchedule || !newCron.trim()}
            >
              {creatingSchedule ? t("schedules.creating") : t("schedules.create")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Edit Schedule Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={(open) => {
        setEditDialogOpen(open)
        if (!open) setEditingSchedule(null)
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("schedules.editTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">{t("schedules.cronExpression")}</label>
              <Input
                value={editCron}
                onChange={(e) => setEditCron(e.target.value)}
                placeholder={t("schedules.cronPlaceholder")}
              />
              <CronPresetPicker presets={cronPresets} value={editCron} onChange={setEditCron} />
            </div>
            <Button
              className="w-full"
              onClick={async () => {
                if (!editingSchedule || !editCron.trim()) return
                try {
                  await schedulesApi.updateCron(functionName, editingSchedule.id, editCron.trim())
                  setEditDialogOpen(false)
                  setEditingSchedule(null)
                  await refreshSchedules()
                } catch (err) {
                  console.error("Failed to update schedule:", err)
                }
              }}
              disabled={!editCron.trim() || editCron.trim() === editingSchedule?.cron_expression}
            >
              {t("schedules.save")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <SectionTableFrame>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/50">
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("schedules.table.colCron")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("schedules.table.colStatus")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("schedules.table.colLastRun")}</th>
              <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("schedules.table.colCreated")}</th>
              <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">{t("schedules.table.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {schedules.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                  {t("schedules.table.empty")}
                </td>
              </tr>
            ) : (
              schedules.map((s) => (
                <tr key={s.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-2.5">
                    <code className="font-mono text-xs">{s.cron_expression}</code>
                  </td>
                  <td className="px-4 py-2.5">
                    <Badge
                      variant="secondary"
                      className={cn(
                        "text-[10px]",
                        s.enabled
                          ? "bg-success/10 text-success border-0"
                          : "bg-muted text-muted-foreground border-0"
                      )}
                    >
                      {s.enabled ? t("schedules.statusActive") : t("schedules.statusDisabled")}
                    </Badge>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    {s.last_run_at ? new Date(s.last_run_at).toLocaleString() : t("schedules.never")}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    {new Date(s.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() => {
                          setEditingSchedule(s)
                          setEditCron(s.cron_expression)
                          setEditDialogOpen(true)
                        }}
                        title={t("actions.edit")}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={async () => {
                          await schedulesApi.toggle(functionName, s.id, !s.enabled)
                          await refreshSchedules()
                        }}
                        title={s.enabled ? t("actions.disable") : t("actions.enable")}
                      >
                        {s.enabled ? (
                          <ToggleRight className="h-3.5 w-3.5 text-success" />
                        ) : (
                          <ToggleLeft className="h-3.5 w-3.5 text-muted-foreground" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={async () => {
                          await schedulesApi.delete(functionName, s.id)
                          await refreshSchedules()
                        }}
                      >
                        <Trash2 className="h-3.5 w-3.5 text-destructive" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </SectionTableFrame>
    </div>
  )
}

function CronPresetPicker({
  presets,
  value,
  onChange,
}: {
  presets: { label: string; value: string }[]
  value: string
  onChange: (v: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {presets.map((preset) => (
        <button
          key={preset.value}
          type="button"
          className={cn(
            "px-2 py-0.5 rounded text-xs border transition-colors",
            value === preset.value
              ? "bg-primary text-primary-foreground border-primary"
              : "border-border text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          )}
          onClick={() => onChange(preset.value)}
        >
          {preset.label}
        </button>
      ))}
    </div>
  )
}

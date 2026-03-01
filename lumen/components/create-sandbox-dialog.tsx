"use client"

import { useState } from "react"
import { useTranslations } from "next-intl"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { CreateSandboxRequest } from "@/lib/api"

interface CreateSandboxDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (data: CreateSandboxRequest) => Promise<void>
}

const TEMPLATES = ["python", "node", "ubuntu", "go", "rust", "ruby"]

export function CreateSandboxDialog({ open, onOpenChange, onCreate }: CreateSandboxDialogProps) {
  const t = useTranslations("pages.sandboxes.createDialog")
  const [template, setTemplate] = useState("python")
  const [memoryMB, setMemoryMB] = useState(512)
  const [vcpus, setVcpus] = useState(1)
  const [timeoutS, setTimeoutS] = useState(3600)
  const [onIdleS, setOnIdleS] = useState(300)
  const [networkPolicy, setNetworkPolicy] = useState("restricted")
  const [creating, setCreating] = useState(false)

  const handleSubmit = async () => {
    setCreating(true)
    try {
      await onCreate({
        template,
        memory_mb: memoryMB,
        vcpus,
        timeout_s: timeoutS,
        on_idle_s: onIdleS,
        network_policy: networkPolicy,
      })
      onOpenChange(false)
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label>{t("template")}</Label>
            <Select value={template} onValueChange={setTemplate}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TEMPLATES.map((tmpl) => (
                  <SelectItem key={tmpl} value={tmpl}>
                    {tmpl}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t("memory")}</Label>
              <Input
                type="number"
                min={128}
                max={8192}
                value={memoryMB}
                onChange={(e) => setMemoryMB(Number(e.target.value))}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("vcpus")}</Label>
              <Input
                type="number"
                min={1}
                max={8}
                value={vcpus}
                onChange={(e) => setVcpus(Number(e.target.value))}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t("timeout")}</Label>
              <Input
                type="number"
                min={60}
                max={86400}
                value={timeoutS}
                onChange={(e) => setTimeoutS(Number(e.target.value))}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("idleTimeout")}</Label>
              <Input
                type="number"
                min={60}
                max={3600}
                value={onIdleS}
                onChange={(e) => setOnIdleS(Number(e.target.value))}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t("networkPolicy")}</Label>
            <Select value={networkPolicy} onValueChange={setNetworkPolicy}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="restricted">{t("networkRestricted")}</SelectItem>
                <SelectItem value="open">{t("networkOpen")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={creating}>
            {t("cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={creating}>
            {creating ? t("creating") : t("create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

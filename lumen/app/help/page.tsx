"use client"

import { useState } from "react"
import { useTranslations } from "next-intl"
import Image from "next/image"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { SubNav } from "@/components/sub-nav"
import {
  Code2,
  Network,
  RadioTower,
  Zap,
  GitBranch,
  Play,
  Clock3,
  History,
  Camera,
  Lock,
  KeyRound,
  Settings,
  Building2,
  ShieldCheck,
  HardDrive,
  Layers,
  Server,
  FileText,
  Bell,
  Activity,
  SlidersHorizontal,
  RotateCcw,
  Terminal,
  ChevronDown,
  ChevronRight,
  type LucideIcon,
} from "lucide-react"
import { cn } from "@/lib/utils"

interface StepInfo {
  textKey: string
  image?: string
}

interface FeatureGuide {
  key: string
  icon: LucideIcon
  steps: StepInfo[]
}

const featureGuides: FeatureGuide[] = [
  {
    key: "functions",
    icon: Code2,
    steps: [
      { textKey: "step1", image: "/help/functions.png" },
      { textKey: "step2", image: "/help/functions-create.png" },
      { textKey: "step3", image: "/help/functions-detail.png" },
      { textKey: "step4", image: "/help/functions-invoke.png" },
      { textKey: "step5", image: "/help/functions-logs.png" },
    ],
  },
  {
    key: "gateway",
    icon: Network,
    steps: [
      { textKey: "step1", image: "/help/gateway.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "events",
    icon: RadioTower,
    steps: [
      { textKey: "step1", image: "/help/events.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "triggers",
    icon: Zap,
    steps: [
      { textKey: "step1", image: "/help/triggers.png" },
      { textKey: "step2", image: "/help/triggers-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "workflows",
    icon: GitBranch,
    steps: [
      { textKey: "step1", image: "/help/workflows.png" },
      { textKey: "step2", image: "/help/workflows-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "runtimes",
    icon: Play,
    steps: [
      { textKey: "step1", image: "/help/runtimes.png" },
      { textKey: "step2", image: "/help/runtimes-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "asyncJobs",
    icon: Clock3,
    steps: [
      { textKey: "step1", image: "/help/asyncJobs.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "history",
    icon: History,
    steps: [
      { textKey: "step1", image: "/help/history.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "replay",
    icon: RotateCcw,
    steps: [
      { textKey: "step1", image: "/help/replay.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "snapshots",
    icon: Camera,
    steps: [
      { textKey: "step1", image: "/help/snapshots.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "tenancy",
    icon: Building2,
    steps: [
      { textKey: "step1", image: "/help/tenancy.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "rbac",
    icon: ShieldCheck,
    steps: [
      { textKey: "step1", image: "/help/rbac.png" },
      { textKey: "step2", image: "/help/rbac-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "secrets",
    icon: Lock,
    steps: [
      { textKey: "step1", image: "/help/secrets.png" },
      { textKey: "step2", image: "/help/secrets-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "apiKeys",
    icon: KeyRound,
    steps: [
      { textKey: "step1", image: "/help/apiKeys.png" },
      { textKey: "step2", image: "/help/apiKeys-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "configurations",
    icon: Settings,
    steps: [
      { textKey: "step1", image: "/help/configurations.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "layers",
    icon: Layers,
    steps: [
      { textKey: "step1", image: "/help/layers.png" },
      { textKey: "step2", image: "/help/layers-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "volumes",
    icon: HardDrive,
    steps: [
      { textKey: "step1", image: "/help/volumes.png" },
      { textKey: "step2", image: "/help/volumes-create.png" },
      { textKey: "step3" },
    ],
  },
  {
    key: "cluster",
    icon: Server,
    steps: [
      { textKey: "step1", image: "/help/cluster.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "tuning",
    icon: SlidersHorizontal,
    steps: [
      { textKey: "step1", image: "/help/tuning.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "notifications",
    icon: Bell,
    steps: [
      { textKey: "step1", image: "/help/notifications.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "alerts",
    icon: Activity,
    steps: [
      { textKey: "step1", image: "/help/alerts.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
  {
    key: "apiDocs",
    icon: FileText,
    steps: [
      { textKey: "step1", image: "/help/apiDocs.png" },
      { textKey: "step2" },
      { textKey: "step3" },
    ],
  },
]

function FeatureSection({ guide }: { guide: FeatureGuide }) {
  const t = useTranslations("helpPage")
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-3 p-5 text-left transition-colors hover:bg-muted/50"
      >
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted shrink-0">
          <guide.icon className="h-5 w-5 text-foreground" />
        </div>
        <div className="flex-1 min-w-0">
          <h3 className="font-semibold text-card-foreground">
            {t(`features.${guide.key}.title`)}
          </h3>
          <p className="text-sm text-muted-foreground truncate">
            {t(`features.${guide.key}.desc`)}
          </p>
        </div>
        {expanded ? (
          <ChevronDown className="h-5 w-5 text-muted-foreground shrink-0" />
        ) : (
          <ChevronRight className="h-5 w-5 text-muted-foreground shrink-0" />
        )}
      </button>

      {expanded && (
        <div className="border-t border-border px-5 pb-5">
          <div className="space-y-6 pt-4">
            {guide.steps.map((step, idx) => (
              <div key={idx} className="space-y-3">
                <div className="flex items-start gap-3">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground">
                    {idx + 1}
                  </span>
                  <p className="text-sm leading-relaxed text-foreground pt-0.5">
                    {t(`features.${guide.key}.${step.textKey}`)}
                  </p>
                </div>
                {step.image && (
                  <div className="ml-10 overflow-hidden rounded-lg border border-border shadow-sm">
                    <Image
                      src={step.image}
                      alt={t(`features.${guide.key}.${step.textKey}`)}
                      width={1440}
                      height={900}
                      className="w-full h-auto"
                      unoptimized
                    />
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export default function HelpPage() {
  const t = useTranslations("helpPage")
  const tp = useTranslations("pages")

  return (
    <DashboardLayout>
      <Header title={t("title")} description={t("description")} />
      <div className="px-6 pt-4">
        <SubNav items={[
          { label: t("title"), href: "/help" },
          { label: tp("apiDocs.title"), href: "/api-docs" },
        ]} />
      </div>

      <div className="p-6 space-y-6">
        {/* Quick Start */}
        <section className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-center gap-3 mb-4">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
              <Terminal className="h-5 w-5 text-primary" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-card-foreground">{t("quickStart.title")}</h2>
              <p className="text-sm text-muted-foreground">{t("quickStart.subtitle")}</p>
            </div>
          </div>
          <ol className="list-decimal list-inside space-y-2 text-sm text-muted-foreground">
            <li>{t("quickStart.step1")}</li>
            <li>{t("quickStart.step2")}</li>
            <li>{t("quickStart.step3")}</li>
            <li>{t("quickStart.step4")}</li>
          </ol>
          <div className="mt-4 overflow-hidden rounded-lg border border-border shadow-sm">
            <Image
              src="/help/dashboard.png"
              alt="Dashboard"
              width={1440}
              height={900}
              className="w-full h-auto"
              unoptimized
            />
          </div>
        </section>

        {/* Feature Guides */}
        <div className="space-y-3">
          {featureGuides.map((guide) => (
            <FeatureSection key={guide.key} guide={guide} />
          ))}
        </div>
      </div>
    </DashboardLayout>
  )
}

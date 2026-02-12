"use client"

import { useTranslations } from "next-intl"
import Link from "next/link"
import { Gauge } from "lucide-react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { TenantSwitcher } from "@/components/tenant-switcher"

export default function TenancyPage() {
  const t = useTranslations("pages")
  return (
    <DashboardLayout>
      <Header
        title={t("tenancy.title")}
        description={t("tenancy.description")}
      />

      <div className="p-6 space-y-6">
        <TenantSwitcher />

        <Link
          href="/tenancy/quotas"
          className="flex items-center gap-3 rounded-xl border border-border bg-card p-4 transition-colors hover:bg-muted/50"
        >
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
            <Gauge className="h-5 w-5 text-primary" />
          </div>
          <div>
            <div className="text-sm font-medium text-foreground">{t("tenancy.quotas.title")}</div>
            <div className="text-xs text-muted-foreground">{t("tenancy.quotas.description")}</div>
          </div>
        </Link>

        <div className="rounded-xl border border-border bg-card p-4 text-sm text-muted-foreground">
          {t("tenancy.governanceMoved")}
          <span className="ml-1">
            {t("tenancy.selectTenantDetail")}{" "}
            <Link href="/tenancy/default" className="text-foreground underline underline-offset-4">
              {t("tenancy.detailView")}
            </Link>
          </span>
        </div>
      </div>
    </DashboardLayout>
  )
}

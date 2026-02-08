"use client"

import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { TenantSwitcher } from "@/components/tenant-switcher"

export default function TenancyPage() {
  return (
    <DashboardLayout>
      <Header
        title="Tenancy"
        description="Switch and manage tenants and namespaces"
      />

      <div className="p-6 space-y-6">
        <TenantSwitcher />
        <div className="rounded-xl border border-border bg-card p-4 text-sm text-muted-foreground">
          Tenant governance has moved to tenant detail pages.
          <span className="ml-1">
            Select a tenant and open <Link href="/tenancy/default" className="text-foreground underline underline-offset-4">detail view</Link>.
          </span>
        </div>
      </div>
    </DashboardLayout>
  )
}

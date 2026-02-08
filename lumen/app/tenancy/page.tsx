"use client"

import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { TenantSwitcher } from "@/components/tenant-switcher"
import { TenantGovernancePanel } from "@/components/tenant-governance-panel"

export default function TenancyPage() {
  return (
    <DashboardLayout>
      <Header
        title="Tenancy"
        description="Switch and manage tenants and namespaces"
      />

      <div className="p-6 space-y-6">
        <TenantSwitcher />
        <TenantGovernancePanel />
      </div>
    </DashboardLayout>
  )
}

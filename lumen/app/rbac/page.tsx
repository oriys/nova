"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { rbacApi } from "@/lib/api"
import type { RBACRole, RBACPermission, RBACRoleAssignment } from "@/lib/api"
import { ShieldCheck, Plus, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

type Tab = "roles" | "permissions" | "assignments"

export default function RBACPage() {
  const t = useTranslations("pages")
  const tr = useTranslations("rbacPage")
  const tc = useTranslations("common")
  const [tab, setTab] = useState<Tab>("roles")
  const [roles, setRoles] = useState<RBACRole[]>([])
  const [permissions, setPermissions] = useState<RBACPermission[]>([])
  const [assignments, setAssignments] = useState<RBACRoleAssignment[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  // Role form
  const [roleName, setRoleName] = useState("")
  // Permission form
  const [permCode, setPermCode] = useState("")
  const [permResource, setPermResource] = useState("")
  const [permAction, setPermAction] = useState("")
  const [permDesc, setPermDesc] = useState("")
  // Assignment form
  const [assignPrincipalType, setAssignPrincipalType] = useState("")
  const [assignPrincipalId, setAssignPrincipalId] = useState("")
  const [assignRoleId, setAssignRoleId] = useState("")
  const [assignScopeType, setAssignScopeType] = useState("")
  const [assignScopeId, setAssignScopeId] = useState("")

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      if (tab === "roles") {
        const data = await rbacApi.listRoles()
        setRoles(data || [])
      } else if (tab === "permissions") {
        const data = await rbacApi.listPermissions()
        setPermissions(data || [])
      } else {
        const data = await rbacApi.listRoleAssignments()
        setAssignments(data || [])
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tab, tr])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const resetForm = () => {
    setRoleName("")
    setPermCode("")
    setPermResource("")
    setPermAction("")
    setPermDesc("")
    setAssignPrincipalType("")
    setAssignPrincipalId("")
    setAssignRoleId("")
    setAssignScopeType("")
    setAssignScopeId("")
  }

  const handleCreate = async () => {
    try {
      setCreating(true)
      if (tab === "roles") {
        if (!roleName.trim()) return
        await rbacApi.createRole({ id: crypto.randomUUID(), name: roleName.trim() })
      } else if (tab === "permissions") {
        if (!permCode.trim()) return
        await rbacApi.createPermission({ id: crypto.randomUUID(), code: permCode.trim(), resource_type: permResource, action: permAction, description: permDesc })
      } else {
        if (!assignPrincipalType.trim() || !assignPrincipalId.trim() || !assignRoleId.trim() || !assignScopeType.trim()) return
        await rbacApi.createRoleAssignment({ id: crypto.randomUUID(), principal_type: assignPrincipalType.trim(), principal_id: assignPrincipalId.trim(), role_id: assignRoleId.trim(), scope_type: assignScopeType.trim(), scope_id: assignScopeId || undefined })
      }
      setDialogOpen(false)
      resetForm()
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      if (tab === "roles") await rbacApi.deleteRole(id)
      else if (tab === "permissions") await rbacApi.deletePermission(id)
      else await rbacApi.deleteRoleAssignment(id)
      fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToDelete"))
    }
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: "roles", label: tr("roles") },
    { key: "permissions", label: tr("permissions") },
    { key: "assignments", label: tr("assignments") },
  ]

  const createLabel = tab === "roles" ? tr("createRole") : tab === "permissions" ? tr("createPermission") : tr("createAssignment")

  return (
    <DashboardLayout>
      <Header title={t("rbac.title")} description={t("rbac.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center gap-2">
          {tabs.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={cn(
                "px-4 py-2 text-sm font-medium rounded-lg transition-colors",
                tab === t.key ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-muted/80"
              )}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div className="flex items-center justify-between">
          <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                {createLabel}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{createLabel}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                {tab === "roles" && (
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tr("name")}</label>
                    <Input value={roleName} onChange={(e) => setRoleName(e.target.value)} placeholder={tr("name")} />
                  </div>
                )}
                {tab === "permissions" && (
                  <>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("code")}</label>
                      <Input value={permCode} onChange={(e) => setPermCode(e.target.value)} placeholder={tr("code")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("resourceType")}</label>
                      <Input value={permResource} onChange={(e) => setPermResource(e.target.value)} placeholder={tr("resourceType")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("action")}</label>
                      <Input value={permAction} onChange={(e) => setPermAction(e.target.value)} placeholder={tr("action")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("description")}</label>
                      <Input value={permDesc} onChange={(e) => setPermDesc(e.target.value)} placeholder={tr("description")} />
                    </div>
                  </>
                )}
                {tab === "assignments" && (
                  <>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("principalType")}</label>
                      <Input value={assignPrincipalType} onChange={(e) => setAssignPrincipalType(e.target.value)} placeholder={tr("principalType")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("principalId")}</label>
                      <Input value={assignPrincipalId} onChange={(e) => setAssignPrincipalId(e.target.value)} placeholder={tr("principalId")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("roleId")}</label>
                      <Input value={assignRoleId} onChange={(e) => setAssignRoleId(e.target.value)} placeholder={tr("roleId")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("scopeType")}</label>
                      <Input value={assignScopeType} onChange={(e) => setAssignScopeType(e.target.value)} placeholder={tr("scopeType")} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("scopeId")}</label>
                      <Input value={assignScopeId} onChange={(e) => setAssignScopeId(e.target.value)} placeholder={tr("scopeId")} />
                    </div>
                  </>
                )}
                <Button className="w-full" onClick={handleCreate} disabled={creating}>
                  {creating ? tr("creating") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                {tab === "roles" && (
                  <>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("name")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("system")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                  </>
                )}
                {tab === "permissions" && (
                  <>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("code")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("resourceType")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("action")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("description")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                  </>
                )}
                {tab === "assignments" && (
                  <>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("principal")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("roleId")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("scope")}</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                  </>
                )}
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tc("actions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={6} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : tab === "roles" && roles.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-muted-foreground">
                    <ShieldCheck className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tr("noRoles")}
                  </td>
                </tr>
              ) : tab === "permissions" && permissions.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                    <ShieldCheck className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tr("noPermissions")}
                  </td>
                </tr>
              ) : tab === "assignments" && assignments.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                    <ShieldCheck className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tr("noAssignments")}
                  </td>
                </tr>
              ) : tab === "roles" ? (
                roles.map((role) => (
                  <tr key={role.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3 text-sm font-medium">{role.name}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{role.is_system ? "✓" : "—"}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{new Date(role.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <Button variant="ghost" size="sm" onClick={() => handleDelete(role.id)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))
              ) : tab === "permissions" ? (
                permissions.map((perm) => (
                  <tr key={perm.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3 text-sm font-medium font-mono">{perm.code}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{perm.resource_type}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{perm.action}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{perm.description}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{new Date(perm.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <Button variant="ghost" size="sm" onClick={() => handleDelete(perm.id)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))
              ) : (
                assignments.map((a) => (
                  <tr key={a.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3 text-sm font-medium">{a.principal_type}:{a.principal_id}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{a.role_id}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{a.scope_type}{a.scope_id ? `:${a.scope_id}` : ""}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{new Date(a.created_at).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-right">
                      <Button variant="ghost" size="sm" onClick={() => handleDelete(a.id)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </DashboardLayout>
  )
}

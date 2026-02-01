"use client"

import Link from "next/link"
import { cn } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { FunctionData } from "@/lib/types"
import {
  MoreHorizontal,
  Eye,
  Edit,
  Trash2,
  Play,
  Pause,
  Copy,
  Loader2,
} from "lucide-react"

interface FunctionsTableProps {
  functions: FunctionData[]
  onDelete: (name: string) => void
  loading?: boolean
}

export function FunctionsTable({ functions, onDelete, loading }: FunctionsTableProps) {
  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <Loader2 className="h-6 w-6 animate-spin mx-auto text-muted-foreground" />
        <p className="text-sm text-muted-foreground mt-2">Loading functions...</p>
      </div>
    )
  }

  if (functions.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-12 text-center">
        <p className="text-sm text-muted-foreground">
          No functions found. Create your first function to get started.
        </p>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-border bg-muted/30">
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Function
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Runtime
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Memory
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Invocations
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Errors
              </th>
              <th className="px-6 py-4 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Last Modified
              </th>
              <th className="px-6 py-4 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {functions.map((fn) => (
              <tr key={fn.id} className="hover:bg-muted/20 transition-colors">
                <td className="px-6 py-4">
                  <Link
                    href={`/functions/${fn.name}`}
                    className="group"
                  >
                    <p className="text-sm font-medium text-foreground group-hover:text-primary transition-colors">
                      {fn.name}
                    </p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      {fn.region}
                    </p>
                  </Link>
                </td>
                <td className="px-6 py-4">
                  <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
                    <span className="h-2 w-2 rounded-full bg-primary" />
                    {fn.runtime}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-xs font-medium",
                      fn.status === "active" && "bg-success/10 text-success border-0",
                      fn.status === "error" && "bg-destructive/10 text-destructive border-0",
                      fn.status === "inactive" && "bg-muted text-muted-foreground border-0"
                    )}
                  >
                    {fn.status}
                  </Badge>
                </td>
                <td className="px-6 py-4">
                  <span className="text-sm text-muted-foreground">
                    {fn.memory} MB
                  </span>
                </td>
                <td className="px-6 py-4 text-right">
                  <span className="text-sm font-medium text-foreground">
                    {fn.invocations.toLocaleString()}
                  </span>
                </td>
                <td className="px-6 py-4 text-right">
                  <span className={cn(
                    "text-sm font-medium",
                    fn.errors > 0 ? "text-destructive" : "text-muted-foreground"
                  )}>
                    {fn.errors}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <span className="text-sm text-muted-foreground">
                    {formatDate(fn.lastModified)}
                  </span>
                </td>
                <td className="px-6 py-4 text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="h-8 w-8">
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-48">
                      <DropdownMenuItem asChild>
                        <Link href={`/functions/${fn.name}`}>
                          <Eye className="mr-2 h-4 w-4" />
                          View Details
                        </Link>
                      </DropdownMenuItem>
                      <DropdownMenuItem asChild>
                        <Link href={`/functions/${fn.name}?tab=config`}>
                          <Edit className="mr-2 h-4 w-4" />
                          Edit Function
                        </Link>
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled>
                        <Copy className="mr-2 h-4 w-4" />
                        Duplicate
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      {fn.status === "active" ? (
                        <DropdownMenuItem disabled>
                          <Pause className="mr-2 h-4 w-4" />
                          Deactivate
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem disabled>
                          <Play className="mr-2 h-4 w-4" />
                          Activate
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        className="text-destructive focus:text-destructive"
                        onClick={() => onDelete(fn.name)}
                      >
                        <Trash2 className="mr-2 h-4 w-4" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

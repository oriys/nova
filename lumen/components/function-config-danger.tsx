"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { FunctionData } from "@/lib/types"
import { functionsApi } from "@/lib/api"
import { Trash2, Loader2, AlertTriangle } from "lucide-react"

interface FunctionConfigDangerProps {
  func: FunctionData
}

export function FunctionConfigDanger({ func }: FunctionConfigDangerProps) {
  const router = useRouter()
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [deleteConfirmName, setDeleteConfirmName] = useState("")

  const handleDelete = async () => {
    if (deleteConfirmName !== func.name) return
    try {
      setDeleting(true)
      await functionsApi.delete(func.name)
      router.push("/functions")
    } catch (err) {
      console.error("Failed to delete function:", err)
      setDeleting(false)
    }
  }

  return (
    <>
      <div className="rounded-xl border border-destructive/50 bg-card p-6">
        <h3 className="text-lg font-semibold text-destructive mb-2">
          Danger Zone
        </h3>
        <p className="text-sm text-muted-foreground mb-4">
          These actions are irreversible. Please proceed with caution.
        </p>
        <div className="flex gap-3">
          <Button
            variant="outline"
            className="border-destructive text-destructive hover:bg-destructive hover:text-destructive-foreground bg-transparent"
            onClick={() => setShowDeleteDialog(true)}
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Delete Function
          </Button>
        </div>
      </div>

      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              Delete Function
            </DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the function
              and all its versions and aliases.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-muted-foreground">
              Please type <span className="font-mono font-semibold text-foreground">{func.name}</span> to confirm.
            </p>
            <Input
              placeholder="Enter function name"
              value={deleteConfirmName}
              onChange={(e) => setDeleteConfirmName(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setShowDeleteDialog(false); setDeleteConfirmName(""); }}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteConfirmName !== func.name || deleting}
            >
              {deleting ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="mr-2 h-4 w-4" />
              )}
              Delete Function
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

"use client"

import Link from "next/link"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { CheckCircle2, Circle, CircleDot, ArrowRight } from "lucide-react"

export type FlowStepStatus = "done" | "current" | "pending"

export interface FlowStepAction {
  label: string
  href?: string
  onClick?: () => void
  disabled?: boolean
}

export interface FlowStep {
  id: string
  title: string
  description: string
  status: FlowStepStatus
  action?: FlowStepAction
}

interface StoreFlowRoadmapProps {
  title: string
  description?: string
  steps: FlowStep[]
  className?: string
}

function statusBadgeVariant(status: FlowStepStatus): "default" | "secondary" | "outline" {
  if (status === "done") return "default"
  if (status === "current") return "secondary"
  return "outline"
}

function statusLabel(status: FlowStepStatus): string {
  if (status === "done") return "Done"
  if (status === "current") return "Next"
  return "Pending"
}

function StepIcon({ status }: { status: FlowStepStatus }) {
  if (status === "done") return <CheckCircle2 className="h-4 w-4 text-primary" />
  if (status === "current") return <CircleDot className="h-4 w-4 text-muted-foreground" />
  return <Circle className="h-4 w-4 text-muted-foreground" />
}

export function StoreFlowRoadmap({ title, description, steps, className }: StoreFlowRoadmapProps) {
  return (
    <section className={cn("rounded-lg border border-border bg-card p-4", className)}>
      <div className="mb-3">
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        {description ? <p className="mt-1 text-xs text-muted-foreground">{description}</p> : null}
      </div>

      <div className="space-y-2">
        {steps.map((step, index) => {
          const isLast = index === steps.length - 1
          return (
            <div key={step.id} className="rounded-md border border-border/70 bg-card/70 p-3">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <StepIcon status={step.status} />
                    <p className="text-sm font-medium text-foreground">{step.title}</p>
                    <Badge variant={statusBadgeVariant(step.status)}>{statusLabel(step.status)}</Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">{step.description}</p>
                </div>

                {step.action ? (
                  step.action.href ? (
                    <Button asChild size="sm" variant={step.status === "current" ? "default" : "outline"}>
                      <Link href={step.action.href}>
                        {step.action.label}
                        <ArrowRight className="ml-2 h-3.5 w-3.5" />
                      </Link>
                    </Button>
                  ) : (
                    <Button
                      size="sm"
                      variant={step.status === "current" ? "default" : "outline"}
                      onClick={step.action.onClick}
                      disabled={step.action.disabled}
                    >
                      {step.action.label}
                      <ArrowRight className="ml-2 h-3.5 w-3.5" />
                    </Button>
                  )
                ) : null}
              </div>

              {!isLast ? <div className="mt-3 h-px w-full bg-border/70" /> : null}
            </div>
          )
        })}
      </div>
    </section>
  )
}

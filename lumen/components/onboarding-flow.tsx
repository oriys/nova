"use client"

import { useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { CheckCircle2, Circle, Sparkles } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  dismissOnboarding,
  getOnboardingState,
  isOnboardingComplete,
  subscribeOnboardingState,
} from "@/lib/onboarding-state"

interface OnboardingFlowProps {
  hasFunctionCreated?: boolean
  hasFunctionInvoked?: boolean
  hasGatewayRouteCreated?: boolean
  onCreateFunction?: () => void
  onCreateGatewayRoute?: () => void
}

export function OnboardingFlow({
  hasFunctionCreated,
  hasFunctionInvoked,
  hasGatewayRouteCreated,
  onCreateFunction,
  onCreateGatewayRoute,
}: OnboardingFlowProps) {
  const [state, setState] = useState(getOnboardingState())

  useEffect(() => {
    setState(getOnboardingState())
    return subscribeOnboardingState(setState)
  }, [])

  const merged = useMemo(() => {
    return {
      function_created: state.steps.function_created || Boolean(hasFunctionCreated),
      function_invoked: state.steps.function_invoked || Boolean(hasFunctionInvoked),
      gateway_route_created:
        state.steps.gateway_route_created || Boolean(hasGatewayRouteCreated),
      dismissed: state.dismissed,
    }
  }, [state, hasFunctionCreated, hasFunctionInvoked, hasGatewayRouteCreated])

  const complete =
    merged.function_created &&
    merged.function_invoked &&
    merged.gateway_route_created

  if (merged.dismissed || complete || isOnboardingComplete(state)) {
    return null
  }

  const completedCount = [
    merged.function_created,
    merged.function_invoked,
    merged.gateway_route_created,
  ].filter(Boolean).length

  const steps = [
    {
      key: "function_created",
      done: merged.function_created,
      title: "创建第一个函数",
      description: "先把业务逻辑放进函数。",
    },
    {
      key: "function_invoked",
      done: merged.function_invoked,
      title: "完成一次函数调用",
      description: "确认函数可执行并有调用记录。",
    },
    {
      key: "gateway_route_created",
      done: merged.gateway_route_created,
      title: "创建网关路由",
      description: "把 HTTP 路由绑定到函数。",
    },
  ]

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="flex items-center gap-2 text-sm font-medium text-foreground">
            <Sparkles className="h-4 w-4 text-primary" />
            新手引导
          </p>
          <p className="mt-1 text-xs text-muted-foreground">已完成 {completedCount}/3</p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => dismissOnboarding(true)}
        >
          稍后再说
        </Button>
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-3">
        {steps.map((step) => (
          <div key={step.key} className="rounded-lg border border-border bg-muted/20 p-3">
            <div className="flex items-center gap-2">
              {step.done ? (
                <CheckCircle2 className="h-4 w-4 text-success" />
              ) : (
                <Circle className="h-4 w-4 text-muted-foreground" />
              )}
              <p className="text-sm font-medium text-foreground">{step.title}</p>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{step.description}</p>
          </div>
        ))}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {!merged.function_created && onCreateFunction && (
          <Button size="sm" onClick={onCreateFunction}>
            创建函数
          </Button>
        )}
        {!merged.function_invoked && (
          <Button asChild variant="outline" size="sm">
            <Link href="/functions">去调用函数</Link>
          </Button>
        )}
        {!merged.gateway_route_created && onCreateGatewayRoute && (
          <Button variant="outline" size="sm" onClick={onCreateGatewayRoute}>
            创建路由
          </Button>
        )}
        {!merged.gateway_route_created && !onCreateGatewayRoute && (
          <Button asChild variant="outline" size="sm">
            <Link href="/gateway">去网关配置</Link>
          </Button>
        )}
      </div>
    </div>
  )
}

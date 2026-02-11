"use client"

import { useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { useTranslations } from "next-intl"
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
  const t = useTranslations("onboarding")
  const tc = useTranslations("common")
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
      title: t("createFirstFunction"),
      description: t("createFirstFunctionDesc"),
    },
    {
      key: "function_invoked",
      done: merged.function_invoked,
      title: t("runFunctionOnce"),
      description: t("runFunctionOnceDesc"),
    },
    {
      key: "gateway_route_created",
      done: merged.gateway_route_created,
      title: t("createGatewayRoute"),
      description: t("createGatewayRouteDesc"),
    },
  ]

  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="flex items-center gap-2 text-sm font-medium text-foreground">
            <Sparkles className="h-4 w-4 text-primary" />
            {t("gettingStarted")}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">{t("completedCount", { count: completedCount })}</p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => dismissOnboarding(true)}
        >
          {t("dismiss")}
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
            {tc("create")}
          </Button>
        )}
        {!merged.function_invoked && (
          <Button asChild variant="outline" size="sm">
            <Link href="/functions">{t("invokeFunction")}</Link>
          </Button>
        )}
        {!merged.gateway_route_created && onCreateGatewayRoute && (
          <Button variant="outline" size="sm" onClick={onCreateGatewayRoute}>
            {t("createRoute")}
          </Button>
        )}
        {!merged.gateway_route_created && !onCreateGatewayRoute && (
          <Button asChild variant="outline" size="sm">
            <Link href="/gateway">{t("openGateway")}</Link>
          </Button>
        )}
      </div>
    </div>
  )
}

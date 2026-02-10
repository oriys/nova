"use client"

export type OnboardingStepKey =
  | "function_created"
  | "function_invoked"
  | "gateway_route_created"

export interface OnboardingSteps {
  function_created: boolean
  function_invoked: boolean
  gateway_route_created: boolean
}

export interface OnboardingState {
  steps: OnboardingSteps
  dismissed: boolean
}

const STORAGE_KEY = "nova_onboarding_state_v1"
const UPDATE_EVENT = "nova:onboarding-updated"

const defaultState: OnboardingState = {
  steps: {
    function_created: false,
    function_invoked: false,
    gateway_route_created: false,
  },
  dismissed: false,
}

function isBrowser(): boolean {
  return typeof window !== "undefined"
}

function emitUpdate(state: OnboardingState) {
  if (!isBrowser()) return
  window.dispatchEvent(new CustomEvent<OnboardingState>(UPDATE_EVENT, { detail: state }))
}

function writeState(next: OnboardingState) {
  if (!isBrowser()) return
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  emitUpdate(next)
}

export function getOnboardingState(): OnboardingState {
  if (!isBrowser()) return defaultState
  const raw = window.localStorage.getItem(STORAGE_KEY)
  if (!raw) return defaultState
  try {
    const parsed = JSON.parse(raw) as Partial<OnboardingState>
    return {
      steps: {
        function_created: Boolean(parsed.steps?.function_created),
        function_invoked: Boolean(parsed.steps?.function_invoked),
        gateway_route_created: Boolean(parsed.steps?.gateway_route_created),
      },
      dismissed: Boolean(parsed.dismissed),
    }
  } catch {
    return defaultState
  }
}

export function isOnboardingComplete(state: OnboardingState = getOnboardingState()): boolean {
  return (
    state.steps.function_created &&
    state.steps.function_invoked &&
    state.steps.gateway_route_created
  )
}

export function markOnboardingStep(step: OnboardingStepKey, value: boolean = true) {
  const current = getOnboardingState()
  if (current.steps[step] === value) return
  const next: OnboardingState = {
    ...current,
    steps: {
      ...current.steps,
      [step]: value,
    },
  }
  writeState(next)
}

export function dismissOnboarding(value: boolean = true) {
  const current = getOnboardingState()
  if (current.dismissed === value) return
  writeState({
    ...current,
    dismissed: value,
  })
}

export function resetOnboarding() {
  writeState(defaultState)
}

export function syncOnboardingStateFromData(data: {
  hasFunctionCreated?: boolean
  hasFunctionInvoked?: boolean
  hasGatewayRouteCreated?: boolean
}) {
  const current = getOnboardingState()
  const next: OnboardingState = {
    ...current,
    steps: {
      function_created: current.steps.function_created || Boolean(data.hasFunctionCreated),
      function_invoked: current.steps.function_invoked || Boolean(data.hasFunctionInvoked),
      gateway_route_created:
        current.steps.gateway_route_created || Boolean(data.hasGatewayRouteCreated),
    },
  }

  if (
    next.steps.function_created !== current.steps.function_created ||
    next.steps.function_invoked !== current.steps.function_invoked ||
    next.steps.gateway_route_created !== current.steps.gateway_route_created
  ) {
    writeState(next)
  }
}

export function subscribeOnboardingState(listener: (state: OnboardingState) => void): () => void {
  if (!isBrowser()) return () => {}
  const handler = (event: Event) => {
    const custom = event as CustomEvent<OnboardingState>
    listener(custom.detail || getOnboardingState())
  }
  window.addEventListener(UPDATE_EVENT, handler)
  return () => window.removeEventListener(UPDATE_EVENT, handler)
}

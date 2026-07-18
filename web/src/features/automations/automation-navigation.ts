export interface AutomationNavigationTarget {
  taskId: string
  runId?: string
  inboxId?: string
  workspace?: string
}

export const AUTOMATION_NAVIGATION_EVENT = 'nova:open-automation'

let pendingTarget: AutomationNavigationTarget | null = null

export function requestAutomationNavigation(target: AutomationNavigationTarget) {
  pendingTarget = { ...target }
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent<AutomationNavigationTarget>(AUTOMATION_NAVIGATION_EVENT, { detail: pendingTarget }))
  }
}

export function consumeAutomationNavigation(): AutomationNavigationTarget | null {
  const target = pendingTarget
  pendingTarget = null
  return target
}

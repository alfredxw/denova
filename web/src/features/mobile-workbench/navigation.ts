export const MOBILE_WORKBENCH_NAVIGATE_EVENT = 'nova:mobile-workbench:navigate'

export interface MobileWorkbenchNavigationTarget {
  destinationId: string
  mode?: 'ide' | 'interactive'
}

export function requestMobileWorkbenchDestination(destinationId: string, mode?: 'ide' | 'interactive') {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<MobileWorkbenchNavigationTarget>(MOBILE_WORKBENCH_NAVIGATE_EVENT, { detail: { destinationId, mode } }))
}

export function mobileWorkbenchDestinationFromEvent(event: Event): MobileWorkbenchNavigationTarget | null {
  if (!(event instanceof CustomEvent)) return null
  const detail = event.detail as MobileWorkbenchNavigationTarget | null
  if (typeof detail?.destinationId !== 'string' || detail.destinationId === '') return null
  return detail
}

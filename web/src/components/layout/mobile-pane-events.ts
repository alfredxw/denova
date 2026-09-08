export const MOBILE_PANES_CLOSE_EVENT = 'nova:mobile-close-panes'
export const MOBILE_NAVIGATION_OPEN_EVENT = 'nova:mobile-navigation-open'
export const MOBILE_PROJECT_OPEN_EVENT = 'nova:mobile-project-open'

/** Complete a navigation selection without dismissing desktop surface panes. */
export function closeMobilePanes() {
  if (window.matchMedia('(max-width: 1023px)').matches) {
    window.dispatchEvent(new Event(MOBILE_PANES_CLOSE_EVENT))
  }
}

export const MOBILE_WRITING_VIEW_EVENT = 'nova:mobile-writing-view'
export type MobileWritingView = 'editor' | 'agent'

/** Reveal the destination only after an accepted action, without changing desktop panes. */
export function showMobileWritingView(view: MobileWritingView) {
  if (window.matchMedia('(max-width: 1023px)').matches) {
    window.dispatchEvent(new CustomEvent(MOBILE_WRITING_VIEW_EVENT, { detail: view }))
  }
}

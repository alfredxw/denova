export const ONBOARDING_STORAGE_KEY = 'nova:onboarding:v1'

export interface OnboardingStoredState {
  version: 1
  skipped?: boolean
  completed?: boolean
}

export function readOnboardingState(): OnboardingStoredState {
  if (typeof window === 'undefined') return { version: 1 }
  try {
    const raw = window.localStorage.getItem(ONBOARDING_STORAGE_KEY)
    if (!raw) return { version: 1 }
    const parsed = JSON.parse(raw) as Partial<OnboardingStoredState>
    return {
      version: 1,
      skipped: parsed.skipped === true,
      completed: parsed.completed === true,
    }
  } catch {
    return { version: 1 }
  }
}

export function writeOnboardingState(state: OnboardingStoredState) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(ONBOARDING_STORAGE_KEY, JSON.stringify({ ...state, version: 1 }))
}

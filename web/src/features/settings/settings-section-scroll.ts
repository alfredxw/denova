import { animate } from 'motion/react'
import type { AnimationPlaybackControls } from 'motion/react'
import { novaEase } from '@/features/motion/motion-tokens'

export const SETTINGS_SECTION_SCROLL_DURATION_SECONDS = 0.22

interface SettingsSectionScrollOptions {
  reducedMotion: boolean
  onComplete?: () => void
}

/** Scrolls the settings viewport in a fixed time, independent of section distance. */
export function scrollSettingsSectionIntoView(
  container: HTMLElement,
  section: HTMLElement,
  { reducedMotion, onComplete }: SettingsSectionScrollOptions,
): AnimationPlaybackControls | null {
  const scrollMarginTop = Number.parseFloat(getComputedStyle(section).scrollMarginTop) || 0
  const requestedScrollTop = container.scrollTop
    + section.getBoundingClientRect().top
    - container.getBoundingClientRect().top
    - scrollMarginTop
  const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight)
  const targetScrollTop = Math.min(maxScrollTop, Math.max(0, requestedScrollTop))

  if (reducedMotion || Math.abs(targetScrollTop - container.scrollTop) < 1) {
    container.scrollTop = targetScrollTop
    onComplete?.()
    return null
  }

  return animate(container.scrollTop, targetScrollTop, {
    type: 'tween',
    duration: SETTINGS_SECTION_SCROLL_DURATION_SECONDS,
    ease: novaEase,
    onUpdate: (value) => {
      container.scrollTop = value
    },
    onComplete,
  })
}

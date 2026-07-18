import { describe, expect, it, vi } from 'vitest'
import {
  AUTOMATION_NAVIGATION_EVENT,
  consumeAutomationNavigation,
  requestAutomationNavigation,
} from './automation-navigation'

describe('automation navigation', () => {
  it('retains a navigation intent until the global automation view mounts', () => {
    consumeAutomationNavigation()
    const listener = vi.fn()
    window.addEventListener(AUTOMATION_NAVIGATION_EVENT, listener)

    requestAutomationNavigation({ taskId: 'workspace-a:task-1', runId: 'run-1', workspace: '/books/a' })

    expect(listener).toHaveBeenCalledTimes(1)
    expect(consumeAutomationNavigation()).toEqual({ taskId: 'workspace-a:task-1', runId: 'run-1', workspace: '/books/a' })
    expect(consumeAutomationNavigation()).toBeNull()
    window.removeEventListener(AUTOMATION_NAVIGATION_EVENT, listener)
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { maybeNotifyActionableTask } from './task-notifications'

class MockNotification {
  static permission: NotificationPermission = 'default'
  static requestPermission = vi.fn(async (): Promise<NotificationPermission> => 'granted')
  static instances: Array<{ title: string; options?: NotificationOptions }> = []
  readonly title: string
  readonly options?: NotificationOptions

  constructor(title: string, options?: NotificationOptions) {
    this.title = title
    this.options = options
    MockNotification.instances.push({ title, options })
  }
}

describe('maybeNotifyActionableTask', () => {
  beforeEach(() => {
    MockNotification.permission = 'default'
    MockNotification.requestPermission.mockReset().mockResolvedValue('granted')
    MockNotification.instances = []
    vi.stubGlobal('Notification', MockNotification)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('does nothing when system notifications are disabled', async () => {
    const notified = await maybeNotifyActionableTask({
      enabled: false,
      typeLabel: 'Agent',
      projectName: 'lost-garden',
    })

    expect(notified).toBe(false)
    expect(MockNotification.instances).toEqual([])
    expect(MockNotification.requestPermission).not.toHaveBeenCalled()
  })

  it('requests permission on first actionable task after enabling and shows only type and project', async () => {
    const notified = await maybeNotifyActionableTask({
      enabled: true,
      typeLabel: 'Agent',
      projectName: 'lost-garden',
    })

    expect(MockNotification.requestPermission).toHaveBeenCalledTimes(1)
    expect(notified).toBe(true)
    expect(MockNotification.instances).toHaveLength(1)
    expect(MockNotification.instances[0].title).toBe('Agent · lost-garden')
    expect(MockNotification.instances[0].options?.body).toBeUndefined()
  })

  it('does not show a notification when permission is denied', async () => {
    MockNotification.permission = 'denied'

    const notified = await maybeNotifyActionableTask({
      enabled: true,
      typeLabel: 'Agent',
      projectName: 'lost-garden',
    })

    expect(notified).toBe(false)
    expect(MockNotification.instances).toEqual([])
  })
})

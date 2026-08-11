import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse } from 'msw'
import { setConfiguredLocale } from '@/i18n'
import { server } from '@/test/msw/server'
import { WorkbenchShell } from './WorkbenchShell'

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => true,
}))

describe('Mobile Workbench task notifications', () => {
  beforeEach(() => {
    MockNotification.instances = []
    setConfiguredLocale('zh-CN')
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    vi.stubGlobal('Notification', MockNotification)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses the latest notification preference and locale after settings load', async () => {
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 1,
        tasks: [{
          id: 'automation:settings-loaded',
          type: 'automation',
          status: 'waiting_user',
          title: '敏感的自动化任务标题',
          project: { name: 'project-one', path: '/projects/one' },
          started_at: '2026-08-11T10:00:00Z',
          updated_at: '2026-08-11T10:01:00Z',
          recovery: {
            kind: 'automation_run',
            workspace: '/projects/one',
            task_id: 'review-one',
            run_id: 'run-one',
          },
        }],
      })),
    )

    const view = render(
      <WorkbenchShell {...workbenchProps()} systemNotificationsEnabled={false} />,
    )

    expect(await screen.findByLabelText('1 项任务需要处理')).toBeInTheDocument()
    expect(MockNotification.instances).toEqual([])

    act(() => setConfiguredLocale('en-US'))
    view.rerender(
      <WorkbenchShell {...workbenchProps()} systemNotificationsEnabled />,
    )

    await waitFor(() => {
      expect(MockNotification.instances).toEqual([{ title: 'Automation · project-one' }])
    })
    expect(MockNotification.instances[0].title).not.toContain('敏感')
  })
})

class MockNotification {
  static permission: NotificationPermission = 'granted'
  static instances: Array<{ title: string; options?: NotificationOptions }> = []

  constructor(title: string, options?: NotificationOptions) {
    MockNotification.instances.push({ title, options })
  }
}

function workbenchProps() {
  return {
    mode: 'ide' as const,
    booksReturnMode: 'ide' as const,
    currentBookName: '失落花园',
    workspace: '/tmp/lost-garden',
    books: [{ name: '失落花园', path: '/tmp/lost-garden', author: '', last_opened_at: '' }],
    appVersion: 'test',
    summary: null,
    currentChapter: {
      path: 'chapters/ch22.md', file_name: 'ch22.md', display_title: '第二十二章', index: 22,
      words: 1200, status: 'draft', confirmed: false, updated_at: '2026-08-11T10:00:00Z',
      volume: '第一卷', volume_path: 'volumes/01',
    },
    isStreaming: false,
    projectVisible: true,
    activityBarExpanded: false,
    rightPanel: null,
    settingsOpen: false,
    interactiveSubmode: 'story' as const,
    sidebar: <div>创作结构</div>,
    main: <div>正文内容</div>,
    rightPanelContent: <div>Agent 会话</div>,
    onSetMode: vi.fn(),
    onToggleActivityBarExpanded: vi.fn(),
    onSetInteractiveSubmode: vi.fn(),
    onSetRightPanel: vi.fn(),
    onToggleSettings: vi.fn(),
    onCloseSettings: vi.fn(),
    onQuickSwitchBook: vi.fn().mockResolvedValue(true),
  }
}

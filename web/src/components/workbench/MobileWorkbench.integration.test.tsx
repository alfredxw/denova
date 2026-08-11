import { useState, type ReactNode } from 'react'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse } from 'msw'
import { setConfiguredLocale } from '@/i18n'
import type { InteractiveSubmode } from '@/features/interactive/types'
import { server } from '@/test/msw/server'
import { WorkbenchShell } from './WorkbenchShell'
import { registerExecutableDraft, unregisterExecutableDraft, useExecutableDraftGuard } from '@/features/config-guard/executable-draft-guard'

const responsiveState = vi.hoisted(() => ({ mobile: true, phone: true }))
const automationNavigation = vi.hoisted(() => ({ request: vi.fn() }))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: (query?: string) => query?.includes('639px') ? responsiveState.phone : responsiveState.mobile,
}))

vi.mock('@/features/automations/automation-navigation', () => ({
  requestAutomationNavigation: automationNavigation.request,
}))

describe('Mobile Workbench', () => {
  beforeEach(() => {
    useExecutableDraftGuard.setState({ entries: {} })
    responsiveState.mobile = true
    responsiveState.phone = true
    localStorage.clear()
    automationNavigation.request.mockClear()
    setConfiguredLocale('zh-CN')
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses four writing destinations and exposes mode switching only from More', async () => {
    const user = userEvent.setup()
    const onSetMode = vi.fn()

    render(<WorkbenchShell {...workbenchProps(<div>正文内容</div>)} onSetMode={onSetMode} />)

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(within(navigation).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual([
      '正文',
      '项目',
      'Agent',
      '更多',
    ])
    expect(within(navigation).getByRole('button', { name: '正文' })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByRole('group', { name: '模式切换' })).not.toBeInTheDocument()

    await user.click(within(navigation).getByRole('button', { name: '更多' }))

    expect(onSetMode).not.toHaveBeenCalled()
    expect(screen.getByRole('group', { name: '模式切换' })).toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: '游戏模式' }))
    expect(onSetMode).toHaveBeenCalledWith('interactive')
  })

  it('opens shared workspaces from More without changing the content mode', async () => {
    const user = userEvent.setup()
    const onSetMode = vi.fn()
    const onToggleSettings = vi.fn()

    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onSetMode={onSetMode}
        onToggleSettings={onToggleSettings}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))

    expect(screen.getByRole('button', { name: '版本管理' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Skills' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Agents' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '自动化' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '设置' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '设置' }))

    expect(onToggleSettings).toHaveBeenCalledOnce()
    expect(onSetMode).not.toHaveBeenCalled()
    expect(within(navigation).getAllByRole('button').filter((button) => button.getAttribute('aria-current') === 'page')).toHaveLength(1)
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')
  })

  it('移动工作台可见控件与图片都有可访问名称', async () => {
    const user = userEvent.setup()
    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onQuickSwitchBook={vi.fn().mockResolvedValue(true)}
      />,
    )

    const assertVisibleNames = () => {
      for (const button of screen.getAllByRole('button')) {
        expect(button).toHaveAccessibleName()
      }
      for (const img of document.querySelectorAll('img')) {
        expect(img).toHaveAccessibleName()
      }
    }

    assertVisibleNames()

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    assertVisibleNames()

    await user.click(screen.getByRole('button', { name: '任务中心' }))
    const taskRegion = screen.getByRole('region', { name: '任务中心' })
    expect(taskRegion).toHaveAccessibleName()
    assertVisibleNames()
  })

  it('未保存自动化配置时返回会弹出继续编辑/放弃修改', async () => {
    const user = userEvent.setup()
    const onSetMode = vi.fn()
    const discard = vi.fn()
    registerExecutableDraft('automations', { hasPending: true, discard })

    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        mode="automations"
        booksReturnMode="ide"
        onSetMode={onSetMode}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '正文' }))

    expect(await screen.findByRole('alertdialog', { name: '放弃未保存的配置？' })).toBeInTheDocument()
    expect(onSetMode).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '继续编辑' }))
    expect(onSetMode).not.toHaveBeenCalled()

    await user.click(within(navigation).getByRole('button', { name: '正文' }))
    await user.click(screen.getByRole('button', { name: '放弃修改' }))

    expect(discard).toHaveBeenCalled()
    expect(onSetMode).toHaveBeenCalledWith('ide')
    unregisterExecutableDraft('automations')
  })

  it('未保存游戏模式预设时从设置面返回会弹出继续编辑/放弃修改', async () => {
    const user = userEvent.setup()
    const onSetMode = vi.fn()
    const onSetInteractiveSubmode = vi.fn()
    const discard = vi.fn()
    registerExecutableDraft('setting-panel', { hasPending: true, discard })

    render(
      <WorkbenchShell
        {...workbenchProps(<div>故事正文</div>)}
        mode="interactive"
        booksReturnMode="interactive"
        interactiveSubmode="teller"
        onSetMode={onSetMode}
        onSetInteractiveSubmode={onSetInteractiveSubmode}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '故事' }))

    expect(await screen.findByRole('alertdialog', { name: '放弃未保存的配置？' })).toBeInTheDocument()
    expect(onSetInteractiveSubmode).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '继续编辑' }))
    expect(onSetInteractiveSubmode).not.toHaveBeenCalled()

    await user.click(within(navigation).getByRole('button', { name: '故事' }))
    await user.click(screen.getByRole('button', { name: '放弃修改' }))

    expect(discard).toHaveBeenCalled()
    expect(onSetMode).toHaveBeenCalledWith('interactive')
    expect(onSetInteractiveSubmode).toHaveBeenCalledWith('story')
    unregisterExecutableDraft('setting-panel')
  })

  it('重复点击当前互动目的地不会触发未保存守卫', async () => {
    const user = userEvent.setup()
    const onSetInteractiveSubmode = vi.fn()
    const discard = vi.fn()
    registerExecutableDraft('setting-panel', { hasPending: true, discard })

    render(
      <WorkbenchShell
        {...workbenchProps(<div>故事正文</div>)}
        mode="interactive"
        booksReturnMode="interactive"
        interactiveSubmode="lore"
        onSetInteractiveSubmode={onSetInteractiveSubmode}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '资料' }))

    expect(screen.queryByRole('alertdialog', { name: '放弃未保存的配置？' })).not.toBeInTheDocument()
    expect(discard).not.toHaveBeenCalled()
    unregisterExecutableDraft('setting-panel')
  })

  it('shows actionable tasks in More and resumes the selected source workflow', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 2,
        tasks: [
          {
            id: 'automation:run-failed',
            type: 'automation',
            status: 'failed',
            title: 'Project one review',
            project: { name: 'project-one', path: '/projects/one' },
            started_at: '2026-08-03T09:30:00Z',
            updated_at: '2026-08-03T09:32:00Z',
            recovery: { kind: 'automation_run', workspace: '/projects/one', task_id: 'review-one', run_id: 'run-failed' },
            error: 'model unavailable',
          },
          {
            id: 'automation-inbox:inbox-waiting',
            type: 'automation',
            status: 'waiting_user',
            title: 'Continue chapter',
            project: { name: 'project-two', path: '/projects/two' },
            started_at: '2026-08-03T10:00:00Z',
            updated_at: '2026-08-03T10:00:00Z',
            recovery: { kind: 'automation_inbox', workspace: '/projects/two', task_id: 'continue-two', inbox_id: 'inbox-waiting' },
          },
          {
            id: 'agent:task-agent',
            type: 'agent',
            status: 'running',
            title: 'Drafting session',
            project: { name: 'project-agent', path: '/projects/agent' },
            started_at: '2026-08-03T10:10:00Z',
            updated_at: '2026-08-03T10:11:00Z',
            recovery: { kind: 'agent_session', workspace: '/projects/agent', task_id: 'task-agent', session_id: 'session-agent' },
          },
          {
            id: 'interactive-story:task-story',
            type: 'interactive_story',
            status: 'running',
            title: 'Harbor story',
            project: { name: 'project-story', path: '/projects/story' },
            started_at: '2026-08-03T10:20:00Z',
            updated_at: '2026-08-03T10:21:00Z',
            recovery: { kind: 'interactive_story', workspace: '/projects/story', task_id: 'task-story', story_id: 'story-harbor', branch_id: 'night' },
          },
          {
            id: 'agent:task-config',
            type: 'agent',
            status: 'running',
            title: '调整写作模式方案预设',
            project: { name: 'project-config', path: '/projects/config' },
            started_at: '2026-08-03T10:30:00Z',
            updated_at: '2026-08-03T10:31:00Z',
            recovery: { kind: 'config_manager', workspace: '/projects/config', task_id: 'task-config', origin: 'agents', resource_id: 'agents:main' },
          },
        ],
      })),
    )
    const user = userEvent.setup()
    const onSetMode = vi.fn()
    const onQuickSwitchBook = vi.fn().mockResolvedValue(true)
    const agentTargets: unknown[] = []
    const storyTargets: unknown[] = []
    const receiveAgent = (event: Event) => agentTargets.push((event as CustomEvent).detail)
    const receiveStory = (event: Event) => storyTargets.push((event as CustomEvent).detail)
    window.addEventListener('nova:open-agent-session', receiveAgent)
    window.addEventListener('nova:open-interactive-story', receiveStory)
    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onSetMode={onSetMode}
        onQuickSwitchBook={onQuickSwitchBook}
      />,
    )

    expect(await screen.findByLabelText('2 项任务需要处理')).toBeInTheDocument()
    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))

    const taskCenterEntry = screen.getByRole('button', { name: '任务中心，2 项需要处理' })
    expect(taskCenterEntry).toBeInTheDocument()
    await user.click(taskCenterEntry)

    expect(screen.getByRole('heading', { name: '任务中心' })).toBeInTheDocument()
    expect(screen.getByText('project-one')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()
    expect(screen.getByText('等待处理')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开任务：Project one review' }))
    await waitFor(() => {
      expect(automationNavigation.request).toHaveBeenCalledWith({
        taskId: 'review-one',
        runId: 'run-failed',
        workspace: '/projects/one',
      })
    })
    expect(onSetMode).toHaveBeenCalledWith('automations')
    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '任务中心，2 项需要处理' }))
    await user.click(screen.getByRole('button', { name: '打开任务：Drafting session' }))
    await waitFor(() => {
      expect(onQuickSwitchBook).toHaveBeenCalledWith('/projects/agent')
      expect(agentTargets).toEqual([{ sessionId: 'session-agent', taskId: 'task-agent' }])
    })
    expect(onSetMode).toHaveBeenCalledWith('ide')
    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).not.toHaveAttribute('aria-current', 'page')

    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心，2 项需要处理' }))
    await user.click(screen.getByRole('button', { name: '打开任务：Harbor story' }))
    await waitFor(() => {
      expect(onQuickSwitchBook).toHaveBeenCalledWith('/projects/story')
      expect(storyTargets).toEqual([{ storyId: 'story-harbor', branchId: 'night', taskId: 'task-story' }])
    })
    expect(onSetMode).toHaveBeenCalledWith('interactive')
    expect(onSetMode).not.toHaveBeenCalledWith('books')
    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).not.toHaveAttribute('aria-current', 'page')

    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心，2 项需要处理' }))
    await user.click(screen.getByRole('button', { name: '打开任务：调整写作模式方案预设' }))
    await waitFor(() => {
      expect(onQuickSwitchBook).toHaveBeenCalledWith('/projects/config')
    })
    expect(onSetMode).toHaveBeenCalledWith('agents')
    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()

    window.removeEventListener('nova:open-agent-session', receiveAgent)
    window.removeEventListener('nova:open-interactive-story', receiveStory)
  })

  it('switches back to the source project for an image generation task', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 0,
        tasks: [
          {
            id: 'image-generation:7',
            type: 'image_generation',
            status: 'running',
            title: 'project-image',
            project: { name: 'project-image', path: '/projects/image' },
            started_at: '2026-08-03T10:00:00Z',
            updated_at: '2026-08-03T10:01:00Z',
            recovery: { kind: 'image_generation', workspace: '/projects/image', task_id: '7' },
          },
        ],
      })),
    )
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn().mockResolvedValue(true)
    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onQuickSwitchBook={onQuickSwitchBook}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心' }))
    await user.click(screen.getByRole('button', { name: '打开任务：project-image' }))

    expect(onQuickSwitchBook).toHaveBeenCalledWith('/projects/image')
  })

  it('switches back to the imported book for an import/export task', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 0,
        tasks: [
          {
            id: 'import-export:1',
            type: 'import_export',
            status: 'completed',
            title: 'imported-book',
            project: { name: 'imported-book', path: '/books/imported' },
            started_at: '2026-08-04T10:00:00Z',
            updated_at: '2026-08-04T10:02:00Z',
            recovery: { kind: 'import_export', workspace: '/books/imported' },
          },
        ],
      })),
    )
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn().mockResolvedValue(true)
    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onQuickSwitchBook={onQuickSwitchBook}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心' }))
    await user.click(screen.getByRole('button', { name: '打开任务：imported-book' }))

    expect(onQuickSwitchBook).toHaveBeenCalledWith('/books/imported')
  })

  it('shows a privacy-safe system notification for an actionable task when enabled', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    vi.stubGlobal('Notification', MockNotification)
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 1,
        tasks: [
          {
            id: 'agent:1',
            type: 'agent',
            status: 'waiting_user',
            title: '会话：请处理草稿中的敏感段落',
            project: { name: 'project-one', path: '/projects/one' },
            started_at: '2026-08-04T10:00:00Z',
            updated_at: '2026-08-04T10:01:00Z',
            recovery: { kind: 'agent_session', workspace: '/projects/one', session_id: 'session-1', task_id: 'agent:1' },
          },
        ],
      })),
    )

    render(<WorkbenchShell {...workbenchProps(<div>正文内容</div>)} systemNotificationsEnabled />)

    await waitFor(() => {
      expect(MockNotification.instances).toHaveLength(1)
    })
    expect(MockNotification.instances[0].title).toBe('Agent · project-one')
    expect(MockNotification.instances[0].options?.body).toBeUndefined()
    expect(MockNotification.instances[0].title).not.toContain('敏感')
  })

  it('keeps long book names from covering header and navigation controls', async () => {
    const user = userEvent.setup()
    const longName = '很长的书名'.repeat(20)
    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        currentBookName={longName}
      />,
    )

    const switcher = screen.getByRole('button', { name: `切换书籍，当前：${longName}` })
    expect(switcher).toBeInTheDocument()

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))

    expect(screen.getByRole('group', { name: '模式切换' })).toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')
  })

  it('aborts task recovery when the source project switch fails', async () => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    server.use(
      http.get('/api/tasks', () => HttpResponse.json({
        action_required_count: 1,
        tasks: [
          {
            id: 'agent:blocked',
            type: 'agent',
            status: 'waiting_user',
            title: 'Blocked session',
            project: { name: 'project-one', path: '/projects/one' },
            started_at: '2026-08-04T10:00:00Z',
            updated_at: '2026-08-04T10:01:00Z',
            recovery: { kind: 'agent_session', workspace: '/projects/one', session_id: 'session-1', task_id: 'agent:blocked' },
          },
        ],
      })),
    )
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn().mockResolvedValue(false)
    const onSetMode = vi.fn()
    const agentTargets: unknown[] = []
    const receiveAgent = (event: Event) => agentTargets.push((event as CustomEvent).detail)
    window.addEventListener('nova:open-agent-session', receiveAgent)

    render(
      <WorkbenchShell
        {...workbenchProps(<div>正文内容</div>)}
        onQuickSwitchBook={onQuickSwitchBook}
        onSetMode={onSetMode}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心，1 项需要处理' }))
    await user.click(screen.getByRole('button', { name: '打开任务：Blocked session' }))

    expect(onQuickSwitchBook).toHaveBeenCalledWith('/projects/one')
    expect(agentTargets).toEqual([])
    expect(onSetMode).not.toHaveBeenCalledWith('ide')
    window.removeEventListener('nova:open-agent-session', receiveAgent)
  })

  it('uses the same return path for the task center header and browser back', async () => {
    const user = userEvent.setup()
    render(<WorkbenchShell {...workbenchProps(<div>正文内容</div>)} />)

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心' }))
    expect(screen.getByRole('heading', { name: '任务中心' })).toBeInTheDocument()

    act(() => window.dispatchEvent(new PopStateEvent('popstate')))

    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()
    expect(screen.getByRole('group', { name: '模式切换' })).toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: '任务中心' }))
    await user.click(screen.getByRole('button', { name: '返回更多' }))

    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()
    expect(screen.getByRole('group', { name: '模式切换' })).toBeInTheDocument()
    await waitFor(() => {
      expect(document.activeElement).toBe(document.querySelector('[data-mobile-more-button]'))
    })
  })

  it('closes the task center with Escape like the header back control', async () => {
    const user = userEvent.setup()
    render(<WorkbenchShell {...workbenchProps(<div>正文内容</div>)} />)

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '任务中心' }))
    expect(screen.getByRole('heading', { name: '任务中心' })).toBeInTheDocument()
    await waitFor(() => {
      expect(document.activeElement).toBe(document.getElementById('mobile-task-center-back'))
    })

    await user.keyboard('{Escape}')

    expect(screen.queryByRole('heading', { name: '任务中心' })).not.toBeInTheDocument()
    expect(screen.getByRole('group', { name: '模式切换' })).toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')
    await waitFor(() => {
      expect(document.activeElement).toBe(document.querySelector('[data-mobile-more-button]'))
    })
  })

  it('leaves a shared workspace when a primary destination is selected', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [settingsOpen, setSettingsOpen] = useState(true)
      return (
        <WorkbenchShell
          {...workbenchProps(<div>正文内容</div>)}
          settingsOpen={settingsOpen}
          onCloseSettings={() => setSettingsOpen(false)}
        />
      )
    }

    render(<Harness />)

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(within(navigation).getByRole('button', { name: '更多' })).toHaveAttribute('aria-current', 'page')

    await user.click(within(navigation).getByRole('button', { name: '正文' }))

    expect(within(navigation).getByRole('button', { name: '正文' })).toHaveAttribute('aria-current', 'page')
    expect(within(navigation).getByRole('button', { name: '更多' })).not.toHaveAttribute('aria-current')
  })

  it('restores each mode destination after switching modes and remounting the workspace', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [mode, setMode] = useState<'ide' | 'interactive'>('ide')
      const [interactiveSubmode, setInteractiveSubmode] = useState<InteractiveSubmode>('story')
      return (
        <WorkbenchShell
          {...workbenchProps(<div>当前工作面</div>)}
          mode={mode}
          booksReturnMode={mode}
          interactiveSubmode={interactiveSubmode}
          onSetMode={(nextMode) => {
            if (nextMode === 'ide' || nextMode === 'interactive') setMode(nextMode)
          }}
          onSetInteractiveSubmode={setInteractiveSubmode}
        />
      )
    }

    const firstRender = render(<Harness />)
    let navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })

    await user.click(within(navigation).getByRole('button', { name: '项目' }))
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '游戏模式' }))

    navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(within(navigation).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual([
      '故事',
      '剧情线',
      '资料',
      '更多',
    ])
    await user.click(within(navigation).getByRole('button', { name: '剧情线' }))
    await user.click(within(navigation).getByRole('button', { name: '更多' }))
    await user.click(screen.getByRole('button', { name: '写作模式' }))

    navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(within(navigation).getByRole('button', { name: '项目' })).toHaveAttribute('aria-current', 'page')

    firstRender.unmount()
    render(<WorkbenchShell {...workbenchProps(<div>当前工作面</div>)} />)

    navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(within(navigation).getByRole('button', { name: '项目' })).toHaveAttribute('aria-current', 'page')
  })

  it('gives the bottom area to the focused input while the software keyboard is visible', async () => {
    const originalVisualViewport = Object.getOwnPropertyDescriptor(window, 'visualViewport')
    const originalInnerHeight = Object.getOwnPropertyDescriptor(window, 'innerHeight')
    const viewport = new MockVisualViewport(800)
    Object.defineProperty(window, 'visualViewport', { configurable: true, value: viewport })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })

    try {
      render(<WorkbenchShell {...workbenchProps(<textarea aria-label="正文输入" />)} />)
      expect(screen.getByRole('navigation', { name: '移动端工作台导航' })).toBeInTheDocument()

      const input = screen.getByRole('textbox', { name: '正文输入' })
      act(() => {
        input.focus()
        viewport.height = 420
        viewport.dispatchEvent(new Event('resize'))
      })

      expect(screen.queryByRole('navigation', { name: '移动端工作台导航' })).not.toBeInTheDocument()

      act(() => input.blur())
      expect(screen.getByRole('navigation', { name: '移动端工作台导航' })).toBeInTheDocument()
    } finally {
      if (originalVisualViewport) Object.defineProperty(window, 'visualViewport', originalVisualViewport)
      else Reflect.deleteProperty(window, 'visualViewport')
      if (originalInnerHeight) Object.defineProperty(window, 'innerHeight', originalInnerHeight)
    }
  })

  it('returns to the manuscript after a project document is opened', async () => {
    const user = userEvent.setup()
    render(
      <WorkbenchShell
        {...workbenchProps(<div>第二十二章正文</div>)}
        sidebar={(
          <button
            type="button"
            onClick={() => window.dispatchEvent(new CustomEvent('nova:mobile-workbench:navigate', { detail: { destinationId: 'manuscript' } }))}
          >
            打开第二十二章
          </button>
        )}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '项目' }))
    expect(screen.getByRole('button', { name: '打开第二十二章' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开第二十二章' }))

    expect(screen.getByText('第二十二章正文')).toBeInTheDocument()
    expect(within(navigation).getByRole('button', { name: '正文' })).toHaveAttribute('aria-current', 'page')
  })

  it('moves the same four destinations to a side rail on compact wide screens', () => {
    responsiveState.phone = false

    render(<WorkbenchShell {...workbenchProps(<div>正文内容</div>)} />)

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    expect(navigation).toHaveAttribute('data-placement', 'side')
    expect(within(navigation).getAllByRole('button').map((button) => button.textContent?.trim())).toEqual([
      '正文',
      '项目',
      'Agent',
      '更多',
    ])
  })
})

class MockVisualViewport extends EventTarget {
  height: number
  offsetTop = 0

  constructor(height: number) {
    super()
    this.height = height
  }
}

class MockNotification {
  static permission: NotificationPermission = 'granted'
  static instances: Array<{ title: string; options?: NotificationOptions }> = []

  constructor(title: string, options?: NotificationOptions) {
    MockNotification.instances.push({ title, options })
  }
}

function workbenchProps(main: ReactNode) {
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
      words: 1200, status: 'draft', confirmed: false, updated_at: '2026-08-03T10:00:00Z',
      volume: '第一卷', volume_path: 'volumes/01',
    },
    isStreaming: false,
    projectVisible: true,
    activityBarExpanded: false,
    rightPanel: null,
    settingsOpen: false,
    interactiveSubmode: 'story' as const,
    sidebar: <div>创作结构</div>,
    main,
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

import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import {
  agentTabForProject,
  project,
  renderView,
  setDocumentReviewFeedback,
  terminalSession,
} from './AgentChatView.test-harness'
import {
  addAgentChatProject,
  archiveAgentChatProject,
  createAgentChatSession,
  getAgentChatHistory,
  getAgentChatProjects,
  relinkAgentChatProject,
  selectAgentChatProjectDirectory,
} from './api'
import { persistWorkbenchState, readStoredWorkbenchState } from './tab-state'
import { closeTerminalSession, getTerminalRuntimeStatus } from './terminal/api'
import { AgentChatView } from './AgentChatView'
import { consumeAgentChatSessionNavigation, requestAgentChatSessionNavigation } from './session-navigation'
import { useToolNavigation } from '@/components/Chat/tool-navigation'

function FlushableProjectPage({
  flush,
  onFlushHandlerChange,
}: {
  flush: EditorFlushHandler
  onFlushHandlerChange: (handler: EditorFlushHandler | null) => void
}) {
  useEffect(() => {
    onFlushHandlerChange(flush)
    return () => onFlushHandlerChange(null)
  }, [flush, onFlushHandlerChange])
  return <div data-testid="flushable-project-page">project page</div>
}

function ToolNavigationFileProbe() {
  const navigation = useToolNavigation()
  return (
    <button type="button" onClick={() => navigation?.open({ kind: 'workspace_file', path: 'src/from-tool.ts' })}>
      open tool file
    </button>
  )
}

describe('AgentChatView project workbenches', () => {
  beforeEach(() => {
    window.localStorage.clear()
    consumeAgentChatSessionNavigation()
    setDocumentReviewFeedback(null)
    vi.mocked(getAgentChatProjects)
      .mockReset()
      .mockResolvedValue([project('/books/a', 'Project A', 'session-a', 'Chat A'), project('/books/b', 'Project B', 'session-b', 'Chat B')])
    vi.mocked(createAgentChatSession).mockReset()
    vi.mocked(addAgentChatProject).mockReset()
    vi.mocked(archiveAgentChatProject).mockReset().mockResolvedValue()
    vi.mocked(relinkAgentChatProject).mockReset()
    vi.mocked(selectAgentChatProjectDirectory).mockReset()
    vi.mocked(getAgentChatHistory).mockReset().mockResolvedValue({ items: [], total: 0, offset: 0, has_more: false })
    vi.mocked(closeTerminalSession).mockReset().mockResolvedValue()
    vi.mocked(getTerminalRuntimeStatus)
      .mockReset()
      .mockResolvedValue({
        enabled: true,
        shell: '/bin/sh',
        commands: [
          { id: 'codex', name: 'Codex CLI' },
          { id: 'claude', name: 'Claude Code' },
        ],
        default_cwd: '/books/a',
        max_sessions: 8,
        scrollback_kb: 256,
        sessions: [],
      })
  })

  it('shows structural placeholders instead of an empty Project state during initial loading', async () => {
    let resolveProjects!: (projects: Awaited<ReturnType<typeof getAgentChatProjects>>) => void
    vi.mocked(getAgentChatProjects).mockReset().mockImplementation(() => new Promise((resolve) => {
      resolveProjects = resolve
    }))

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    expect(screen.queryByText('还没有可管理的项目，请先添加一个目录。')).not.toBeInTheDocument()
    expect(document.querySelector('[data-slot="loading-state-conversation"]')).toBeInTheDocument()
    expect(document.querySelector('[data-slot="loading-state-list"]')).toBeInTheDocument()

    resolveProjects([])
    await waitFor(() => expect(screen.getByText('还没有可管理的项目，请先添加一个目录。')).toBeInTheDocument())
  })

  it('opens a queued durable conversation after its Project snapshot is reconciled', async () => {
    requestAgentChatSessionNavigation({ projectId: 'project-a', sessionId: 'session-a' })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    expect(await screen.findByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
  })

  it('offers both Project entry paths before adding a selected folder', async () => {
    const user = userEvent.setup()
    const added = project('/projects/story', 'story', '', '')
    added.id = 'project-story'
    added.type = 'general'
    added.total = 0
    added.sessions = []
    vi.mocked(getAgentChatProjects).mockReset().mockResolvedValueOnce([]).mockResolvedValue([added])
    vi.mocked(selectAgentChatProjectDirectory).mockResolvedValue({ path: '/projects/story', canceled: false })
    vi.mocked(addAgentChatProject).mockResolvedValue(added)

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click((await screen.findAllByRole('button', { name: '添加项目' }))[0])
    const dialog = await screen.findByRole('dialog', { name: '添加项目' })
    expect(within(dialog).getByRole('button', { name: /打开目录/ })).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: /创建新书籍/ })).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: /打开目录/ }))
    await waitFor(() => expect(addAgentChatProject).toHaveBeenCalledWith('/projects/story'))
    expect(selectAgentChatProjectDirectory).toHaveBeenCalledWith(undefined)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'story' })).toBeInTheDocument()
  })

  it('opens the shared Book form after the creation guard succeeds', async () => {
    const user = userEvent.setup()
    const onBeforeCreateBook = vi.fn(async () => true)
    vi.mocked(getAgentChatProjects).mockReset().mockResolvedValue([])

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        novaDir="/nova"
        renderPage={() => null}
        renderReview={() => null}
        onBeforeCreateBook={onBeforeCreateBook}
      />,
    )

    await user.click((await screen.findAllByRole('button', { name: '添加项目' }))[0])
    await user.click(within(await screen.findByRole('dialog', { name: '添加项目' })).getByRole('button', { name: /创建新书籍/ }))

    expect(onBeforeCreateBook).toHaveBeenCalledOnce()
    expect(await screen.findByRole('dialog', { name: '新建书籍' })).toBeInTheDocument()
  })

  it('does not relink a Project when one mounted page draft cannot flush', async () => {
    const user = userEvent.setup()
    const flush = vi.fn<EditorFlushHandler>().mockResolvedValue(false)
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [{
            kind: 'page',
            id: 'reader-tab',
            projectId: 'project-a',
            workspace: '/books/a',
            group: 'primary',
            pageId: 'reader',
          }],
          activeTabIds: { primary: 'reader-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    vi.mocked(selectAgentChatProjectDirectory).mockResolvedValue({ path: '/books/a-moved', canceled: false })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_projectId, _workspace, _pageId, context) => (
          <FlushableProjectPage flush={flush} onFlushHandlerChange={context.onFlushHandlerChange} />
        )}
        renderReview={() => null}
      />,
    )

    expect(await screen.findByTestId('flushable-project-page')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Project A 的项目操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '重新关联目录' }))

    await waitFor(() => expect(flush).toHaveBeenCalledTimes(1))
    expect(selectAgentChatProjectDirectory).toHaveBeenCalledWith('/books/a')
    expect(relinkAgentChatProject).not.toHaveBeenCalled()
  })

  it('keeps the archive confirmation open when one mounted page draft cannot flush', async () => {
    const user = userEvent.setup()
    const flush = vi.fn<EditorFlushHandler>().mockResolvedValue(false)
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [{
            kind: 'page',
            id: 'lore-tab',
            projectId: 'project-a',
            workspace: '/books/a',
            group: 'primary',
            pageId: 'lore',
          }],
          activeTabIds: { primary: 'lore-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_projectId, _workspace, _pageId, context) => (
          <FlushableProjectPage flush={flush} onFlushHandlerChange={context.onFlushHandlerChange} />
        )}
        renderReview={() => null}
      />,
    )

    expect(await screen.findByTestId('flushable-project-page')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Project A 的项目操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '从项目中移除' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: '从项目中移除' }))

    await waitFor(() => expect(flush).toHaveBeenCalledTimes(1))
    expect(archiveAgentChatProject).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })

  it('toggles the activity tree into a persistent compact rail', async () => {
    const user = userEvent.setup()
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: '隐藏项目导航' }))
    expect(screen.getByRole('button', { name: '显示项目导航' })).toBeInTheDocument()
    expect(window.localStorage.getItem('nova.agentchat.sidebarVisible.v1')).toBe('false')

    await user.click(screen.getByRole('button', { name: '显示项目导航' }))
    expect(screen.getByRole('button', { name: '隐藏项目导航' })).toBeInTheDocument()
    expect(window.localStorage.getItem('nova.agentchat.sidebarVisible.v1')).toBe('true')
  })

  it('keeps a separate tab set and mounted conversation for every selected project', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'tab-a',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'tab-a', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
        'project-b': {
          tabs: [
            {
              kind: 'agent',
              id: 'tab-b',
              projectId: 'project-b',
              workspace: '/books/b',
              group: 'primary',
              sessionId: 'session-b',
            },
          ],
          activeTabIds: { primary: 'tab-b', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    expect(await screen.findByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.queryByTestId('conversation:/books/b:session-b')).not.toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: /^Chat A/ }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat A/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: /^Chat A/ })).toHaveAttribute('aria-current', 'page')

    await user.click(screen.getByRole('button', { name: '展开 Project B' }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat B/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('active')

    await user.click(screen.getByRole('button', { name: '收起 Project A' }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat A/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('hidden')

    await waitFor(() => {
      const stored = readStoredWorkbenchState()
      expect(stored.activeProjectId).toBe('project-a')
      expect(stored.projects['project-a'].tabs).toHaveLength(1)
      expect(stored.projects['project-b'].tabs).toHaveLength(1)
    })
  })

  it('mounts a restored background conversation only when its session is still running', async () => {
    const projectA = project('/books/a', 'Project A', 'session-a', 'Chat A')
    const projectB = project('/books/b', 'Project B', 'session-b', 'Chat B')
    projectB.sessions[0].running = true
    vi.mocked(getAgentChatProjects).mockResolvedValue([projectA, projectB])
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [agentTabForProject('tab-a', 'project-a', '/books/a', 'session-a')],
          activeTabIds: { primary: 'tab-a', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
        'project-b': {
          tabs: [agentTabForProject('tab-b', 'project-b', '/books/b', 'session-b')],
          activeTabIds: { primary: 'tab-b', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    expect(await screen.findByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(await screen.findByTestId('conversation:/books/b:session-b')).toHaveTextContent('hidden')
  })

  it('keeps a detached running conversation in the activity list after its tab closes', async () => {
    const user = userEvent.setup()
    const runningProject = project('/books/a', 'Project A', 'session-a', 'Chat A')
    runningProject.sessions[0].running = true
    vi.mocked(getAgentChatProjects).mockResolvedValue([runningProject])

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: /Chat A.*运行中/ }))
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')

    await user.click(await screen.findByRole('button', { name: '关闭 Chat A' }))
    expect(screen.queryByTestId('conversation:/books/a:session-a')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Chat A.*运行中/ })).toBeInTheDocument()
  })

  it('opens one local draft without creating a backend conversation', async () => {
    const user = userEvent.setup()
    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const createButton = await screen.findByRole('button', {
      name: '在 Project A 中新建对话',
    })
    await user.click(createButton)
    await user.click(createButton)

    expect(createAgentChatSession).not.toHaveBeenCalled()
    expect(screen.getAllByTestId('draft-conversation')).toHaveLength(1)
  })

  it('uses the full split separator as one visible resize target', async () => {
    const splitProject = project('/books/a', 'Project A', 'session-a', 'Chat A')
    splitProject.total = 2
    splitProject.sessions.push({
      ...splitProject.sessions[0],
      id: 'session-secondary',
      title: 'Secondary',
    })
    vi.mocked(getAgentChatProjects).mockResolvedValue([splitProject])
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
            {
              kind: 'agent',
              id: 'secondary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
              sessionId: 'session-secondary',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: 'secondary-tab' },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const separator = await screen.findByRole('separator', {
      name: '调整分栏宽度',
    })
    expect(separator).toHaveClass('nova-resize-handle', 'nova-resize-divider', 'nova-resize-divider-vertical', 'w-2')
  })

  it('hides and restores the secondary pane without unmounting its conversation', async () => {
    const user = userEvent.setup()
    const splitProject = project('/books/a', 'Project A', 'session-a', 'Chat A')
    splitProject.total = 2
    splitProject.sessions.push({
      ...splitProject.sessions[0],
      id: 'session-secondary',
      title: 'Secondary',
    })
    vi.mocked(getAgentChatProjects).mockResolvedValue([splitProject])
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
            {
              kind: 'agent',
              id: 'secondary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
              sessionId: 'session-secondary',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: 'secondary-tab' },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    const hideButton = await screen.findByRole('button', { name: '隐藏右侧工作区' })
    const fixedControlHost = hideButton.closest('[data-slot="agent-chat-secondary-pane-control-host"]')
    expect(fixedControlHost).toBeInTheDocument()
    expect(hideButton.closest('[data-agent-chat-group]')).toBeNull()
    await user.click(hideButton)
    expect(screen.queryByRole('separator', { name: '调整分栏宽度' })).not.toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('hidden')
    await waitFor(() => expect(readStoredWorkbenchState().projects['project-a'].secondaryVisible).toBe(false))

    const showButton = screen.getByRole('button', { name: '显示右侧工作区' })
    expect(showButton).toBe(hideButton)
    expect(showButton.closest('[data-slot="agent-chat-secondary-pane-control-host"]')).toBe(fixedControlHost)
    await user.click(showButton)
    expect(await screen.findByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: '隐藏右侧工作区' })).toBe(hideButton)
  })

  it('lets the first secondary-pane click choose what to open there', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'primary-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'primary-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_projectId, _workspace, pageId) => <div data-testid="secondary-page">{pageId}</div>}
        renderReview={() => null}
      />,
    )

    await user.click(await screen.findByRole('button', { name: '显示右侧工作区' }))
    await user.click(await screen.findByRole('menuitem', { name: '写作' }))

    expect(await screen.findByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()
    expect(screen.getByTestId('secondary-page')).toHaveTextContent('reader')
    expect(readStoredWorkbenchState().projects['project-a'].secondaryVisible).toBe(true)
  })

  it('opens the general Files tab in the secondary workspace without replacing the active chat', async () => {
    const user = userEvent.setup()
    const general = project('/books/a', 'Project A', 'session-a', 'Chat A')
    general.type = 'general'
    vi.mocked(getAgentChatProjects).mockResolvedValue([general])
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [agentTabForProject('agent-tab', 'project-a', '/books/a', 'session-a')],
          activeTabIds: { primary: 'agent-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)
    await user.click(await screen.findByRole('button', { name: '显示右侧工作区' }))
    await user.click(await screen.findByRole('menuitem', { name: '文件' }))

    expect(await screen.findByTestId('project-files-tab')).toHaveTextContent('no-selection')
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(readStoredWorkbenchState().projects['project-a'].tabs).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'files', group: 'secondary' }),
    ]))
  })

  it('reuses a temporary child-Agent tab in the opposite workspace without persisting it', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            agentTabForProject('agent-tab', 'project-a', '/books/a', 'session-a'),
            {
              kind: 'files',
              id: 'files-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
            },
          ],
          activeTabIds: { primary: 'agent-tab', secondary: 'files-tab' },
          focusedGroup: 'primary',
          secondaryVisible: true,
        },
      },
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await user.click(await screen.findByRole('button', { name: 'open child Agent' }))
    expect(await screen.findByRole('tab', { name: /Researcher/ })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(screen.getByRole('tab', { name: /文件/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'open child Agent' }))
    expect(screen.getAllByRole('tab', { name: /Researcher/ })).toHaveLength(1)
    await waitFor(() => {
      const stored = Array.from({ length: window.localStorage.length }, (_, index) => window.localStorage.getItem(window.localStorage.key(index) || '') || '').join('\n')
      expect(stored).not.toContain('"kind":"subagent"')
    })

    expect(screen.queryByRole('button', { name: '关闭 SubAgent 详情' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '关闭 Researcher' }))
    expect(await screen.findByTestId('project-files-tab')).toHaveTextContent('no-selection')
  })

  it('opens a tool path in the Files tab owned by that Agent Chat project', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-b',
      projects: {
        'project-b': {
          tabs: [{
            kind: 'page',
            id: 'lore-b',
            projectId: 'project-b',
            workspace: '/books/b',
            group: 'primary',
            pageId: 'lore',
          }],
          activeTabIds: { primary: 'lore-b', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => <ToolNavigationFileProbe />}
        renderReview={() => null}
      />,
    )
    await user.click(await screen.findByRole('button', { name: 'open tool file' }))

    expect(await screen.findByTestId('project-files-tab')).toHaveTextContent('src/from-tool.ts')
    expect(readStoredWorkbenchState().projects['project-b'].tabs).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'files', projectId: 'project-b', workspace: '/books/b', selectedPath: 'src/from-tool.ts' }),
    ]))
    expect(readStoredWorkbenchState().projects['project-a']?.tabs || []).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'files', selectedPath: 'src/from-tool.ts' }),
    ]))
  })

  it('synchronizes Files and project pages without echoing the mutation owner', async () => {
    const user = userEvent.setup()
    vi.mocked(getAgentChatProjects).mockResolvedValue([
      project('/books/a', 'Project A', 'session-a', 'Chat A'),
    ])
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [agentTabForProject('agent-tab', 'project-a', '/books/a', 'session-a')],
          activeTabIds: { primary: 'agent-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_projectId, _workspace, _pageId, context) => (
          <div data-testid="project-page">
            <output data-testid="project-page-refresh">page:{context.refreshSignal}</output>
            <button
              type="button"
              onClick={() => context.onWorkspaceChanged(
                ['chapters/ch01.md'],
                { impact: 'content', origin: 'project-page' },
              )}
            >
              simulate page save
            </button>
          </div>
        )}
        renderReview={() => null}
      />,
    )
    await user.click(await screen.findByRole('button', { name: '显示右侧工作区' }))
    await user.click(await screen.findByRole('menuitem', { name: '文件' }))

    const files = await screen.findByTestId('project-files-tab')
    expect(screen.getByTestId('project-files-refresh')).toHaveTextContent('editor:0|tree:0')
    await user.click(within(files).getByRole('button', { name: 'simulate local save' }))
    await user.click(within(files).getByRole('button', { name: 'simulate local file operation' }))
    expect(screen.getByTestId('project-files-refresh')).toHaveTextContent('editor:0|tree:0')

    const secondaryPane = files.closest('aside')
    expect(secondaryPane).not.toBeNull()
    await user.click(within(secondaryPane as HTMLElement).getByRole('button', { name: '新建标签页' }))
    await user.click(await screen.findByRole('menuitem', { name: '写作' }))
    expect(await screen.findByTestId('project-page-refresh')).toHaveTextContent('page:2')

    await user.click(screen.getByRole('button', { name: 'simulate page save' }))
    await waitFor(() => expect(screen.getByTestId('project-files-refresh')).toHaveTextContent('editor:1|tree:0'))
    expect(screen.getByTestId('project-page-refresh')).toHaveTextContent('page:2')

    const conversation = screen.getByTestId('conversation:/books/a:session-a')
    await user.click(within(conversation).getByRole('button', { name: 'simulate external content' }))
    await waitFor(() => expect(screen.getByTestId('project-files-refresh')).toHaveTextContent('editor:2|tree:0'))
    expect(screen.getByTestId('project-page-refresh')).toHaveTextContent('page:3')
    await user.click(within(conversation).getByRole('button', { name: 'simulate external structure' }))
    await waitFor(() => expect(screen.getByTestId('project-files-refresh')).toHaveTextContent('editor:3|tree:1'))
    expect(screen.getByTestId('project-page-refresh')).toHaveTextContent('page:4')
  })

  it('routes a reviewed source file into the reusable Files tab in the same pane', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            agentTabForProject('agent-tab', 'project-a', '/books/a', 'session-a'),
            {
              kind: 'review',
              id: 'review-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'secondary',
              threadID: 'thread-one',
            },
          ],
          activeTabIds: { primary: 'agent-tab', secondary: 'review-tab' },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={() => null}
        renderReview={(_tab, _disabled, context) => (
          <button type="button" onClick={() => context.openFile('src/main.ts')}>open reviewed source</button>
        )}
      />,
    )
    await user.click(await screen.findByRole('button', { name: 'open reviewed source' }))

    expect(await screen.findByTestId('project-files-tab')).toHaveTextContent('src/main.ts')
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('active')
    expect(readStoredWorkbenchState().projects['project-a'].tabs).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'files', group: 'secondary', selectedPath: 'src/main.ts' }),
    ]))
  })

  it('releases only detached terminal sessions that no persisted tab owns', async () => {
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'terminal',
              id: 'owned-by-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              profileId: 'shell',
              title: '',
            },
          ],
          activeTabIds: { primary: 'owned-by-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    vi.mocked(getTerminalRuntimeStatus).mockResolvedValue({
      enabled: true,
      shell: '/bin/sh',
      commands: [
        { id: 'codex', name: 'Codex CLI' },
        { id: 'claude', name: 'Claude Code' },
      ],
      default_cwd: '/books/a',
      max_sessions: 8,
      scrollback_kb: 256,
      sessions: [
        terminalSession('owned-session', 'owned-by-tab'),
        terminalSession('orphan-session', 'missing-tab'),
        terminalSession('active-orphan-session', 'active-missing-tab', 1),
      ],
    })

    renderView(<AgentChatView composerSettings={{} as never} tellers={[]} imagePresets={[]} renderPage={() => null} renderReview={() => null} />)

    await waitFor(() => expect(closeTerminalSession).toHaveBeenCalledWith('orphan-session'))
    expect(closeTerminalSession).toHaveBeenCalledTimes(1)
  })

  it('opens a document comment in the matching shared project page beside its conversation', async () => {
    const user = userEvent.setup()
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [
            {
              kind: 'agent',
              id: 'agent-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              group: 'primary',
              sessionId: 'session-a',
            },
          ],
          activeTabIds: { primary: 'agent-tab', secondary: null },
          focusedGroup: 'primary',
          secondaryVisible: false,
        },
      },
    })
    const feedback = {
      source: 'document' as const,
      reviewThreadId: 'document-thread',
      comments: [
        {
          id: 'lore-comment',
          body: '补足人物动机',
          target: { kind: 'lore_item' as const, id: 'lin-chuan' },
        },
      ],
    }
    setDocumentReviewFeedback(feedback)

    renderView(
      <AgentChatView
        composerSettings={{} as never}
        tellers={[]}
        imagePresets={[]}
        renderPage={(_projectId, _workspace, pageId, context) => (
          <div data-testid="review-target-page">
            {pageId}|{context.navigationIntent?.target.id || 'none'}|{context.navigationIntent?.commentID || 'none'}|{context.navigationIntent?.nonce || 0}
          </div>
        )}
        renderReview={() => null}
      />,
    )

    await user.click(
      await screen.findByRole('button', {
        name: 'open pending document feedback',
      }),
    )
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|1'))
    expect(screen.getByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'open pending document feedback' }))
    await waitFor(() => expect(screen.getByTestId('review-target-page')).toHaveTextContent('lore|lin-chuan|lore-comment|2'))
  })
})

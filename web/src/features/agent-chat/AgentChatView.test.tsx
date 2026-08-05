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

describe('AgentChatView project workbenches', () => {
  beforeEach(() => {
    window.localStorage.clear()
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

  it('adds a selected folder without asking for a name or Agent type', async () => {
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
    await waitFor(() => expect(addAgentChatProject).toHaveBeenCalledWith('/projects/story'))
    expect(selectAgentChatProjectDirectory).toHaveBeenCalledWith(undefined)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(await screen.findByTitle('/projects/story')).toBeInTheDocument()
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

    await user.click(await screen.findByRole('button', { name: '隐藏活动列表' }))
    expect(screen.getByRole('button', { name: '显示活动列表' })).toBeInTheDocument()
    expect(window.localStorage.getItem('nova.agentchat.sidebarVisible.v1')).toBe('false')

    await user.click(screen.getByRole('button', { name: '显示活动列表' }))
    expect(screen.getByRole('button', { name: '隐藏活动列表' })).toBeInTheDocument()
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

    await user.click(screen.getByRole('button', { name: /Chat B/ }))
    expect(
      within(screen.getAllByRole('tablist')[0]).getByRole('tab', {
        name: /Chat B/,
      }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-a')).toHaveTextContent('hidden')
    expect(screen.getByTestId('conversation:/books/b:session-b')).toHaveTextContent('active')

    await user.click(screen.getByTitle('/books/a'))
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
    expect(hideButton.closest('[data-agent-chat-group]')).toHaveAttribute('data-agent-chat-group', 'secondary')
    await user.click(hideButton)
    expect(screen.queryByRole('separator', { name: '调整分栏宽度' })).not.toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('hidden')
    await waitFor(() => expect(readStoredWorkbenchState().projects['project-a'].secondaryVisible).toBe(false))

    const showButton = screen.getByRole('button', { name: '显示右侧工作区' })
    expect(showButton.closest('[data-agent-chat-group]')).toHaveAttribute('data-agent-chat-group', 'primary')
    await user.click(showButton)
    expect(await screen.findByRole('separator', { name: '调整分栏宽度' })).toBeInTheDocument()
    expect(screen.getByTestId('conversation:/books/a:session-secondary')).toHaveTextContent('active')
    expect(screen.getByRole('button', { name: '隐藏右侧工作区' }).closest('[data-agent-chat-group]'))
      .toHaveAttribute('data-agent-chat-group', 'secondary')
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
    await user.click(await screen.findByRole('menuitem', { name: '阅读器' }))

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
    await user.click(await screen.findByRole('menuitem', { name: '阅读器' }))
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
              profileId: 'shell',
              title: '',
            },
            {
              kind: 'terminal',
              id: 'legacy-tab',
              projectId: 'project-a',
              workspace: '/books/a',
              profileId: 'shell',
              title: '',
              terminalSessionId: 'legacy-session',
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
        terminalSession('legacy-session', undefined),
        terminalSession('orphan-session', 'missing-tab'),
        terminalSession('active-legacy-session', undefined, 1),
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

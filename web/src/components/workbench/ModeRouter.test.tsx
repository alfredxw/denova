import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState, type ComponentProps, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { ModeRouter } from './ModeRouter'
import type { Tab } from './TabController'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))
const useDocumentReviewMock = vi.hoisted(() => vi.fn())
const agentPanelLifecycle = vi.hoisted(() => ({ mounts: 0, unmounts: 0, renders: 0 }))
const agentChatRouteLifecycle = vi.hoisted(() => ({ renders: 0 }))
const markdownEditorLifecycle = vi.hoisted(() => ({ mounts: 0, unmounts: 0 }))
const loreLibraryFlushMock = vi.hoisted(() => vi.fn(async () => true))
const agentChatFlushMock = vi.hoisted(() => vi.fn(async () => true))
const writingChangeReviewMock = vi.hoisted(() => ({
  activeReviewThreadID: '',
  activeReviewRequest: null,
  reviewFeedback: null,
  submittedReviewCommentIDs: new Set<string>(),
  openChangeReview: vi.fn(),
  closeChangeReview: vi.fn(),
  selectReviewFeedback: vi.fn(),
  removeReviewFeedback: vi.fn(),
  submitReviewFeedback: vi.fn(),
  restoreReviewFeedback: vi.fn(),
}))

function withAppProviders(ui: ReactNode) {
  return <TooltipProvider>{ui}</TooltipProvider>
}

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@/hooks/usePersistedUserSettings', () => ({
  usePersistedUserSettings: vi.fn(),
}))

vi.mock('@/components/Chat/AgentPanel', () => ({
  WRITING_COMPOSER_SETTING_DEFAULTS: {
    ide_story_teller_id: 'rhythm',
    interactive_story_teller_id: 'rhythm',
    ide_image_preset_id: 'game-cg',
    writing_skill_default: 'novel-lite',
  },
  AgentPanel: ({ reviewFeedback, onReviewFeedbackOpen }: {
    reviewFeedback?: Array<{ comments: Array<{ id: string }> }>
    onReviewFeedbackOpen?: (selection: unknown, comment: unknown) => void
  }) => {
    const [localState, setLocalState] = useState(0)
    agentPanelLifecycle.renders += 1
    useEffect(() => {
      agentPanelLifecycle.mounts += 1
      return () => {
        agentPanelLifecycle.unmounts += 1
      }
    }, [])
    const selection = reviewFeedback?.[0]
    const comment = selection?.comments[0]
    return (
      <>
        <button type="button" onClick={() => setLocalState((current) => current + 1)}>
          agent panel state {localState}
        </button>
        <button type="button" disabled={!selection || !comment} onClick={() => onReviewFeedbackOpen?.(selection, comment)}>
          open document feedback
        </button>
      </>
    )
  },
}))

vi.mock('@/features/agent-chat/WritingAgentWorkspace', () => ({
  readWritingSessionRailVisibility: () => true,
  writeWritingSessionRailVisibility: vi.fn(),
  WRITING_SESSION_RAIL_STORAGE_KEY: 'nova.writingAgent.sessionRailVisible.v1',
  WritingAgentWorkspace: ({ reviewFeedback, onReviewFeedbackOpen }: {
    reviewFeedback?: Array<{ comments: Array<{ id: string }> }>
    onReviewFeedbackOpen?: (selection: unknown, comment: unknown) => void
  }) => {
    const [localState, setLocalState] = useState(0)
    agentPanelLifecycle.renders += 1
    useEffect(() => {
      agentPanelLifecycle.mounts += 1
      return () => {
        agentPanelLifecycle.unmounts += 1
      }
    }, [])
    const selection = reviewFeedback?.[0]
    const comment = selection?.comments[0]
    return (
      <>
        <button type="button" onClick={() => setLocalState((current) => current + 1)}>
          agent panel state {localState}
        </button>
        <button type="button" disabled={!selection || !comment} onClick={() => onReviewFeedbackOpen?.(selection, comment)}>
          open document feedback
        </button>
      </>
    )
  },
}))

vi.mock('@/features/agent-chat/AgentChatRoute', () => ({
  AgentChatRoute: ({ projectId, onFlushHandlerChange }: {
    projectId: string
    onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  }) => {
    agentChatRouteLifecycle.renders += 1
    useEffect(() => {
      onFlushHandlerChange?.(agentChatFlushMock)
      return () => onFlushHandlerChange?.(null)
    }, [onFlushHandlerChange])
    return (
      <div
        data-testid="agent-chat-route"
        data-project-id={projectId}
      >
        project-scoped agent route
      </div>
    )
  },
}))

vi.mock('@/features/lore/LoreWorkspaceTab', () => ({
  LoreWorkspaceTab: ({ navigationIntent }: {
    navigationIntent?: { commentID: string; nonce: number } | null
  }) => (
    <div data-testid="lore-workspace-navigation">
      {navigationIntent?.commentID || 'none'}|{navigationIntent?.nonce || 0}
    </div>
  ),
}))

vi.mock('@/features/interactive/components/SettingPanel', () => ({
  SettingPanel: ({ onFlushHandlerChange }: {
    onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  }) => {
    useEffect(() => {
      onFlushHandlerChange?.(loreLibraryFlushMock)
      return () => onFlushHandlerChange?.(null)
    }, [onFlushHandlerChange])
    return <div data-testid="full-lore-library">full lore library</div>
  },
}))

vi.mock('@/components/Editor/WritingDocumentEditor', () => ({
  WritingDocumentEditor: ({ fileName, chapterSummary, onRevealChapter, documentReviewNavigationIntent, searchIntent }: {
    fileName: string | null
    chapterSummary?: { path: string }
    onRevealChapter?: (path: string) => void
    documentReviewNavigationIntent?: { commentID: string; nonce: number } | null
    searchIntent?: { query: string; line: number; nonce: number } | null
  }) => {
    useEffect(() => {
      markdownEditorLifecycle.mounts += 1
      return () => {
        markdownEditorLifecycle.unmounts += 1
      }
    }, [])
    return (
      <>
        <div data-testid="markdown-editor-navigation">
          {fileName || 'none'}|{documentReviewNavigationIntent?.commentID || 'none'}|{documentReviewNavigationIntent?.nonce || 0}
        </div>
        <div data-testid="markdown-search-navigation">
          {searchIntent ? `${searchIntent.query}|${searchIntent.line}|${searchIntent.nonce}` : 'none'}
        </div>
        {chapterSummary ? (
          <button type="button" onClick={() => onRevealChapter?.(chapterSummary.path)}>reveal chapter in outline</button>
        ) : null}
      </>
    )
  },
}))

vi.mock('@/components/Editor/WritingSourceEditor', () => ({
  WritingSourceEditor: ({ document }: { document: { path: string; content?: string } }) => (
    <div data-testid="writing-source-editor" data-content={document.content}>{document.path}</div>
  ),
}))

vi.mock('@/features/interactive/api', () => ({
  getImagePresets: vi.fn(async () => []),
  getInteractiveTellers: vi.fn(async () => []),
}))

vi.mock('@/features/interactive/stores/interactive-store', () => ({
  useInteractiveStore: (selector: (state: { submode: string; setSubmode: () => void }) => unknown) => selector({
    submode: 'story',
    setSubmode: vi.fn(),
  }),
}))

vi.mock('@/features/project-explorer/WritingProjectExplorer', () => ({
  WritingProjectExplorer: ({ projectId, structureRefreshSignal, onSelectFile }: {
    projectId: string
    structureRefreshSignal: number
    onSelectFile: (path: string) => void
  }) => (
    <div data-testid="writing-project-explorer" data-project-id={projectId} data-structure-refresh-signal={structureRefreshSignal}>
      <button type="button">lore</button>
      <button type="button" onClick={() => onSelectFile('setting/lore/items.json')}>items.json</button>
    </div>
  ),
}))

vi.mock('@/features/changes/use-writing-change-review', () => ({
  useWritingChangeReview: () => writingChangeReviewMock,
}))

vi.mock('@/features/changes/review/ChangeReviewWorkspace', () => ({
  ChangeReviewWorkspace: () => <div data-testid="change-review-workspace">change review</div>,
}))

vi.mock('@/features/document-review/use-document-review', () => ({
  useDocumentReview: useDocumentReviewMock,
}))

vi.mock('./WorkbenchShell', () => ({
  WorkbenchShell: ({ onQuickSwitchBook, sidebar, main, rightPanelContent }: {
    onQuickSwitchBook: (path: string) => Promise<boolean>
    sidebar: ReactNode
    main: ReactNode
    rightPanelContent: ReactNode
  }) => (
    <>
      <button type="button" onClick={() => { void onQuickSwitchBook('/book-b') }}>
        quick switch
      </button>
      {sidebar}
      {main}
      {rightPanelContent}
    </>
  ),
}))

describe('ModeRouter autosave navigation policy', () => {
  beforeEach(() => {
    toastMock.warning.mockReset()
    useDocumentReviewMock.mockReset()
    loreLibraryFlushMock.mockReset()
    loreLibraryFlushMock.mockResolvedValue(true)
    agentChatFlushMock.mockReset()
    agentChatFlushMock.mockResolvedValue(true)
    agentPanelLifecycle.mounts = 0
    agentPanelLifecycle.unmounts = 0
    agentPanelLifecycle.renders = 0
    agentChatRouteLifecycle.renders = 0
    markdownEditorLifecycle.mounts = 0
    markdownEditorLifecycle.unmounts = 0
    writingChangeReviewMock.activeReviewThreadID = ''
    useDocumentReviewMock.mockReturnValue({
      feedback: null,
      thread: { comments: [] },
      addComment: vi.fn(),
      editComment: vi.fn(),
      removeComment: vi.fn(),
      removeFeedback: vi.fn(),
      submitFeedback: vi.fn(),
      restoreFeedback: vi.fn(),
    })
    vi.mocked(usePersistedUserSettings).mockReturnValue({
      values: {
        ide_story_teller_id: 'classic',
        interactive_story_teller_id: 'rhythm',
        ide_image_preset_id: 'game-cg',
        writing_skill_default: 'novel-lite',
      },
      loading: false,
      isSaving: vi.fn(() => true),
      persist: vi.fn(async () => true),
      reload: vi.fn(async () => null),
      flushPending: vi.fn(async () => false),
    })
  })

  it('stacks Diff Review above retained document-tab overlays', () => {
    writingChangeReviewMock.activeReviewThreadID = 'review-thread'

    const { container } = render(withAppProviders(<ModeRouter {...modeRouterProps()} />))

    const writingLayer = container.querySelector('[data-writing-content-layer="true"]')
    const reviewLayer = container.querySelector('[data-change-review-layer="true"]')
    expect(writingLayer).toHaveClass('z-0')
    expect(writingLayer).toHaveAttribute('aria-hidden', 'true')
    expect(reviewLayer).toHaveClass('z-10')
    expect(screen.getByTestId('change-review-workspace')).toBeInTheDocument()
  })

  it('continues a workspace switch and warns when preference flush remains pending', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />))

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(onQuickSwitchBook).toHaveBeenCalledWith('/book-b'))
    expect(toastMock.warning).toHaveBeenCalledWith('偏好设置暂未保存', {
      description: '本地修改已保留，将在下次自动保存时重试。',
    })
  })

  it('does not wait for a slow preference request before switching workspaces', async () => {
    const user = userEvent.setup()
    let resolveFlush!: (saved: boolean) => void
    const pendingFlush = new Promise<boolean>((resolve) => { resolveFlush = resolve })
    vi.mocked(usePersistedUserSettings).mockReturnValue({
      values: {
        ide_story_teller_id: 'classic',
        interactive_story_teller_id: 'rhythm',
        ide_image_preset_id: 'game-cg',
        writing_skill_default: 'novel-lite',
      },
      loading: false,
      isSaving: vi.fn(() => true),
      persist: vi.fn(async () => true),
      reload: vi.fn(async () => null),
      flushPending: vi.fn(() => pendingFlush),
    })
    const onQuickSwitchBook = vi.fn(async () => true)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />))

    await user.click(screen.getByRole('button', { name: 'quick switch' }))
    expect(onQuickSwitchBook).toHaveBeenCalledWith('/book-b')

    resolveFlush(false)
    await waitFor(() => expect(toastMock.warning).toHaveBeenCalled())
  })

  it('flushes the retained full Lore library before switching workspaces', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    loreLibraryFlushMock.mockResolvedValue(false)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'lore', onQuickSwitchBook })} />))
    await screen.findByTestId('full-lore-library')

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(loreLibraryFlushMock).toHaveBeenCalledTimes(1))
    expect(onQuickSwitchBook).not.toHaveBeenCalled()
  })

  it('keeps the active book when an AgentChat project page cannot flush', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    agentChatFlushMock.mockResolvedValue(false)
    render(withAppProviders(<ModeRouter {...modeRouterProps({ mode: 'agentchat', onQuickSwitchBook })} />))
    await screen.findByTestId('agent-chat-route')

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(agentChatFlushMock).toHaveBeenCalledTimes(1))
    expect(onQuickSwitchBook).not.toHaveBeenCalled()
  })

  it('opens the referenced chapter before revealing its document review comment', async () => {
    const user = userEvent.setup()
    const comment = {
      id: 'document-comment',
      thread_id: 'document-thread',
      target: { kind: 'workspace_file' as const, id: 'chapters/ch02.md' },
      body: '正文这里需要更克制',
      created_at: '',
      updated_at: '',
      review_line: 111,
    }
    const feedback = {
      source: 'document' as const,
      reviewThreadId: 'document-thread',
      comments: [comment],
    }
    useDocumentReviewMock.mockReturnValue({
      feedback,
      thread: { comments: [comment] },
      addComment: vi.fn(),
      editComment: vi.fn(),
      removeComment: vi.fn(),
      removeFeedback: vi.fn(),
      submitFeedback: vi.fn(),
      restoreFeedback: vi.fn(),
    })
    const handleSelectFile = vi.fn(async (_path: string) => true)

    function Harness() {
      const [selectedFile, setSelectedFile] = useState('chapters/ch03.md')
      return (
        <ModeRouter
          {...modeRouterProps({
            rightPanel: 'ai',
            selectedFile,
            openTabs: [{ kind: 'file', path: 'chapters/ch03.md' }],
            activeTabKey: 'file:chapters/ch03.md',
            onSelectFile: async (path) => {
              const navigated = await handleSelectFile(path)
              if (navigated !== false) setSelectedFile(path)
              return navigated
            },
          })}
        />
      )
    }

    render(withAppProviders(<Harness />))
    await user.click(screen.getByRole('button', { name: 'open document feedback' }))

    await waitFor(() => expect(handleSelectFile).toHaveBeenCalledWith(comment.target.id))
    await waitFor(() => expect(screen.getByTestId('markdown-editor-navigation')).toHaveTextContent(
      `${comment.target.id}|${comment.id}|1`,
    ))

    await user.click(screen.getByRole('button', { name: 'open document feedback' }))
    await waitFor(() => expect(screen.getByTestId('markdown-editor-navigation')).toHaveTextContent(
      `${comment.target.id}|${comment.id}|2`,
    ))
    expect(handleSelectFile).toHaveBeenCalledTimes(1)
  })

  it('opens the Lore tab before revealing feedback for a lore item', async () => {
    const user = userEvent.setup()
    const comment = {
      id: 'lore-comment',
      thread_id: 'document-thread',
      target: { kind: 'lore_item' as const, id: 'lin-chuan', field: 'content' as const },
      body: '补足人物动机',
      created_at: '',
      updated_at: '',
      review_line: 12,
    }
    useDocumentReviewMock.mockReturnValue({
      feedback: {
        source: 'document' as const,
        reviewThreadId: 'document-thread',
        comments: [comment],
      },
      thread: { comments: [comment] },
      addComment: vi.fn(),
      editComment: vi.fn(),
      removeComment: vi.fn(),
      removeFeedback: vi.fn(),
      submitFeedback: vi.fn(),
      restoreFeedback: vi.fn(),
    })
    const openLoreTab = vi.fn(async () => true)

    function Harness() {
      const [loreOpen, setLoreOpen] = useState(false)
      return (
        <ModeRouter
          {...modeRouterProps({
            rightPanel: 'ai',
            openTabs: loreOpen ? [{ kind: 'lore' }] : [],
            activeTabKey: loreOpen ? 'lore' : null,
            onOpenLoreTab: async () => {
              const opened = await openLoreTab()
              if (opened) setLoreOpen(true)
              return opened
            },
          })}
        />
      )
    }

    render(withAppProviders(<Harness />))
    await user.click(screen.getByRole('button', { name: 'open document feedback' }))

    await waitFor(() => expect(openLoreTab).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByTestId('lore-workspace-navigation')).toHaveTextContent(
      `${comment.id}|1`,
    ))
  })

  it('routes setting/lore/items.json from the project tree to the Lore tab', async () => {
    const user = userEvent.setup()
    const onOpenLoreTab = vi.fn(async () => true)
    const onSelectFile = vi.fn()
    render(withAppProviders(<ModeRouter {...modeRouterProps({
      sidebarView: 'files',
      tree: [{
        name: 'setting',
        type: 'dir',
        children: [{
          name: 'lore',
          type: 'dir',
          children: [{ name: 'items.json', type: 'file' }],
        }],
      }],
      onOpenLoreTab,
      onSelectFile,
      projectExplorerRefreshSignal: 7,
      versionRefreshSignal: 11,
    })} />))

    expect(screen.getByTestId('writing-project-explorer')).toHaveAttribute('data-structure-refresh-signal', '7')

    await user.click(screen.getByText('lore'))
    await user.click(screen.getByText('items.json'))

    await waitFor(() => expect(onOpenLoreTab).toHaveBeenCalledTimes(1))
    expect(onSelectFile).not.toHaveBeenCalled()
  })

  it('opens the project sidebar and switches to the outline before locating the editor chapter', async () => {
    const user = userEvent.setup()
    const onToggleProjectVisible = vi.fn()
    const onSetSidebarView = vi.fn()
    const chapter = {
      path: 'chapters/ch01.md',
      file_name: 'ch01.md',
      display_title: '第一章',
      index: 1,
      words: 1200,
      status: 'draft',
      confirmed: false,
      updated_at: '2026-07-25T00:00:00Z',
      volume: '第一卷',
      volume_path: 'chapters/volume-01',
    }

    render(withAppProviders(<ModeRouter {...modeRouterProps({
      projectVisible: false,
      selectedFile: chapter.path,
      currentChapter: chapter,
      openTabs: [{ kind: 'file', path: chapter.path }],
      activeTabKey: `file:${chapter.path}`,
      onToggleProjectVisible,
      onSetSidebarView,
    })} />))

    await user.click(screen.getByRole('button', { name: 'reveal chapter in outline' }))

    expect(onToggleProjectVisible).toHaveBeenCalledTimes(1)
    expect(onSetSidebarView).toHaveBeenCalledWith('outline')
  })

  it('keeps the foreground Writing Agent mounted while Workspace owns separate scoped conversations', async () => {
    const user = userEvent.setup()
    const baseProps = modeRouterProps({ rightPanel: 'ai' })
    const { rerender } = render(withAppProviders(<ModeRouter {...baseProps} />))

    await user.click(screen.getByRole('button', { name: 'agent panel state 0' }))
    expect(screen.getByRole('button', { name: 'agent panel state 1' })).toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)

    rerender(withAppProviders(<ModeRouter {...baseProps} mode="agentchat" />))
    await waitFor(() => expect(screen.getByTestId('agent-chat-route')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: 'agent panel state 1' })).not.toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)
    expect(agentPanelLifecycle.unmounts).toBe(0)

    rerender(withAppProviders(<ModeRouter {...baseProps} mode="ide" />))
    expect(screen.getByRole('button', { name: 'agent panel state 1' })).toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)
    expect(agentPanelLifecycle.unmounts).toBe(0)
  })

  it('keeps the retained AgentChat route inert during writing chapter switches', async () => {
    const firstPath = 'chapters/ch01.md'
    const secondPath = 'chapters/ch02.md'
    const updatedSummary = { title: 'Updated Book' } as NonNullable<ComponentProps<typeof ModeRouter>['summary']>
    const baseProps = modeRouterProps({ mode: 'agentchat', selectedFile: firstPath })
    const { rerender } = render(withAppProviders(<ModeRouter {...baseProps} />))

    const agentChatRoute = await screen.findByTestId('agent-chat-route')
    expect(agentChatRoute).toHaveAttribute('data-project-id', baseProps.projectId)

    rerender(withAppProviders(<ModeRouter {...baseProps} mode="ide" />))
    await waitFor(() => expect(agentChatRoute.closest('section')).toHaveAttribute('hidden'))
    const rendersAfterHiding = agentChatRouteLifecycle.renders

    rerender(withAppProviders(<ModeRouter {...baseProps} mode="ide" selectedFile={secondPath} summary={updatedSummary} />))
    await act(async () => { await Promise.resolve() })
    expect(agentChatRouteLifecycle.renders).toBe(rendersAfterHiding)

    rerender(withAppProviders(<ModeRouter {...baseProps} mode="agentchat" selectedFile={secondPath} summary={updatedSummary} />))
    await waitFor(() => expect(agentChatRoute.closest('section')).not.toHaveAttribute('hidden'))
    expect(agentChatRouteLifecycle.renders).toBe(rendersAfterHiding)
    expect(agentChatRoute).toHaveAttribute('data-project-id', baseProps.projectId)
  })

  it('keeps the text editor mounted while switching files so per-file view positions survive', async () => {
    const firstPath = 'chapters/ch01.md'
    const secondPath = 'chapters/ch02.md'
    const openTabs: Tab[] = [
      { kind: 'file', path: firstPath },
      { kind: 'file', path: secondPath },
    ]
    const { rerender } = render(withAppProviders(<ModeRouter {...modeRouterProps({
      selectedFile: firstPath,
      fileContent: 'first',
      openTabs,
      activeTabKey: `file:${firstPath}`,
    })} />))

    expect(markdownEditorLifecycle.mounts).toBe(1)
    expect(markdownEditorLifecycle.unmounts).toBe(0)

    await act(async () => {
      rerender(withAppProviders(<ModeRouter {...modeRouterProps({
        selectedFile: secondPath,
        fileContent: 'second',
        openTabs,
        activeTabKey: `file:${secondPath}`,
      })} />))
    })

    expect(screen.getByTestId('markdown-editor-navigation')).toHaveTextContent(`${secondPath}|none|0`)
    expect(markdownEditorLifecycle.mounts).toBe(1)
    expect(markdownEditorLifecycle.unmounts).toBe(0)
  })

  it('delivers a global search reveal only after the target document is loaded', async () => {
    const firstPath = 'chapters/ch01.md'
    const secondPath = 'chapters/ch02.md'
    const intent = { path: secondPath, query: 'target', line: 12, nonce: 7 }
    const openTabs: Tab[] = [{ kind: 'file', path: secondPath }]
    const staleDocument = {
      project_id: 'project-book-a',
      path: firstPath,
      content: 'first',
      revision: 'r1',
      kind: 'text' as const,
      mime_type: 'text/markdown',
      size: 5,
      editable: true,
    }
    const { rerender } = render(withAppProviders(<ModeRouter {...modeRouterProps({
      selectedFile: secondPath,
      fileDocument: staleDocument,
      fileContent: staleDocument.content,
      fileRevision: staleDocument.revision,
      openTabs,
      activeTabKey: `file:${secondPath}`,
      editorSearchIntent: intent,
    })} />))

    await act(async () => { await Promise.resolve() })
    expect(screen.getByTestId('markdown-search-navigation')).toHaveTextContent('none')

    await act(async () => {
      rerender(withAppProviders(<ModeRouter {...modeRouterProps({
        selectedFile: secondPath,
        fileDocument: { ...staleDocument, path: secondPath, content: 'target', revision: 'r2' },
        fileContent: 'target',
        fileRevision: 'r2',
        openTabs,
        activeTabKey: `file:${secondPath}`,
        editorSearchIntent: intent,
      })} />))
      await Promise.resolve()
    })

    expect(screen.getByTestId('markdown-search-navigation')).toHaveTextContent('target|12|7')
  })

  it('renders a temporary SubAgent as the active writing content tab', async () => {
    const user = userEvent.setup()
    const onCloseTab = vi.fn()
    const subAgentTab: Tab = {
      kind: 'subagent',
      parentSessionId: 'parent-session',
      sessionKey: 'child-session',
      title: 'Researcher',
      returnTabKey: 'file:chapters/ch01.md',
    }
    render(withAppProviders(<ModeRouter {...modeRouterProps({
      openTabs: [{ kind: 'file', path: 'chapters/ch01.md' }, subAgentTab],
      activeTabKey: 'subagent',
      writingAgentConversation: { sessionId: 'parent-session', messages: [], isStreaming: true },
      onCloseTab,
    })} />))

    expect(await screen.findByRole('tab', { name: /Researcher/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: /SubAgent/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '关闭 SubAgent 详情' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '关闭 Researcher' }))
    expect(onCloseTab).toHaveBeenCalledWith(subAgentTab)
  })

  it('routes non-Markdown text tabs to Monaco source editing without transforming content', async () => {
    const path = 'data/events.jsonl'
    const content = '{"event":1}\nnot-json'
    render(withAppProviders(<ModeRouter {...modeRouterProps({
      selectedFile: path,
      fileDocument: {
        project_id: 'project-book-a',
        path,
        content,
        revision: 'r-source',
        kind: 'text',
        mime_type: 'application/x-ndjson',
        size: content.length,
        editable: true,
      },
      fileContent: content,
      fileRevision: 'r-source',
      openTabs: [{ kind: 'file', path }],
      activeTabKey: `file:${path}`,
    })} />))

    await waitFor(() => expect(screen.getByTestId('writing-source-editor')).toHaveAttribute('data-content', content))
    expect(screen.getByTestId('writing-source-editor')).toHaveTextContent(path)
    expect(screen.queryByTestId('markdown-editor-navigation')).not.toBeInTheDocument()
  })
})

function modeRouterProps(
  overrides: Partial<ComponentProps<typeof ModeRouter>> = {},
): ComponentProps<typeof ModeRouter> {
  return {
    mode: 'ide',
    lastCreationRoute: 'ide',
    currentBookName: 'Book A',
    workspace: '/book-a',
    projectId: 'project-book-a',
    summary: null,
    isStreaming: false,
    isExecutionActive: false,
    projectVisible: true,
    activityBarExpanded: true,
    rightPanel: null,
    settingsOpen: false,
    interactiveRightVisible: false,
    novaDir: '/book-a/.nova',
    books: [],
    bookSortMode: 'recent',
    tree: [],
    loading: false,
    selectedFile: null,
    fileDocument: null,
    fileContent: '',
    fileRevision: '',
    openTabs: [],
    activeTabKey: null,
    sidebarView: 'outline',
    editorSearchIntent: null,
    saveSignal: 0,
    editorAutoSaveEnabled: true,
    editorAutoSaveDelayMs: 1000,
    projectExplorerRefreshSignal: 0,
    versionRefreshSignal: 0,
    messages: [],
    sessions: [],
    activeSessionId: '',
    activityContent: '',
    hasEarlierMessages: false,
    isLoadingEarlierHistory: false,
    references: [],
    loreReferences: [],
    loreItems: [],
    styleScenes: [],
    textSelections: [],
    writingAgentConversation: { sessionId: '', messages: [], isStreaming: false },
    onOpenWritingSubAgentSession: vi.fn(),
    chatPlanMode: false,
    onSetMode: vi.fn(),
    onToggleActivityBarExpanded: vi.fn(),
    onToggleProjectVisible: vi.fn(),
    onSetRightPanel: vi.fn(),
    onToggleSettings: vi.fn(),
    onCloseSettings: vi.fn(),
    onToggleInteractiveRightPanel: vi.fn(),
    onSwitchBook: vi.fn(),
    onQuickSwitchBook: vi.fn(async () => true),
    onBeforeWorkspaceSwitch: vi.fn(async () => true),
    onBooksChange: vi.fn(),
    onAgentChatBookCreated: vi.fn(),
    onOpenCharacterCardImport: vi.fn(),
    onSetSidebarView: vi.fn(),
    onSelectSearchResult: vi.fn(),
    onSelectFile: vi.fn(),
    onSetChapterConfirmed: vi.fn(),
    onReferenceFile: vi.fn(),
    onCreateItem: vi.fn(),
    onDeleteItem: vi.fn(),
    onRenameItem: vi.fn(),
    onCopyItem: vi.fn(),
    onMoveItem: vi.fn(),
    onRefreshWorkspace: vi.fn(),
    onActivateTab: vi.fn(),
    onCloseTab: vi.fn(),
    onToggleTabPin: vi.fn(),
    onMoveTab: vi.fn(),
    onOpenLoreTab: vi.fn(async () => true),
    onSaveCurrentFile: vi.fn(),
    onEditorFlushHandlerChange: vi.fn(),
    onWorkspaceChanged: vi.fn(),
    onQuoteSelection: vi.fn(),
    onCreateChatSession: vi.fn(),
    onSwitchChatSession: vi.fn(),
    onRenameChatSession: vi.fn(),
    onDeleteChatSession: vi.fn(),
    onLoadEarlierHistory: vi.fn(),
    onRefreshChatHistory: vi.fn(),
    onSend: vi.fn(),
    onAnalyzeContext: vi.fn(async () => ({} as any)),
    onStop: vi.fn(),
    onReferenceRemove: vi.fn(),
    onLoreReferenceAdd: vi.fn(),
    onLoreReferenceRemove: vi.fn(),
    onStyleSceneAdd: vi.fn(),
    onStyleSceneRemove: vi.fn(),
    onTextSelectionRemove: vi.fn(),
    onChatPlanModeChange: vi.fn(),
    onChatPlanModeToggle: vi.fn(),
    onApproveProposedPlan: vi.fn(),
    onExitChatPlanMode: vi.fn(),
    ...overrides,
  }
}

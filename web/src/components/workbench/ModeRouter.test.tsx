import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState, type ComponentProps, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { ModeRouter } from './ModeRouter'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))
const useDocumentReviewMock = vi.hoisted(() => vi.fn())
const agentPanelLifecycle = vi.hoisted(() => ({ mounts: 0, unmounts: 0, renders: 0 }))
const loreLibraryFlushMock = vi.hoisted(() => vi.fn(async () => true))

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

vi.mock('@/features/agent-chat/AgentChatRoute', () => ({
  AgentChatRoute: () => <div data-testid="agent-chat-route">project-scoped agent route</div>,
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

vi.mock('@/components/Editor/MarkdownEditor', () => ({
  MarkdownEditor: ({ fileName, chapterSummary, onRevealChapter, documentReviewNavigationIntent }: {
    fileName: string | null
    chapterSummary?: { path: string }
    onRevealChapter?: (path: string) => void
    documentReviewNavigationIntent?: { commentID: string; nonce: number } | null
  }) => (
    <>
      <div data-testid="markdown-editor-navigation">
        {fileName || 'none'}|{documentReviewNavigationIntent?.commentID || 'none'}|{documentReviewNavigationIntent?.nonce || 0}
      </div>
      {chapterSummary ? (
        <button type="button" onClick={() => onRevealChapter?.(chapterSummary.path)}>reveal chapter in outline</button>
      ) : null}
    </>
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

vi.mock('@/features/changes/use-writing-change-review', () => ({
  useWritingChangeReview: () => ({
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
  }),
}))

vi.mock('@/features/document-review/use-document-review', () => ({
  useDocumentReview: useDocumentReviewMock,
}))

vi.mock('./WorkbenchShell', () => ({
  WorkbenchShell: ({ onQuickSwitchBook, main, rightPanelContent }: {
    onQuickSwitchBook: (path: string) => Promise<boolean>
    main: ReactNode
    rightPanelContent: ReactNode
  }) => (
    <>
      <button type="button" onClick={() => { void onQuickSwitchBook('/book-b') }}>
        quick switch
      </button>
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
    agentPanelLifecycle.mounts = 0
    agentPanelLifecycle.unmounts = 0
    agentPanelLifecycle.renders = 0
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

  it('continues a workspace switch and warns when preference flush remains pending', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    render(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />)

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
    render(<ModeRouter {...modeRouterProps({ onQuickSwitchBook })} />)

    await user.click(screen.getByRole('button', { name: 'quick switch' }))
    expect(onQuickSwitchBook).toHaveBeenCalledWith('/book-b')

    resolveFlush(false)
    await waitFor(() => expect(toastMock.warning).toHaveBeenCalled())
  })

  it('flushes the retained full Lore library before switching workspaces', async () => {
    const user = userEvent.setup()
    const onQuickSwitchBook = vi.fn(async () => true)
    loreLibraryFlushMock.mockResolvedValue(false)
    render(<ModeRouter {...modeRouterProps({ rightPanel: 'lore', onQuickSwitchBook })} />)
    await screen.findByTestId('full-lore-library')

    await user.click(screen.getByRole('button', { name: 'quick switch' }))

    await waitFor(() => expect(loreLibraryFlushMock).toHaveBeenCalledTimes(1))
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

    render(<Harness />)
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

    render(<Harness />)
    await user.click(screen.getByRole('button', { name: 'open document feedback' }))

    await waitFor(() => expect(openLoreTab).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByTestId('lore-workspace-navigation')).toHaveTextContent(
      `${comment.id}|1`,
    ))
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

    render(<ModeRouter {...modeRouterProps({
      projectVisible: false,
      selectedFile: chapter.path,
      currentChapter: chapter,
      openTabs: [{ kind: 'file', path: chapter.path }],
      activeTabKey: `file:${chapter.path}`,
      onToggleProjectVisible,
      onSetSidebarView,
    })} />)

    await user.click(screen.getByRole('button', { name: 'reveal chapter in outline' }))

    expect(onToggleProjectVisible).toHaveBeenCalledTimes(1)
    expect(onSetSidebarView).toHaveBeenCalledWith('outline')
  })

  it('keeps the foreground Writing Agent mounted while Workspace owns separate scoped conversations', async () => {
    const user = userEvent.setup()
    const baseProps = modeRouterProps({ rightPanel: 'ai' })
    const { rerender } = render(<ModeRouter {...baseProps} />)

    await user.click(screen.getByRole('button', { name: 'agent panel state 0' }))
    expect(screen.getByRole('button', { name: 'agent panel state 1' })).toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)

    rerender(<ModeRouter {...baseProps} mode="agentchat" />)
    await waitFor(() => expect(screen.getByTestId('agent-chat-route')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: 'agent panel state 1' })).not.toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)
    expect(agentPanelLifecycle.unmounts).toBe(0)

    rerender(<ModeRouter {...baseProps} mode="ide" />)
    expect(screen.getByRole('button', { name: 'agent panel state 1' })).toBeInTheDocument()
    expect(agentPanelLifecycle.mounts).toBe(1)
    expect(agentPanelLifecycle.unmounts).toBe(0)
  })
})

function modeRouterProps(
  overrides: Partial<ComponentProps<typeof ModeRouter>> = {},
): ComponentProps<typeof ModeRouter> {
  return {
    mode: 'ide',
    booksReturnMode: 'ide',
    currentBookName: 'Book A',
    workspace: '/book-a',
    appVersion: 'test',
    summary: null,
    chapterStats: {},
    isStreaming: false,
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
    fileContent: '',
    fileRevision: '',
    openTabs: [],
    activeTabKey: null,
    sidebarView: 'outline',
    editorSearchIntent: null,
    saveSignal: 0,
    editorAutoSaveEnabled: true,
    editorAutoSaveDelayMs: 1000,
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
    onActivateTab: vi.fn(),
    onCloseTab: vi.fn(),
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
    onSubmitPlanQuestion: vi.fn(),
    onApproveProposedPlan: vi.fn(),
    onExitChatPlanMode: vi.fn(),
    ...overrides,
  }
}

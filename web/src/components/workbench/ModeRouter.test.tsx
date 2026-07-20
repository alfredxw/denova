import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ComponentProps, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { ModeRouter } from './ModeRouter'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))
const useDocumentReviewMock = vi.hoisted(() => vi.fn())

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@/hooks/usePersistedUserSettings', () => ({
  usePersistedUserSettings: vi.fn(),
}))

vi.mock('@/components/Chat/AgentPanel', () => ({
  WRITING_COMPOSER_SETTING_DEFAULTS: {
    ide_story_teller_id: 'classic',
    ide_image_preset_id: 'game-cg',
    writing_skill_default: 'novel-lite',
  },
  AgentPanel: ({ reviewFeedback, onReviewFeedbackOpen }: {
    reviewFeedback?: Array<{ comments: Array<{ id: string }> }>
    onReviewFeedbackOpen?: (selection: unknown, comment: unknown) => void
  }) => {
    const selection = reviewFeedback?.[0]
    const comment = selection?.comments[0]
    return (
      <button type="button" disabled={!selection || !comment} onClick={() => onReviewFeedbackOpen?.(selection, comment)}>
        open document feedback
      </button>
    )
  },
}))

vi.mock('@/components/Editor/MarkdownEditor', () => ({
  MarkdownEditor: ({ fileName, documentReviewNavigationIntent }: {
    fileName: string | null
    documentReviewNavigationIntent?: { commentID: string; nonce: number } | null
  }) => (
    <div data-testid="markdown-editor-navigation">
      {fileName || 'none'}|{documentReviewNavigationIntent?.commentID || 'none'}|{documentReviewNavigationIntent?.nonce || 0}
    </div>
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

  it('opens the referenced chapter before revealing its document review comment', async () => {
    const user = userEvent.setup()
    const comment = {
      id: 'document-comment',
      thread_id: 'document-thread',
      path: 'chapters/ch02.md',
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

    await waitFor(() => expect(handleSelectFile).toHaveBeenCalledWith(comment.path))
    await waitFor(() => expect(screen.getByTestId('markdown-editor-navigation')).toHaveTextContent(
      `${comment.path}|${comment.id}|1`,
    ))

    await user.click(screen.getByRole('button', { name: 'open document feedback' }))
    await waitFor(() => expect(screen.getByTestId('markdown-editor-navigation')).toHaveTextContent(
      `${comment.path}|${comment.id}|2`,
    ))
    expect(handleSelectFile).toHaveBeenCalledTimes(1)
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
    onSaveCurrentFile: vi.fn(),
    onEditorFlushHandlerChange: vi.fn(),
    onWorkspaceChanged: vi.fn(),
    onQuoteSelection: vi.fn(),
    onCreateChatSession: vi.fn(),
    onSwitchChatSession: vi.fn(),
    onRenameChatSession: vi.fn(),
    onDeleteChatSession: vi.fn(),
    onLoadEarlierHistory: vi.fn(),
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

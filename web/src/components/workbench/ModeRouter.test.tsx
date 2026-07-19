import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { ModeRouter } from './ModeRouter'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@/hooks/usePersistedUserSettings', () => ({
  usePersistedUserSettings: vi.fn(),
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
  useDocumentReview: () => ({
    feedback: null,
    thread: { comments: [] },
    addComment: vi.fn(),
    editComment: vi.fn(),
    removeComment: vi.fn(),
    removeFeedback: vi.fn(),
    submitFeedback: vi.fn(),
    restoreFeedback: vi.fn(),
  }),
}))

vi.mock('./WorkbenchShell', () => ({
  WorkbenchShell: ({ onQuickSwitchBook }: { onQuickSwitchBook: (path: string) => Promise<boolean> }) => (
    <button type="button" onClick={() => { void onQuickSwitchBook('/book-b') }}>
      quick switch
    </button>
  ),
}))

describe('ModeRouter autosave navigation policy', () => {
  beforeEach(() => {
    toastMock.warning.mockReset()
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

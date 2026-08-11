import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import type { FileNode } from '@/hooks/useWorkspace'
import { ModeRouter } from './ModeRouter'

const toastMock = vi.hoisted(() => ({ warning: vi.fn() }))
const useDocumentReviewMock = vi.hoisted(() => vi.fn())
const responsiveState = vi.hoisted(() => ({ mobile: false }))

vi.mock('sonner', () => ({ toast: toastMock }))

vi.mock('@/hooks/usePersistedUserSettings', () => ({
  usePersistedUserSettings: vi.fn(),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.mobile,
}))

vi.mock('@/lib/api', () => ({
  getTasks: vi.fn(async () => ({ tasks: [], action_required_count: 0 })),
}))

vi.mock('@/components/Chat/AgentPanel', () => ({
  WRITING_COMPOSER_SETTING_DEFAULTS: {
    ide_story_teller_id: 'classic',
    ide_image_preset_id: 'game-cg',
    writing_skill_default: 'novel-lite',
  },
  AgentPanel: () => <div data-testid="agent-panel" />,
}))

vi.mock('@/components/Editor/MarkdownEditor', () => ({
  MarkdownEditor: () => <div data-testid="markdown-editor-navigation" />,
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

describe('ModeRouter mobile project restore', () => {
  beforeEach(() => {
    responsiveState.mobile = true
    setConfiguredLocale('zh-CN')
    toastMock.warning.mockReset()
    useDocumentReviewMock.mockReset()
    useDocumentReviewMock.mockReturnValue({
      feedback: null,
      thread: { comments: [] },
      addComment: vi.fn(),
      editComment: vi.fn(),
      removeComment: vi.fn(),
      removeFeedback: vi.fn(),
      submitReviewFeedback: vi.fn(),
      restoreReviewFeedback: vi.fn(),
    })
    vi.mocked(usePersistedUserSettings).mockReturnValue({
      values: {
        ide_story_teller_id: 'classic',
        ide_image_preset_id: 'game-cg',
        writing_skill_default: 'novel-lite',
      },
      loading: false,
      isSaving: vi.fn(() => false),
      persist: vi.fn(async () => true),
      reload: vi.fn(async () => null),
      flushPending: vi.fn(async () => true),
    })
    window.localStorage.clear()
  })

  it('restores project expanded folders and scroll after switching destinations', async () => {
    const user = userEvent.setup()
    const tree: FileNode[] = [
      {
        name: 'manuscripts',
        type: 'dir',
        children: [
          { name: 'chapters', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] },
        ],
      },
      { name: 'ideas.md', type: 'file' },
    ]

    render(
      <ModeRouter
        {...modeRouterProps({
          mode: 'ide',
          sidebarView: 'files',
          tree,
        })}
      />,
    )

    const navigation = screen.getByRole('navigation', { name: '移动端工作台导航' })
    await user.click(within(navigation).getByRole('button', { name: '项目' }))

    const folder = await screen.findByRole('button', { name: 'manuscripts' })
    expect(screen.queryByText('ch01.md')).not.toBeInTheDocument()
    fireEvent.click(folder)
    expect(screen.getByText('ch01.md')).toBeInTheDocument()

    const scrollArea = screen.getByTestId('project-sidebar-scroll')
    fireEvent.scroll(scrollArea, { target: { scrollTop: 128 } })

    await user.click(within(navigation).getByRole('button', { name: '正文' }))
    expect(screen.queryByRole('button', { name: 'manuscripts' })).not.toBeInTheDocument()

    await user.click(within(navigation).getByRole('button', { name: '项目' }))

    expect(await screen.findByRole('button', { name: 'manuscripts' })).toBeInTheDocument()
    expect(screen.getByText('ch01.md')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByTestId('project-sidebar-scroll').scrollTop).toBe(128)
    })
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
    onAnalyzeContext: vi.fn(async () => ({} as never)),
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

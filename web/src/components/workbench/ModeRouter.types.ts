import type { EditorFlushHandler } from '@/components/Editor/MarkdownEditor'
import type { WorkbenchNotice } from '@/features/notices/use-workbench-notice'
import type { FileNode } from '@/hooks/useWorkspace'
import type { ChatSendOptions } from '@/hooks/useAgentChat'
import type {
  ActiveChatTask,
  AgentRuntimeQueuedCommand,
  BookRecord,
  BookSortMode,
  ChapterSummary,
  ContextAnalysis,
  LoreItem,
  SessionSummary,
  TextSelection,
  WorkspaceSearchResult,
  WorkspaceSummary,
} from '@/lib/api'
import type { AgentPartRef } from '@/lib/agent-message-view'
import type { AgentUIMessage } from '@/lib/agent-ui'
import type { RightPanel, WorkspaceMode } from '@/stores/workspace-store'
import type { Tab } from './TabController'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'

/** Public composition contract for the workbench route host. */
export interface ModeRouterProps {
  mode: WorkspaceMode
  booksReturnMode: 'ide' | 'interactive'
  currentBookName: string
  workspace: string
  projectId: string
  appVersion: string
  summary: WorkspaceSummary | null
  currentChapter?: ChapterSummary
  chapterStats: Record<string, ChapterSummary>
  isStreaming: boolean
  sessionTransitionPending?: boolean
  isExecutionActive: boolean
  runtimeProjection?: ActiveChatTask | null
  abortPending?: boolean
  commandSubmitting?: boolean
  queueActionPendingCommandID?: string
  projectVisible: boolean
  activityBarExpanded: boolean
  rightPanel: RightPanel
  settingsOpen: boolean
  developerMode?: boolean
  interactiveRightVisible: boolean
  novaDir: string
  books: BookRecord[]
  bookSortMode: BookSortMode
  tree: FileNode[]
  loading: boolean
  selectedFile: string | null
  fileContent: string
  fileRevision: string
  openTabs: Tab[]
  activeTabKey: string | null
  sidebarView: 'outline' | 'files' | 'search'
  editorSearchIntent: { path: string; query: string; line: number; nonce: number } | null
  saveSignal: number
  editorAutoSaveEnabled: boolean
  editorAutoSaveDelayMs: number
  projectExplorerRefreshSignal: number
  versionRefreshSignal: number
  messages: AgentUIMessage[]
  sessions: SessionSummary[]
  activeSessionId: string
  activityContent: string
  references: string[]
  loreReferences: string[]
  loreItems: LoreItem[]
  styleScenes: string[]
  textSelections: TextSelection[]
  chatPlanMode: boolean
  hasEarlierMessages: boolean
  isLoadingEarlierHistory: boolean
  notice?: WorkbenchNotice | null
  onSetMode: (mode: WorkspaceMode) => void
  onToggleActivityBarExpanded: () => void
  onToggleProjectVisible: () => void
  onSetRightPanel: (panel: RightPanel) => void
  onToggleSettings: () => void
  onCloseSettings: () => void
  onToggleInteractiveRightPanel: () => void
  onSwitchBook: (path: string) => void
  onQuickSwitchBook: (path: string) => Promise<boolean>
  onBeforeWorkspaceSwitch: EditorFlushHandler
  onBooksChange: () => void | Promise<void>
  /** Synchronizes a Book created inside Agent Chat without leaving the shared Agent Chat route. */
  onAgentChatBookCreated: (workspace: string) => void | Promise<void>
  onOpenCharacterCardImport: () => void
  onSetSidebarView: (view: 'outline' | 'files' | 'search') => void
  onSelectSearchResult: (result: WorkspaceSearchResult, query: string) => void | Promise<void>
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onSetChapterConfirmed: (path: string, confirmed: boolean) => void | Promise<void>
  onReferenceFile: (path: string) => void
  onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
  onDeleteItem: (path: string) => Promise<void>
  onRenameItem: (path: string, newName: string) => Promise<void>
  onCopyItem: (from: string, to: string) => Promise<void>
  onMoveItem: (from: string, to: string) => Promise<void>
  onRefreshWorkspace: () => void | Promise<void>
  onActivateTab: (tab: Tab) => void
  onCloseTab: (tab: Tab) => void
  onToggleTabPin: (tab: Tab) => void
  onMoveTab: (sourceKey: string, targetKey: string) => void
  onOpenLoreTab: () => Promise<boolean>
  onSaveCurrentFile: (path: string, content: string, baseRevision: string) => Promise<{ revision?: string }>
  onEditorFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  onWorkspaceChanged: (paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
  onQuoteSelection: (selection: TextSelection) => void
  onCreateChatSession: (title?: string) => void | Promise<void>
  onSwitchChatSession: (id: string) => void | Promise<void>
  onRenameChatSession: (id: string, title: string) => void | Promise<void>
  onDeleteChatSession: (id: string) => void | Promise<void>
  onLoadEarlierHistory: () => void | Promise<void>
  onRefreshChatHistory: (sessionId?: string) => void | Promise<void>
  onSend: (message: string, options?: ChatSendOptions) => boolean | Promise<boolean>
  onAnalyzeContext: (message: string, options?: { writingSkill?: string; ideContext?: { currentFile?: string; openFiles?: string[] }; imagePresetId?: string; tellerId?: string }) => Promise<ContextAnalysis>
  onStop: () => void
  onSteerQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onDeleteQueuedCommand?: (item: AgentRuntimeQueuedCommand) => boolean | Promise<boolean>
  onEditQueuedCommand?: (item: AgentRuntimeQueuedCommand) => string | null | Promise<string | null>
  onReferenceRemove: (path: string) => void
  onLoreReferenceAdd: (id: string) => void
  onLoreReferenceRemove: (id: string) => void
  onStyleSceneAdd: (scene: string) => void
  onStyleSceneRemove: (scene: string) => void
  onTextSelectionRemove: (index: number) => void
  onChatPlanModeChange: (value: boolean) => void
  onChatPlanModeToggle: () => void
  onApproveProposedPlan: (ref: AgentPartRef) => void
  onExitChatPlanMode: () => void
  onDismissNotice?: () => void
}

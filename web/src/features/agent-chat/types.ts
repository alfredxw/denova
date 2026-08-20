import type { DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import type { DocumentReviewTarget } from '@/features/document-review/types'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { AgentChatProjectType } from './api'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'

/**
 * Tab model for the AgentChat workspace.
 *
 * The centre area of AgentChat is a multi-tab workbench: a tab is either a built-in
 * agent conversation, local terminal, project file workspace, review, or one of the
 * existing project pages hosted inline so the whole project can be managed from a
 * single screen.
 */

/** Project pages adapted for hosting inside an AgentChat workbench tab. */
export type AgentChatPageId = 'reader' | 'lore'

export const AGENT_CHAT_PAGE_IDS: readonly AgentChatPageId[] = ['reader', 'lore']

const GENERAL_PROJECT_PAGE_IDS: readonly AgentChatPageId[] = []

/** Declares page availability independently from tab-menu presentation. */
export function agentChatPageIdsForProjectType(projectType: AgentChatProjectType): readonly AgentChatPageId[] {
  return projectType === 'book' ? AGENT_CHAT_PAGE_IDS : GENERAL_PROJECT_PAGE_IDS
}

/** A persisted review target plus a nonce for revealing it inside an AgentChat project page. */
export interface AgentChatDocumentReviewNavigation extends DocumentReviewNavigationIntent {
  projectId: string
  target: DocumentReviewTarget
}

/** Capabilities supplied by the workbench to one mounted project-page tab. */
export interface AgentChatPageRenderContext {
  projectType: AgentChatProjectType
  navigationIntent: AgentChatDocumentReviewNavigation | null
  toolNavigationIntent: ToolNavigationIntent | null
  documentReview: DocumentReviewController
  refreshSignal: number
  onFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  openPage: (pageId: AgentChatPageId) => void
  onWorkspaceChanged: (paths: string[], metadata: WorkspaceChangeMetadata) => void | Promise<void>
}

/** Menu-safe terminal command metadata; executable command lines remain backend-only. */
export interface TerminalCommandProfile {
  id: string
  name: string
}

/** `shell` is fixed; every other current ID belongs to a configured command. */
export type TerminalProfileId = string

/**
 * Side of the split that hosts a tab. Two groups is the whole model: it is enough to talk to
 * the agent on one side while watching what it changes on the other, and it keeps the layout
 * describable by a single field per tab instead of a nested pane tree.
 */
export type AgentChatGroupId = 'primary' | 'secondary'

export const AGENT_CHAT_GROUP_IDS: readonly AgentChatGroupId[] = ['primary', 'secondary']

/** Everything a tab carries regardless of what it hosts. */
interface AgentChatTabCommon {
  id: string
  /** Stable Project identity. Tabs survive a display rename or directory relink. */
  projectId: string
  /** Current content directory, used only as the working root and page context. */
  workspace: string
  /** Title typed by the user; overrides the one derived from the session, profile or page. */
  customTitle?: string
  /** Pinned tabs are held at the front of their strip and are spared by bulk closes. */
  pinned?: boolean
  /** Group hosting the tab. */
  group: AgentChatGroupId
}

/**
 * Built-in agent conversation tab. A tab carries its Project identity because AgentChat
 * spans the whole library without changing the foreground Writing Book.
 */
export interface AgentChatAgentTab extends AgentChatTabCommon {
  kind: 'agent'
  sessionId: string
  /** Local-only blank conversation. The backend session is created by its first submitted turn. */
  draft?: boolean
  /** First-message title shown until the refreshed server summary carries the durable title. */
  pendingTitle?: string
}

/**
 * Terminal tab. `terminalSessionId` is written once the first attach succeeds so a page
 * reload can re-attach to the pty that is still running.
 */
export interface AgentChatTerminalTab extends AgentChatTabCommon {
  kind: 'terminal'
  profileId: TerminalProfileId
  /** Display snapshot used while settings load or after the command is renamed/deleted. */
  profileName?: string
  /** Latest program-owned OSC window title, or the resolved launch title before one is emitted. */
  title: string
  terminalSessionId?: string
}

/** Project page tab. Only one instance of each page may exist at a time. */
export interface AgentChatPageTab extends AgentChatTabCommon {
  kind: 'page'
  pageId: AgentChatPageId
}

/** General project file workspace. One instance is reused per project. */
export interface AgentChatFilesTab extends AgentChatTabCommon {
  kind: 'files'
  /** Last source path remains with the tab across project and app navigation. */
  selectedPath?: string
}

/**
 * Diff review for one change thread. Opening it as a tab is what lets the conversation that
 * produced the changes stay visible on the other side of the split while they are reviewed.
 */
export interface AgentChatReviewTab extends AgentChatTabCommon {
  kind: 'review'
  threadID: string
  /** Change group to scroll to, when the review was opened from a specific one. */
  groupID?: string
}

/** Navigation and mutation capabilities supplied to a mounted review tab. */
export interface AgentChatReviewRenderContext {
  openFile: (path: string) => void
  onWorkspaceChanged: (paths: string[]) => void | Promise<void>
}

export type AgentChatTab = AgentChatAgentTab | AgentChatTerminalTab | AgentChatFilesTab | AgentChatPageTab | AgentChatReviewTab

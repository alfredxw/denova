import { render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import type { ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import type { AgentChatProject } from './api'
import type { TerminalSessionInfo } from './terminal/api'

vi.mock('./api', () => ({
  addAgentChatProject: vi.fn(),
  archiveAgentChatProject: vi.fn(),
  createAgentChatSession: vi.fn(),
  deleteAgentChatSession: vi.fn(),
  getAgentChatHistory: vi.fn(),
  getAgentChatProjects: vi.fn(),
  relinkAgentChatProject: vi.fn(),
  renameAgentChatSession: vi.fn(),
  selectAgentChatProjectDirectory: vi.fn(),
}))

const documentReviewState = vi.hoisted(() => ({
  feedback: null as ReviewFeedbackSelection | null,
}))

export function setDocumentReviewFeedback(feedback: ReviewFeedbackSelection | null) {
  documentReviewState.feedback = feedback
}

vi.mock('@/features/document-review/use-document-review', () => ({
  useDocumentReview: () => ({
    visibleComments: [],
    feedback: documentReviewState.feedback,
    addComment: vi.fn(),
    editComment: vi.fn(),
    removeComment: vi.fn(),
    removeFeedback: vi.fn(),
    submitFeedback: vi.fn(),
    restoreFeedback: vi.fn(),
  }),
}))

vi.mock('./AgentChatConversationTab', () => ({
  AgentChatConversationTab: ({
    workspace,
    sessionId,
    active,
    draft,
    reviewFeedback,
    onReviewFeedbackOpen,
    onWorkspaceChanged,
  }: {
    workspace: string
    sessionId: string
    active: boolean
    draft?: boolean
    reviewFeedback?: Array<{
      comments: Array<{
        id: string
        body: string
        target?: { kind: 'workspace_file' | 'lore_item'; id: string }
      }>
    }>
    onReviewFeedbackOpen?: (selection: unknown, comment: unknown) => void
    onWorkspaceChanged?: (workspace: string, paths: string[], metadata: WorkspaceChangeMetadata) => void
  }) => {
    const selection = reviewFeedback?.[0]
    const comment = selection?.comments[0]
    return (
      <div data-testid={draft ? 'draft-conversation' : `conversation:${workspace}:${sessionId}`}>
        {active ? 'active' : 'hidden'}
        {selection && comment ? (
          <button type="button" onClick={() => onReviewFeedbackOpen?.(selection, comment)}>
            open pending document feedback
          </button>
        ) : null}
        <button type="button" onClick={() => onWorkspaceChanged?.(workspace, ['src/main.ts'], { impact: 'content', origin: 'external' })}>
          simulate external content
        </button>
        <button type="button" onClick={() => onWorkspaceChanged?.(workspace, ['src/new.ts'], { impact: 'structure', origin: 'external' })}>
          simulate external structure
        </button>
      </div>
    )
  },
}))

vi.mock('./terminal/api', () => ({
  closeTerminalSession: vi.fn(),
  getTerminalRuntimeStatus: vi.fn(),
}))

vi.mock('./terminal/TerminalTabView', () => ({
  TerminalTabView: () => <div>terminal</div>,
  terminalTabLabel: () => 'Terminal',
}))

vi.mock('@/features/files/FilesTab', () => ({
  FilesTab: ({ selectedPath, workspace, editorRefreshSignal, treeRefreshSignal, onWorkspaceChanged }: {
    selectedPath: string | null
    workspace: string
    editorRefreshSignal: number
    treeRefreshSignal: number
    onWorkspaceChanged?: (workspace: string, paths: string[], metadata: WorkspaceChangeMetadata) => void
  }) => (
    <div data-testid="project-files-tab">
      {selectedPath || 'no-selection'}
      <output data-testid="project-files-refresh">editor:{editorRefreshSignal}|tree:{treeRefreshSignal}</output>
      <button type="button" onClick={() => onWorkspaceChanged?.(workspace, ['src/main.ts'], { impact: 'content', origin: 'files-tab' })}>
        simulate local save
      </button>
      <button type="button" onClick={() => onWorkspaceChanged?.(workspace, ['src/new.ts'], { impact: 'structure', origin: 'files-tab' })}>
        simulate local file operation
      </button>
    </div>
  ),
}))

vi.mock('@/components/layout/adaptive-surface', () => ({
  AdaptiveSurface: ({
    left,
    right,
    rightResize,
    desktopOverlay,
    children,
  }: {
    left: { enabled?: boolean; content: ReactNode; desktopVisible?: boolean; desktopCollapsedContent?: ReactNode }
    right?: { content: ReactNode; desktopVisible?: boolean }
    rightResize?: { label: string }
    desktopOverlay?: ReactNode
    children: ReactNode | ((controls: {
      isMobile: boolean
      openPaneId: string | null
      openLeft: () => void
      openRight: () => void
      closePane: () => void
    }) => ReactNode)
  }) => (
    <div>
      {left.enabled === false ? null : left.desktopVisible === false ? left.desktopCollapsedContent : left.content}
      <main>{typeof children === 'function' ? children({
        isMobile: false,
        openPaneId: null,
        openLeft: vi.fn(),
        openRight: vi.fn(),
        closePane: vi.fn(),
      }) : children}</main>
      {right ? (
        <aside hidden={right.desktopVisible === false}>
          {right.desktopVisible === false || !rightResize ? null : (
            <div
              role="separator"
              aria-label={rightResize.label}
              className="nova-resize-handle nova-resize-divider nova-resize-divider-vertical w-2"
            />
          )}
          {right.content}
        </aside>
      ) : null}
      {desktopOverlay}
    </div>
  ),
}))

export function renderView(ui: ReactNode) {
  return render(<TooltipProvider delayDuration={0}>{ui}</TooltipProvider>)
}

export function project(path: string, name: string, sessionId: string, title: string): AgentChatProject {
  return {
    id: `project-${path.split('/').pop()}`,
    type: 'book',
    status: 'available',
    path,
    name,
    current: false,
    total: 1,
    sessions: [
      {
        id: sessionId,
        title,
        active: false,
        running: false,
        message_count: 0,
        created_at: '2026-07-26T00:00:00Z',
        updated_at: '2026-07-26T00:00:00Z',
      },
    ],
  }
}

export function agentTabForProject(id: string, projectId: string, workspace: string, sessionId: string) {
  return { kind: 'agent' as const, id, projectId, workspace, sessionId }
}

export function terminalSession(id: string, ownerTabId: string | undefined, attached = 0): TerminalSessionInfo {
  return {
    id,
    owner_tab_id: ownerTabId,
    profile_id: 'shell',
    title: 'shell',
    command: '/bin/sh',
    args: [],
    cwd: '/books/a',
    workspace: '/books/a',
    cols: 80,
    rows: 24,
    created_at: '2026-07-26T00:00:00Z',
    attached,
    exited: false,
    exit_code: 0,
    token: `token-${id}`,
  }
}

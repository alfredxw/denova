import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatRoute } from './AgentChatRoute'

const view = vi.hoisted(() => ({
  projectId: 'book-b',
  workspace: '/books/b',
  pageId: 'lore' as 'lore' | 'reader',
}))

vi.mock('./AgentChatView', () => ({
  AgentChatView: (props: {
    renderPage: (projectId: string, workspace: string, pageId: 'lore' | 'reader', context: unknown) => React.ReactNode
  }) => props.renderPage(view.projectId, view.workspace, view.pageId, {
    navigationIntent: null,
    documentReview: { comments: [{ id: 'comment-1' }] },
    refreshSignal: 7,
    onFlushHandlerChange: vi.fn(),
    openPage: vi.fn(),
    onWorkspaceChanged: vi.fn(),
  }),
}))

vi.mock('@/features/lore/LoreWorkspaceTab', () => ({
  LoreWorkspaceTab: ({ projectId, workspace, documentReview, refreshSignal }: {
    projectId: string
    workspace: string
    documentReview: { comments: unknown[] }
    refreshSignal: number
  }) => (
    <div data-testid="shared-lore-workspace">
      {projectId}|{workspace}|{documentReview.comments.length}|{refreshSignal}
    </div>
  ),
}))

vi.mock('@/features/writing/ProjectWritingSurface', () => ({
  ProjectWritingSurface: ({ projectId, workspace }: { projectId: string; workspace: string }) => (
    <div data-testid="shared-project-writing">{projectId}|{workspace}</div>
  ),
}))

vi.mock('@/features/interactive/components/SettingPanel', () => ({
  SettingPanel: ({ mode }: { mode: string }) => <div data-testid={`setting-panel:${mode}`}>{mode}</div>,
}))

vi.mock('@/features/skills/SkillsView', () => ({ SkillsView: () => null }))
vi.mock('@/features/agents/AgentsView', () => ({ AgentsView: () => null }))
vi.mock('@/features/automations/AutomationsView', () => ({ AutomationsView: () => null }))
vi.mock('@/features/changes/review/ChangeReviewWorkspace', () => ({ ChangeReviewWorkspace: () => null }))

function routeProps(): React.ComponentProps<typeof AgentChatRoute> {
  return {
    projectId: 'book-a',
    composerSettings: {} as never,
    tellers: [],
    imagePresets: [],
    onTellersChange: vi.fn(),
    onImagePresetsChange: vi.fn(),
  }
}

describe('AgentChatRoute resource pages', () => {
  beforeEach(() => {
    view.projectId = 'book-b'
    view.workspace = '/books/b'
    view.pageId = 'lore'
  })

  it('opens a background Book lore tab through its stable Project ID', async () => {
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-lore-workspace')).toHaveTextContent('book-b|/books/b|1|7')
    expect(screen.queryByTestId('setting-panel:lore')).not.toBeInTheDocument()
  })

  it('uses the shared Project Writing surface without activating the background Book', async () => {
    view.pageId = 'reader'
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-project-writing')).toHaveTextContent('book-b|/books/b')
  })
})

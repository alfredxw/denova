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

vi.mock('@/features/writing/ProjectWritingSurface', () => ({
  ProjectWritingSurface: ({ projectId }: { projectId: string }) => (
    <div data-testid="shared-project-writing">{projectId}</div>
  ),
}))

vi.mock('@/features/interactive/components/SettingPanel', () => ({
  SettingPanel: ({ projectId, mode, documentReview, refreshSignal }: {
    projectId: string
    mode: string
    documentReview?: { comments: unknown[] }
    refreshSignal?: number
  }) => <div data-testid={`setting-panel:${mode}`}>{projectId}|{documentReview?.comments.length || 0}|{refreshSignal}</div>,
}))

vi.mock('@/features/skills/SkillsView', () => ({ SkillsView: () => null }))
vi.mock('@/features/agents/AgentsView', () => ({ AgentsView: () => null }))
vi.mock('@/features/automations/AutomationsView', () => ({ AutomationsView: () => null }))
vi.mock('@/components/Versions/VersionPanel', () => ({ VersionPanel: () => null }))
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

  it('opens the shared full library through the background Book Project ID', async () => {
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('setting-panel:lore')).toHaveTextContent('book-b|1|7')
  })

  it('uses the shared Project Writing surface without activating the background Book', async () => {
    view.pageId = 'reader'
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-project-writing')).toHaveTextContent('book-b')
  })
})

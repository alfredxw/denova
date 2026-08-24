import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatRoute } from './AgentChatRoute'

const view = vi.hoisted(() => ({
  projectId: 'book-b',
  workspace: '/books/b',
  pageId: 'lore' as 'lore' | 'reader',
}))
const readingTypography = {
  fontFamily: 'source-han-serif',
  fontSize: 18,
  loading: false,
  status: 'saved' as const,
  onFontFamilyChange: vi.fn(),
  onFontSizeChange: vi.fn(),
  onRetry: vi.fn(),
}

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
  ProjectWritingSurface: ({ projectId, readingTypography: typography }: {
    projectId: string
    readingTypography: { fontFamily: string; fontSize: number }
  }) => (
    <div data-testid="shared-project-writing">{projectId}|{typography.fontFamily}|{typography.fontSize}</div>
  ),
}))

vi.mock('@/features/lore/LoreWorkspaceTab', () => ({
  LoreWorkspaceTab: ({ projectId, documentReview, refreshSignal }: {
    projectId: string
    documentReview: { comments: unknown[] }
    refreshSignal?: number
  }) => <div data-testid="shared-lore-workspace">{projectId}|{documentReview.comments.length}|{refreshSignal}</div>,
}))

vi.mock('@/features/changes/review/ChangeReviewWorkspace', () => ({ ChangeReviewWorkspace: () => null }))

function routeProps(): React.ComponentProps<typeof AgentChatRoute> {
  return {
    projectId: 'book-a',
    novaDir: '/books',
    composerSettings: {} as never,
    tellers: [],
    imagePresets: [],
    readingTypography,
    onBeforeCreateBook: vi.fn(async () => true),
    onBookCreated: vi.fn(),
    onBooksChange: vi.fn(),
  }
}

describe('AgentChatRoute resource pages', () => {
  beforeEach(() => {
    view.projectId = 'book-b'
    view.workspace = '/books/b'
    view.pageId = 'lore'
  })

  it('opens the same focused Lore workspace as Writing through the background Book Project ID', async () => {
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-lore-workspace')).toHaveTextContent('book-b|1|7')
  })

  it('uses the shared Project Writing surface without activating the background Book', async () => {
    view.pageId = 'reader'
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-project-writing')).toHaveTextContent('book-b|source-han-serif|18')
  })
})

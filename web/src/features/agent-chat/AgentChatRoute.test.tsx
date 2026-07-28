import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatRoute } from './AgentChatRoute'

const view = vi.hoisted(() => ({ workspace: '/books/a', pageId: 'lore' as const }))

vi.mock('./AgentChatView', () => ({
  AgentChatView: (props: {
    renderPage: (workspace: string, pageId: string, context: unknown) => React.ReactNode
    onActivateWorkspace?: (workspace: string) => Promise<boolean>
  }) => props.renderPage(view.workspace, view.pageId, {
    navigationIntent: null,
    onFlushHandlerChange: vi.fn(),
    openPage: vi.fn(),
    activateWorkspace: () => props.onActivateWorkspace?.(view.workspace) || Promise.resolve(false),
  }),
}))

vi.mock('@/features/lore/LoreWorkspaceTab', () => ({
  LoreWorkspaceTab: ({ workspace, documentReview, onOpenLibrary, onReferenceItem }: {
    workspace: string
    documentReview: { comments: unknown[] }
    onOpenLibrary?: () => void
    onReferenceItem?: () => void
  }) => (
    <div data-testid="shared-lore-workspace">
      {workspace}|{documentReview.comments.length}|{String(Boolean(onOpenLibrary))}|{String(Boolean(onReferenceItem))}
    </div>
  ),
}))

vi.mock('./AgentChatReader', () => ({
  AgentChatReader: () => <div data-testid="shared-agent-chat-reader">reader</div>,
}))

vi.mock('@/features/interactive/components/SettingPanel', () => ({
  SettingPanel: ({ mode }: { mode: string }) => <div data-testid={`setting-panel:${mode}`}>{mode}</div>,
}))

vi.mock('@/features/skills/SkillsView', () => ({ SkillsView: () => null }))
vi.mock('@/features/agents/AgentsView', () => ({ AgentsView: () => null }))
vi.mock('@/features/automations/AutomationsView', () => ({ AutomationsView: () => null }))
vi.mock('@/features/changes/review/ChangeReviewWorkspace', () => ({ ChangeReviewWorkspace: () => null }))

function routeProps(overrides: Partial<React.ComponentProps<typeof AgentChatRoute>> = {}): React.ComponentProps<typeof AgentChatRoute> {
  return {
    workspace: '/books/a',
    composerSettings: {} as never,
    tree: [],
    summary: null,
    selectedFile: null,
    tellers: [],
    imagePresets: [],
    documentReview: { comments: [{ id: 'comment-1' }] } as never,
    onTellersChange: vi.fn(),
    onImagePresetsChange: vi.fn(),
    onSetChapterConfirmed: vi.fn(),
    onSaveFile: vi.fn(async () => ({ revision: 'saved' })),
    onActivateWorkspace: vi.fn(async () => true),
    ...overrides,
  }
}

describe('AgentChatRoute resource pages', () => {
  beforeEach(() => {
    view.workspace = '/books/a'
    view.pageId = 'lore'
  })

  it('hosts the focused Lore workspace instead of embedding the full Lore library', async () => {
    render(<AgentChatRoute {...routeProps()} />)

    expect(await screen.findByTestId('shared-lore-workspace')).toHaveTextContent('/books/a|1|false|false')
    expect(screen.queryByTestId('setting-panel:lore')).not.toBeInTheDocument()
  })

  it('requires an explicit book switch before editing a background project', async () => {
    const user = userEvent.setup()
    const onActivateWorkspace = vi.fn(async () => true)
    view.workspace = '/books/b'
    render(<AgentChatRoute {...routeProps({ onActivateWorkspace })} />)

    await user.click(await screen.findByRole('button', { name: '切换到此书' }))
    await waitFor(() => expect(onActivateWorkspace).toHaveBeenCalledWith('/books/b'))
    expect(screen.queryByTestId('shared-lore-workspace')).not.toBeInTheDocument()
  })
})

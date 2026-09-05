import { act, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { OnboardingGuide } from './OnboardingGuide'

vi.mock('@/features/settings/api', () => ({ fetchSettings: vi.fn().mockResolvedValue({ effective: {} }) }))
vi.mock('./model-status', () => ({ hasUsableLanguageModel: () => true }))

beforeEach(() => window.localStorage.clear())

it('waits for startup workspace data before deciding whether to open the guide', async () => {
  const props = {
    mode: 'ide' as const,
    rightPanel: 'ai' as const,
    settingsOpen: false,
    workspaceReady: false,
    workspace: '',
    booksCount: 0,
    currentBookName: '',
    messages: [],
    isStreaming: false,
    onNavigate: vi.fn(),
  }
  const view = render(<OnboardingGuide {...props} />)
  // Let settings finish first, reproducing production startup ordering.
  await act(async () => {})
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  view.rerender(<OnboardingGuide {...props} workspaceReady workspace="/book" booksCount={1} />)
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  view.rerender(<OnboardingGuide {...props} workspaceReady />)
  expect(screen.getByRole('dialog')).toBeVisible()
})

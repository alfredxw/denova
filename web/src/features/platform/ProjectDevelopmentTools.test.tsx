import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProjectDevelopmentTools } from './ProjectDevelopmentTools'
import { management, type RuntimeSnapshot } from './api'

vi.mock('./api', () => ({ management: vi.fn().mockResolvedValue([]), platformError: () => 'failed' }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en-US' } }) }))
vi.mock('./development-context', () => ({
  useProjectDevelopment: () => ({ sources: [], source: { developmentId: 'source', projectId: 'project', relativePath: '.', kind: 'game', manifest: { id: 'test.game' } } }),
  useDevelopmentContext: { getState: () => ({ recordFeedback: vi.fn() }) },
}))
vi.mock('./InstallDialog', () => ({ InstallDialog: ({ onPlay }: { onPlay: (runtime: RuntimeSnapshot) => void }) => <button onClick={() => onPlay({ id: 'preview' } as RuntimeSnapshot)}>Open preview</button> }))
vi.mock('./BuildTerminal', () => ({ BuildTerminal: () => null }))
vi.mock('./GamePlayer', () => ({ GamePlayer: () => <p>Game preview</p> }))
vi.mock('./SourceManifestEditor', () => ({ SourceManifestEditor: () => null }))
vi.mock('./extension-navigation', () => ({ openInstalledExtension: vi.fn() }))

afterEach(() => { cleanup(); vi.clearAllMocks() })

describe('workbench source actions', () => {
  it('does not check a package until open editors have saved', async () => {
    const beforeAction = vi.fn().mockResolvedValue(false)
    render(<QueryClientProvider client={new QueryClient()}><ProjectDevelopmentTools projectId="project" visible refreshSignal={0} beforeAction={beforeAction} onOpenFile={vi.fn()} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'platform.checkAndPackage' }))
    await waitFor(() => expect(beforeAction).toHaveBeenCalled())
    expect(vi.mocked(management).mock.calls.some(([path]) => path.endsWith('/check'))).toBe(false)
  })

  it('can stop a preview even when an editor cannot save', async () => {
    const beforeAction = vi.fn().mockResolvedValue(false)
    render(<QueryClientProvider client={new QueryClient()}><ProjectDevelopmentTools projectId="project" visible refreshSignal={0} beforeAction={beforeAction} onOpenFile={vi.fn()} /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Open preview' }))
    await screen.findByText('Game preview')
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(management).toHaveBeenCalledWith('/runtimes/preview/stop', 'POST', {}))
    expect(beforeAction).not.toHaveBeenCalled()
  })
})

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GamePlayer } from './GamePlayer'
import { management, type Instance, type RuntimeSnapshot } from './api'

vi.mock('./api', () => ({ management: vi.fn(), platformError: () => 'handoff failed' }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en-US' } }) }))
vi.mock('next-themes', () => ({ useTheme: () => ({ resolvedTheme: 'dark' }) }))

afterEach(() => { cleanup(); vi.resetAllMocks() })

const id = '12345678-1234-1234-1234-123456789abc'
const runtime = (environment: 'preview' | 'installed') => ({
  id: 'current', viewUrl: 'http://127.0.0.1:18090/view',
  context: { environment, scope: { projectId: 'book' }, source: { package: { id: 'engine' } } },
}) as RuntimeSnapshot

function requestHandoff() {
  const frame = screen.getByTitle('platform.gameFrame') as HTMLIFrameElement
  fireEvent(window, new MessageEvent('message', {
    source: frame.contentWindow, origin: 'http://127.0.0.1:18090',
    data: { type: 'denova:open-instance', instanceId: id },
  }))
}

describe('game instance handoff', () => {
  it.each(['preview', 'installed'] as const)('opens another %s instance in the same project and engine', async environment => {
    const instance = { instanceId: id, projectId: 'book', gameId: 'engine', preview: environment === 'preview' } as Instance
    vi.mocked(management).mockImplementation(async path => {
      if (path === '/runtimes/current/stop') expect(screen.queryByTitle('platform.gameFrame')).toBeNull()
      return instance as never
    })
    const onOpenInstance = vi.fn()
    render(<GamePlayer runtime={runtime(environment)} visible onExit={vi.fn()} onOpenInstance={onOpenInstance} />)
    requestHandoff()
    await waitFor(() => expect(onOpenInstance).toHaveBeenCalledWith(instance))
    expect(management).toHaveBeenCalledWith('/runtimes/current/stop', 'POST', {})
  })

  it.each([
    { projectId: 'other-book', gameId: 'engine', preview: true },
    { projectId: 'book', gameId: 'other-engine', preview: true },
    { projectId: 'book', gameId: 'engine', preview: false },
  ])('rejects a different scope: %j', async instance => {
    vi.mocked(management).mockResolvedValue(instance)
    const onOpenInstance = vi.fn()
    render(<GamePlayer runtime={runtime('preview')} visible onExit={vi.fn()} onOpenInstance={onOpenInstance} />)
    requestHandoff()
    await screen.findByText('platform.errors.PERMISSION_DENIED')
    expect(onOpenInstance).not.toHaveBeenCalled()
    expect(management).not.toHaveBeenCalledWith('/runtimes/current/stop', 'POST', {})
  })

  it('shows an asynchronous navigation failure', async () => {
    vi.mocked(management).mockResolvedValue({ instanceId: id, projectId: 'book', gameId: 'engine', preview: true })
    render(<GamePlayer runtime={runtime('preview')} visible onExit={vi.fn()} onOpenInstance={async () => { throw new Error('Child runtime unavailable') }} />)
    requestHandoff()
    await screen.findByText('handoff failed')
  })
})

function frameMessage(data: Record<string, unknown>, source?: MessageEventSource | null) {
  const frame = screen.getByTitle('platform.gameFrame') as HTMLIFrameElement
  fireEvent(window, new MessageEvent('message', { source: source ?? frame.contentWindow, origin: 'http://127.0.0.1:18090', data }))
}

it('keeps story controls inside an optional overlay while the game remains mounted', () => {
  render(<GamePlayer runtime={runtime('installed')} visible onExit={vi.fn()} menuContent={<button>Saved stories</button>} />)
  const frame = screen.getByTitle('platform.gameFrame')
  expect(screen.queryByText('Saved stories')).toBeNull()
  expect(screen.queryByText('platform.exitGame')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'platform.storyMenu' }))
  expect(screen.getByText('Saved stories')).toBeVisible()
  expect(screen.getByText('platform.exitGame')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'common.close' }))
  expect(screen.getByTitle('platform.gameFrame')).toBe(frame)
})

describe('game exit handshake', () => {
  it('waits for the selected frame to save and stop, ignores unrelated acknowledgements, then exits once', async () => {
    vi.mocked(management).mockResolvedValue(undefined)
    const onExit = vi.fn()
    render(<GamePlayer runtime={runtime('installed')} visible onExit={onExit} />)
    const frame = screen.getByTitle('platform.gameFrame') as HTMLIFrameElement
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    frameMessage({ type: 'denova:ready', nonce: 'init', exitGuard: true })
    fireEvent.click(screen.getByRole('button', { name: 'platform.storyMenu' }))
    fireEvent.click(screen.getByText('platform.exitGame'))
    const request = post.mock.calls.find(([value]) => value.type === 'denova:prepare-exit')![0]
    expect(management).not.toHaveBeenCalled()
    frameMessage({ type: 'denova:exit-ready', requestId: 'wrong', ok: true })
    frameMessage({ type: 'denova:exit-ready', requestId: request.requestId, ok: true }, window)
    expect(management).not.toHaveBeenCalled()
    frameMessage({ type: 'denova:exit-ready', requestId: request.requestId, ok: true })
    await waitFor(() => expect(onExit).toHaveBeenCalledTimes(1))
    expect(management).toHaveBeenCalledWith('/runtimes/current/stop', 'POST', {})
  })

  it('keeps the same frame and local state when saving refuses exit', async () => {
    const onExit = vi.fn()
    render(<GamePlayer runtime={runtime('installed')} visible onExit={onExit} />)
    const frame = screen.getByTitle('platform.gameFrame') as HTMLIFrameElement
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    frameMessage({ type: 'denova:ready', nonce: 'init', exitGuard: true })
    frameMessage({ type: 'denova:exit' })
    const request = post.mock.calls.find(([value]) => value.type === 'denova:prepare-exit')![0]
    frameMessage({ type: 'denova:exit-ready', requestId: request.requestId, ok: false })
    await screen.findByText('platform.exitNotReady')
    expect(screen.getByTitle('platform.gameFrame')).toBe(frame)
    expect(management).not.toHaveBeenCalled()
    expect(onExit).not.toHaveBeenCalled()
  })

  it('keeps the document mounted if native stop fails', async () => {
    vi.mocked(management).mockRejectedValue(new Error('Stop failed'))
    render(<GamePlayer runtime={runtime('installed')} visible onExit={vi.fn()} />)
    const frame = screen.getByTitle('platform.gameFrame')
    fireEvent.click(screen.getByRole('button', { name: 'platform.storyMenu' }))
    fireEvent.click(screen.getByText('platform.exitGame'))
    await screen.findByText('handoff failed')
    expect(screen.getByTitle('platform.gameFrame')).toBe(frame)
  })
})

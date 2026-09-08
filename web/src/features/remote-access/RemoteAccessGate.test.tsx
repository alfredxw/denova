import { StrictMode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { queryClient } from '@/lib/query-client'
import { handleRemoteAccessChallenge } from '@/lib/api-client/client'
import { RemoteAccessGate } from './RemoteAccessGate'
import { remoteAccessQuery } from './api'

function mount() {
  return render(<StrictMode><QueryClientProvider client={queryClient}><RemoteAccessGate><div>Private workbench</div></RemoteAccessGate></QueryClientProvider></StrictMode>)
}

beforeEach(() => {
  queryClient.clear()
  setConfiguredLocale('en-US')
  window.history.replaceState(null, '', '/')
  window.sessionStorage.clear()
})
afterEach(() => { vi.unstubAllGlobals(); queryClient.clear() })

function json(body: unknown, status = 200) { return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }) }

describe('RemoteAccessGate', () => {
  it('waits for authentication, clears legacy passwords, and mounts the workbench after login', async () => {
    window.sessionStorage.setItem('nova.remoteAccess.credentials', 'legacy password')
    const fetchMock = vi.fn(async (url: RequestInfo | URL) => String(url).endsWith('/login')
      ? json({ authenticated: true, local: false })
      : json({ authenticated: false, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    mount()
    expect(screen.queryByText('Private workbench')).not.toBeInTheDocument()
    await screen.findByRole('heading', { name: 'Sign in to Denova' })
    expect(window.sessionStorage.getItem('nova.remoteAccess.credentials')).toBeNull()
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'reader' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: /^Sign in$/ }))
    await screen.findByText('Private workbench')
    expect(window.sessionStorage.getItem('nova.remoteAccess.credentials')).toBeNull()
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/login'))).toHaveLength(1)
  })

  it('consumes a fragment link once and removes its token before making the request', async () => {
    window.history.replaceState(null, '', '/?view=game#pair=one-use-secret')
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      expect(window.location.hash).toBe('')
      expect(window.location.search).toBe('?view=game')
      if (String(_url).endsWith('/status')) return json({ authenticated: false, local: false })
      expect(JSON.parse(String(init?.body))).toEqual({ token: 'one-use-secret' })
      return json({ authenticated: true, local: false })
    })
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByText('Private workbench')
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/pair'))).toHaveLength(1)
  })

  it('offers password login for an expired link and a retry for a connection failure', async () => {
    window.history.replaceState(null, '', '/#pair=expired')
    const fetchMock = vi.fn(async (url: RequestInfo | URL) => String(url).endsWith('/status') ? json({ authenticated: false, local: false }) : json({ error: 'Connection link expired' }, 401))
    vi.stubGlobal('fetch', fetchMock)
    const view = mount()
    await screen.findByText('Connection link expired')
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    view.unmount()
    queryClient.clear()
    fetchMock.mockImplementation(async () => { throw new Error('Network unavailable') })
    mount()
    await screen.findByText('Network unavailable', {}, { timeout: 2500 })
    expect(screen.queryByText('Private workbench')).not.toBeInTheDocument()
    fetchMock.mockImplementation(async () => json({ authenticated: true, local: true }))
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Private workbench')
  })

  it('keeps an existing login when opening an old connection link', async () => {
    window.history.replaceState(null, '', '/#pair=already-used')
    const fetchMock = vi.fn(async (_url: RequestInfo | URL) => json({ authenticated: true, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByText('Private workbench')
    expect(window.location.hash).toBe('')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/auth/status')
  })

  it('rechecks a failed startup when the PWA returns to the foreground', async () => {
    const fetchMock = vi.fn(async (): Promise<Response> => { throw new Error('Network unavailable') })
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByText('Network unavailable', {}, { timeout: 2500 })
    fetchMock.mockImplementation(async () => json({ authenticated: true, local: false }))
    act(() => { fireEvent(document, new Event('visibilitychange')) })
    await screen.findByText('Private workbench')
  })

  it('continues startup when connectivity recovers after the initial query retry', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockImplementation(async () => json({ authenticated: true, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByText('Private workbench', {}, { timeout: 2500 })
    expect(fetchMock).toHaveBeenCalledTimes(4)
  })

  it('handles scanned links opened in an existing login tab, including invalid links', async () => {
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      if (String(url).endsWith('/status')) return json({ authenticated: false, local: false })
      return JSON.parse(String(init?.body)).token === 'valid'
        ? json({ authenticated: true, local: false })
        : json({ error: 'Connection link expired' }, 401)
    })
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByRole('heading', { name: 'Sign in to Denova' })
    act(() => {
      window.history.replaceState(null, '', '/#pair=expired')
      fireEvent(window, new Event('hashchange'))
    })
    await screen.findByText('Connection link expired')
    expect(window.location.hash).toBe('')
    act(() => {
      window.history.replaceState(null, '', '/#pair=valid')
      fireEvent(window, new Event('hashchange'))
    })
    await screen.findByText('Private workbench')
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/pair'))).toHaveLength(2)
  })

  it('checks a late challenge against the current cookie before hiding the workbench', async () => {
    const fetchMock = vi.fn(async () => json({ authenticated: true, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await screen.findByText('Private workbench')
    await act(async () => { handleRemoteAccessChallenge() })
    expect(screen.getByText('Private workbench')).toBeInTheDocument()
    fetchMock.mockImplementation(async () => json({ authenticated: false, local: false }))
    await act(async () => { handleRemoteAccessChallenge() })
    await waitFor(() => expect(screen.queryByText('Private workbench')).not.toBeInTheDocument())
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
  })

  it('recovers a failed startup status read without losing or replaying the pairing token', async () => {
    vi.useFakeTimers()
    window.history.replaceState(null, '', '/#pair=one-use-secret')
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValueOnce(json({ authenticated: false, local: false }))
      .mockResolvedValueOnce(json({ authenticated: true, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    const result = queryClient.fetchQuery(remoteAccessQuery)
    const assertion = expect(result).resolves.toEqual({ authenticated: true, local: false })
    await vi.advanceTimersByTimeAsync(1000)
    await assertion
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/auth/status', '/api/auth/status', '/api/auth/pair'])
    expect(JSON.parse(fetchMock.mock.calls[2]?.[1].body)).toEqual({ token: 'one-use-secret' })
    expect(window.location.hash).toBe('')
  })

  it('never automatically replays a pairing POST after a lost response', async () => {
    vi.useFakeTimers()
    window.history.replaceState(null, '', '/#pair=one-use-secret')
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json({ authenticated: false, local: false }))
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValue(json({ authenticated: true, local: false }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(queryClient.fetchQuery(remoteAccessQuery)).rejects.toThrow('Load failed')
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetchMock.mock.calls.filter(([url]) => url === '/api/auth/pair')).toHaveLength(1)
  })
})

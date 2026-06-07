import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingPanel } from './SettingPanel'

describe('SettingPanel', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn(async (input) => {
      const rawUrl = typeof input === 'string' ? input : input.url
      const path = new URL(rawUrl, 'http://localhost').pathname
      const payloads: Record<string, unknown> = {
        '/api/lore/items': { items: [] },
        '/api/lore/versions': { versions: [] },
        '/api/lore/agent/messages': [],
      }
      return new Response(JSON.stringify(payloads[path] ?? {}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }) as typeof fetch
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('prefills the lore-init instruction from the empty Lore Agent shortcut', async () => {
    const user = userEvent.setup()
    render(<SettingPanel mode="lore" workspace="/books/demo" />)

    expect(await screen.findByText('和资料库 Agent 对话')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '初始化故事设定' }))

    await waitFor(() => {
      expect(screen.getByDisplayValue(/lore-init/)).toBeInTheDocument()
    })
  })
})

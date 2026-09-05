import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AgentSubAgentSessionPanel } from './AgentSubAgentSessionPanel'

describe('AgentSubAgentSessionPanel chrome', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('leaves closing to the tab strip in tab mode', () => {
    const onClose = vi.fn()
    const { rerender } = render(
      <AgentSubAgentSessionPanel chrome="tab" messages={[]} sessionKey="child-session" />,
    )

    expect(screen.queryByRole('button', { name: 'Close SubAgent details' })).not.toBeInTheDocument()

    rerender(
      <AgentSubAgentSessionPanel messages={[]} sessionKey="child-session" onClose={onClose} />,
    )
    expect(screen.getByRole('button', { name: 'Close SubAgent details' })).toBeInTheDocument()
  })
})

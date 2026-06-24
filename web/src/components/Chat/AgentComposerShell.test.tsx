import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AgentComposerShell } from './AgentComposerShell'

describe('AgentComposerShell', () => {
  it('keeps input and end controls in separate component slots', () => {
    const { container } = render(
      <AgentComposerShell
        input={<textarea aria-label="Prompt" />}
        toolbarStart={<button type="button">Actions</button>}
        toolbarEnd={<button type="button">Choices</button>}
        submitControl={<button type="button">Send</button>}
      />,
    )

    const inputSlot = container.querySelector('[data-slot="agent-composer-input"]')
    const startSlot = container.querySelector('[data-slot="agent-composer-start"]')
    const endSlot = container.querySelector('[data-slot="agent-composer-end"]')

    expect(inputSlot).toContainElement(screen.getByLabelText('Prompt'))
    expect(startSlot).toContainElement(screen.getByRole('button', { name: 'Actions' }))
    expect(endSlot).toContainElement(screen.getByRole('button', { name: 'Choices' }))
    expect(endSlot).toContainElement(screen.getByRole('button', { name: 'Send' }))
    expect(inputSlot).not.toContainElement(screen.getByRole('button', { name: 'Send' }))
  })

  it('keeps the multiline input marker on the input element inside the input slot', () => {
    const { container } = render(
      <AgentComposerShell
        input={<textarea aria-label="Prompt" data-nova-multiline="true" />}
        submitControl={<button type="button">Send</button>}
      />,
    )

    const inputSlot = container.querySelector('[data-slot="agent-composer-input"]')
    const endSlot = container.querySelector('[data-slot="agent-composer-end"]')

    expect(inputSlot?.querySelector('[data-nova-multiline="true"]')).toBe(screen.getByLabelText('Prompt'))
    expect(endSlot).toContainElement(screen.getByRole('button', { name: 'Send' }))
  })
})

import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MobilePaneTrigger } from './mobile-pane-trigger'

describe('MobilePaneTrigger', () => {
  it('keeps the compact trigger independent from the legacy 40px icon style', () => {
    render(
      <MobilePaneTrigger
        side="left"
        label="Open lore directory"
        appearance="compact"
        onClick={vi.fn()}
      />,
    )

    const trigger = screen.getByRole('button', { name: 'Open lore directory' })
    expect(trigger).toHaveAttribute('data-size', 'icon-xs')
    expect(trigger).not.toHaveClass('nova-icon-button')
    expect(trigger).not.toHaveAttribute('title')
  })
})

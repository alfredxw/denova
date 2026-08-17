import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LoadingState } from './LoadingState'

describe('LoadingState', () => {
  it('keeps the localized progress label accessible while showing a content skeleton', () => {
    render(<LoadingState label="正在准备创作空间…" />)

    expect(screen.getByRole('status', { name: '正在准备创作空间…' })).toHaveTextContent('正在准备创作空间…')
    expect(screen.getByText('正在准备创作空间…')).toHaveClass('sr-only')
    expect(screen.getByRole('status')).toHaveAttribute('data-layout', 'content')
    expect(document.querySelector('[data-slot="loading-state-content"]')).toBeInTheDocument()
    expect(document.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(6)
  })

  it('uses a compact list skeleton for panel loading', () => {
    render(<LoadingState label="Loading..." variant="panel" />)

    expect(screen.getByRole('status')).toHaveAttribute('data-variant', 'panel')
    expect(screen.getByRole('status')).toHaveAttribute('data-layout', 'list')
    expect(document.querySelector('[data-slot="loading-state-list"]')).toBeInTheDocument()
  })

  it('supports a bottom-aligned conversation skeleton', () => {
    render(<LoadingState label="Loading messages..." layout="conversation" />)

    expect(screen.getByRole('status')).toHaveAttribute('data-layout', 'conversation')
    expect(document.querySelector('[data-slot="loading-state-conversation"]')).toBeInTheDocument()
  })
})

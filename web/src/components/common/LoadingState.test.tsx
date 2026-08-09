import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LoadingState } from './LoadingState'

describe('LoadingState', () => {
  it('exposes the localized progress label without duplicating it for decoration', () => {
    render(<LoadingState label="正在准备创作空间…" />)

    expect(screen.getByRole('status', { name: '正在准备创作空间…' })).toHaveTextContent('正在准备创作空间…')
    expect(screen.getAllByText('正在准备创作空间…')).toHaveLength(1)
  })

  it('supports compact panel loading without changing the status semantics', () => {
    render(<LoadingState label="Loading..." variant="panel" />)

    expect(screen.getByRole('status')).toHaveAttribute('data-variant', 'panel')
  })
})

import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { Toaster } from './sonner'

vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: 'light' }),
}))

vi.mock('sonner', () => ({
  Toaster: ({ icons }: { icons?: { error?: ReactNode } }) => (
    <div data-testid="error-icon">{icons?.error}</div>
  ),
}))

describe('Toaster', () => {
  it('keeps the dismiss x visually distinct from the error status icon', () => {
    render(<Toaster />)

    const errorIcon = screen.getByTestId('error-icon').querySelector('svg')
    expect(errorIcon).toHaveClass('lucide-octagon-alert')
    expect(errorIcon).not.toHaveClass('lucide-octagon-x')
  })
})

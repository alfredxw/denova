import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Textarea } from './textarea'

describe('Textarea', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('auto-resizes up to the configured row cap', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize maxRows={10} aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement
    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 360 })

    fireEvent.input(textarea, { target: { value: 'line\n'.repeat(20) } })

    expect(textarea.style.height).toBe('218px')
    expect(textarea.style.overflowY).toBe('auto')
  })
})

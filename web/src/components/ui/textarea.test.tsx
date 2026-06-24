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
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')
  })

  it('marks multiline auto-resize only after content exceeds one row', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 36 })
    fireEvent.input(textarea, { target: { value: 'short prompt' } })
    expect(textarea).not.toHaveAttribute('data-nova-multiline')

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 58 })
    fireEvent.input(textarea, { target: { value: 'long prompt that wraps onto another visual row' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')
  })

  it('keeps an empty auto-resize textarea at one row even when placeholder would wrap', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize aria-label="prompt" placeholder="a long placeholder" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement
    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 58 })

    fireEvent.input(textarea, { target: { value: '' } })

    expect(textarea.style.height).toBe('38px')
    expect(textarea.style.overflowY).toBe('hidden')
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
  })

  it('keeps sticky multiline until the content is cleared', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize multilineMode="sticky-until-empty" aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 58 })
    fireEvent.input(textarea, { target: { value: 'long prompt that wraps onto another visual row' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 36 })
    fireEvent.input(textarea, { target: { value: 'still non-empty' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')
    expect(textarea.style.height).toBe('38px')

    fireEvent.input(textarea, { target: { value: '' } })
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
  })
})

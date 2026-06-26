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

  it('defaults auto-resize to a 10 row cap before scrolling', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
      minHeight: '38px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement
    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 360 })

    fireEvent.input(textarea, { target: { value: 'line\n'.repeat(20) } })

    expect(textarea.style.height).toBe('218px')
    expect(textarea.style.overflowY).toBe('auto')
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

  it('does not treat the configured min-height as a wrapped line', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
      minHeight: '42px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 42 })
    fireEvent.input(textarea, { target: { value: '短句' } })

    expect(textarea.style.height).toBe('42px')
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
  })

  it('enters multiline when text reaches the compact composer input width', () => {
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => {
      if (element instanceof HTMLElement && element.dataset.slot === 'agent-composer-layout') {
        return { columnGap: '8px', gap: '8px' } as CSSStyleDeclaration
      }
      return {
        font: '16px sans-serif',
        lineHeight: '20px',
        paddingLeft: '4px',
        paddingRight: '4px',
        paddingTop: '8px',
        paddingBottom: '8px',
        borderTopWidth: '1px',
        borderBottomWidth: '1px',
        minHeight: '38px',
      } as CSSStyleDeclaration
    })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      measureText: (text: string) => ({ width: text.length * 10 }),
    } as CanvasRenderingContext2D)

    render(
      <div className="nova-agent-composer">
        <div data-slot="agent-composer-layout" className="nova-agent-composer-toolbar">
          <div data-slot="agent-composer-start">menu</div>
          <Textarea autoResize aria-label="prompt" />
          <div data-slot="agent-composer-end">send</div>
        </div>
      </div>,
    )
    const toolbar = screen.getByText('menu').parentElement as HTMLElement
    const start = screen.getByText('menu')
    const end = screen.getByText('send')
    vi.spyOn(toolbar, 'getBoundingClientRect').mockReturnValue({ width: 220 } as DOMRect)
    vi.spyOn(start, 'getBoundingClientRect').mockReturnValue({ width: 40 } as DOMRect)
    vi.spyOn(end, 'getBoundingClientRect').mockReturnValue({ width: 40 } as DOMRect)

    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement
    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 38 })

    fireEvent.input(textarea, { target: { value: '12345678901234' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')

    fireEvent.input(textarea, { target: { value: '1234567890' } })
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
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

  it('shrinks back to one row when auto-resized content no longer wraps', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
      minHeight: '38px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 58 })
    fireEvent.input(textarea, { target: { value: 'long prompt that wraps onto another visual row' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 38 })
    fireEvent.input(textarea, { target: { value: 'short prompt' } })
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
    expect(textarea.style.height).toBe('38px')
  })

  it('keeps sticky multiline until the content is cleared when requested', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
      minHeight: '38px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize multilineMode="sticky-until-empty" aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 58 })
    fireEvent.input(textarea, { target: { value: 'long prompt that wraps onto another visual row' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 38 })
    fireEvent.input(textarea, { target: { value: 'still non-empty' } })
    expect(textarea).toHaveAttribute('data-nova-multiline', 'true')
    expect(textarea.style.height).toBe('38px')

    fireEvent.input(textarea, { target: { value: '' } })
    expect(textarea).not.toHaveAttribute('data-nova-multiline')
  })

  it('keeps capped auto-resize textarea pinned to bottom after browser scroll reset', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      lineHeight: '20px',
      paddingTop: '8px',
      paddingBottom: '8px',
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
    } as CSSStyleDeclaration)
    render(<Textarea autoResize maxRows={10} aria-label="prompt" />)
    const textarea = screen.getByLabelText('prompt') as HTMLTextAreaElement
    let scrollTop = 800
    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 1000 })
    Object.defineProperty(textarea, 'clientHeight', { configurable: true, value: 200 })
    Object.defineProperty(textarea, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: (value) => {
        scrollTop = Math.max(0, Math.min(value, textarea.scrollHeight - textarea.clientHeight))
      },
    })
    let height = ''
    Object.defineProperty(textarea.style, 'height', {
      configurable: true,
      get: () => height,
      set: (value) => {
        height = value
        if (value === 'auto') scrollTop = 0
      },
    })

    fireEvent.input(textarea, { target: { value: 'line\n'.repeat(30) + '中文上屏' } })

    expect(textarea.style.overflowY).toBe('auto')
    expect(textarea.scrollTop).toBe(800)
  })
})

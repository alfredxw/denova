import { createRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SearchHighlightTextarea } from './SearchHighlightTextarea'

describe('SearchHighlightTextarea', () => {
  it('highlights matches and keeps a fixed-height overlay scrolled with the textarea', () => {
    const ref = createRef<HTMLTextAreaElement>()
    const { container } = render(
      <SearchHighlightTextarea
        ref={ref}
        aria-label="Raw lore"
        autoResize={false}
        containerClassName="flex-1"
        className="h-full font-mono"
        highlightQuery="target"
        value={'before\ntarget\nafter'}
        readOnly
      />,
    )

    expect(container.querySelector('mark')).toHaveTextContent('target')
    expect(ref.current).toBe(screen.getByRole('textbox', { name: 'Raw lore' }))
    expect(ref.current?.style.caretColor).toBe('var(--nova-text)')

    const overlay = container.querySelector('pre') as HTMLPreElement
    const textarea = ref.current!
    textarea.scrollTop = 40
    textarea.scrollLeft = 12
    fireEvent.scroll(textarea)

    expect(overlay.scrollTop).toBe(40)
    expect(overlay.scrollLeft).toBe(12)
  })
})

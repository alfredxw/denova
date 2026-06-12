import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { InputArea } from './InputArea'

describe('InputArea', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows only skills in skills command mode and keeps keyboard selection visible', () => {
    const scrollSpy = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    render(
      <InputArea
        onSend={vi.fn()}
        disabled={false}
        commandScope="skills"
        skills={[
          { name: 'outline', description: 'Outline the next arc' },
          { name: 'rewrite', description: 'Rewrite selected prose' },
          { name: 'worldbuild', description: 'Expand setting details' },
        ]}
      />,
    )

    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '/' } })

    expect(screen.getByText('/outline')).toBeInTheDocument()
    expect(screen.getByText('/rewrite')).toBeInTheDocument()
    expect(screen.queryByText('/plan')).not.toBeInTheDocument()

    scrollSpy.mockClear()
    fireEvent.keyDown(input, { key: 'ArrowDown' })

    expect(scrollSpy).toHaveBeenCalledWith({ block: 'nearest' })
  })
})

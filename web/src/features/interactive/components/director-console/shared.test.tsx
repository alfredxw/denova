import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StateValue } from './shared'

describe('StateValue', () => {
  it('renders long simple-array entries as bounded rounded rectangles', () => {
    const longItem = "一截黑色断剑碎片（已确认为三千年前古剑'渊玄'的剑尖残片，品阶古宝之上，剑脊残留半道断纹）"

    render(<StateValue value={[longItem]} />)

    const item = screen.getByText(longItem)
    expect(item).toHaveClass('max-w-full', 'min-w-0', 'rounded-md', 'whitespace-normal', 'break-words', '[overflow-wrap:anywhere]')
    expect(item).not.toHaveClass('rounded-full')
  })
})

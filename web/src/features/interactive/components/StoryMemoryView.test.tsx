import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { StoryMemoryView } from './StoryMemoryView'

describe('StoryMemoryView', () => {
  it('shows an explicit return action in the manager header', async () => {
    const onBackToStory = vi.fn()

    render(<StoryMemoryView onBackToStory={onBackToStory} />)

    await userEvent.click(screen.getByRole('button', { name: '返回剧情' }))
    expect(onBackToStory).toHaveBeenCalledTimes(1)
  })
})

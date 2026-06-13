import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StoryPicker } from './StoryPicker'

describe('StoryPicker', () => {
  it('passes reply target chars when creating a story', () => {
    const onCreate = vi.fn()

    render(
      <StoryPicker
        stories={[]}
        currentStoryId=""
        tellers={[
          {
            version: 3,
            id: 'classic',
            name: '经典叙事',
            description: '',
            random_event_rate: 0.15,
            tags: [],
            context_policy: {
              creator: 'always',
              lore: 'relevant',
              runtime_state: 'always',
              recent_turns: 8,
            },
            slots: [],
            custom: false,
          },
        ]}
        onSelect={vi.fn()}
        onCreate={onCreate}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    fireEvent.change(screen.getByText('每轮目标字数').parentElement?.querySelector('input') as HTMLInputElement, { target: { value: '650' } })
    fireEvent.click(screen.getByRole('button', { name: '创建' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        story_teller_id: 'classic',
        reply_target_chars: 650,
      }),
    )
  })
})

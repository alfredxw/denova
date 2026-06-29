import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StoryPicker } from './StoryPicker'

describe('StoryPicker', () => {
  it('shows every story option immediately when opened', () => {
    const stories = Array.from({ length: 12 }, (_, index) => story(`st_${index + 1}`, `故事线 ${index + 1}`))

    render(
      <StoryPicker
        stories={stories}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))

    expect(screen.getAllByRole('option')).toHaveLength(12)
    expect(screen.getByRole('option', { name: '故事线 12' })).toBeInTheDocument()
  })

  it('selects a story option and closes the panel', () => {
    const onSelect = vi.fn()

    render(
      <StoryPicker
        stories={[story('st_1', '主线'), story('st_2', '支线')]}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={onSelect}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))
    fireEvent.click(screen.getByRole('option', { name: '支线' }))

    expect(onSelect).toHaveBeenCalledWith('st_2')
    expect(screen.queryByRole('option', { name: '支线' })).not.toBeInTheDocument()
  })

  it('keeps delete action inside the story selector panel', () => {
    render(
      <StoryPicker
        stories={[story('st_1', '主线')]}
        currentStoryId="st_1"
        tellers={[]}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    expect(screen.queryByRole('button', { name: '删除故事线' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))

    expect(screen.getByRole('button', { name: '删除故事线' })).toBeInTheDocument()
  })

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

function story(id: string, title: string) {
  return {
    id,
    title,
    origin: '',
    story_teller_id: 'classic',
    reply_target_chars: 900,
    opening: { mode: 'ai' as const },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 1,
  }
}

import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { StorySummary } from '../types'
import { StoryPicker } from './StoryPicker'

const story: StorySummary = {
  id: 'story-1',
  title: '港口的灯逐盏熄灭',
  title_source: 'generated',
  origin: '',
  protagonist: { mode: 'custom', name: '林川' },
  story_teller_id: 'classic',
  story_director_id: 'default',
  planning_mode: 'enabled',
  reply_target_chars: 2000,
  choice_count: 5,
  opening: { mode: 'custom', custom_text: '港口的灯逐盏熄灭。' },
  created_at: '2026-08-31T00:00:00Z',
  updated_at: '2026-08-31T00:00:00Z',
  branches: 1,
  events: 1,
  turn_count: 1,
}

describe('StoryPicker', () => {
  it('renames the current story from the picker', async () => {
    const user = userEvent.setup()
    const onRenameStory = vi.fn().mockResolvedValue(undefined)
    render(
      <StoryPicker
        stories={[story]}
        currentStoryId={story.id}
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDeleteStories={vi.fn()}
        onRenameStory={onRenameStory}
      />,
    )

    await user.click(screen.getByRole('button', { name: '选择故事线' }))
    await user.click(screen.getByRole('button', { name: '重命名当前故事线' }))
    const nameInput = screen.getByRole('textbox', { name: '故事线名称' })
    expect(nameInput).toHaveValue(story.title)
    await user.clear(nameInput)
    await user.type(nameInput, '雾港来信')
    await user.click(screen.getByRole('button', { name: '保存名称' }))

    await waitFor(() => expect(onRenameStory).toHaveBeenCalledWith(story.id, '雾港来信'))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })
})

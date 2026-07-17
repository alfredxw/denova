import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StoryPicker } from './StoryPicker'

describe('StoryPicker', () => {
  it('shows and selects every story from the selector', () => {
    const onSelect = vi.fn()
    const stories = Array.from({ length: 12 }, (_, index) => story(`st_${index + 1}`, `故事线 ${index + 1}`))
    render(<StoryPicker stories={stories} currentStoryId="st_1" onSelect={onSelect} onCreate={vi.fn()} onDelete={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))
    expect(screen.getAllByRole('option')).toHaveLength(12)
    fireEvent.click(screen.getByRole('option', { name: '故事线 12' }))
    expect(onSelect).toHaveBeenCalledWith('st_12')
  })

  it('starts inline creation without opening a popover form', () => {
    const onCreate = vi.fn()
    render(<StoryPicker stories={[story('st_1', '主线')]} currentStoryId="st_1" onSelect={vi.fn()} onCreate={onCreate} onDelete={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    expect(onCreate).toHaveBeenCalledOnce()
    expect(screen.queryByText('每轮目标字数')).not.toBeInTheDocument()
  })
})

function story(id: string, title: string) {
  return { id, title, origin: '', story_teller_id: 'classic', story_director_id: 'default', choice_count: 5, reply_target_chars: 2000, opening: { mode: 'ai' as const }, created_at: '', updated_at: '', branches: 1, events: 0 }
}

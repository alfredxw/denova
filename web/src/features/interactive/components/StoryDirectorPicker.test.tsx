import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StoryDirectorPicker } from './StoryDirectorPicker'
import type { StoryDirector } from '../types'

describe('StoryDirectorPicker', () => {
  it('shows every director option immediately when opened', () => {
    const directors = Array.from({ length: 10 }, (_, index) => storyDirector(`director_${index + 1}`, `导演 ${index + 1}`))

    render(
      <StoryDirectorPicker
        story={story('director_1')}
        storyDirectors={directors}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事导演' }))

    expect(screen.getAllByRole('option')).toHaveLength(10)
    expect(screen.getByRole('option', { name: '导演 10' })).toBeInTheDocument()
  })

  it('selects a director and closes the panel', () => {
    const onChange = vi.fn()

    render(
      <StoryDirectorPicker
        story={story('default')}
        storyDirectors={[storyDirector('default', '默认导演'), storyDirector('high-fantasy', '玄幻导演')]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事导演' }))
    fireEvent.click(screen.getByRole('option', { name: '玄幻导演' }))

    expect(onChange).toHaveBeenCalledWith('high-fantasy')
    expect(screen.queryByRole('option', { name: '玄幻导演' })).not.toBeInTheDocument()
  })
})

function storyDirector(id: string, name: string): StoryDirector {
  return {
    version: 1,
    id,
    name,
    description: '',
    strategy: { enabled: true },
    stat_system: {},
    trpg_system: {},
    opening_selector: { enabled: true },
    tags: [],
    custom: false,
  }
}

function story(storyDirectorId: string) {
  return {
    id: 'story-1',
    title: '故事线',
    origin: '',
    story_teller_id: 'classic',
    story_director_id: storyDirectorId,
    reply_target_chars: 900,
    opening: { mode: 'ai' as const },
    created_at: '',
    updated_at: '',
    branches: 1,
    events: 1,
  }
}

import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TellerPicker } from './TellerPicker'
import type { Teller } from '../types'

describe('TellerPicker', () => {
  it('shows every narrative option immediately when opened', () => {
    const tellers = Array.from({ length: 12 }, (_, index) => teller(`teller_${index + 1}`, `叙事风格 ${index + 1}`))

    render(
      <TellerPicker
        story={{
          id: 'st_1',
          title: '故事线',
          origin: '',
          story_teller_id: 'teller_1',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 900,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 1,
          events: 1,
        }}
        tellers={tellers}
        onChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择叙事' }))

    expect(screen.getAllByRole('option')).toHaveLength(12)
    expect(screen.getByRole('option', { name: '叙事风格 12' })).toBeInTheDocument()
  })

  it('selects a narrative option and closes the panel', () => {
    const onChange = vi.fn()

    render(
      <TellerPicker
        story={{
          id: 'st_1',
          title: '故事线',
          origin: '',
          story_teller_id: 'classic',
          story_director_id: 'default',
          choice_count: 5,
          reply_target_chars: 900,
          opening: { mode: 'ai' },
          created_at: '',
          updated_at: '',
          branches: 1,
          events: 1,
        }}
        tellers={[teller('classic', '经典叙事'), teller('dark', '黑暗叙事')]}
        onChange={onChange}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择叙事' }))
    fireEvent.click(screen.getByRole('option', { name: '黑暗叙事' }))

    expect(onChange).toHaveBeenCalledWith('dark')
    expect(screen.queryByRole('option', { name: '黑暗叙事' })).not.toBeInTheDocument()
  })
})

function teller(id: string, name: string): Teller {
  return {
    version: 1,
    id,
    name,
    description: '',
    context_policy: {
      creator: 'summary',
      lore: 'summary',
      runtime_state: 'full',
    },
    slots: [],
    custom: false,
    updated_at: '',
  }
}

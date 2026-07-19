import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { StorySummary } from '../types'
import { ReplyTargetCharsControl } from './ReplyTargetCharsControl'

it('autosaves reply target changes without a Save button', async () => {
  const onChange = vi.fn().mockResolvedValue(undefined)
  render(<ReplyTargetCharsControl story={storySummary()} onChange={onChange} />)

  fireEvent.click(screen.getByRole('button', { name: '设置每轮目标字数' }))
  const input = screen.getByRole('spinbutton')
  fireEvent.change(input, { target: { value: '1200' } })
  expect(screen.queryByRole('button', { name: '保存' })).not.toBeInTheDocument()
  fireEvent.keyDown(input, { key: 's', ctrlKey: true })

  await waitFor(() => expect(onChange).toHaveBeenCalledWith(1200))
  expect(screen.getByText('每轮目标字数')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '关闭' }))
  await waitFor(() => expect(screen.queryByText('每轮目标字数')).not.toBeInTheDocument())
})

it('loads an external reply target update while the open editor is clean', async () => {
  const onChange = vi.fn().mockResolvedValue(undefined)
  const view = render(<ReplyTargetCharsControl story={storySummary()} onChange={onChange} />)

  fireEvent.click(screen.getByRole('button', { name: '设置每轮目标字数' }))
  const input = screen.getByRole('spinbutton')
  expect(input).toHaveValue(900)
  view.rerender(<ReplyTargetCharsControl story={{ ...storySummary(), reply_target_chars: 1100, updated_at: 'r2' }} onChange={onChange} />)

  await waitFor(() => expect(input).toHaveValue(1100))
  fireEvent.keyDown(input, { key: 's', ctrlKey: true })
  expect(onChange).not.toHaveBeenCalled()
})

it('keeps a local reply target edit when an external update arrives', async () => {
  const onChange = vi.fn().mockResolvedValue(undefined)
  const view = render(<ReplyTargetCharsControl story={storySummary()} onChange={onChange} />)

  fireEvent.click(screen.getByRole('button', { name: '设置每轮目标字数' }))
  const input = screen.getByRole('spinbutton')
  fireEvent.change(input, { target: { value: '1200' } })
  view.rerender(<ReplyTargetCharsControl story={{ ...storySummary(), reply_target_chars: 1100, updated_at: 'r2' }} onChange={onChange} />)

  expect(input).toHaveValue(1200)
  fireEvent.keyDown(input, { key: 's', ctrlKey: true })
  await waitFor(() => expect(onChange).toHaveBeenCalledWith(1200))
})

function storySummary(): StorySummary {
  return {
    id: 'story-1',
    title: 'Demo',
    origin: 'test',
    story_teller_id: 'teller',
    story_director_id: 'director',
    reply_target_chars: 900,
    choice_count: 3,
    opening: { mode: 'ai' },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    branches: 1,
    events: 0,
  }
}

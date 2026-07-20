import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18n, { setConfiguredLocale } from '@/i18n'
import { StoryPicker } from './StoryPicker'

afterEach(async () => {
  setConfiguredLocale('zh-CN')
  await i18n.changeLanguage('zh-CN')
})

describe('StoryPicker', () => {
  it('shows and selects every story from the selector', () => {
    const onSelect = vi.fn()
    const stories = Array.from({ length: 12 }, (_, index) => story(`st_${index + 1}`, `故事线 ${index + 1}`))
    render(<StoryPicker stories={stories} currentStoryId="st_1" onSelect={onSelect} onCreate={vi.fn()} onDeleteStories={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))
    expect(screen.getAllByRole('button', { name: /^故事线 \d+$/ })).toHaveLength(12)
    fireEvent.click(screen.getByRole('button', { name: '故事线 12' }))
    expect(onSelect).toHaveBeenCalledWith('st_12')
  })

  it('starts inline creation without opening a popover form', () => {
    const onCreate = vi.fn()
    render(<StoryPicker stories={[story('st_1', '主线')]} currentStoryId="st_1" onSelect={vi.fn()} onCreate={onCreate} onDeleteStories={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '新建' }))
    expect(onCreate).toHaveBeenCalledOnce()
    expect(screen.queryByText('每轮目标字数')).not.toBeInTheDocument()
  })

  it('selects multiple stories and confirms one batch deletion', async () => {
    const onDeleteStories = vi.fn().mockResolvedValue(undefined)
    render(
      <StoryPicker
        stories={[story('st_1', '主线'), story('st_2', '黑暗线'), story('st_3', '光明线')]}
        currentStoryId="st_1"
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDeleteStories={onDeleteStories}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '选择故事线' }))
    fireEvent.click(screen.getByRole('button', { name: '批量删除故事线' }))

    expect(screen.getByRole('button', { name: '主线' })).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(screen.getByRole('button', { name: '黑暗线' }))
    expect(screen.getByText('已选择 2 条')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '删除 2 条' }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('删除 2 条故事线？')
    expect(screen.getByRole('alertdialog')).toHaveTextContent('主线')
    expect(screen.getByRole('alertdialog')).toHaveTextContent('黑暗线')

    fireEvent.click(screen.getByRole('button', { name: '删除' }))
    await waitFor(() => expect(onDeleteStories).toHaveBeenCalledWith(['st_1', 'st_2']))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('renders the batch deletion flow in English with singular confirmation copy', async () => {
    setConfiguredLocale('en-US')
    await i18n.changeLanguage('en-US')
    render(
      <StoryPicker
        stories={[story('st_1', 'Main story')]}
        currentStoryId="st_1"
        onSelect={vi.fn()}
        onCreate={vi.fn()}
        onDeleteStories={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Choose Story' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete multiple stories' }))
    expect(screen.getByText('1 selected')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete 1' }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('Delete 1 story?')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })
})

function story(id: string, title: string) {
  return { id, title, origin: '', story_teller_id: 'classic', story_director_id: 'default', choice_count: 5, reply_target_chars: 2000, opening: { mode: 'ai' as const }, created_at: '', updated_at: '', branches: 1, events: 0 }
}

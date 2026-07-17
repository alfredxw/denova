import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { InlineCommentThread } from './InlineCommentThread'

describe('InlineCommentThread', () => {
  it('uses the submitted-comment body treatment while editing instead of nesting another rounded panel', async () => {
    const user = userEvent.setup()
    const view = render(
      <InlineCommentThread
        comments={[{ id: 'comment-1', group_id: 'group-1', body: '原评论' }]}
        onUpdate={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '修改评论' }))
    expect(screen.getByRole('textbox')).toHaveClass(
      'nova-review-inline-editor',
      'rounded-none',
      'border-0',
      'bg-transparent',
      'resize-none',
      'focus:outline-none',
      'focus-visible:outline-none',
    )

    view.rerender(
      <InlineCommentThread
        draft={{
          body: '新评论草稿',
          submitting: false,
          onChange: vi.fn(),
          onSubmit: vi.fn(),
          onCancel: vi.fn(),
        }}
      />,
    )
    expect(screen.getByRole('textbox')).toHaveClass(
      'nova-review-inline-editor',
      'rounded-none',
      'border-0',
      'bg-transparent',
      'resize-none',
      'focus:outline-none',
      'focus-visible:outline-none',
    )
  })

  it('focuses comment editors without allowing the browser to scroll their outer review surface', async () => {
    const user = userEvent.setup()
    const focus = vi.spyOn(HTMLTextAreaElement.prototype, 'focus')
    const view = render(
      <InlineCommentThread
        comments={[{ id: 'comment-1', group_id: 'group-1', body: '原评论' }]}
        onUpdate={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '修改评论' }))

    await waitFor(() => expect(focus).toHaveBeenCalledWith({ preventScroll: true }))

    view.unmount()
    focus.mockClear()
    render(
      <InlineCommentThread
        draft={{
          body: '',
          submitting: false,
          onChange: vi.fn(),
          onSubmit: vi.fn(),
          onCancel: vi.fn(),
        }}
      />,
    )
    await waitFor(() => expect(focus).toHaveBeenCalledWith({ preventScroll: true }))
    focus.mockRestore()
  })

  it('keeps edited text available when the update mutation fails', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn().mockRejectedValue(new Error('offline'))
    render(
      <InlineCommentThread
        comments={[{ id: 'comment-1', group_id: 'group-1', body: '原评论' }]}
        onUpdate={onUpdate}
      />,
    )

    await user.click(screen.getByRole('button', { name: '修改评论' }))
    const textbox = screen.getByRole('textbox')
    await user.clear(textbox)
    await user.type(textbox, '不要丢失这段意见')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('textbox')).toHaveValue('不要丢失这段意见')
  })

  it('saves a comment after existing characters are deleted', async () => {
    const user = userEvent.setup()
    const comment = { id: 'comment-1', group_id: 'group-1', body: '需要删除的原评论' }
    const onUpdate = vi.fn().mockResolvedValue(undefined)
    render(<InlineCommentThread comments={[comment]} onUpdate={onUpdate} />)

    await user.click(screen.getByRole('button', { name: '修改评论' }))
    const textbox = screen.getByRole('textbox')
    await user.clear(textbox)
    await user.type(textbox, '精简评论')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith(comment, '精简评论'))
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('reports the whole lifetime of an existing comment edit draft', async () => {
    const user = userEvent.setup()
    const onEditingChange = vi.fn()
    const view = render(
      <InlineCommentThread
        comments={[{ id: 'comment-1', group_id: 'group-1', body: '原评论' }]}
        onUpdate={vi.fn()}
        onEditingChange={onEditingChange}
      />,
    )

    await user.click(screen.getByRole('button', { name: '修改评论' }))
    expect(onEditingChange).toHaveBeenLastCalledWith(true)
    await user.click(screen.getByRole('button', { name: '取消' }))
    expect(onEditingChange).toHaveBeenLastCalledWith(false)

    await user.click(screen.getByRole('button', { name: '修改评论' }))
    view.unmount()
    expect(onEditingChange).toHaveBeenLastCalledWith(false)
  })
})

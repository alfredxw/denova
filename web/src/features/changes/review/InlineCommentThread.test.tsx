import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { InlineCommentThread } from './InlineCommentThread'

describe('InlineCommentThread', () => {
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
})

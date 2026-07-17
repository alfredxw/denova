import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ReviewThreadFile } from '../types'
import { ReviewFileNavigator } from './ReviewFileNavigator'

describe('ReviewFileNavigator', () => {
  it('provides a dropdown jump list when the review surface is compact', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <ReviewFileNavigator
        files={[reviewFile('chapters/one.md'), reviewFile('setting/progress.md')]}
        selectedPath="chapters/one.md"
        onSelect={onSelect}
        onCollapse={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: /项目文件.*2/ }))
    const progress = screen.getByRole('menuitemcheckbox', { name: /setting\/progress\.md/ })
    expect(progress).toBeVisible()

    await user.click(progress)
    expect(onSelect).toHaveBeenCalledWith('setting/progress.md')
  })
})

function reviewFile(path: string): ReviewThreadFile {
  return {
    path,
    before_content: 'before',
    after_content: 'after',
    base_revision: 'before-revision',
    revision: 'after-revision',
    base_group_id: 'group-1',
    base_change_set_id: 'set-1',
    latest_group_id: 'group-1',
    latest_change_set_id: 'set-1',
    group_ids: ['group-1'],
    change_set_ids: ['set-1'],
    pending_edit_ids: [],
    review_status: 'pending',
    apply_state: 'applied',
    continuity: 'continuous',
  }
}

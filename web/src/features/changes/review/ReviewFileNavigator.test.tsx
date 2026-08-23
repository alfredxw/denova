import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import type { ReviewThreadFile } from '../types'
import { ReviewFileNavigator } from './ReviewFileNavigator'

describe('ReviewFileNavigator', () => {
  it('presents Review changes and statuses through the shared Pierre tree', async () => {
    const view = render(
      <ReviewFileNavigator
        files={[
          reviewFile('chapters/added.md', { before_exists: false, after_exists: true }),
          reviewFile('chapters/modified.md'),
          reviewFile('setting/deleted.md', { before_exists: true, after_exists: false }),
        ]}
        selectedPath="chapters/modified.md"
        onSelect={vi.fn()}
      />,
    )

    await waitFor(() => expect(fileTreeRow('chapters/added.md', '文件导航')).toHaveAttribute('data-item-git-status', 'added'))
    expect(fileTreeRow('chapters/modified.md', '文件导航')).toHaveAttribute('data-item-git-status', 'modified')
    expect(fileTreeRow('setting/deleted.md', '文件导航')).toHaveAttribute('data-item-git-status', 'deleted')
    expect(fileTreeRow('chapters/modified.md', '文件导航')).toHaveAttribute('aria-selected', 'true')
    expect(fileTreeShadow('文件导航').querySelector('[data-file-tree-search-input]')).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: '文件导航' })).not.toHaveTextContent('变更文件')

    view.rerender(
      <ReviewFileNavigator
        files={[
          reviewFile('chapters/added.md', { before_exists: false, after_exists: true }),
          reviewFile('chapters/reloaded.md'),
        ]}
        selectedPath="chapters/reloaded.md"
        onSelect={vi.fn()}
      />,
    )
    await waitFor(() => expect(fileTreeRow('chapters/reloaded.md', '文件导航')).toHaveAttribute('aria-selected', 'true'))
  })

  it('bridges Pierre selection back to the continuous diff', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <ReviewFileNavigator
        files={[reviewFile('chapters/one.md'), reviewFile('setting/progress.md')]}
        selectedPath="chapters/one.md"
        onSelect={onSelect}
      />,
    )

    await user.click(fileTreeRow('setting/progress.md', '文件导航'))
    expect(onSelect).toHaveBeenCalledWith('setting/progress.md')
  })
})

function reviewFile(path: string, overrides: Partial<ReviewThreadFile> = {}): ReviewThreadFile {
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
    ...overrides,
  }
}

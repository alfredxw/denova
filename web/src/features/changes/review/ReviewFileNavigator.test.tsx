import { render, screen, within } from '@testing-library/react'
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

    await user.click(screen.getByRole('button', { name: /变更文件.*2/ }))
    const progress = screen.getByRole('menuitemcheckbox', { name: /setting\/progress\.md/ })
    expect(progress).toBeVisible()

    await user.click(progress)
    expect(onSelect).toHaveBeenCalledWith('setting/progress.md')
  })

  it('groups only changed files, toggles directories from the whole row, and marks change kinds', async () => {
    const user = userEvent.setup()
    render(
      <ReviewFileNavigator
        files={[
          reviewFile('chapters/added.md', { before_exists: false, after_exists: true }),
          reviewFile('chapters/modified.md'),
          reviewFile('setting/deleted.md', { before_exists: true, after_exists: false }),
        ]}
        selectedPath="chapters/modified.md"
        onSelect={vi.fn()}
        onCollapse={vi.fn()}
      />,
    )

    expect(screen.getAllByLabelText('新增文件').length).toBeGreaterThan(0)
    expect(screen.getAllByLabelText('修改文件').length).toBeGreaterThan(0)
    expect(screen.getAllByLabelText('删除文件').length).toBeGreaterThan(0)

    const tree = screen.getByRole('tree', { name: '文件导航' })
    const chapters = within(tree).getByRole('treeitem', { name: 'chapters' })
    expect(chapters).toHaveAttribute('aria-expanded', 'true')
    await user.click(chapters)
    expect(chapters).toHaveAttribute('aria-expanded', 'false')
    expect(within(tree).queryByRole('treeitem', { name: /chapters\/added\.md/ })).not.toBeInTheDocument()

    await user.type(screen.getByRole('textbox', { name: '筛选文件…' }), 'deleted')
    const filteredTree = screen.getByRole('tree', { name: '文件导航' })
    expect(within(filteredTree).getByRole('treeitem', { name: /setting\/deleted\.md/ })).toBeInTheDocument()
    expect(within(filteredTree).queryByRole('treeitem', { name: /chapters\/modified\.md/ })).not.toBeInTheDocument()
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

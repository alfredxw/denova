import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ReviewThreadFile } from '../types'
import { ReviewFileNavigator } from './ReviewFileNavigator'

const treeState = vi.hoisted(() => ({
  instance: null as null | {
    getSelectedPaths: () => string[]
    selectFromTree: (path: string) => void
  },
  options: null as TreeOptions | null,
}))

interface TreeOptions {
  paths: string[]
  gitStatus: Array<{ path: string; status: string }>
  flattenEmptyDirectories: boolean
  initialExpansion: string
  fileTreeSearchMode: string
  search: boolean
  onSelectionChange: (paths: readonly string[]) => void
}

vi.mock('@pierre/trees', () => ({
  FileTree: class MockFileTree {
    selected = new Set<string>()
    scrollToPath = vi.fn()
    render = vi.fn()
    cleanUp = vi.fn()
    options: TreeOptions

    constructor(options: TreeOptions) {
      this.options = options
      treeState.instance = this
      treeState.options = options
    }

    getSelectedPaths() {
      return [...this.selected]
    }

    getItem(path: string) {
      if (!this.options.paths.includes(path)) return null
      return {
        deselect: () => this.selected.delete(path),
        isSelected: () => this.selected.has(path),
        select: () => this.selected.add(path),
      }
    }

    selectFromTree(path: string) {
      this.selected = new Set([path])
      this.options.onSelectionChange([path])
    }
  },
}))

describe('ReviewFileNavigator', () => {
  it('feeds changed paths and statuses directly into the Pierre tree', () => {
    render(
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

    expect(treeState.options).toMatchObject({
      paths: ['chapters/added.md', 'chapters/modified.md', 'setting/deleted.md'],
      gitStatus: [
        { path: 'chapters/added.md', status: 'added' },
        { path: 'chapters/modified.md', status: 'modified' },
        { path: 'setting/deleted.md', status: 'deleted' },
      ],
      flattenEmptyDirectories: true,
      initialExpansion: 'open',
      fileTreeSearchMode: 'hide-non-matches',
      search: true,
    })
    expect(treeState.instance?.getSelectedPaths()).toEqual(['chapters/modified.md'])
    expect(screen.getByRole('complementary', { name: '文件导航' })).not.toHaveTextContent('变更文件')
  })

  it('bridges tree selection back to the continuous diff', () => {
    const onSelect = vi.fn()
    render(
      <ReviewFileNavigator
        files={[reviewFile('chapters/one.md'), reviewFile('setting/progress.md')]}
        selectedPath="chapters/one.md"
        onSelect={onSelect}
      />,
    )

    act(() => treeState.instance?.selectFromTree('setting/progress.md'))
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

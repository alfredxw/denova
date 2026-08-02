import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ProjectFileExplorerNode } from './project-file-explorer-model'
import { ProjectFilesSidebar } from './ProjectFilesSidebar'

const selectedPath = 'chapters/volume/chapter.md'

function explorerNodes(): ProjectFileExplorerNode[] {
  return [{
    id: 'chapters',
    path: 'chapters',
    name: 'chapters',
    type: 'dir',
    ignored: false,
    symlink: false,
    loaded: true,
    loading: false,
    children: [{
      id: 'chapters/volume',
      path: 'chapters/volume',
      name: 'volume',
      type: 'dir',
      ignored: false,
      symlink: false,
      loaded: true,
      loading: false,
      children: [{
        id: selectedPath,
        path: selectedPath,
        name: 'chapter.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      }],
    }],
  }]
}

describe('ProjectFilesSidebar', () => {
  it('reveals the selected file again after collapsing every directory', async () => {
    render(
      <ProjectFilesSidebar
        nodes={explorerNodes()}
        selectedPath={selectedPath}
        expandedPaths={['chapters', 'chapters/volume']}
        loading={false}
        loadingPaths={new Set()}
        error={null}
        showIgnored={false}
        onShowIgnoredChange={vi.fn()}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onCollapseAll={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={vi.fn()}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '折叠全部文件夹' }))
    expect(screen.queryByRole('treeitem', { name: /chapter\.md/ })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '定位当前文件' }))
    await waitFor(() => expect(screen.getByRole('treeitem', { name: /chapter\.md/ })).toHaveAttribute('aria-selected', 'true'))
  })
})

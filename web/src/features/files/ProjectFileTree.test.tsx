import type { TreeApi } from 'react-arborist'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ProjectFileExplorerNode } from './project-file-explorer-model'
import { ProjectFileTree } from './ProjectFileTree'

describe('ProjectFileTree', () => {
  it('keeps a large directory DOM bounded through virtualization', () => {
    const nodes: ProjectFileExplorerNode[] = Array.from({ length: 2_000 }, (_, index) => ({
      id: `file-${index}.ts`,
      path: `file-${index}.ts`,
      name: `file-${index}.ts`,
      type: 'file',
      ignored: false,
      symlink: false,
      loaded: false,
      loading: false,
    }))
    render(
      <ProjectFileTree
        treeRef={createRef<TreeApi<ProjectFileExplorerNode>>()}
        nodes={nodes}
        selectedPath={null}
        expandedPaths={[]}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={vi.fn()}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
      />,
    )

    const renderedRows = screen.getAllByRole('treeitem')
    expect(renderedRows.length).toBeGreaterThan(0)
    expect(renderedRows.length).toBeLessThan(80)
  })
})

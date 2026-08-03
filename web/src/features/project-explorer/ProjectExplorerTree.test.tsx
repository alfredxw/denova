import type { TreeApi } from 'react-arborist'
import { createRef } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ProjectFileExplorerNode } from './model'
import { ProjectExplorerTree } from './ProjectExplorerTree'

describe('ProjectExplorerTree', () => {
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
      <ProjectExplorerTree
        treeRef={createRef<TreeApi<ProjectFileExplorerNode>>()}
        nodes={nodes}
        workspace="/projects/one"
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

  it('uses Explorer-style copy and paste instead of a destination dialog', async () => {
    const user = userEvent.setup()
    const onCopyItem = vi.fn().mockResolvedValue(undefined)
    const onMoveItem = vi.fn().mockResolvedValue(undefined)
    const nodes: ProjectFileExplorerNode[] = [
      {
        id: 'docs',
        path: 'docs',
        name: 'docs',
        type: 'dir',
        ignored: false,
        symlink: false,
        loaded: true,
        loading: false,
        children: [],
      },
      {
        id: 'notes.md',
        path: 'notes.md',
        name: 'notes.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
    ]
    render(
      <ProjectExplorerTree
        treeRef={createRef<TreeApi<ProjectFileExplorerNode>>()}
        nodes={nodes}
        workspace="/projects/one"
        selectedPath={null}
        expandedPaths={['docs']}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={vi.fn()}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={onCopyItem}
        onMoveItem={onMoveItem}
      />,
    )

    fireEvent.contextMenu(screen.getByText('notes.md'))
    expect(screen.queryByRole('menuitem', { name: /^移动$/ })).not.toBeInTheDocument()
    await user.click(await screen.findByRole('menuitem', { name: /^复制$/ }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    fireEvent.contextMenu(screen.getByText('docs'))
    await user.click(await screen.findByRole('menuitem', { name: /^粘贴$/ }))

    await waitFor(() => expect(onCopyItem).toHaveBeenCalledWith('notes.md', 'docs/notes.md'))

    fireEvent.contextMenu(screen.getByText('notes.md'))
    await user.click(await screen.findByRole('menuitem', { name: /^剪切$/ }))
    fireEvent.contextMenu(screen.getByText('docs'))
    await user.click(await screen.findByRole('menuitem', { name: /^粘贴$/ }))

    await waitFor(() => expect(onMoveItem).toHaveBeenCalledWith('notes.md', 'docs/notes.md'))
  })

  it('uses F2 for inline rename and Enter to open the focused file', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const onRenameItem = vi.fn().mockResolvedValue(undefined)
    const nodes: ProjectFileExplorerNode[] = [{
      id: 'chapter.md',
      path: 'chapter.md',
      name: 'chapter.md',
      type: 'file',
      ignored: false,
      symlink: false,
      loaded: false,
      loading: false,
    }]
    render(
      <ProjectExplorerTree
        treeRef={createRef<TreeApi<ProjectFileExplorerNode>>()}
        nodes={nodes}
        workspace="/projects/one"
        selectedPath={null}
        expandedPaths={[]}
        onSelectFile={onSelectFile}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={vi.fn()}
        onDeleteItem={vi.fn()}
        onRenameItem={onRenameItem}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
      />,
    )

    await user.click(screen.getByText('chapter.md'))
    fireEvent.keyDown(screen.getByRole('tree'), { key: 'Enter' })
    expect(onSelectFile).toHaveBeenLastCalledWith('chapter.md')
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

    fireEvent.keyDown(screen.getByRole('tree'), { key: 'F2' })
    const input = await screen.findByRole('textbox') as HTMLInputElement
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 7])
    await user.clear(input)
    await user.type(input, 'renamed.md{Enter}')

    await waitFor(() => expect(onRenameItem).toHaveBeenCalledWith('chapter.md', 'renamed.md'))
  })
})

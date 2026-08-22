import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ProjectFileExplorerNode } from './model'
import { ProjectExplorerPane } from './ProjectExplorerPane'

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

describe('ProjectExplorerPane', () => {
  it('reveals the selected file again after collapsing every directory', async () => {
    render(
      <ProjectExplorerPane
        nodes={explorerNodes()}
        workspace="/projects/one"
        selectedPath={selectedPath}
        expandedPaths={['chapters', 'chapters/volume']}
        loading={false}
        loadingPaths={new Set()}
        error={null}
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

  it('creates beside the focused file with an inline tree input and no project heading', async () => {
    const user = userEvent.setup()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    render(
      <ProjectExplorerPane
        nodes={explorerNodes()}
        workspace="/projects/one"
        selectedPath={selectedPath}
        expandedPaths={['chapters', 'chapters/volume']}
        loading={false}
        loadingPaths={new Set()}
        error={null}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onCollapseAll={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={onCreateItem}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    await waitFor(() => expect(screen.getByRole('treeitem', { name: /chapter\.md/ })).toHaveAttribute('aria-selected', 'true'))
    expect(screen.queryByText('项目文件')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '隐藏生成目录' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '新建文件' }))
    const input = await screen.findByRole('textbox')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.type(input, 'notes.md{Enter}')

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith('chapters/volume/notes.md', 'file'))
  })

  it('only separates the toolbar after the file tree leaves the top', async () => {
    render(
      <ProjectExplorerPane
        nodes={explorerNodes()}
        workspace="/projects/one"
        selectedPath={selectedPath}
        expandedPaths={['chapters', 'chapters/volume']}
        loading={false}
        loadingPaths={new Set()}
        error={null}
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

    const toolbar = document.querySelector('[data-slot="project-explorer-toolbar"]')
    const scrollViewport = screen.getByRole('tree', { name: '文件' }).firstElementChild
    if (!(toolbar instanceof HTMLElement) || !(scrollViewport instanceof HTMLElement)) {
      throw new Error('Project explorer chrome is not rendered')
    }
    expect(toolbar).toHaveClass('border-transparent')

    fireEvent.scroll(scrollViewport, { target: { scrollTop: 1 } })
    await waitFor(() => expect(toolbar).toHaveClass('border-[var(--nova-border)]'))

    fireEvent.scroll(scrollViewport, { target: { scrollTop: 0 } })
    await waitFor(() => expect(toolbar).toHaveClass('border-transparent'))
  })

  it('creates a root folder inline when the project is empty', async () => {
    const user = userEvent.setup()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    render(
      <ProjectExplorerPane
        nodes={[]}
        workspace="/projects/one"
        selectedPath={null}
        expandedPaths={[]}
        loading={false}
        loadingPaths={new Set()}
        error={null}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onCollapseAll={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={onCreateItem}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    expect(screen.getByText('项目目录为空。')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '新建目录' }))
    await user.type(await screen.findByRole('textbox'), 'drafts{Enter}')

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith('drafts', 'dir'))
  })

  it('uses the selected folder as the create target and resets to root after clicking empty space', async () => {
    const user = userEvent.setup()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    const nodes: ProjectFileExplorerNode[] = [{
      id: 'docs',
      path: 'docs',
      name: 'docs',
      type: 'dir',
      ignored: false,
      symlink: false,
      loaded: true,
      loading: false,
      children: [],
    }]
    render(
      <ProjectExplorerPane
        nodes={nodes}
        workspace="/projects/one"
        selectedPath={null}
        expandedPaths={[]}
        loading={false}
        loadingPaths={new Set()}
        error={null}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onDirectoryExpandedChange={vi.fn()}
        onCollapseAll={vi.fn()}
        onLoadMore={vi.fn()}
        onCreateItem={onCreateItem}
        onDeleteItem={vi.fn()}
        onRenameItem={vi.fn()}
        onCopyItem={vi.fn()}
        onMoveItem={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    await user.click(screen.getByText('docs'))
    await user.click(screen.getByRole('button', { name: '新建目录' }))
    await user.type(await screen.findByRole('textbox'), 'guides{Enter}')
    await waitFor(() => expect(onCreateItem).toHaveBeenLastCalledWith('docs/guides', 'dir'))

    fireEvent.click(screen.getByRole('tree'))
    await user.click(screen.getByRole('button', { name: '新建文件' }))
    await user.type(await screen.findByRole('textbox'), 'nested/deep/story.md{Enter}')
    await waitFor(() => expect(onCreateItem).toHaveBeenLastCalledWith('nested/deep/story.md', 'file'))
  })

  it('keeps writing-only metadata and actions behind explorer extension points', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    render(
      <ProjectExplorerPane
        nodes={explorerNodes()}
        workspace="/projects/one"
        selectedPath={selectedPath}
        expandedPaths={['chapters', 'chapters/volume']}
        loading={false}
        loadingPaths={new Set()}
        error={null}
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
        extensions={{
          deleteRecovery: 'version-history',
          renderNodeMeta: (node) => node.path === selectedPath ? <span>1.2k · draft</span> : null,
          getNodeActions: ({ node, paths }) => node.type === 'file' && paths.length === 1 ? [{
            id: 'reference',
            label: '引用到对话',
            onSelect: () => onReferenceFile(node.path),
          }] : [],
        }}
      />,
    )

    expect(await screen.findByText('1.2k · draft')).toBeInTheDocument()
    fireEvent.contextMenu(screen.getByLabelText('chapter.md'))
    await user.click(await screen.findByRole('menuitem', { name: '引用到对话' }))
    expect(onReferenceFile).toHaveBeenCalledWith(selectedPath)
  })
})

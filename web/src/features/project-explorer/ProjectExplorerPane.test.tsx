import { FileTree } from '@pierre/trees'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import type { ProjectFileExplorerNode } from './model'
import { ProjectExplorerPane } from './ProjectExplorerPane'

const selectedPath = 'chapters/volume/chapter.md'

describe('ProjectExplorerPane', () => {
  afterEach(() => vi.restoreAllMocks())

  it('does not move the viewport while synchronizing the current file selection', async () => {
    const scrollToPath = vi.spyOn(FileTree.prototype, 'scrollToPath')

    renderPane()

    await waitFor(() => expect(fileTreeRow(selectedPath)).toHaveAttribute('aria-selected', 'true'))
    expect(scrollToPath).not.toHaveBeenCalled()
  })

  it('collapses all folders and reveals the selected file again', async () => {
    renderPane()

    fireEvent.click(screen.getByRole('button', { name: '折叠全部文件夹' }))
    await waitFor(() => expect(fileTreeRow('chapters/volume/')).toHaveAttribute('aria-expanded', 'false'))

    fireEvent.click(screen.getByRole('button', { name: '定位当前文件' }))
    await waitFor(() => expect(fileTreeRow(selectedPath)).toHaveAttribute('aria-selected', 'true'))
  })

  it('creates beside the focused file with Pierre inline rename', async () => {
    const user = userEvent.setup()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    renderPane({ onCreateItem })

    await user.click(fileTreeRow(selectedPath))
    await user.click(screen.getByRole('button', { name: '新建文件' }))
    const input = await shadowInput()
    fireEvent.input(input, { target: { value: 'notes.md' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith('chapters/volume/notes.md', 'file'))
    expect(screen.queryByText('项目文件')).not.toBeInTheDocument()
  })

  it('separates the toolbar only after the Pierre viewport scrolls', async () => {
    renderPane()
    const toolbar = document.querySelector('[data-slot="project-explorer-toolbar"]')
    const scroll = fileTreeShadow().querySelector('[data-file-tree-virtualized-scroll]')
    if (!(toolbar instanceof HTMLElement) || !(scroll instanceof HTMLElement)) throw new Error('Explorer chrome is unavailable')
    expect(toolbar).toHaveClass('border-transparent')

    Object.defineProperty(scroll, 'scrollTop', { configurable: true, get: () => 1, set: () => {} })
    fireEvent.scroll(scroll)
    await waitFor(() => expect(toolbar).toHaveClass('border-[var(--nova-border)]'))
  })

  it('keeps writing metadata and actions behind extension points', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    renderPane({
      extensions: {
        getRowDecoration: (node) => node.path === selectedPath ? { text: '1.2k · draft' } : null,
        getNodeActions: ({ node, paths }) => node.type === 'file' && paths.length === 1 ? [{
          id: 'reference',
          label: '引用到对话',
          onSelect: () => onReferenceFile(node.path),
        }] : [],
      },
    })

    expect(fileTreeRow(selectedPath)).toHaveTextContent('1.2k · draft')
    fireEvent.contextMenu(fileTreeRow(selectedPath), { clientX: 10, clientY: 10 })
    await user.click(await screen.findByRole('menuitem', { name: '引用到对话' }))
    expect(onReferenceFile).toHaveBeenCalledWith(selectedPath)
  })
})

function renderPane(overrides: Partial<React.ComponentProps<typeof ProjectExplorerPane>> = {}) {
  return render(
    <ProjectExplorerPane
      projectId="project-one"
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
      onCreateItem={vi.fn().mockResolvedValue(undefined)}
      onDeleteItem={vi.fn().mockResolvedValue(undefined)}
      onRenameItem={vi.fn().mockResolvedValue(undefined)}
      onCopyItem={vi.fn().mockResolvedValue(undefined)}
      onMoveItem={vi.fn().mockResolvedValue(undefined)}
      onRefresh={vi.fn()}
      {...overrides}
    />,
  )
}

async function shadowInput() {
  let input: HTMLInputElement | null = null
  await waitFor(() => {
    input = fileTreeShadow().querySelector<HTMLInputElement>('[data-item-rename-input]')
    expect(input).not.toBeNull()
  })
  return input!
}

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

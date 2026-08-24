import { FileTree } from '@pierre/trees'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import type { ProjectFileExplorerNode } from './model'
import { ProjectExplorerPane } from './ProjectExplorerPane'

const volumePath = 'chapters/v00001-第一卷-风起'
const selectedPath = `${volumePath}/ch00011-第十章-交锋.md`

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
    await waitFor(() => expect(fileTreeRow('chapters/')).toHaveAttribute('aria-expanded', 'false'))

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

    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith(`${volumePath}/notes.md`, 'file'))
    expect(screen.queryByText('项目文件')).not.toBeInTheDocument()
  })

  it('toggles writing filename affixes without changing rename values or ordinary files', async () => {
    const user = userEvent.setup()
    renderPane()

    const showAffixes = screen.getByRole('button', { name: '显示章节/卷排序前缀和 .md 后缀' })
    const collapseAll = screen.getByRole('button', { name: '折叠全部文件夹' })
    expect(showAffixes.nextElementSibling).toBe(collapseAll)
    await waitFor(() => {
      expect(fileTreeRow(selectedPath)).toHaveAttribute('aria-label', '第十章-交锋')
      expect(fileTreeRow(selectedPath).querySelector('[data-nova-file-tree-display-name-value]')).toHaveTextContent('第十章-交锋')
      expect(fileTreeRow('chapters/notes.md')).toHaveAttribute('aria-label', 'notes.md')
      expect(fileTreeRow('chapters/notes.md').querySelector('[data-nova-file-tree-display-name-value]')).not.toBeInTheDocument()
      expect(fileTreeRow(`${volumePath}/`)).toHaveAttribute('aria-label', expect.stringContaining('第一卷-风起'))
    })

    fireEvent.contextMenu(fileTreeRow(selectedPath), { clientX: 10, clientY: 10 })
    await user.click(await screen.findByRole('menuitem', { name: '重命名' }))
    const input = await shadowInput()
    expect(input).toHaveValue('ch00011-第十章-交锋.md')
    fireEvent.keyDown(input, { key: 'Escape' })

    await user.click(showAffixes)
    await waitFor(() => {
      expect(fileTreeRow(selectedPath)).toHaveAttribute('aria-label', 'ch00011-第十章-交锋.md')
      expect(fileTreeRow(selectedPath).querySelector('[data-nova-file-tree-display-name-value]')).not.toBeInTheDocument()
      expect(fileTreeRow(`${volumePath}/`)).toHaveAttribute('aria-label', expect.stringContaining('v00001-第一卷-风起'))
    })
    expect(screen.getByRole('button', { name: '隐藏章节/卷排序前缀和 .md 后缀' })).toBeInTheDocument()
  })

  it('shows toolbar tooltips below the icon buttons', async () => {
    renderPane()

    fireEvent.focus(screen.getByRole('button', { name: '新建文件' }))

    await screen.findByRole('tooltip')
    const tooltip = document.querySelector('[data-slot="tooltip-content"]')
    expect(tooltip).toHaveTextContent('新建文件')
    expect(tooltip).toHaveAttribute('data-side', 'bottom')
  })

  it('keeps the toolbar borderless while the Pierre viewport scrolls', () => {
    renderPane()
    const toolbar = document.querySelector('[data-slot="project-explorer-toolbar"]')
    const scroll = fileTreeShadow().querySelector('[data-file-tree-virtualized-scroll]')
    if (!(toolbar instanceof HTMLElement) || !(scroll instanceof HTMLElement)) throw new Error('Explorer chrome is unavailable')
    expect(toolbar).not.toHaveClass('border-b')

    Object.defineProperty(scroll, 'scrollTop', { configurable: true, get: () => 1, set: () => {} })
    fireEvent.scroll(scroll)
    expect(toolbar).not.toHaveClass('border-b')
  })

  it('keeps writing actions behind extension points', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    renderPane({
      extensions: {
        getNodeActions: ({ node, paths }) => node.type === 'file' && paths.length === 1 ? [{
          id: 'reference',
          label: '引用到对话',
          onSelect: () => onReferenceFile(node.path),
        }] : [],
      },
    })

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
      expandedPaths={['chapters', volumePath]}
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
      id: volumePath,
      path: volumePath,
      name: 'v00001-第一卷-风起',
      type: 'dir',
      ignored: false,
      symlink: false,
      loaded: true,
      loading: false,
      children: [{
        id: selectedPath,
        path: selectedPath,
        name: 'ch00011-第十章-交锋.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      }],
    }, {
      id: 'chapters/notes.md',
      path: 'chapters/notes.md',
      name: 'notes.md',
      type: 'file',
      ignored: false,
      symlink: false,
      loaded: false,
      loading: false,
    }],
  }]
}

import { createRef } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { fileTreeRow, fileTreeShadow } from '@/test/file-tree'
import type { ProjectFileExplorerNode } from './model'
import { ProjectExplorerTree, type ProjectExplorerTreeHandle } from './ProjectExplorerTree'

describe('ProjectExplorerTree', () => {
  it('virtualizes large projects and presents version status through Pierre', async () => {
    const nodes: ProjectFileExplorerNode[] = Array.from({ length: 2_000 }, (_, index) => file(`file-${index}.ts`))
    renderTree({ nodes, gitStatus: [{ path: 'file-0.ts', status: 'modified' }] })

    const rows = fileTreeShadow().querySelectorAll('[data-item-path]')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(80)
    expect(fileTreeRow('file-0.ts')).toHaveAttribute('data-item-git-status', 'modified')
    expect(fileTreeRow('file-0.ts').querySelector('[data-item-section="action"]')).not.toBeInTheDocument()
    expect((fileTreeShadow().host as HTMLElement).style.getPropertyValue('--trees-item-height')).toBe('30px')
    expect(fileTreeShadow().querySelector('[data-file-tree-unsafe-css]')).toHaveTextContent(
      '[data-type="item"][data-item-focused="true"]:not(:focus-visible)::before',
    )
    expect(fileTreeShadow().querySelector('[data-file-tree-unsafe-css]')).toHaveTextContent(
      '[data-type="item"] > [data-item-section="git"]',
    )
    await waitFor(() => expect(fileTreeShadow().querySelector('[data-file-tree-search-input]')).toHaveAttribute('placeholder', '按名称筛选…'))
  })

  it('opens files and toggles a directory from the whole row', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const onDirectoryExpandedChange = vi.fn()
    renderTree({
      nodes: [directory('docs', [file('docs/guide.md')])],
      expandedPaths: [],
      onSelectFile,
      onDirectoryExpandedChange,
    })

    await user.click(fileTreeRow('docs/'))
    expect(onDirectoryExpandedChange).toHaveBeenCalledWith('docs', true)
    await user.click(fileTreeRow('docs/guide.md'))
    expect(onSelectFile).toHaveBeenCalledWith('docs/guide.md')
  })

  it('creates and renames with Pierre inline inputs', async () => {
    const user = userEvent.setup()
    const ref = createRef<ProjectExplorerTreeHandle>()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    const onRenameItem = vi.fn().mockResolvedValue(undefined)
    renderTree({ ref, nodes: [file('chapter.md')], onCreateItem, onRenameItem })

    act(() => ref.current?.beginCreate('file'))
    const createInput = await shadowInput()
    fireEvent.input(createInput, { target: { value: 'notes.md' } })
    fireEvent.keyDown(createInput, { key: 'Enter' })
    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith('notes.md', 'file'))

    fireEvent.contextMenu(fileTreeRow('chapter.md'), { clientX: 10, clientY: 10 })
    await user.click(await screen.findByRole('menuitem', { name: '重命名' }))
    const renameInput = await shadowInput()
    fireEvent.input(renameInput, { target: { value: 'renamed.md' } })
    fireEvent.keyDown(renameInput, { key: 'Enter' })
    await waitFor(() => expect(onRenameItem).toHaveBeenCalledWith('chapter.md', 'renamed.md'))
  })

  it('composes Explorer copy and paste actions inside Pierre', async () => {
    const user = userEvent.setup()
    const onCopyItem = vi.fn().mockResolvedValue(undefined)
    renderTree({
      nodes: [directory('docs'), file('notes.md')],
      onCopyItem,
    })

    fireEvent.contextMenu(fileTreeRow('notes.md'), { clientX: 10, clientY: 10 })
    await user.click(await screen.findByRole('menuitem', { name: /^复制$/ }))
    fireEvent.contextMenu(fileTreeRow('docs/'), { clientX: 10, clientY: 10 })
    await user.click(await screen.findByRole('menuitem', { name: /^粘贴$/ }))

    await waitFor(() => expect(onCopyItem).toHaveBeenCalledWith('notes.md', 'docs/notes.md'))
  })

  it('offers the expanded native-style file menu without product-specific preview actions', async () => {
    renderTree({ nodes: [file('chapter.md')] })

    fireEvent.contextMenu(fileTreeRow('chapter.md'), { clientX: 10, clientY: 10 })

    for (const label of ['新建文件', '新建目录', '复制', '复制路径', '复制相对路径', '创建副本', '在文件管理器中显示', '重命名', '删除']) {
      expect(await screen.findByRole('menuitem', { name: label })).toBeInTheDocument()
    }
    expect(screen.queryByText(/Orca Browser/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Markdown Preview/i)).not.toBeInTheDocument()
  })

  it('creates beside a file and runs duplicate and reveal actions', async () => {
    const user = userEvent.setup()
    const onCreateItem = vi.fn().mockResolvedValue(undefined)
    const onCopyItem = vi.fn().mockResolvedValue(undefined)
    const onRevealItem = vi.fn().mockResolvedValue(undefined)
    renderTree({
      nodes: [directory('docs', [file('docs/chapter.md')])],
      expandedPaths: ['docs'],
      onCreateItem,
      onCopyItem,
      onRevealItem,
    })

    const openMenu = () => fireEvent.contextMenu(fileTreeRow('docs/chapter.md'), { clientX: 10, clientY: 10 })
    openMenu()
    await user.click(await screen.findByRole('menuitem', { name: '新建文件' }))
    const createInput = await shadowInput()
    fireEvent.input(createInput, { target: { value: 'notes.md' } })
    fireEvent.keyDown(createInput, { key: 'Enter' })
    await waitFor(() => expect(onCreateItem).toHaveBeenCalledWith('docs/notes.md', 'file'))

    openMenu()
    await user.click(await screen.findByRole('menuitem', { name: '创建副本' }))
    await waitFor(() => expect(onCopyItem).toHaveBeenCalledWith('docs/chapter.md', 'docs/chapter copy.md'))

    openMenu()
    await user.click(await screen.findByRole('menuitem', { name: '在文件管理器中显示' }))
    await waitFor(() => expect(onRevealItem).toHaveBeenCalledWith('docs/chapter.md'))
  })
})

function renderTree(overrides: Partial<React.ComponentProps<typeof ProjectExplorerTree>> & { ref?: React.Ref<ProjectExplorerTreeHandle> } = {}) {
  const { ref, ...props } = overrides
  return render(
    <ProjectExplorerTree
      ref={ref}
      nodes={[]}
      workspace="/projects/one"
      selectedPath={null}
      expandedPaths={[]}
      onSelectFile={vi.fn()}
      onDirectoryExpand={vi.fn()}
      onDirectoryExpandedChange={vi.fn()}
      onCreateItem={vi.fn().mockResolvedValue(undefined)}
      onDeleteItem={vi.fn().mockResolvedValue(undefined)}
      onRenameItem={vi.fn().mockResolvedValue(undefined)}
      onCopyItem={vi.fn().mockResolvedValue(undefined)}
      onMoveItem={vi.fn().mockResolvedValue(undefined)}
      onRevealItem={vi.fn().mockResolvedValue(undefined)}
      {...props}
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

function file(path: string): ProjectFileExplorerNode {
  return {
    id: path,
    path,
    name: path.slice(path.lastIndexOf('/') + 1),
    type: 'file',
    ignored: false,
    symlink: false,
    loaded: false,
    loading: false,
  }
}

function directory(path: string, children: ProjectFileExplorerNode[] = []): ProjectFileExplorerNode {
  return {
    id: path,
    path,
    name: path.slice(path.lastIndexOf('/') + 1),
    type: 'dir',
    ignored: false,
    symlink: false,
    loaded: true,
    loading: false,
    children,
  }
}

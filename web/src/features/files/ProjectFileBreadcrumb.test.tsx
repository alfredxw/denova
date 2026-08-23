import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { fileTreeRow } from '@/test/file-tree'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import { ProjectFileBreadcrumb } from './ProjectFileBreadcrumb'

describe('ProjectFileBreadcrumb', () => {
  it('opens a Pierre browser for a segment and switches sibling files', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    renderBreadcrumb({ onSelectFile })

    expect(screen.getByRole('navigation', { name: '文件路径' })).toHaveTextContent('src')
    const current = screen.getByRole('button', { name: '浏览 main.ts' })
    expect(current).toHaveAttribute('aria-current', 'page')
    await user.click(current)
    await user.click(fileTreeRow('src/utils.ts', '文件浏览器'))

    expect(onSelectFile).toHaveBeenCalledWith('src/utils.ts')
    expect(document.querySelector('file-tree-container')).not.toBeInTheDocument()
  })

  it('uses the same compact tree for the full project and exposes Git status', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    renderBreadcrumb({ onSelectFile, gitStatus: [{ path: 'docs/guide.md', status: 'modified' }] })

    await user.click(screen.getByRole('button', { name: '浏览 one' }))
    expect(fileTreeRow('docs/guide.md', '文件浏览器')).toHaveAttribute('data-item-git-status', 'modified')
    await user.click(fileTreeRow('docs/guide.md', '文件浏览器'))
    expect(onSelectFile).toHaveBeenCalledWith('docs/guide.md')
  })

  it('lists project-root siblings from a root file segment', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    renderBreadcrumb({
      nodes: [...nodes, file('README.md'), file('CHANGELOG.md')],
      selectedPath: 'README.md',
      onSelectFile,
    })

    await user.click(screen.getByRole('button', { name: '浏览 README.md' }))
    await user.click(fileTreeRow('CHANGELOG.md', '文件浏览器'))
    expect(onSelectFile).toHaveBeenCalledWith('CHANGELOG.md')
  })

  it('shows a localized empty state when the project has no files', async () => {
    const user = userEvent.setup()
    renderBreadcrumb({ nodes: [], selectedPath: null })

    await user.click(screen.getByRole('button', { name: '浏览 one' }))
    expect(screen.getByText('此目录为空。')).toBeInTheDocument()
  })
})

function renderBreadcrumb(overrides: Partial<React.ComponentProps<typeof ProjectFileBreadcrumb>> = {}) {
  return render(
    <ProjectFileBreadcrumb
      workspace="/projects/one"
      nodes={nodes}
      selectedPath="src/main.ts"
      loading={false}
      onSelectFile={vi.fn()}
      onDirectoryExpand={vi.fn()}
      onLoadMore={vi.fn()}
      {...overrides}
    />,
  )
}

const nodes: ProjectFileExplorerNode[] = [
  directory('docs', [file('docs/guide.md')]),
  directory('src', [file('src/main.ts'), file('src/utils.ts')]),
]

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

function directory(path: string, children: ProjectFileExplorerNode[]): ProjectFileExplorerNode {
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

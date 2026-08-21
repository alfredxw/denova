import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ProjectFileExplorerNode } from '@/features/project-explorer/model'
import { ProjectFileBreadcrumb } from './ProjectFileBreadcrumb'

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
    children: [{
      id: 'docs/guide.md',
      path: 'docs/guide.md',
      name: 'guide.md',
      type: 'file',
      ignored: false,
      symlink: false,
      loaded: false,
      loading: false,
    }],
  },
  {
    id: 'src',
    path: 'src',
    name: 'src',
    type: 'dir',
    ignored: false,
    symlink: false,
    loaded: true,
    loading: false,
    children: [
      {
        id: 'src/main.ts',
        path: 'src/main.ts',
        name: 'main.ts',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
      {
        id: 'src/utils.ts',
        path: 'src/utils.ts',
        name: 'utils.ts',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
    ],
  },
]

const deepNodes: ProjectFileExplorerNode[] = [{
  id: 'chapters',
  path: 'chapters',
  name: 'chapters',
  type: 'dir',
  ignored: false,
  symlink: false,
  loaded: true,
  loading: false,
  children: [{
    id: 'chapters/volume-one',
    path: 'chapters/volume-one',
    name: 'volume-one',
    type: 'dir',
    ignored: false,
    symlink: false,
    loaded: true,
    loading: false,
    children: [
      {
        id: 'chapters/volume-one/chapter-one.md',
        path: 'chapters/volume-one/chapter-one.md',
        name: 'chapter-one.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
      {
        id: 'chapters/volume-one/chapter-two.md',
        path: 'chapters/volume-one/chapter-two.md',
        name: 'chapter-two.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
    ],
  }],
}]

describe('ProjectFileBreadcrumb', () => {
  it('opens the selected file siblings and switches files', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    render(
      <ProjectFileBreadcrumb
        workspace="/projects/one"
        nodes={nodes}
        selectedPath="src/main.ts"
        loading={false}
        onSelectFile={onSelectFile}
        onDirectoryExpand={vi.fn()}
        onLoadMore={vi.fn()}
      />,
    )

    expect(screen.getByRole('navigation', { name: '文件路径' })).toHaveTextContent('one')
    expect(screen.getByRole('navigation', { name: '文件路径' })).toHaveClass('nova-file-breadcrumb', 'overflow-x-auto')
    expect(screen.getByRole('navigation', { name: '文件路径' })).toHaveAttribute('data-slot', 'breadcrumb')
    expect(screen.getByRole('navigation', { name: '文件路径' })).toHaveTextContent('src')
    const workspaceTrigger = screen.getByRole('button', { name: '浏览 one' })
    const directoryTrigger = screen.getByRole('button', { name: '浏览 src' })
    const currentTrigger = screen.getByRole('button', { name: '浏览 main.ts' })
    expect(workspaceTrigger.querySelector('svg')).not.toBeInTheDocument()
    expect(directoryTrigger.querySelector('svg')).not.toBeInTheDocument()
    expect(currentTrigger.querySelector('svg')).not.toBeInTheDocument()
    expect(currentTrigger).toHaveAttribute('aria-current', 'page')
    expect(currentTrigger).not.toHaveClass('max-w-48')
    expect(currentTrigger.querySelector('[data-slot="breadcrumb-page"]')).toHaveClass('whitespace-nowrap')
    expect(currentTrigger.closest('[data-slot="breadcrumb-item"]')?.nextElementSibling).toBeNull()

    await user.click(currentTrigger)
    expect(screen.getByRole('tree', { name: '文件浏览器' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'utils.ts' }))

    expect(onSelectFile).toHaveBeenCalledWith('src/utils.ts')
    expect(screen.queryByRole('tree', { name: '文件浏览器' })).not.toBeInTheDocument()
  })

  it('expands a folder from the full project menu by clicking its row', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    render(
      <ProjectFileBreadcrumb
        workspace="/projects/one"
        nodes={nodes}
        selectedPath="src/main.ts"
        loading={false}
        onSelectFile={onSelectFile}
        onDirectoryExpand={vi.fn()}
        onLoadMore={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '浏览 one' }))
    await user.click(screen.getByRole('button', { name: '展开 docs' }))
    await user.click(screen.getByRole('button', { name: 'guide.md' }))

    expect(onSelectFile).toHaveBeenCalledWith('docs/guide.md')
  })

  it('lists project-root siblings from a root file segment', async () => {
    const user = userEvent.setup()
    const onSelectFile = vi.fn()
    const rootFiles: ProjectFileExplorerNode[] = [
      ...nodes,
      {
        id: 'README.md',
        path: 'README.md',
        name: 'README.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
      {
        id: 'CHANGELOG.md',
        path: 'CHANGELOG.md',
        name: 'CHANGELOG.md',
        type: 'file',
        ignored: false,
        symlink: false,
        loaded: false,
        loading: false,
      },
    ]
    render(
      <ProjectFileBreadcrumb
        workspace="/projects/one"
        nodes={rootFiles}
        selectedPath="README.md"
        loading={false}
        onSelectFile={onSelectFile}
        onDirectoryExpand={vi.fn()}
        onLoadMore={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '浏览 README.md' }))
    await user.click(screen.getByRole('button', { name: 'CHANGELOG.md' }))

    expect(onSelectFile).toHaveBeenCalledWith('CHANGELOG.md')
  })

  it('opens only the current directory and scrolls to the deepest visible location', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')

    try {
      render(
        <ProjectFileBreadcrumb
          workspace="/projects/one"
          nodes={deepNodes}
          selectedPath="chapters/volume-one/chapter-two.md"
          loading={false}
          onSelectFile={vi.fn()}
          onDirectoryExpand={vi.fn()}
          onLoadMore={vi.fn()}
        />,
      )

      await user.click(screen.getByRole('button', { name: '浏览 one' }))

      expect(screen.getByRole('button', { name: '折叠 chapters' })).toHaveClass('sticky', 'top-0')
      expect(screen.getByRole('button', { name: '展开 volume-one' })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'chapter-two.md' })).not.toBeInTheDocument()
      const currentLocation = document.querySelector('[data-breadcrumb-current-location="true"]')
      expect(currentLocation).toHaveTextContent('volume-one')
      expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', inline: 'nearest' })
      expect(scrollIntoView.mock.instances.at(-1)).toBe(currentLocation)
    } finally {
      scrollIntoView.mockRestore()
    }
  })

  it('shows one child level and centers the selected file from its directory segment', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')

    try {
      render(
        <ProjectFileBreadcrumb
          workspace="/projects/one"
          nodes={deepNodes}
          selectedPath="chapters/volume-one/chapter-two.md"
          loading={false}
          onSelectFile={vi.fn()}
          onDirectoryExpand={vi.fn()}
          onLoadMore={vi.fn()}
        />,
      )

      await user.click(screen.getByRole('button', { name: '浏览 volume-one' }))

      expect(screen.getByRole('button', { name: '折叠 volume-one' })).toHaveClass('sticky', 'top-0')
      expect(screen.getByRole('button', { name: 'chapter-one.md' })).toBeInTheDocument()
      const currentLocation = document.querySelector('[data-breadcrumb-current-location="true"]')
      expect(currentLocation).toHaveTextContent('chapter-two.md')
      expect(currentLocation).toHaveAttribute('aria-selected', 'true')
      expect(scrollIntoView.mock.instances.at(-1)).toBe(currentLocation)
    } finally {
      scrollIntoView.mockRestore()
    }
  })

  it('keeps the current segment visible when the breadcrumb width changes', () => {
    let resize: ResizeObserverCallback | undefined
    const observe = vi.fn()
    const disconnect = vi.fn()
    const NativeResizeObserver = globalThis.ResizeObserver
    globalThis.ResizeObserver = class ResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        resize = callback
      }
      observe = observe
      unobserve = vi.fn()
      disconnect = disconnect
    }

    try {
      render(
        <ProjectFileBreadcrumb
          workspace="/projects/one"
          nodes={nodes}
          selectedPath="src/main.ts"
          loading={false}
          onSelectFile={vi.fn()}
          onDirectoryExpand={vi.fn()}
          onLoadMore={vi.fn()}
        />,
      )
      const breadcrumb = screen.getByRole('navigation', { name: '文件路径' })
      Object.defineProperty(breadcrumb, 'scrollWidth', { configurable: true, value: 420 })
      resize?.([], {} as ResizeObserver)

      expect(observe).toHaveBeenCalledWith(breadcrumb)
      expect(breadcrumb.scrollLeft).toBe(420)
    } finally {
      globalThis.ResizeObserver = NativeResizeObserver
    }
  })

  it('shows a localized empty state when the project has no files', async () => {
    const user = userEvent.setup()
    render(
      <ProjectFileBreadcrumb
        workspace="/projects/empty"
        nodes={[]}
        selectedPath={null}
        loading={false}
        onSelectFile={vi.fn()}
        onDirectoryExpand={vi.fn()}
        onLoadMore={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '浏览 empty' }))
    expect(screen.getByText('此目录为空。')).toBeInTheDocument()
  })
})

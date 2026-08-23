import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createVersion, getVersionDiff, getVersionRestorePlan, getVersions, getVersionStatus, restoreVersion } from '@/lib/api'
import type { VersionDiff, VersionEntry, VersionRestorePlan } from '@/lib/api'
import { VersionPanel } from './VersionPanel'

vi.mock('@/lib/api', () => ({
  createVersion: vi.fn(),
  getVersionDiff: vi.fn(),
  getVersionRestorePlan: vi.fn(),
  getVersions: vi.fn(),
  getVersionStatus: vi.fn(),
  restoreVersion: vi.fn(),
}))

vi.mock('@/features/diff/CodeDiffSurface', () => ({
  CodeDiffSurface: ({ files, navigation, renderHeaderAction, empty }: {
    files: Array<{ path: string }>
    navigation: { collapsedPaths: Set<string> }
    renderHeaderAction?: (file: { path: string }) => React.ReactNode
    empty: React.ReactNode
  }) => files.length ? (
    <div>
      <main>
        {files.map((file) => (
          <section key={file.path} data-review-file={file.path}>
            <span>{file.path}</span>
            {renderHeaderAction?.(file)}
            <div data-review-file-content={file.path} hidden={navigation.collapsedPaths.has(file.path)}>Diff</div>
          </section>
        ))}
      </main>
      <aside role="complementary" aria-label="文件导航" className="border-l">
        <div role="tree" aria-label="文件导航">
          {files.map((file) => <div key={file.path} role="treeitem" aria-label={file.path}>{file.path}</div>)}
        </div>
      </aside>
    </div>
  ) : empty,
}))

describe('VersionPanel', () => {
  beforeEach(() => {
    vi.mocked(createVersion).mockReset()
    vi.mocked(getVersionDiff).mockReset()
    vi.mocked(getVersionRestorePlan).mockReset()
    vi.mocked(getVersions).mockReset()
    vi.mocked(getVersionStatus).mockReset()
    vi.mocked(restoreVersion).mockReset()
    vi.mocked(getVersionStatus).mockResolvedValue({
      has_versions: true,
      clean: false,
      changes: [{ path: 'chapters/current.md', status: 'modified' }],
      latest: versionEntry('second', '第二版本', ['chapters/second.md']),
      auto: {
        timed_enabled: false,
        timed_interval_minutes: 10,
        retention: 100,
      },
    })
    vi.mocked(getVersions).mockResolvedValue([
      versionEntry('second', '第二版本', ['chapters/second.md']),
      versionEntry('first', '第一版本', ['chapters/first.md']),
    ])
    vi.mocked(getVersionDiff).mockImplementation(async (_projectId, id, path, comparison = 'workspace') => versionDiff(id, path, comparison))
  })

  it('ignores stale restore preview responses after another restore dialog opens', async () => {
    const user = userEvent.setup()
    const firstPreview = deferred<VersionRestorePlan>()
    const secondPreview = deferred<VersionRestorePlan>()
    vi.mocked(getVersionRestorePlan)
      .mockReturnValueOnce(firstPreview.promise)
      .mockReturnValueOnce(secondPreview.promise)

    renderVersionPanel()

    const restoreButton = await screen.findByRole('button', { name: '恢复版本' })
    await user.click(restoreButton)
    expect(await screen.findByText('正在计算恢复影响…')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '取消' }))
    await user.click(screen.getByRole('button', { name: /第一版本/ }))
    await user.click(await screen.findByRole('button', { name: '恢复版本' }))

    await act(async () => {
      secondPreview.resolve(restorePlan('first', 'chapters/first.md'))
      await secondPreview.promise
    })
    expect(await within(screen.getByRole('alertdialog')).findByText('chapters/first.md')).toBeInTheDocument()

    await act(async () => {
      firstPreview.resolve(restorePlan('second', 'chapters/second.md'))
      await firstPreview.promise
    })

    await waitFor(() => {
      expect(within(screen.getByRole('alertdialog')).queryByText('chapters/second.md')).not.toBeInTheDocument()
    })
    expect(within(screen.getByRole('alertdialog')).getByText('chapters/first.md')).toBeInTheDocument()
  })

  it('compares history with its parent and current changes with the workspace', async () => {
    const user = userEvent.setup()
    renderVersionPanel()

    expect((await screen.findAllByText('chapters/second.md')).length).toBeGreaterThan(0)
    expect(getVersionDiff).toHaveBeenCalledWith('project-version', 'second', undefined, 'parent')
    expect(getVersionDiff).not.toHaveBeenCalledWith('project-version', 'second', 'chapters/second.md', 'parent')

    await user.click(screen.getByRole('button', { name: /当前变更/ }))
    await waitFor(() => {
      expect(getVersionDiff).toHaveBeenCalledWith('project-version', 'second', undefined, 'workspace')
    })
  })

  it('shows every changed file continuously with a synchronized right navigator and one-click collapse', async () => {
    const user = userEvent.setup()
    renderVersionPanel()

    const navigator = await screen.findByRole('complementary', { name: '文件导航' })
    expect(within(navigator).getByRole('tree', { name: '文件导航' })).toBeInTheDocument()
    expect(await within(navigator).findByRole('treeitem', { name: /chapters\/second\.md/ })).toBeInTheDocument()
    expect(navigator).toHaveClass('border-l')
    expect(document.querySelectorAll('[data-review-file]')).toHaveLength(2)

    await user.click(screen.getByRole('button', { name: '折叠全部 Diff' }))
    expect(document.querySelectorAll('[data-review-file-content][hidden]')).toHaveLength(2)
  })

  it('shows a useful empty state before the first version is saved', async () => {
    vi.mocked(getVersions).mockResolvedValue([])
    vi.mocked(getVersionStatus).mockResolvedValue({
      has_versions: false,
      clean: true,
      changes: [],
      auto: { timed_enabled: false, timed_interval_minutes: 10, retention: 100 },
    })

    renderVersionPanel()

    expect((await screen.findAllByText('暂无版本历史')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('保存第一个版本后即可查看历史和恢复。').length).toBeGreaterThan(0)
  })
})

function renderVersionPanel() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <VersionPanel projectId="project-version" workspace="/workspace" refreshSignal={0} visible onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

function versionEntry(id: string, message: string, changedPaths: string[]): VersionEntry {
  return {
    id,
    message,
    created_at: '2026-07-01T12:00:00Z',
    source: 'manual',
    file_count: changedPaths.length,
    total_bytes: 128,
    changed_paths: changedPaths,
  }
}

function restorePlan(id: string, path: string): VersionRestorePlan {
  return {
    target: versionEntry(id, id, [path]),
    scope: 'paths',
    paths: [path],
    changes: [{ path, status: 'modified', text: true, binary: false }],
    will_create_backup: false,
    current_dirty: true,
  }
}

function versionDiff(id: string, path: string | undefined, comparison: 'workspace' | 'parent'): VersionDiff {
  const selectedPath = comparison === 'workspace' ? 'chapters/current.md' : `chapters/${id}.md`
  const files = [
    {
      path: selectedPath,
      status: 'modified',
      original: 'before',
      modified: 'after',
      text: true,
      binary: false,
    },
    ...(comparison === 'parent' && id === 'second' ? [{
      path: 'setting/deleted.md',
      status: 'deleted',
      original: 'deleted',
      text: true,
      binary: false,
      missing_in_modified: true,
    }] : []),
  ]
  return {
    version: versionEntry(id, id === 'second' ? '第二版本' : '第一版本', [selectedPath]),
    comparison,
    changes: files.map(file => ({ path: file.path, status: file.status })),
    files,
    path,
    text: false,
    binary: false,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

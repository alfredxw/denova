import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createVersion, getVersionRestorePlan, getVersions, getVersionStatus, restoreVersion } from '@/lib/api'
import type { VersionEntry, VersionRestorePlan } from '@/lib/api'
import { VersionPanel } from './VersionPanel'

vi.mock('@/lib/api', () => ({
  createVersion: vi.fn(),
  getVersionDiff: vi.fn(),
  getVersionRestorePlan: vi.fn(),
  getVersions: vi.fn(),
  getVersionStatus: vi.fn(),
  restoreVersion: vi.fn(),
}))

describe('VersionPanel', () => {
  beforeEach(() => {
    vi.mocked(createVersion).mockReset()
    vi.mocked(getVersionRestorePlan).mockReset()
    vi.mocked(getVersions).mockReset()
    vi.mocked(getVersionStatus).mockReset()
    vi.mocked(restoreVersion).mockReset()
    vi.mocked(getVersionStatus).mockResolvedValue({
      has_versions: true,
      clean: true,
      changes: [],
      latest: versionEntry('second', '第二版本', ['chapters/second.md']),
      auto: {
        timed_enabled: false,
        timed_interval_minutes: 10,
        retention: 100,
      },
    })
    vi.mocked(getVersions).mockResolvedValue([
      versionEntry('first', '第一版本', ['chapters/first.md']),
      versionEntry('second', '第二版本', ['chapters/second.md']),
    ])
  })

  it('ignores stale restore preview responses after another restore dialog opens', async () => {
    const user = userEvent.setup()
    const firstPreview = deferred<VersionRestorePlan>()
    const secondPreview = deferred<VersionRestorePlan>()
    vi.mocked(getVersionRestorePlan)
      .mockReturnValueOnce(firstPreview.promise)
      .mockReturnValueOnce(secondPreview.promise)

    renderVersionPanel()

    const rollbackButtons = await screen.findAllByRole('button', { name: '回滚' })
    await user.click(rollbackButtons[0])
    expect(await screen.findByText('正在计算恢复影响…')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '取消' }))
    await user.click(rollbackButtons[1])

    await act(async () => {
      secondPreview.resolve(restorePlan('second', 'chapters/second.md'))
      await secondPreview.promise
    })
    expect(await screen.findByText('chapters/second.md')).toBeInTheDocument()

    await act(async () => {
      firstPreview.resolve(restorePlan('first', 'chapters/first.md'))
      await firstPreview.promise
    })

    await waitFor(() => {
      expect(screen.queryByText('chapters/first.md')).not.toBeInTheDocument()
    })
    expect(screen.getByText('chapters/second.md')).toBeInTheDocument()
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
      <VersionPanel workspace="/workspace" refreshSignal={0} visible onClose={vi.fn()} />
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

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

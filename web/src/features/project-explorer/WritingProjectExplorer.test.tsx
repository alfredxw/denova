import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw/server'
import { WritingProjectExplorer } from './WritingProjectExplorer'

const chapterPath = 'chapters/第一章.md'

describe('WritingProjectExplorer', () => {
  beforeEach(() => window.localStorage.clear())

  it('adds Writing metadata, references, recovery, and external refresh to the shared Explorer', async () => {
    const user = userEvent.setup()
    const onReferenceFile = vi.fn()
    const onRefreshWorkspace = vi.fn().mockResolvedValue(undefined)
    let resolveRequests = 0
    server.use(
      http.post('/api/projects/book-project/files/resolve', async ({ request }) => {
        resolveRequests += 1
        const body = await request.json() as { targets: Array<{ path: string }> }
        return HttpResponse.json({
          project_id: 'book-project',
          results: body.targets.map((target) => ({
            path: target.path,
            ok: true,
            directories: [{
              path: target.path,
              revision: `revision-${resolveRequests}-${target.path}`,
              entries: target.path === ''
                ? [
                    { name: 'chapters', path: 'chapters', type: 'dir' },
                    { name: 'setting', path: 'setting', type: 'dir' },
                  ]
                : target.path === 'chapters'
                  ? [{ name: '第一章.md', path: chapterPath, type: 'file' }]
                  : [],
              children_state: 'complete',
            }],
          })),
        })
      }),
    )
    const props = {
      projectId: 'book-project',
      workspace: '/books/one',
      selectedPath: chapterPath,
      chapterStats: {
        [chapterPath]: {
          path: chapterPath,
          file_name: '第一章.md',
          display_title: '第一章',
          index: 1,
          words: 1200,
          status: 'draft',
          confirmed: false,
          updated_at: '',
          volume: '',
          volume_path: '',
        },
      },
      structureRefreshSignal: 0,
      onSelectFile: vi.fn(),
      onReferenceFile,
      onCreateItem: vi.fn().mockResolvedValue(undefined),
      onDeleteItem: vi.fn().mockResolvedValue(undefined),
      onRenameItem: vi.fn().mockResolvedValue(undefined),
      onCopyItem: vi.fn().mockResolvedValue(undefined),
      onMoveItem: vi.fn().mockResolvedValue(undefined),
      onRefreshWorkspace,
    }
    const { rerender } = render(<WritingProjectExplorer {...props} />)

    expect(await screen.findByText('1.2k')).toBeInTheDocument()
    expect(screen.getByText('draft')).toBeInTheDocument()

    fireEvent.contextMenu(screen.getByLabelText('第一章.md'))
    await user.click(await screen.findByRole('menuitem', { name: '引用到 Chat' }))
    expect(onReferenceFile).toHaveBeenCalledWith(chapterPath)

    fireEvent.contextMenu(screen.getByLabelText('第一章.md'))
    await user.click(await screen.findByRole('menuitem', { name: '删除' }))
    expect(await screen.findByText(/版本历史恢复/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '取消' }))

    const requestsBeforeSignal = resolveRequests
    rerender(<WritingProjectExplorer {...props} structureRefreshSignal={1} />)
    await waitFor(() => expect(resolveRequests).toBeGreaterThan(requestsBeforeSignal))

    await user.click(screen.getByRole('button', { name: '刷新' }))
    await waitFor(() => expect(onRefreshWorkspace).toHaveBeenCalledTimes(1))
  })
})

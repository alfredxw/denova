import { act, render } from '@testing-library/react'
import { useEffect } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { readFile, saveFile } from '@/lib/api'
import { useWorkspaceFileAutosave, type WorkspaceFileDraft } from './use-workspace-file-autosave'

vi.mock('@/lib/api', () => ({
  readFile: vi.fn(),
  saveFile: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

describe('useWorkspaceFileAutosave', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    controls = null
  })

  it('reloads, rebases, and retries a workspace file revision conflict', async () => {
    vi.mocked(readFile).mockResolvedValue({
      workspace: '/books/demo',
      path: 'CREATOR.md',
      content: 'Title\n\nExternal detail\n',
      revision: 'r2',
    })
    vi.mocked(saveFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce({ path: 'CREATOR.md', message: 'saved', revision: 'r3' })
    const onSaved = vi.fn()
    render(
      <Harness
        content={'Local title\n\nDetail\n'}
        baselineContent={'Title\n\nDetail\n'}
        onSaved={onSaved}
      />,
    )

    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(readFile).toHaveBeenCalledWith('CREATOR.md')
    expect(saveFile).toHaveBeenNthCalledWith(1, 'CREATOR.md', 'Local title\n\nDetail\n', 'r1', '/books/demo')
    expect(saveFile).toHaveBeenNthCalledWith(2, 'CREATOR.md', 'Local title\n\nExternal detail\n', 'r2', '/books/demo')
    expect(onSaved).toHaveBeenCalledWith(
      expect.objectContaining({ content: 'Local title\n\nExternal detail\n', updated_at: 'r3' }),
      expect.objectContaining({ content: 'Local title\n\nDetail\n', updated_at: 'r1' }),
    )
  })

  it('archives overlapping text before retrying with the local version', async () => {
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-1',
      path: 'conflicts/conflict-1.json',
      storage: 'server',
    })
    vi.mocked(readFile).mockResolvedValue({
      workspace: '/books/demo',
      path: 'CREATOR.md',
      content: 'External title\n',
      revision: 'r2',
    })
    vi.mocked(saveFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce({ path: 'CREATOR.md', message: 'saved', revision: 'r3' })
    render(
      <Harness
        content={'Local title\n'}
        baselineContent={'Original title\n'}
        onSaved={vi.fn()}
      />,
    )

    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'workspace_file',
      scope: '/books/demo',
      id: 'CREATOR.md',
      base: { revision: 'r1', value: 'Original title\n' },
      local: { revision: 'r1', value: 'Local title\n' },
      external: { revision: 'r2', value: 'External title\n' },
      merged: { revision: 'r2', value: 'Local title\n' },
      conflict_paths: [[]],
    }))
    expect(saveFile).toHaveBeenNthCalledWith(2, 'CREATOR.md', 'Local title\n', 'r2', '/books/demo')
  })
})

let controls: ReturnType<typeof useWorkspaceFileAutosave> | null = null

function Harness({
  content,
  baselineContent,
  onSaved,
}: {
  content: string
  baselineContent: string
  onSaved: (saved: WorkspaceFileDraft, submitted: WorkspaceFileDraft) => void
}) {
  const autosave = useWorkspaceFileAutosave({
    path: 'CREATOR.md',
    content,
    revision: 'r1',
    fileWorkspace: '/books/demo',
    active: true,
    scopeKey: '/books/demo',
    onSaved,
  })
  useEffect(() => {
    autosave.resetBaseline({
      id: 'CREATOR.md',
      content: baselineContent,
      workspace: '/books/demo',
      updated_at: 'r1',
    })
  }, [autosave.resetBaseline, baselineContent])
  controls = autosave
  return null
}

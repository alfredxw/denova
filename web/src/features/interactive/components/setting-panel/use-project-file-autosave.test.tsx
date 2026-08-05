import { act, render } from '@testing-library/react'
import { useEffect } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { readProjectFile, saveProjectFile } from '@/lib/api'
import { useProjectFileAutosave, type ProjectFileDraft } from './use-project-file-autosave'

vi.mock('@/lib/api', () => ({
  readProjectFile: vi.fn(),
  saveProjectFile: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

const PROJECT_ID = 'project-demo'

describe('useProjectFileAutosave', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    controls = null
  })

  it('reloads, rebases, and retries a workspace file revision conflict', async () => {
    vi.mocked(readProjectFile).mockResolvedValue({
      project_id: PROJECT_ID,
      path: 'CREATOR.md',
      content: 'Title\n\nExternal detail\n',
      revision: 'r2',
      kind: 'text',
      mime_type: 'text/markdown',
      size: 24,
      editable: true,
    })
    vi.mocked(saveProjectFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce({ project_id: PROJECT_ID, path: 'CREATOR.md', changed: true, revision: 'r3' })
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

    expect(readProjectFile).toHaveBeenCalledWith(PROJECT_ID, 'CREATOR.md')
    expect(saveProjectFile).toHaveBeenNthCalledWith(1, PROJECT_ID, 'CREATOR.md', 'Local title\n\nDetail\n', 'r1')
    expect(saveProjectFile).toHaveBeenNthCalledWith(2, PROJECT_ID, 'CREATOR.md', 'Local title\n\nExternal detail\n', 'r2')
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
    vi.mocked(readProjectFile).mockResolvedValue({
      project_id: PROJECT_ID,
      path: 'CREATOR.md',
      content: 'External title\n',
      revision: 'r2',
      kind: 'text',
      mime_type: 'text/markdown',
      size: 15,
      editable: true,
    })
    vi.mocked(saveProjectFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce({ project_id: PROJECT_ID, path: 'CREATOR.md', changed: true, revision: 'r3' })
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
      resource: 'project_file',
      scope: PROJECT_ID,
      id: 'CREATOR.md',
      base: { revision: 'r1', value: 'Original title\n' },
      local: { revision: 'r1', value: 'Local title\n' },
      external: { revision: 'r2', value: 'External title\n' },
      merged: { revision: 'r2', value: 'Local title\n' },
      conflict_paths: [[]],
    }))
    expect(saveProjectFile).toHaveBeenNthCalledWith(2, PROJECT_ID, 'CREATOR.md', 'Local title\n', 'r2')
  })
})

let controls: ReturnType<typeof useProjectFileAutosave> | null = null

function Harness({
  content,
  baselineContent,
  onSaved,
}: {
  content: string
  baselineContent: string
  onSaved: (saved: ProjectFileDraft, submitted: ProjectFileDraft) => void
}) {
  const autosave = useProjectFileAutosave({
    projectId: PROJECT_ID,
    path: 'CREATOR.md',
    content,
    revision: 'r1',
    fileProjectId: PROJECT_ID,
    active: true,
    onSaved,
  })
  useEffect(() => {
    autosave.resetBaseline({
      id: 'CREATOR.md',
      content: baselineContent,
      project_id: PROJECT_ID,
      updated_at: 'r1',
    })
  }, [autosave.resetBaseline, baselineContent])
  controls = autosave
  return null
}

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestJSON } from './client'
import { preserveAutosaveConflict } from './autosave-conflicts'

vi.mock('./client', () => ({
  jsonHeaders: { 'content-type': 'application/json' },
  requestJSON: vi.fn(),
}))

const record = {
  resource: 'workspace_file',
  scope: '/books/demo',
  id: 'chapters/ch01.md',
  base: { revision: 'r1', value: 'base' },
  local: { value: 'local' },
  external: { revision: 'r2', value: 'external' },
  merged: { revision: 'r2', value: 'local' },
  strategy: 'merge_non_overlap_prefer_local',
  conflict_paths: [[]],
}

describe('preserveAutosaveConflict', () => {
  beforeEach(() => {
    vi.mocked(requestJSON).mockReset()
    window.localStorage.clear()
  })

  it('persists the complete conflict through the server journal', async () => {
    vi.mocked(requestJSON).mockResolvedValue({ id: 'conflict-1', path: '/data/conflicts/conflict-1.json' })

    await expect(preserveAutosaveConflict(record)).resolves.toEqual({
      id: 'conflict-1',
      path: '/data/conflicts/conflict-1.json',
      storage: 'server',
    })
    expect(requestJSON).toHaveBeenCalledWith('/api/autosave-conflicts', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(record),
    }))
  })

  it('keeps a durable browser copy when the server journal is temporarily unavailable', async () => {
    vi.mocked(requestJSON).mockRejectedValue(new Error('offline'))

    const saved = await preserveAutosaveConflict(record)

    expect(saved.storage).toBe('browser')
    expect(JSON.parse(window.localStorage.getItem(`nova:autosave-conflict:${saved.id}`) || '')).toMatchObject(record)
  })
})

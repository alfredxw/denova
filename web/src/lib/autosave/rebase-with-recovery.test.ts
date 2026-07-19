import { beforeEach, describe, expect, it, vi } from 'vitest'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { rebaseJSONWithRecovery, rebaseTextWithRecovery } from './rebase-with-recovery'

vi.mock('@/lib/api-client/autosave-conflicts', () => ({ preserveAutosaveConflict: vi.fn() }))

describe('rebase with recovery', () => {
  beforeEach(() => {
    vi.mocked(preserveAutosaveConflict).mockReset().mockResolvedValue({
      id: 'conflict-1',
      path: '/conflicts/conflict-1.json',
      storage: 'server',
    })
  })

  it('merges non-overlapping text without creating a conflict record', async () => {
    const value = await rebaseTextWithRecovery({
      resource: 'skill', scope: 'user', id: 'writer',
      baseline: { revision: 'r1', value: 'A\nM\nB\n' },
      local: { value: 'Local A\nM\nB\n' },
      external: { revision: 'r2', value: 'A\nM\nAgent B\n' },
    })

    expect(value).toBe('Local A\nM\nAgent B\n')
    expect(preserveAutosaveConflict).not.toHaveBeenCalled()
  })

  it('archives overlapping JSON leaves before returning the local-preferred merge', async () => {
    const value = await rebaseJSONWithRecovery({
      resource: 'settings', scope: 'user', id: 'settings',
      baseline: { revision: 'r1', value: { theme: 'system' } },
      local: { value: { theme: 'dark' } },
      external: { revision: 'r2', value: { theme: 'light' } },
    })

    expect(value).toEqual({ theme: 'dark' })
    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      conflict_paths: [['theme']],
      external: { revision: 'r2', value: { theme: 'light' } },
      merged: { revision: 'r2', value: { theme: 'dark' } },
    }))
  })
})

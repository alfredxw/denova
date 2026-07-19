import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { readStyleReferenceFile, updateStyleReferenceFile } from '../../api'
import type { StyleReferenceFileDocument } from '../../types'
import { useStyleReferenceAutosave } from './use-style-reference-autosave'

vi.mock('../../api', () => ({
  readStyleReferenceFile: vi.fn(),
  updateStyleReferenceFile: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

describe('useStyleReferenceAutosave', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    controls = null
  })

  it('archives overlapping text and retries with the local style content', async () => {
    const baseline = styleDocument('Original style\n', 'r1')
    const external = styleDocument('External style\n', 'r2')
    const saved = styleDocument('Local style\n', 'r3')
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-1',
      path: 'conflicts/conflict-1.json',
      storage: 'server',
    })
    vi.mocked(readStyleReferenceFile).mockResolvedValue(external)
    vi.mocked(updateStyleReferenceFile)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(saved)
    render(<Harness document={baseline} content={'Local style\n'} />)

    await act(async () => {
      await controls?.flush(true)
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'style_reference',
      scope: 'user',
      id: '.denova/styles/style.md',
      base: { revision: 'r1', value: 'Original style\n' },
      local: { revision: 'r1', value: 'Local style\n' },
      external: { revision: 'r2', value: 'External style\n' },
      merged: { revision: 'r2', value: 'Local style\n' },
      conflict_paths: [[]],
    }))
    expect(updateStyleReferenceFile).toHaveBeenNthCalledWith(2, {
      path: '.denova/styles/style.md',
      content: 'Local style\n',
      base_revision: 'r2',
    })
  })
})

let controls: ReturnType<typeof useStyleReferenceAutosave> | null = null

function Harness({ document, content }: { document: StyleReferenceFileDocument; content: string }) {
  controls = useStyleReferenceAutosave({
    document,
    content,
    active: true,
    onSaved: vi.fn(),
    onError: vi.fn(),
  })
  return null
}

function styleDocument(content: string, revision: string): StyleReferenceFileDocument {
  return {
    reference: {
      name: 'Style',
      description: '',
      path: '/tmp/.denova/styles/style.md',
      display_path: '.denova/styles/style.md',
    },
    content,
    revision,
  }
}

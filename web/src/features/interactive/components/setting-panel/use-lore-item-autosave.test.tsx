import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import { getLoreItems, updateLoreItem, type LoreItem } from '@/lib/api'
import { useLoreItemAutosave, type LoreAutosaveDraft } from './use-lore-item-autosave'

vi.mock('@/lib/api', () => ({
  getLoreItems: vi.fn(),
  updateLoreItem: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

describe('useLoreItemAutosave', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    controls = null
  })

  it('does not save when the rich editor only normalizes the final newline', async () => {
    vi.useFakeTimers()
    const baseline = loreItem({ content: 'Body', updated_at: 'r1' })
    const draft = loreItem({ content: 'Body\n', updated_at: 'r1' })

    render(<Harness draft={draft} baseline={{ ...baseline, tag_draft: '' }} onSaved={vi.fn()} />)
    await act(async () => { await vi.advanceTimersByTimeAsync(1300) })

    expect(updateLoreItem).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('reloads, rebases, and retries a Lore revision conflict', async () => {
    const baseline = loreItem({ name: 'Original', content: 'Old body', updated_at: 'r1' })
    const draft = { ...baseline, name: 'Local name' }
    const latest = loreItem({ name: 'Original', content: 'External body', updated_at: 'r2' })
    const saved = loreItem({ name: 'Local name', content: 'External body', updated_at: 'r3' })
    vi.mocked(getLoreItems).mockResolvedValue([latest])
    vi.mocked(updateLoreItem)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(saved)
    const onSaved = vi.fn()
    render(<Harness draft={draft} baseline={{ ...baseline, tag_draft: '' }} onSaved={onSaved} />)

    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(getLoreItems).toHaveBeenCalledOnce()
    expect(updateLoreItem).toHaveBeenNthCalledWith(1, 'lore-1', expect.objectContaining({
      name: 'Local name',
      content: 'Old body',
    }), 'r1')
    expect(updateLoreItem).toHaveBeenNthCalledWith(2, 'lore-1', expect.objectContaining({
      name: 'Local name',
      content: 'External body',
    }), 'r2')
    expect(vi.mocked(updateLoreItem).mock.calls[1]?.[1]).not.toHaveProperty('updated_at')
    expect(onSaved).toHaveBeenCalledWith(saved, expect.objectContaining({ name: 'Local name' }))
  })

  it('archives an overlapping Lore field and retries without blocking the edit', async () => {
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-1',
      path: 'conflicts/conflict-1.json',
      storage: 'server',
    })
    const baseline = loreItem({ name: 'Original', updated_at: 'r1' })
    const draft = loreItem({ name: 'Local name', updated_at: 'r1' })
    const latest = loreItem({ name: 'External name', updated_at: 'r2' })
    vi.mocked(getLoreItems).mockResolvedValue([latest])
    vi.mocked(updateLoreItem)
      .mockRejectedValueOnce(new APIError('revision conflict', { status: 409 }))
      .mockResolvedValueOnce(loreItem({ name: 'Local name', updated_at: 'r3' }))
    render(<Harness draft={draft} baseline={{ ...baseline, tag_draft: '' }} onSaved={vi.fn()} />)

    await act(async () => {
      await controls?.saveNow('manual')
    })

    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'lore_item',
      scope: '/books/demo',
      id: 'lore-1',
      base: { revision: 'r1', value: expect.objectContaining({ name: 'Original' }) },
      local: { revision: 'r1', value: expect.objectContaining({ name: 'Local name' }) },
      external: { revision: 'r2', value: expect.objectContaining({ name: 'External name' }) },
      merged: { revision: 'r2', value: expect.objectContaining({ name: 'Local name', updated_at: 'r2' }) },
      conflict_paths: [['name']],
    }))
    expect(updateLoreItem).toHaveBeenNthCalledWith(
      2,
      'lore-1',
      expect.objectContaining({ name: 'Local name' }),
      'r2',
    )
  })
})

let controls: ReturnType<typeof useLoreItemAutosave> | null = null

function Harness({
  draft,
  baseline,
  onSaved,
}: {
  draft: LoreItem
  baseline: LoreAutosaveDraft
  onSaved: (item: LoreItem, submitted: LoreAutosaveDraft) => void
}) {
  controls = useLoreItemAutosave({
    draft,
    tagDraft: '',
    baseline,
    active: true,
    workspace: '/books/demo',
    onSaved,
    onAutoSaveError: vi.fn(),
  })
  return null
}

function loreItem(overrides: Partial<LoreItem> = {}): LoreItem {
  return {
    id: 'lore-1',
    enabled: true,
    type: 'world',
    type_source: 'manual',
    name: 'Original',
    importance: 'important',
    load_mode: 'auto',
    tags: [],
    brief_description: '',
    keywords: [],
    content: 'Old body',
    created_at: 'created',
    updated_at: 'r1',
    ...overrides,
  }
}

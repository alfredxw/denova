import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { preserveAutosaveConflict } from '@/lib/api-client/autosave-conflicts'
import {
  createLoreItem,
  deleteLoreItem,
  getLoreItems,
  updateLoreItem,
  type LoreItem,
} from '@/lib/api'
import { notifyLoreUpdated } from './events'
import { useLoreWorkspace } from './use-lore-workspace'

vi.mock('@/lib/api', () => ({
  createLoreItem: vi.fn(),
  deleteLoreItem: vi.fn(),
  getLoreItems: vi.fn(),
  updateLoreItem: vi.fn(),
}))

vi.mock('@/lib/api-client/autosave-conflicts', () => ({
  preserveAutosaveConflict: vi.fn(),
}))

describe('useLoreWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    vi.mocked(preserveAutosaveConflict).mockResolvedValue({
      id: 'conflict-1',
      path: 'conflicts/conflict-1.json',
      storage: 'server',
    })
  })

  it('rebases an Agent update into a dirty draft and archives overlapping fields', async () => {
    const initial = loreItem()
    const external = loreItem({
      name: 'Agent name',
      content: 'Agent body',
      updated_at: 'r2',
    })
    vi.mocked(getLoreItems)
      .mockResolvedValueOnce([initial])
      .mockResolvedValueOnce([external])

    const { result } = renderHook(() => useLoreWorkspace({
      workspace: '/books/demo',
      onFlushHandlerChange: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft?.id).toBe('lore-1'))

    act(() => {
      result.current.setDraft({ ...result.current.draft!, name: 'Local name' })
    })
    act(() => notifyLoreUpdated({ source: 'writing-agent' }))

    await waitFor(() => expect(getLoreItems).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(result.current.draft).toMatchObject({
      name: 'Local name',
      content: 'Agent body\n',
      updated_at: 'r2',
    }))
    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'lore_item',
      scope: '/books/demo',
      id: 'lore-1',
      conflict_paths: [['name']],
    }))
    expect(updateLoreItem).not.toHaveBeenCalled()
  })

  it('archives a dirty local draft before accepting an Agent deletion', async () => {
    vi.mocked(getLoreItems)
      .mockResolvedValueOnce([loreItem()])
      .mockResolvedValueOnce([])

    const { result } = renderHook(() => useLoreWorkspace({
      workspace: '/books/demo',
      onFlushHandlerChange: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft?.id).toBe('lore-1'))

    act(() => {
      result.current.setDraft({ ...result.current.draft!, content: 'Unsaved local body' })
    })
    act(() => notifyLoreUpdated({ source: 'writing-agent' }))

    await waitFor(() => expect(result.current.draft).toBeNull())
    expect(preserveAutosaveConflict).toHaveBeenCalledWith(expect.objectContaining({
      resource: 'lore_item',
      id: 'lore-1',
      external: { revision: 'deleted', value: null },
    }))
    expect(updateLoreItem).not.toHaveBeenCalled()
  })

  it('preserves the item flushed immediately before creating another item', async () => {
    const initial = loreItem()
    const saved = loreItem({ name: 'Saved name', updated_at: 'r2' })
    const created = loreItem({
      id: 'lore-2',
      name: 'New character',
      created_at: 'r3',
      updated_at: 'r3',
    })
    vi.mocked(getLoreItems).mockResolvedValue([initial])
    vi.mocked(updateLoreItem).mockResolvedValue(saved)
    vi.mocked(createLoreItem).mockResolvedValue(created)

    const { result } = renderHook(() => useLoreWorkspace({
      workspace: '/books/demo',
      onFlushHandlerChange: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft?.id).toBe('lore-1'))

    act(() => {
      result.current.setDraft({ ...result.current.draft!, name: 'Saved name' })
    })
    await act(async () => {
      await result.current.createItem({ name: 'New character' })
    })

    expect(updateLoreItem).toHaveBeenCalledWith(
      '/books/demo',
      'lore-1',
      expect.objectContaining({ name: 'Saved name' }),
      'r1',
    )
    expect(result.current.items).toEqual([saved, created])
    expect(result.current.draft).toMatchObject({ id: 'lore-2', name: 'New character' })
  })

  it('flushes the selected item before deleting it and selects the next item', async () => {
    const initial = loreItem()
    const next = loreItem({
      id: 'lore-2',
      name: 'Next character',
      created_at: 'r2',
      updated_at: 'r2',
    })
    const saved = loreItem({ name: 'Saved before delete', updated_at: 'r3' })
    vi.mocked(getLoreItems).mockResolvedValue([initial, next])
    vi.mocked(updateLoreItem).mockResolvedValue(saved)
    vi.mocked(deleteLoreItem).mockResolvedValue(undefined)

    const { result } = renderHook(() => useLoreWorkspace({
      workspace: '/books/demo',
      onFlushHandlerChange: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft?.id).toBe('lore-1'))

    act(() => {
      result.current.setDraft({
        ...result.current.draft!,
        name: 'Saved before delete',
      })
    })
    await act(async () => {
      expect(await result.current.deleteItem('lore-1')).toBe(true)
    })

    expect(updateLoreItem).toHaveBeenCalledWith(
      '/books/demo',
      'lore-1',
      expect.objectContaining({ name: 'Saved before delete' }),
      'r1',
    )
    expect(deleteLoreItem).toHaveBeenCalledWith('/books/demo', 'lore-1')
    expect(vi.mocked(updateLoreItem).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(deleteLoreItem).mock.invocationCallOrder[0],
    )
    expect(result.current.items).toEqual([next])
    expect(result.current.activeId).toBe('lore-2')
    expect(result.current.draft).toMatchObject({
      id: 'lore-2',
      name: 'Next character',
    })
    expect(window.localStorage.getItem('nova.lore.workspace.selection:/books/demo'))
      .toBe('lore-2')
  })

  it('clears the editor after deleting the final lore item', async () => {
    vi.mocked(getLoreItems).mockResolvedValue([loreItem()])
    vi.mocked(deleteLoreItem).mockResolvedValue(undefined)

    const { result } = renderHook(() => useLoreWorkspace({
      workspace: '/books/demo',
      onFlushHandlerChange: vi.fn(),
    }))
    await waitFor(() => expect(result.current.draft?.id).toBe('lore-1'))

    await act(async () => {
      expect(await result.current.deleteItem('lore-1')).toBe(true)
    })

    expect(result.current.items).toEqual([])
    expect(result.current.activeId).toBe('')
    expect(result.current.draft).toBeNull()
    expect(window.localStorage.getItem('nova.lore.workspace.selection:/books/demo'))
      .toBeNull()
  })
})

function loreItem(overrides: Partial<LoreItem> = {}): LoreItem {
  return {
    id: 'lore-1',
    enabled: true,
    type: 'character',
    type_source: 'manual',
    name: 'Original name',
    importance: 'important',
    load_mode: 'auto',
    tags: [],
    brief_description: '',
    keywords: [],
    content: 'Original body',
    created_at: 'r0',
    updated_at: 'r1',
    ...overrides,
  }
}

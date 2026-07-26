import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { ResourceDirectorySection } from '@/components/resource-directory/types'
import { applyPresetDirectoryOrder, mergeVisiblePresetDirectoryOrder, usePresetDirectoryOrder } from './use-preset-directory-order'

describe('preset directory order', () => {
  beforeEach(() => window.localStorage.clear())

  it('applies known ids first and appends newly discovered presets in source order', () => {
    const sections: ResourceDirectorySection[] = [{
      id: 'teller',
      label: 'Narrative Styles',
      items: [{ id: 'teller:a', title: 'A' }, { id: 'teller:b', title: 'B' }, { id: 'teller:new', title: 'New' }],
    }]

    expect(applyPresetDirectoryOrder(sections, { teller: ['teller:b', 'teller:a', 'teller:removed'] })[0].items.map((item) => item.id))
      .toEqual(['teller:b', 'teller:a', 'teller:new'])
  })

  it('reorders a filtered mode without moving presets hidden from that mode', () => {
    expect(mergeVisiblePresetDirectoryOrder(
      ['teller:a', 'teller:game-only', 'teller:b'],
      undefined,
      ['teller:b', 'teller:a'],
    )).toEqual(['teller:b', 'teller:game-only', 'teller:a'])
  })

  it('persists each workspace independently', () => {
    const { result, rerender } = renderHook(({ workspace }) => usePresetDirectoryOrder(workspace), {
      initialProps: { workspace: '/books/one' },
    })

    act(() => result.current.reorderItems('image', ['image:b', 'image:a'], ['image:a', 'image:b']))
    expect(JSON.parse(window.localStorage.getItem('nova.preset-directory-order:/books/one') || '{}')).toMatchObject({
      version: 1,
      sections: { image: ['image:b', 'image:a'] },
    })

    rerender({ workspace: '/books/two' })
    expect(result.current.order).toEqual({})
  })
})

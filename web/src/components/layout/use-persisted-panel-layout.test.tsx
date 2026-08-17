import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  readPersistedPanelLayout,
  resolvePanelInitialSize,
  usePersistedPanelLayout,
} from './use-persisted-panel-layout'

const STORAGE_KEY = 'test:persisted-panel-layout'

describe('usePersistedPanelLayout', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('restores the saved panel order without letting initialization overwrite it', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ main: 68, sidebar: 32 }))
    const { result } = renderHook(() => usePersistedPanelLayout({
      storageKey: STORAGE_KEY,
      panelIds: ['sidebar', 'main'],
    }))

    expect(result.current.defaultLayout).toEqual({ sidebar: 32, main: 68 })

    act(() => {
      expect(result.current.persistUserLayout({ sidebar: 20, main: 80 })).toBe(false)
    })
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')).toEqual({ main: 68, sidebar: 32 })
  })

  it('persists a valid layout after an explicit pointer resize begins', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ sidebar: 24, main: 76 }))
    const { result } = renderHook(() => usePersistedPanelLayout({
      storageKey: STORAGE_KEY,
      panelIds: ['sidebar', 'main'],
    }))

    act(() => {
      result.current.resizeHandleIntentProps.onPointerDownCapture({} as never)
      expect(result.current.persistUserLayout({ sidebar: 31, main: 69 })).toBe(true)
    })

    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')).toEqual({ sidebar: 31, main: 69 })
  })

  it('keeps persisting through the post-pointerup layout commit so the final width wins', async () => {
    const { result } = renderHook(() => usePersistedPanelLayout({
      storageKey: STORAGE_KEY,
      panelIds: ['sidebar', 'main'],
    }))

    act(() => {
      result.current.resizeHandleIntentProps.onPointerDownCapture({} as never)
      expect(result.current.persistUserLayout({ sidebar: 25, main: 75 })).toBe(true)
      expect(result.current.persistUserLayout({ sidebar: 32, main: 68 })).toBe(true)
    })

    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')).toEqual({ sidebar: 32, main: 68 })

    act(() => {
      window.dispatchEvent(new Event('pointerup'))
    })
    expect(result.current.isUserResizeActive()).toBe(true)
    expect(result.current.persistUserLayout({ sidebar: 40, main: 60 })).toBe(true)
    await waitFor(() => expect(result.current.isUserResizeActive()).toBe(false))
    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')).toEqual({ sidebar: 40, main: 60 })
  })

  it('only treats resize keys as keyboard resize intent', () => {
    const { result } = renderHook(() => usePersistedPanelLayout({
      storageKey: STORAGE_KEY,
      panelIds: ['sidebar', 'main'],
    }))

    act(() => {
      result.current.resizeHandleIntentProps.onKeyDownCapture({ key: 'Enter' } as never)
      expect(result.current.persistUserLayout({ sidebar: 26, main: 74 })).toBe(false)
      result.current.resizeHandleIntentProps.onKeyDownCapture({ key: 'ArrowRight' } as never)
      expect(result.current.persistUserLayout({ sidebar: 26, main: 74 })).toBe(true)
    })

    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) || '{}')).toEqual({ sidebar: 26, main: 74 })
  })
})

describe('readPersistedPanelLayout', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('rejects partial layouts so a changed panel structure falls back safely', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ sidebar: 30, main: 50 }))

    expect(readPersistedPanelLayout(STORAGE_KEY, ['sidebar', 'main'])).toBeUndefined()
  })
})

describe('resolvePanelInitialSize', () => {
  it('turns a persisted panel percentage into an explicit first-open size', () => {
    expect(resolvePanelInitialSize({ main: 63, right: 37 }, 'right', '66%')).toBe('37%')
  })

  it('falls back when the layout has no useful expanded size', () => {
    expect(resolvePanelInitialSize(undefined, 'right', '66%')).toBe('66%')
    expect(resolvePanelInitialSize({ main: 100, right: 0 }, 'right', '66%')).toBe('66%')
  })
})

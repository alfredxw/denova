import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useOnlineStatus } from './use-online-status'

describe('useOnlineStatus', () => {
  it('tracks navigator.onLine through online and offline events', () => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
    const { result } = renderHook(() => useOnlineStatus())
    expect(result.current).toBe(false)

    Object.defineProperty(navigator, 'onLine', { configurable: true, value: true })
    act(() => window.dispatchEvent(new Event('online')))
    expect(result.current).toBe(true)

    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
    act(() => window.dispatchEvent(new Event('offline')))
    expect(result.current).toBe(false)
  })
})

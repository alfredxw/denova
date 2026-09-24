import { act, renderHook } from '@testing-library/react'
import type { PointerEvent, WheelEvent } from 'react'
import { expect, it } from 'vitest'
import { useVirtuosoBottomLock } from './useVirtuosoBottomLock'

it('follows footer expansion from the bottom throughout layout updates', () => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => { result.current.beginAfterContentInteraction(); result.current.releaseBottomLock() })
  for (const height of [1100, 1250, 1400, 1000]) {
    Object.defineProperty(scroller, 'scrollHeight', { value: height, configurable: true })
    act(() => result.current.restoreAfterContentScrollPosition())
    expect(scroller.scrollTop).toBe(height - 500)
  }
})

it('preserves the reading position when expansion starts away from the bottom', () => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 200
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => result.current.beginAfterContentInteraction())
  Object.defineProperty(scroller, 'scrollHeight', { value: 1400, configurable: true })
  act(() => result.current.restoreAfterContentScrollPosition())
  expect(scroller.scrollTop).toBe(200)
})

it.each(['wheel', 'touch'] as const)('stops following when the user scrolls upward with %s during expansion', (input) => {
  const scroller = document.createElement('div')
  Object.defineProperty(scroller, 'clientHeight', { value: 500 })
  Object.defineProperty(scroller, 'scrollHeight', { value: 1000, configurable: true })
  scroller.scrollTop = 500
  const { result } = renderHook(() => useVirtuosoBottomLock({ itemCount: 3, autoFollowEnabled: false, resolveScroller: () => scroller }))
  act(() => result.current.beginAfterContentInteraction())
  if (input === 'wheel') {
    act(() => result.current.onWheel({ deltaY: -100 } as WheelEvent<HTMLDivElement>))
  } else {
    const footer = document.createElement('div')
    footer.setAttribute('data-nova-chat-after-content', '')
    scroller.append(footer)
    act(() => result.current.onPointerDown({ target: footer, currentTarget: scroller, pointerType: 'touch', clientY: 200 } as unknown as PointerEvent<HTMLDivElement>))
    act(() => result.current.onPointerMove({ clientY: 220 } as PointerEvent<HTMLDivElement>))
  }
  scroller.scrollTop = 400
  Object.defineProperty(scroller, 'scrollHeight', { value: 1400, configurable: true })
  act(() => result.current.restoreAfterContentScrollPosition())
  expect(scroller.scrollTop).toBe(400)
})

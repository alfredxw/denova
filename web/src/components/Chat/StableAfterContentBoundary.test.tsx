import { act, render } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { StableAfterContentBoundary } from './StableAfterContentBoundary'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it.each(['collapse animation', 'shorter entity tab'])('does not retain obsolete footer space after a %s', (change) => {
  let height = 800
  let notifyResize = () => {}
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(() => ({ height } as DOMRect))
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback: () => void) { notifyResize = callback }
    observe() {}
    disconnect() {}
  })
  const onLayoutStabilized = vi.fn()
  const props = {
    className: '', onInteractionStart: vi.fn(), onInteraction: vi.fn(),
    onInteractionReset: vi.fn(), onLayoutStabilized,
  }
  const { container } = render(
    <StableAfterContentBoundary {...props}>
      <section data-state-panel-mode="preview">State panel</section>
    </StableAfterContentBoundary>,
  )
  if (change === 'collapse animation') {
    container.querySelector('section')!.setAttribute('data-state-panel-mode', 'collapsed')
    // A collapse changes mode before Motion has reduced the content height.
    act(() => notifyResize())
  }
  onLayoutStabilized.mockClear()
  for (const nextHeight of [600, 300, change === 'collapse animation' ? 42 : 180]) {
    height = nextHeight
    act(() => notifyResize())
    const reserve = container.querySelector<HTMLElement>('[data-nova-chat-after-content-reserve]')
    expect(Number.parseFloat(reserve?.style.height || '0')).toBe(0)
  }
  expect(onLayoutStabilized).toHaveBeenCalledTimes(3)
})

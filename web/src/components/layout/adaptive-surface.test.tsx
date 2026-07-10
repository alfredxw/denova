import { useEffect, useState } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AdaptiveSurface } from './adaptive-surface'
import { MobilePaneHost } from './mobile-pane-host'

const defaultResizeObserver = globalThis.ResizeObserver

describe('AdaptiveSurface', () => {
  beforeEach(() => {
    setMobileViewport(false)
  })

  afterEach(() => {
    setResizeObserver(defaultResizeObserver)
    vi.restoreAllMocks()
  })

  it('renders side panes inline on desktop', () => {
    const { container } = render(adaptiveSurface())

    expect(screen.getByTestId('left-pane')).toBeVisible()
    expect(screen.getByTestId('main-pane')).toBeVisible()
    expect(screen.getByTestId('right-pane')).toBeVisible()
    expect(container.querySelector('[data-nova-adaptive-container="true"]')).not.toBeInTheDocument()
  })

  it('keeps the main slot height-constrained on desktop', () => {
    render(adaptiveSurface())

    expect(screen.getByTestId('main-pane').parentElement).toHaveClass('h-full', 'min-h-0', 'flex-col')
  })

  it('opens mobile panes through controls', async () => {
    setMobileViewport(true)
    const user = userEvent.setup()
    render(adaptiveSurface())

    expect(screen.queryByTestId('left-pane')).not.toBeInTheDocument()
    expect(screen.getByTestId('main-pane').parentElement).toHaveClass('h-full', 'min-h-0', 'flex-col')

    await user.click(screen.getByRole('button', { name: 'Open left' }))
    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /关闭|Close/ }))
    await user.click(screen.getByRole('button', { name: 'Open right' }))
    expect(screen.getByTestId('right-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('collapses panes at its own width on desktop and expands them again', async () => {
    const resize = installResizeObserverHarness()
    const user = userEvent.setup()
    const { container } = render(adaptiveSurface(700))

    expect(container.querySelector('[data-nova-adaptive-container="true"]')).toBeInTheDocument()
    expect(screen.getByTestId('left-pane')).toBeVisible()

    resize(640)

    expect(screen.queryByTestId('left-pane')).not.toBeInTheDocument()
    expect(container.querySelector('[data-nova-mobile-pane-host="true"]')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Open left' }))
    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()

    resize(900)

    expect(screen.getByTestId('left-pane')).toBeVisible()
    expect(screen.getByTestId('right-pane')).toBeVisible()
    expect(container.querySelector('[data-nova-mobile-pane-host="true"]')).not.toBeInTheDocument()
  })

  it('keeps the stateful main pane mounted while its own width crosses the collapse threshold', async () => {
    const resize = installResizeObserverHarness()
    const user = userEvent.setup()
    let unmountCount = 0

    render(
      <AdaptiveSurface
        collapseAt={700}
        left={{ id: 'left', title: 'Left', side: 'left', content: <div>Left pane</div> }}
      >
        <StatefulMainPane onUnmount={() => { unmountCount += 1 }} />
      </AdaptiveSurface>
    )

    await user.click(screen.getByRole('button', { name: 'Count 0' }))
    resize(640)
    expect(screen.getByRole('button', { name: 'Count 1' })).toBeVisible()

    resize(900)
    expect(screen.getByRole('button', { name: 'Count 1' })).toBeVisible()
    expect(unmountCount).toBe(0)
  })

  it('keeps the stateful main pane mounted while the viewport crosses the mobile breakpoint', async () => {
    const setMobile = installMobileViewportHarness(false)
    const user = userEvent.setup()
    let unmountCount = 0

    render(
      <AdaptiveSurface left={{ id: 'left', title: 'Left', side: 'left', content: <div>Left pane</div> }}>
        <StatefulMainPane onUnmount={() => { unmountCount += 1 }} />
      </AdaptiveSurface>
    )

    await user.click(screen.getByRole('button', { name: 'Count 0' }))
    setMobile(true)
    expect(screen.getByRole('button', { name: 'Count 1' })).toBeVisible()

    setMobile(false)
    expect(screen.getByRole('button', { name: 'Count 1' })).toBeVisible()
    expect(unmountCount).toBe(0)
  })

  it('keeps the drawer layout when the viewport is mobile even above collapseAt', () => {
    setMobileViewport(true)
    const resize = installResizeObserverHarness()
    const { container } = render(adaptiveSurface(700))

    resize(900)

    expect(screen.queryByTestId('left-pane')).not.toBeInTheDocument()
    expect(container.querySelector('[data-nova-mobile-pane-host="true"]')).toBeInTheDocument()
  })

  it('falls back to its initial measured width when ResizeObserver is unavailable', () => {
    setResizeObserver(undefined)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ width: 640 } as DOMRect)

    expect(() => render(adaptiveSurface(700))).not.toThrow()
    expect(screen.queryByTestId('left-pane')).not.toBeInTheDocument()
    expect(document.querySelector('[data-nova-mobile-pane-host="true"]')).toBeInTheDocument()
  })

  it('opens mobile panes from edge swipes', () => {
    setMobileViewport(true)
    const { container } = render(adaptiveSurface())
    const host = container.querySelector('[data-nova-mobile-pane-host="true"]')!

    fireEvent.pointerDown(host, { pointerId: 1, pointerType: 'touch', button: 0, clientX: 1, clientY: 120 })
    fireEvent.pointerUp(window, { pointerId: 1, pointerType: 'touch', clientX: 80, clientY: 124 })

    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('opens mobile panes from mouse edge drags', () => {
    setMobileViewport(true)
    const { container } = render(adaptiveSurface())
    const host = container.querySelector('[data-nova-mobile-pane-host="true"]')!

    fireEvent.mouseDown(host, { button: 0, clientX: 1, clientY: 120 })
    fireEvent.mouseUp(window, { clientX: 80, clientY: 124 })

    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('moves mobile panes with the active edge drag before release', () => {
    setMobileViewport(true)
    const { container } = render(adaptiveSurface())
    const host = container.querySelector('[data-nova-mobile-pane-host="true"]')!

    fireEvent.mouseDown(host, { button: 0, clientX: 1, clientY: 120 })
    fireEvent.mouseMove(window, { clientX: 44, clientY: 122 })

    const drawer = screen.getByTestId('left-pane').closest('[data-nova-mobile-pane-content="true"]') as HTMLElement
    expect(drawer).toBeTruthy()
    expect(drawer.style.transform).toContain('translate3d(-')
    expect(drawer.style.transform).not.toBe('translate3d(0%, 0, 0)')

    fireEvent.mouseUp(window, { clientX: 90, clientY: 124 })
    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('opens mobile panes from text editor edge drags', () => {
    setMobileViewport(true)
    render(
      <AdaptiveSurface right={{ id: 'right', title: 'Right', side: 'right', content: <div data-testid="right-pane">Right pane</div> }}>
        <textarea aria-label="Editor" />
      </AdaptiveSurface>
    )

    fireEvent.mouseDown(screen.getByRole('textbox', { name: 'Editor' }), { button: 0, clientX: 389, clientY: 120 })
    fireEvent.mouseUp(window, { clientX: 320, clientY: 124 })

    expect(screen.getByTestId('right-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('respects explicit swipe opt-out targets', () => {
    setMobileViewport(true)
    render(
      <AdaptiveSurface left={{ id: 'left', title: 'Left', side: 'left', content: <div data-testid="left-pane">Left pane</div> }}>
        <div data-testid="drag-blocker" data-nova-swipe-ignore="true">Ignore gestures</div>
      </AdaptiveSurface>
    )

    fireEvent.mouseDown(screen.getByTestId('drag-blocker'), { button: 0, clientX: 1, clientY: 120 })
    fireEvent.mouseUp(window, { clientX: 80, clientY: 124 })

    expect(screen.queryByTestId('left-pane')).not.toBeInTheDocument()
  })

  it('keeps nested mobile pane gestures scoped to the inner surface', () => {
    setMobileViewport(true)
    const { container } = render(
      <MobilePaneHost closeLabel="Close" panes={[{ id: 'outer', title: 'Project', side: 'left', content: <div data-testid="outer-pane">Project pane</div> }]}>
        <AdaptiveSurface left={{ id: 'inner', title: 'Settings', side: 'left', content: <div data-testid="inner-pane">Settings pane</div> }}>
          <div data-testid="nested-main">Settings content</div>
        </AdaptiveSurface>
      </MobilePaneHost>
    )
    const innerHost = container.querySelectorAll('[data-nova-mobile-pane-host="true"]')[1]!

    fireEvent.mouseDown(innerHost, { button: 0, clientX: 1, clientY: 120 })
    fireEvent.mouseUp(window, { clientX: 80, clientY: 124 })

    expect(screen.getByTestId('inner-pane').closest('[data-state="open"]')).toBeTruthy()
    expect(screen.queryByTestId('outer-pane')).not.toBeInTheDocument()
  })
})

function adaptiveSurface(collapseAt?: number) {
  return (
    <AdaptiveSurface
      collapseAt={collapseAt}
      left={{ id: 'left', title: 'Left', side: 'left', content: <div data-testid="left-pane">Left pane</div> }}
      right={{ id: 'right', title: 'Right', side: 'right', content: <div data-testid="right-pane">Right pane</div> }}
    >
      {({ openLeft, openRight }) => (
        <div data-testid="main-pane">
          <button type="button" onClick={openLeft}>Open left</button>
          <button type="button" onClick={openRight}>Open right</button>
          Main pane
        </div>
      )}
    </AdaptiveSurface>
  )
}

function StatefulMainPane({ onUnmount }: { onUnmount: () => void }) {
  const [count, setCount] = useState(0)
  useEffect(() => onUnmount, [onUnmount])
  return <button type="button" onClick={() => setCount((current) => current + 1)}>Count {count}</button>
}

function setMobileViewport(matches: boolean) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: matches ? 390 : 1280 })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: matches ? 844 : 900 })
  vi.stubGlobal('matchMedia', vi.fn().mockImplementation(() => ({
    matches,
    media: '(max-width: 767px)',
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })))
}

function installMobileViewportHarness(initialMatches: boolean) {
  let matches = initialMatches
  const listeners = new Set<() => void>()
  const mediaQuery = {
    get matches() {
      return matches
    },
    media: '(max-width: 767px)',
    onchange: null,
    addEventListener: (_type: string, listener: () => void) => listeners.add(listener),
    removeEventListener: (_type: string, listener: () => void) => listeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }
  vi.stubGlobal('matchMedia', vi.fn(() => mediaQuery))

  return (nextMatches: boolean) => {
    matches = nextMatches
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: matches ? 390 : 1280 })
    act(() => listeners.forEach((listener) => listener()))
  }
}

function installResizeObserverHarness() {
  let callback: ResizeObserverCallback | null = null
  let observed: Element | null = null
  let instance: ResizeObserver | null = null

  class ResizeObserverHarness {
    constructor(nextCallback: ResizeObserverCallback) {
      callback = nextCallback
      instance = this as ResizeObserver
    }

    observe(target: Element) {
      observed = target
    }

    unobserve() {}
    disconnect() {}
  }

  setResizeObserver(ResizeObserverHarness as typeof ResizeObserver)

  return (width: number) => {
    if (!callback || !observed || !instance) throw new Error('ResizeObserver was not attached')
    const entry = { target: observed, contentRect: { width } as DOMRectReadOnly } as ResizeObserverEntry
    act(() => callback?.([entry], instance as ResizeObserver))
  }
}

function setResizeObserver(value: typeof ResizeObserver | undefined) {
  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    writable: true,
    value,
  })
}

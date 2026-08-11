import { useEffect, useState } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AdaptiveSurface } from './adaptive-surface'

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

  it('lets desktop users resize an enabled right pane', () => {
    render(
      <AdaptiveSurface
        right={{ id: 'config-agent', title: 'Config Agent', side: 'right', content: <div>Agent chat</div> }}
        rightResize={{
          layoutKey: 'test-config-agent-layout',
          label: 'Resize Config Agent',
          defaultSize: '420px',
          minSize: '300px',
          maxSize: '65%',
        }}
      >
        <div>Main content</div>
      </AdaptiveSurface>,
    )

    expect(screen.getByRole('separator', { name: 'Resize Config Agent' })).toBeVisible()
  })

  it('keeps the main slot height-constrained on desktop', () => {
    render(adaptiveSurface())

    expect(screen.getByTestId('main-pane').parentElement).toHaveClass('h-full', 'min-h-0', 'flex-col')
  })

  it('opens mobile panes through controls', async () => {
    setMobileViewport(true)
    const user = userEvent.setup()
    render(adaptiveSurface())

    expect(screen.queryByTestId('left-pane')).not.toBeVisible()
    expect(screen.getByTestId('main-pane').parentElement).toHaveClass('h-full', 'min-h-0', 'flex-col')

    await user.click(screen.getByRole('button', { name: 'Open left' }))
    expect(screen.getByTestId('left-pane').closest('[data-state="open"]')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /关闭|Close/ }))
    await user.click(screen.getByRole('button', { name: 'Open right' }))
    expect(screen.getByTestId('right-pane').closest('[data-state="open"]')).toBeTruthy()
  })

  it('keeps drawer pane content mounted so search state survives close and reopen', async () => {
    setMobileViewport(true)
    const user = userEvent.setup()
    render(
      <AdaptiveSurface
        left={{
          id: 'left',
          title: 'Left',
          side: 'left',
          content: <StatefulSearchInput />,
        }}
      >
        {({ openLeft }) => (
          <div>
            <button type="button" onClick={openLeft}>Open left</button>
          </div>
        )}
      </AdaptiveSurface>,
    )

    await user.click(screen.getByRole('button', { name: 'Open left' }))
    await user.type(screen.getByRole('textbox', { name: 'Search' }), 'persist')
    await user.click(screen.getByRole('button', { name: /关闭|Close/ }))

    expect(screen.queryByRole('textbox', { name: 'Search' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Open left' }))

    expect(screen.getByRole('textbox', { name: 'Search' })).toHaveValue('persist')
  })

  it('collapses panes at its own width on desktop and expands them again', async () => {
    const resize = installResizeObserverHarness()
    const user = userEvent.setup()
    const { container } = render(adaptiveSurface(700))

    expect(container.querySelector('[data-nova-adaptive-container="true"]')).toBeInTheDocument()
    expect(screen.getByTestId('left-pane')).toBeVisible()

    resize(640)

    expect(screen.queryByTestId('left-pane')).not.toBeVisible()
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

    expect(screen.queryByTestId('left-pane')).not.toBeVisible()
    expect(container.querySelector('[data-nova-mobile-pane-host="true"]')).toBeInTheDocument()
  })

  it('falls back to its initial measured width when ResizeObserver is unavailable', () => {
    setResizeObserver(undefined)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ width: 640 } as DOMRect)

    expect(() => render(adaptiveSurface(700))).not.toThrow()
    expect(screen.queryByTestId('left-pane')).not.toBeVisible()
    expect(document.querySelector('[data-nova-mobile-pane-host="true"]')).toBeInTheDocument()
  })

  it('reserves viewport edge drags for system and browser gestures', () => {
    setMobileViewport(true)
    const { container } = render(adaptiveSurface())
    const host = container.querySelector('[data-nova-mobile-pane-host="true"]')!

    fireEvent.pointerDown(host, { pointerId: 1, pointerType: 'touch', button: 0, clientX: 1, clientY: 120 })
    fireEvent.pointerUp(window, { pointerId: 1, pointerType: 'touch', clientX: 80, clientY: 124 })
    fireEvent.mouseDown(host, { button: 0, clientX: 389, clientY: 120 })
    fireEvent.mouseUp(window, { clientX: 320, clientY: 124 })

    expect(screen.queryByTestId('left-pane')).not.toBeVisible()
    expect(screen.queryByTestId('right-pane')).not.toBeVisible()
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

function StatefulSearchInput() {
  const [value, setValue] = useState('')
  return (
    <input
      aria-label="Search"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  )
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

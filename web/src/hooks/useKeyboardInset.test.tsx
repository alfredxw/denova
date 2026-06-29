import { useEffect } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useKeyboardInset } from './useKeyboardInset'

// jsdom has no VisualViewport implementation, so the hook's "unsupported"
// guard would short-circuit. Provide an EventTarget-backed stand-in that the
// hook can subscribe to and that the test can resize/dispatch on.
class MockVisualViewport extends EventTarget {
  height: number
  offsetTop: number
  constructor(height: number, offsetTop = 0) {
    super()
    this.height = height
    this.offsetTop = offsetTop
  }
}

function Probe({ values }: { values: number[] }) {
  const inset = useKeyboardInset()
  useEffect(() => {
    values.push(inset)
  }, [inset, values])
  return <textarea data-testid="field" />
}

describe('useKeyboardInset', () => {
  let viewport: MockVisualViewport
  const originalVisualViewport = Object.getOwnPropertyDescriptor(window, 'visualViewport')
  const originalInnerHeight = Object.getOwnPropertyDescriptor(window, 'innerHeight')

  beforeEach(() => {
    viewport = new MockVisualViewport(800)
    Object.defineProperty(window, 'visualViewport', { configurable: true, value: viewport })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 800 })
  })

  afterEach(() => {
    if (originalVisualViewport) {
      Object.defineProperty(window, 'visualViewport', originalVisualViewport)
    } else {
      // @ts-expect-error redefining a non-standard property in tests
      delete window.visualViewport
    }
    if (originalInnerHeight) {
      Object.defineProperty(window, 'innerHeight', originalInnerHeight)
    }
    const active = document.activeElement as HTMLElement | null
    active?.blur?.()
  })

  it('stays 0 when no editable element is focused', () => {
    const values: number[] = []
    render(<Probe values={values} />)
    act(() => {
      viewport.height = 400
      viewport.dispatchEvent(new Event('resize'))
    })
    expect(values.length).toBeGreaterThan(0)
    expect(values.every((value) => value === 0)).toBe(true)
  })

  it('reports the keyboard inset while an input is focused', () => {
    const values: number[] = []
    render(<Probe values={values} />)
    const field = document.querySelector('textarea') as HTMLTextAreaElement
    act(() => field.focus())
    // Keyboard now covers 400px of the 800px layout viewport.
    act(() => {
      viewport.height = 400
      viewport.dispatchEvent(new Event('resize'))
    })
    expect(values.at(-1)).toBe(400)
  })

  it('accounts for offsetTop and resets to 0 when the input loses focus', () => {
    const values: number[] = []
    render(<Probe values={values} />)
    const field = document.querySelector('textarea') as HTMLTextAreaElement
    act(() => field.focus())
    act(() => {
      viewport.height = 450
      viewport.offsetTop = 50
      viewport.dispatchEvent(new Event('resize'))
    })
    // 800 - 450 - 50 = 300
    expect(values.at(-1)).toBe(300)
    act(() => field.blur())
    expect(values.at(-1)).toBe(0)
  })
})

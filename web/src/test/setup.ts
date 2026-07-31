import '@testing-library/jest-dom/vitest'
import { afterAll, afterEach, beforeAll, vi } from 'vitest'
import { cleanup } from '@testing-library/react'
import { server } from './msw/server'
import { setConfiguredLocale } from '@/i18n'

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

Object.defineProperty(window, 'ResizeObserver', {
  writable: true,
  configurable: true,
  value: ResizeObserverMock,
})

Object.defineProperty(window.navigator, 'languages', {
  configurable: true,
  value: ['zh-CN'],
})

Object.defineProperty(window.navigator, 'language', {
  configurable: true,
  value: 'zh-CN',
})

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  configurable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})

Object.defineProperty(HTMLElement.prototype, 'hasPointerCapture', {
  configurable: true,
  value: () => false,
})

Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
  configurable: true,
  value: () => {},
})

Object.defineProperty(HTMLElement.prototype, 'releasePointerCapture', {
  configurable: true,
  value: () => {},
})

Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
  configurable: true,
  value: () => {},
})

Object.defineProperty(window, 'scrollBy', {
  configurable: true,
  value: () => {},
})

Object.defineProperty(HTMLElement.prototype, 'scrollBy', {
  configurable: true,
  value: () => {},
})

const testDOMRect = {
  width: 1,
  height: 1,
  top: 0,
  left: 0,
  right: 1,
  bottom: 1,
  x: 0,
  y: 0,
  toJSON: () => ({}),
} as DOMRect

const defaultElementBoundingClientRect = HTMLElement.prototype.getBoundingClientRect
const testSeparatorDOMRect = {
  ...testDOMRect,
  top: 10_000,
  left: 10_000,
  right: 10_001,
  bottom: 10_001,
  x: 10_000,
  y: 10_000,
} as DOMRect

// jsdom has no layout and otherwise places every separator at (0, 0), causing the global
// react-resizable-panels pointer handler to consume unrelated user-event clicks.
Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
  configurable: true,
  writable: true,
  value(this: HTMLElement) {
    if (this.hasAttribute('data-separator')) return testSeparatorDOMRect
    return defaultElementBoundingClientRect.call(this)
  },
})

Object.defineProperty(Element.prototype, 'getClientRects', {
  configurable: true,
  value: () => [testDOMRect],
})

Object.defineProperty(Range.prototype, 'getClientRects', {
  configurable: true,
  value: () => [testDOMRect],
})

Object.defineProperty(Range.prototype, 'getBoundingClientRect', {
  configurable: true,
  value: () => testDOMRect,
})

Object.defineProperty(document, 'elementFromPoint', {
  configurable: true,
  value: () => document.body,
})

beforeAll(() => {
  setConfiguredLocale('zh-CN')
  server.listen({ onUnhandledRequest: 'error' })
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
  server.resetHandlers()
})

afterAll(() => {
  server.close()
})

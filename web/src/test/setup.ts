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

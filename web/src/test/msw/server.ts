import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll } from 'vitest'
import { handlers } from './handlers'

export const server = setupServer(...handlers)

// Network mocking is intentionally opt-in. Tests that import this server pay
// for MSW; pure helpers and isolated components do not start an HTTP server.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

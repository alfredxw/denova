import {
  expect,
  test as base,
  type ConsoleMessage,
  type Response,
} from '@playwright/test'

export { expect }
export type { APIRequestContext, Locator, Page } from '@playwright/test'

interface BrowserDiagnostics {
  /** Allow one intentional browser diagnostic in a test by matching its formatted text. */
  allow: (pattern: RegExp) => void
}

interface E2EFixtures {
  browserDiagnostics: BrowserDiagnostics
}

interface BrowserDiagnostic {
  kind: 'console.error' | 'pageerror' | 'http.5xx'
  text: string
}

const knownExpectedBrowserDiagnostics = [
  // A missing optional Book cover is represented by this endpoint as 404.
  /^console\.error: Failed to load resource:.*404 \(Not Found\).*\/api\/books\/cover\?path=/,
  /^console\.error: Failed to load conversation history.*Failed to fetch/,
  /^console\.error: Failed to load conversation history.*Writing history reload was superseded before it could become authoritative/,
]

/** Every browser-backed test fails on uncaught errors, console errors, and unexpected 5xx responses. */
export const test = base.extend<E2EFixtures>({
  browserDiagnostics: [async ({ page }, use) => {
    const diagnostics: BrowserDiagnostic[] = []
    const allowed = [...knownExpectedBrowserDiagnostics]
    const recordConsoleError = (message: ConsoleMessage) => {
      if (message.type() !== 'error') return
      const location = message.location()
      const suffix = location.url ? ` (${location.url}:${(location.lineNumber ?? 0) + 1})` : ''
      diagnostics.push({ kind: 'console.error', text: `${message.text()}${suffix}` })
    }
    const recordPageError = (error: Error) => {
      diagnostics.push({ kind: 'pageerror', text: error.stack || error.message })
    }
    const recordServerError = (response: Response) => {
      if (response.status() < 500) return
      diagnostics.push({
        kind: 'http.5xx',
        text: `${response.request().method()} ${response.url()} returned ${response.status()}`,
      })
    }

    page.on('console', recordConsoleError)
    page.on('pageerror', recordPageError)
    page.on('response', recordServerError)
    await use({ allow: (pattern) => allowed.push(pattern) })
    page.off('console', recordConsoleError)
    page.off('pageerror', recordPageError)
    page.off('response', recordServerError)

    const unexpected = diagnostics
      .map((diagnostic) => `${diagnostic.kind}: ${diagnostic.text}`)
      .filter((diagnostic) => !allowed.some((pattern) => matches(pattern, diagnostic)))
    expect(unexpected, `Unexpected browser diagnostics:\n${unexpected.join('\n')}`).toEqual([])
  }, { auto: true }],
})

function matches(pattern: RegExp, value: string): boolean {
  pattern.lastIndex = 0
  return pattern.test(value)
}

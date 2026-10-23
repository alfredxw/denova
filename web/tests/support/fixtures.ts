import {
  expect,
  test as base,
  type ConsoleMessage,
  type Response,
} from '@playwright/test'
import { createSettingsMergePatch } from '../../src/features/settings/merge-patch'
import type { Settings } from '../../src/features/settings/types'

export { expect }
export type { APIRequestContext, Locator, Page } from '@playwright/test'

interface BrowserDiagnostics {
  /** Allow one intentional browser diagnostic in a test by matching its formatted text. */
  allow: (pattern: RegExp) => void
}

interface E2EFixtures {
  browserDiagnostics: BrowserDiagnostics
  _sharedState: void
}

interface BrowserDiagnostic {
  // `http.untyped409` reports a conflict that reused a stream 409 without the
  // typed rehydration code the tolerant path expects.
  kind: 'console.error' | 'pageerror' | 'http.5xx' | 'http.untyped409' | 'network.external'
  text: string
  url?: string
}

const knownExpectedBrowserDiagnostics = [
  // A missing optional Book cover is represented by this endpoint as 404.
  /^console\.error: Failed to load resource:.*404 \(Not Found\).*\/api\/books\/cover\?path=/,
  /^console\.error: Failed to load conversation history.*Failed to fetch/,
  /^console\.error: Failed to load conversation history.*Writing history reload was superseded before it could become authoritative/,
]

/** Every browser-backed test fails on uncaught errors, console errors, and unexpected 5xx responses. */
export const test = base.extend<E2EFixtures>({
  // Restore shared state after closing the browser so late UI saves cannot
  // leak into the next test. Project-owned settings stay with their unique Project.
  _sharedState: [async ({ context, request }, use) => {
    const beforeResponse = await request.get('/api/settings')
    expect(beforeResponse.ok(), await beforeResponse.text()).toBe(true)
    const before = (await beforeResponse.json()).user as Settings
    const beforeCatalog = await request.get('/api/platform/manage/catalog')
    expect(beforeCatalog.ok(), await beforeCatalog.text()).toBe(true)
    const plugins = new Map<string, { enabled: boolean; removed: boolean }>((await beforeCatalog.json())
      .filter((item: { kind: string }) => item.kind === 'plugin')
      .map((item: { id: string; enabled: boolean; removed?: boolean }) => [item.id, { enabled: Boolean(item.enabled), removed: Boolean(item.removed) }]))
    await use()
    await context.close()
    // Disabled installations still reserve their tool names. Restore package
    // availability and remove each journey's new plugins after its UI closes.
    const afterCatalog = await request.get('/api/platform/manage/catalog')
    expect(afterCatalog.ok(), await afterCatalog.text()).toBe(true)
    for (const item of await afterCatalog.json()) {
      if (item.kind !== 'plugin') continue
      const original = plugins.get(item.id) ?? { enabled: false, removed: true }
      if (Boolean(item.enabled) === original.enabled && Boolean(item.removed) === original.removed) continue
      const restored = await request.patch(`/api/platform/manage/packages/plugin/${item.id}`, { data: original })
      expect(restored.ok(), await restored.text()).toBe(true)
    }
    const afterResponse = await request.get('/api/settings')
    expect(afterResponse.ok(), await afterResponse.text()).toBe(true)
    const after = await afterResponse.json()
    const changes = createSettingsMergePatch(sharedSettings(after.user), sharedSettings(before))
    if (Object.keys(changes).length > 0) {
      const restored = await request.patch('/api/settings', { data: {
        layer: 'user', base_revision: after.revisions.user, changes,
      } })
      expect(restored.ok(), await restored.text()).toBe(true)
      expect(sharedSettings((await restored.json()).user)).toEqual(sharedSettings(before))
    }
  }, { auto: true }],
  browserDiagnostics: [async ({ page, context, _sharedState }, use, testInfo) => {
    const diagnostics: BrowserDiagnostic[] = []
    const allowed = [...knownExpectedBrowserDiagnostics]
    const staleStreamURLs = new Map<string, boolean>()
    const indexConflicts = new Map<string, Array<{ typed: boolean; recovered: boolean }>>()
    const responseChecks: Promise<void>[] = []
    // Page-level mocks take precedence; any unmocked public dependency fails
    // immediately instead of depending on CDN/DNS availability during a run.
    await context.route(/^https?:\/\//, async route => {
      const url = new URL(route.request().url())
      if (['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) {
        await route.continue()
        return
      }
      diagnostics.push({ kind: 'network.external', text: `${route.request().method()} ${url.href}` })
      await route.abort('blockedbyclient')
    })
    const recordConsoleError = (message: ConsoleMessage) => {
      if (message.type() !== 'error') return
      const location = message.location()
      const suffix = location.url ? ` (${location.url}:${(location.lineNumber ?? 0) + 1})` : ''
      diagnostics.push({ kind: 'console.error', text: `${message.text()}${suffix}`, url: location.url })
    }
    const recordPageError = (error: Error) => {
      diagnostics.push({ kind: 'pageerror', text: error.stack || error.message })
    }
    const recordServerError = (response: Response) => {
      const url = response.url()
      const pathname = new URL(url).pathname
      // A display task may settle between the active projection and attachment.
      // The console error is recorded synchronously while the typed body arrives
      // asynchronously, so the expected 409 is accepted here and rejected again
      // once the body proves it is not a typed rehydration request. Deciding from
      // the body alone raced the console diagnostic and failed correct journeys.
      if (response.status() === 409 && response.request().method() === 'GET'
        && /^\/api\/(?:projects\/[^/]+\/agent-chat\/chat|chat|interactive\/chat)\/stream$/.test(pathname)) {
        staleStreamURLs.set(url, true)
        responseChecks.push(response.json().then((body) => {
          if (body?.code !== 'agent_runtime.rehydrate_required') {
            staleStreamURLs.set(url, false)
            diagnostics.push({
              kind: 'http.untyped409',
              text: `GET ${url} returned 409 without agent_runtime.rehydrate_required`,
            })
          }
        }).catch(() => {
          // An aborted fetch can lose its body before teardown reads it. The
          // journey assertions still cover recovery, so keep the expected 409
          // instead of failing on the missing body itself.
        }))
      }
      // Index and item edits share a file revision. A typed conflict is expected
      // only when a later successful index write confirms that recovery finished.
      if (response.request().method() === 'PUT'
        && /^\/api\/projects\/[^/]+\/book\/lore\/index$/.test(pathname)) {
        if (response.status() === 409) {
          const conflict = { typed: false, recovered: false }
          const conflicts = indexConflicts.get(response.url()) ?? []
          conflicts.push(conflict)
          indexConflicts.set(response.url(), conflicts)
          responseChecks.push(response.json().then(body => {
            conflict.typed = body?.code === 'api.resource.revisionConflict' || body?.code === 'revision_conflict'
          }).catch(() => { conflict.typed = false }))
        } else if (response.ok()) {
          for (const conflict of indexConflicts.get(response.url()) ?? []) conflict.recovered = true
        }
      }
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
    // Finish in-flight mock callbacks before Playwright closes the page and
    // its request context. Rapid navigation can leave a settings fetch pending.
    await page.unrouteAll({ behavior: 'wait' })
    // Reading a response body can register further checks, so drain until a full
    // pass adds nothing instead of waiting for one snapshot of the array.
    for (let seen = -1; seen !== responseChecks.length;) {
      seen = responseChecks.length
      await Promise.allSettled(responseChecks)
    }
    page.off('console', recordConsoleError)
    page.off('pageerror', recordPageError)
    page.off('response', recordServerError)
    if (testInfo.status !== testInfo.expectedStatus || diagnostics.length > 0) {
      await testInfo.attach('browser-diagnostics', {
        body: JSON.stringify(diagnostics, null, 2), contentType: 'application/json',
      })
    }

    const recoveredIndexURLs = new Set([...indexConflicts]
      .filter(([, conflicts]) => conflicts.every(conflict => conflict.typed && conflict.recovered))
      .map(([url]) => url))

    const unexpected = diagnostics
      .filter((diagnostic) => !(diagnostic.kind === 'console.error'
        && (staleStreamURLs.get(diagnostic.url ?? '') === true || recoveredIndexURLs.has(diagnostic.url ?? ''))
        && diagnostic.text.startsWith('Failed to load resource: the server responded with a status of 409 (Conflict)')))
      .map((diagnostic) => `${diagnostic.kind}: ${diagnostic.text}`)
      .filter((diagnostic) => !allowed.some((pattern) => matches(pattern, diagnostic)))
    expect(unexpected, `Unexpected browser diagnostics:\n${unexpected.join('\n')}`).toEqual([])
  }, { auto: true }],
})

function sharedSettings(settings: Settings): Settings {
  // Host authentication is intentionally outside this snapshot.
  // Restore every mutable default, including media, tools and context policies.
  const hostKeys = new Set(['remote_access_username', 'remote_access_password', 'remote_access_password_set', 'allow_lan_access', 'backend_port', 'frontend_port'])
  return Object.fromEntries(Object.entries(settings).filter(([key]) => !hostKeys.has(key)))
}

function matches(pattern: RegExp, value: string): boolean {
  pattern.lastIndex = 0
  return pattern.test(value)
}

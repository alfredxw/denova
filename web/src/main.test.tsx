import { beforeEach, describe, expect, it, vi } from 'vitest'

const startupMocks = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
  installGlobalRuntimeLoggers: vi.fn(),
  recordRuntimeLog: vi.fn(),
  render: vi.fn(),
  scheduleWhiteScreenCheck: vi.fn(),
}))

vi.mock('react-dom/client', () => ({
  createRoot: () => ({ render: startupMocks.render }),
}))

// Keep this startup contract test focused on eager mounting. Loading the real
// provider/component graph turns a one-assertion unit test into a bundler
// integration test and can exceed the repository's one-second test budget.
vi.mock('@tanstack/react-query', () => ({ QueryClientProvider: ({ children }: { children: unknown }) => children }))
vi.mock('next-themes', () => ({ ThemeProvider: ({ children }: { children: unknown }) => children }))
vi.mock('@/components/RuntimeErrorBoundary', () => ({ RuntimeErrorBoundary: ({ children }: { children: unknown }) => children }))
vi.mock('@/components/ui/sonner', () => ({ Toaster: () => null }))
vi.mock('@/components/ui/tooltip', () => ({ TooltipProvider: ({ children }: { children: unknown }) => children }))
vi.mock('@/lib/query-client', () => ({ queryClient: {} }))
vi.mock('@/i18n', () => ({ setConfiguredLocale: vi.fn() }))
vi.mock('@/features/settings/font-variables', () => ({
  applyFontSettings: vi.fn(),
  fontSettingsFromEffective: vi.fn(() => ({})),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: startupMocks.fetchSettings,
}))

vi.mock('@/lib/runtimeLog', () => ({
  installGlobalRuntimeLoggers: startupMocks.installGlobalRuntimeLoggers,
  recordRuntimeLog: startupMocks.recordRuntimeLog,
  scheduleWhiteScreenCheck: startupMocks.scheduleWhiteScreenCheck,
}))

vi.mock('./App', () => ({ default: () => null }))

describe('application startup', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    document.body.innerHTML = '<div id="root"></div>'
  })

  it('mounts the application shell while remote settings are still pending', async () => {
    startupMocks.fetchSettings.mockReturnValue(new Promise(() => {}))

    await import('./main')

    expect(startupMocks.render).toHaveBeenCalledTimes(1)
    expect(startupMocks.scheduleWhiteScreenCheck).toHaveBeenCalledWith(document.getElementById('root'))
  })
})

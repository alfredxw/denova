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

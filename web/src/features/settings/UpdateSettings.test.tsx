import { act, renderHook, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { installUpdateStream } from './api'
import { useUpdateSettings } from './UpdateSettings'

vi.mock('./api', () => ({
  applyUpdate: vi.fn(), checkForUpdate: vi.fn(), installUpdateStream: vi.fn(), uploadUpdate: vi.fn(),
}))

it('retains the install stream cause and correlation when returning to idle', async () => {
  vi.mocked(installUpdateStream).mockResolvedValue(new ReadableStream({ start(controller) {
    controller.enqueue({ event: 'error', data: JSON.stringify({
      message: 'Install failed', code: 'api.update.installFailed', request_id: 'update-request',
      details: { detail: 'download checksum mismatch', operation: 'update.install', backend_version: '0.5.0', platform: 'windows/amd64' },
    }) })
    controller.close()
  } }))
  const { result } = renderHook(() => useUpdateSettings({ autoCheckEnabled: false }))
  act(() => result.current.onInstall())
  await waitFor(() => expect(result.current.error).toContain('download checksum mismatch'))
  expect(result.current.error).toContain('update-request')
  expect(result.current.error).toContain('api.update.installFailed')
  expect(result.current.operation).toBe('idle')
  expect(result.current.installResult).toBeNull()
})

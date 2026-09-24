import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { InstalledExtensionSettings } from './InstalledExtensionSettings'
import { management, type CatalogEntry, type Release } from './api'

vi.mock('./api', async original => ({ ...await original<typeof import('./api')>(), management: vi.fn() }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en-US' } }) }))
afterEach(() => { cleanup(); vi.resetAllMocks() })

it('keeps settings and permissions visible and saves their drafts independently', async () => {
  const release: Release = {
    ref: { package: { kind: 'game', id: 'test.game' }, releaseId: 'r1' }, digest: 'digest', installedAt: '',
    manifest: { id: 'test.game', version: '1.0.0', apiMajor: 1, name: { 'en-US': 'Game', 'zh-CN': '游戏' }, settings: { schema: 'settings.json', defaults: 'defaults.toml' }, permissions: { required: ['gameData'], optional: ['tools.invoke'] } },
  }
  const item: CatalogEntry = { kind: 'game', id: 'test.game', enabled: true, currentRelease: 'r1', releases: [release], grants: ['gameData'] }
  const doc = { releaseId: 'r1', revision: 'original', values: { hints: true }, overrides: {}, form: { schema: { type: 'object', properties: { hints: { type: 'boolean', title: 'Show hints' } } }, defaults: { hints: true }, uiSchema: {} } }
  vi.mocked(management).mockResolvedValueOnce(doc).mockResolvedValueOnce({ ok: true }).mockResolvedValueOnce({ ...doc, revision: 'saved', values: { hints: false }, overrides: { hints: false } })
  const onDirty = vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><InstalledExtensionSettings item={item} release={release} active onDirtyChange={onDirty} onSaved={vi.fn()} /></QueryClientProvider>)
  const setting = await screen.findByRole('switch', { name: 'Show hints' })
  const permission = screen.getByRole('switch', { name: 'platform.permission.tools.invoke' })
  expect(setting).toBeVisible()
  expect(permission).toBeVisible()
  expect(screen.queryByRole('tab')).not.toBeInTheDocument()
  fireEvent.click(setting)
  fireEvent.click(permission)
  fireEvent.click(screen.getByRole('button', { name: 'platform.settings.savePermissions' }))
  await waitFor(() => expect(management).toHaveBeenCalledWith('/packages/game/test.game/permissions', 'PUT', { releaseId: 'r1', grants: ['gameData', 'tools.invoke'] }))
  expect(setting).not.toBeChecked()
  expect(onDirty).toHaveBeenLastCalledWith(true)
  fireEvent.click(screen.getByRole('button', { name: 'platform.settings.save' }))
  await waitFor(() => expect(onDirty).toHaveBeenLastCalledWith(false))
  expect(management).toHaveBeenCalledWith('/packages/game/test.game/settings?locale=en-US', 'PUT', { releaseId: 'r1', expectedRevision: 'original', overrides: { hints: false } })
})

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client/client'
import { ExtensionConfiguration } from './ExtensionConfiguration'
import { configurationOverrides } from './ConfigurationForm'
import { management, type ConfigurationDocument } from './api'

vi.mock('./api', async importOriginal => ({ ...await importOriginal<typeof import('./api')>(), management: vi.fn() }))
const language = vi.hoisted(() => ({ value: 'en-US' }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: language.value } }) }))
afterEach(() => { cleanup(); vi.resetAllMocks(); language.value = 'en-US' })

const document: ConfigurationDocument = {
  revision: 'original', releaseId: 'release', overrides: {}, values: { enabled: false },
  form: { schema: { type: 'object', properties: { enabled: { type: 'boolean', title: 'Enable feature', description: 'Used at the next start.' } } }, defaults: { enabled: false }, uiSchema: {} },
}

function editor(onSaved = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><ExtensionConfiguration kind="plugin" packageId="test.example" releaseId="release" onSaved={onSaved} /></QueryClientProvider>)
  return onSaved
}

describe('extension configuration', () => {
  it('saves only changed values with the observed revision and shows boolean help', async () => {
    vi.mocked(management).mockResolvedValue(document)
    const saved = editor()
    fireEvent.click(await screen.findByRole('switch', { name: 'Enable feature' }))
    expect(screen.getByText('Used at the next start.')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'platform.settings.save' }))
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(management).toHaveBeenLastCalledWith('/packages/plugin/test.example/settings?locale=en-US', 'PUT', { releaseId: 'release', expectedRevision: 'original', overrides: { enabled: true } })
  })

  it('repairs invalid persisted settings with a form and preserves rejected drafts', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(management).mockResolvedValueOnce({ ...document, problem: { messageKey: 'platform.errors.INVALID_TOML' } })
      .mockRejectedValue(new APIError('conflict', { status: 409, payload: { messageKey: 'platform.errors.DOCUMENT_CONFLICT' } }))
    const saved = editor()
    fireEvent.click(await screen.findByRole('switch', { name: 'Enable feature' }))
    fireEvent.click(screen.getByRole('button', { name: 'platform.settings.save' }))
    await screen.findByText('platform.errors.DOCUMENT_CONFLICT')
    expect(screen.getByRole('switch', { name: 'Enable feature' })).toBeChecked()
    expect(saved).not.toHaveBeenCalled()
  })

  it('edits a labelled range with the keyboard and saves its numeric value', async () => {
    vi.mocked(management).mockResolvedValue({ ...document, values: { volume: 60 }, form: {
      schema: { type: 'object', properties: { volume: { type: 'integer', title: 'Music volume', minimum: 0, maximum: 100, multipleOf: 5 } } },
      defaults: { volume: 60 }, uiSchema: { volume: { 'ui:widget': 'range', 'ui:options': { suffix: '%' } } },
    } })
    const saved = editor()
    const slider = await screen.findByRole('slider', { name: 'Music volume' })
    expect(slider).toHaveAttribute('aria-valuetext', '60%')
    fireEvent.keyDown(slider, { key: 'ArrowRight' })
    expect(slider).toHaveAttribute('aria-valuenow', '65')
    fireEvent.click(screen.getByRole('button', { name: 'platform.settings.save' }))
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(management).toHaveBeenLastCalledWith('/packages/plugin/test.example/settings?locale=en-US', 'PUT', {
      releaseId: 'release', expectedRevision: 'original', overrides: { volume: 65 },
    })
  })

  it('uses each saved revision for the next inline save and resets the dirty state', async () => {
    const firstSave = { ...document, revision: 'saved-once', values: { enabled: true }, overrides: { enabled: true } }
    vi.mocked(management).mockResolvedValueOnce(document).mockResolvedValueOnce(firstSave).mockResolvedValueOnce({ ...document, revision: 'saved-twice' })
    const saved = editor()
    const toggle = await screen.findByRole('switch', { name: 'Enable feature' })
    const save = screen.getByRole('button', { name: 'platform.settings.save' })
    fireEvent.click(toggle)
    fireEvent.click(save)
    await waitFor(() => expect(saved).toHaveBeenCalledOnce())
    expect(toggle).toBeChecked()
    expect(save).toBeDisabled()
    fireEvent.click(toggle)
    fireEvent.click(save)
    await waitFor(() => expect(saved).toHaveBeenCalledTimes(2))
    expect(management).toHaveBeenLastCalledWith('/packages/plugin/test.example/settings?locale=en-US', 'PUT', { releaseId: 'release', expectedRevision: 'saved-once', overrides: {} })
    expect(save).toBeDisabled()
  })

  it('preserves the draft and its revision across background refreshes', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    vi.mocked(management).mockResolvedValue(document)
    render(<QueryClientProvider client={client}><ExtensionConfiguration kind="plugin" packageId="test.example" releaseId="release" onSaved={vi.fn()} /></QueryClientProvider>)
    const toggle = await screen.findByRole('switch', { name: 'Enable feature' })
    fireEvent.click(toggle)
    await act(async () => { client.setQueryData(['platform', 'settings', 'plugin', 'test.example', 'release', 'en-US'], { ...document, revision: 'external-change' }) })
    await waitFor(() => expect(toggle).toBeChecked())
    fireEvent.click(screen.getByRole('button', { name: 'platform.settings.save' }))
    await waitFor(() => expect(management).toHaveBeenLastCalledWith('/packages/plugin/test.example/settings?locale=en-US', 'PUT', { releaseId: 'release', expectedRevision: 'original', overrides: { enabled: true } }))
  })

  it('retains explicit empty objects and replaces arrays without copying inherited tables', () => {
    expect(configurationOverrides({ display: { scale: 2, theme: 'dark' }, tags: [], optional: {} }, { display: { scale: 1, theme: 'dark' }, tags: ['old'] }))
      .toEqual({ display: { scale: 2 }, tags: [], optional: {} })
  })

  it('keeps the draft across languages but never edits a different release while loading', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    vi.mocked(management).mockResolvedValueOnce(document).mockImplementation(() => new Promise(() => {}))
    const view = (releaseId: string) => <QueryClientProvider client={client}><ExtensionConfiguration kind="plugin" packageId="test.example" releaseId={releaseId} onSaved={vi.fn()} /></QueryClientProvider>
    const { rerender } = render(view('release'))
    fireEvent.click(await screen.findByRole('switch', { name: 'Enable feature' }))
    language.value = 'zh-CN'
    rerender(view('release'))
    expect(screen.getByRole('switch', { name: 'Enable feature' })).toBeChecked()
    expect(screen.getByRole('button', { name: 'platform.settings.save' })).toBeEnabled()
    rerender(view('new-release'))
    expect(screen.queryByRole('button', { name: 'platform.settings.save' })).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('common.loading')
  })
})

import { StrictMode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { MarketEntryDetail } from './MarketEntryDetail'
import { discardPreview, previewSource, type MarketEntry, type Preview } from './api'
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), previewSource: vi.fn(), discardPreview: vi.fn() }))
const detail: MarketEntry = { id: 'mixed', name: { 'zh-CN': '混合包' }, description: { 'zh-CN': '写作与游戏资源' }, author: 'Author', kinds: ['preset.image'], tags: ['writing'], format: 'denova.resource-pack', updated_at: '2026-09-25', source: { kind: 'github', url: 'https://github.com/author/repo', ref: 'main', path: 'resources/mixed' } }
const preview: Preview = { preview_id: 'p', source: detail.source, expires_at: '2030-01-01', candidates: [{ candidate_id: 'c', package: { id: 'p', name: 'Package' }, format: 'denova.resource-pack', resources: [{ id: 'image', kind: 'preset.image', name: 'Ink', path: 'ink.json', digest: 'd' }] }] }
it('loads once on entry, exposes source beneath the title, and preserves the frozen selection', async () => {
  vi.mocked(previewSource).mockReset().mockResolvedValue(preview)
  vi.mocked(discardPreview).mockClear()
  const onImport = vi.fn()
  const view = render(<StrictMode><MarketEntryDetail detail={detail} acquired={0} onBack={vi.fn()} onManage={vi.fn()} onImport={onImport} /></StrictMode>)
  expect(await screen.findByRole('checkbox', { name: 'Ink' })).toBeChecked()
  expect(previewSource).toHaveBeenCalledOnce()
  expect(screen.getByRole('link', { name: 'github.com/author/repo' })).toBeVisible()
  expect(screen.getByText('· resources/mixed')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '导入所选 1 项' }))
  expect(onImport).toHaveBeenCalledWith(preview, { candidateID: 'c', resourceIDs: ['image'] })
  view.unmount()
  expect(discardPreview).toHaveBeenCalledExactlyOnceWith(preview)
})
it('releases an automatic download that completes after leaving the page', async () => {
  let resolve!: (preview: Preview) => void
  vi.mocked(previewSource).mockReset().mockReturnValue(new Promise(done => { resolve = done }))
  vi.mocked(discardPreview).mockClear()
  const view = render(<MarketEntryDetail detail={detail} acquired={0} onBack={vi.fn()} onManage={vi.fn()} onImport={vi.fn()} />)
  view.unmount()
  await act(async () => resolve(preview))
  await waitFor(() => expect(discardPreview).toHaveBeenCalledExactlyOnceWith(preview))
})

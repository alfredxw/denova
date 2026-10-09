import { StrictMode, type ReactElement } from 'react'
import { act, fireEvent, render as renderComponent, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { createBook } from '@/lib/api'
import { notifyLoreUpdated } from '@/features/lore/events'
import { fetchProjectSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import { BookCreationProvider } from '@/components/workbench/book-creation'
import { ImportDialog } from './ImportDialog'
import { discardPreview, previewSource, exchange, type Preview } from './api'
vi.mock('@/lib/api', () => ({ getBooks: vi.fn().mockResolvedValue([]), createBook: vi.fn() }))
vi.mock('@/features/lore/events', () => ({ notifyLoreUpdated: vi.fn() }))
vi.mock('@/features/settings/api', () => ({ fetchProjectSettings: vi.fn(), invalidateSettingsCache: vi.fn() }))
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), exchange: vi.fn(), previewSource: vi.fn(), discardPreview: vi.fn() }))
const beforeCreate = vi.fn<() => Promise<boolean>>()
const onCreated = vi.fn<(workspace: string) => Promise<void>>()
const render = (ui: ReactElement) => renderComponent(ui, {
  wrapper: ({ children }) => <BookCreationProvider value={{ beforeCreate, onCreated }}>{children}</BookCreationProvider>,
})
const preview: Preview = {
  preview_id: 'frozen', source: { kind: 'github', url: 'https://github.com/test/package' }, expires_at: '2030-01-01',
  candidates: [{ candidate_id: 'first', package: { id: 'first', name: 'Other package' }, format: 'skill', resources: [] }, {
    candidate_id: 'second', package: { id: 'second', name: 'Chosen package' }, format: 'denova.resource-pack', resources: [
      { id: 'lore', kind: 'lore.collection', name: 'Not selected', path: 'lore.json', digest: '1' },
      { id: 'image', kind: 'preset.image', name: 'Selected image', path: 'image.json', requires: ['style'], digest: '2' },
      { id: 'style', kind: 'style.reference', name: 'Required style', path: 'style.md', digest: '3' },
    ],
  }],
}
beforeEach(() => {
  vi.mocked(fetchProjectSettings).mockResolvedValue({ workspace: {}, effective: {}, user: {} } as LayeredSettings)
  vi.mocked(exchange).mockReset()
  vi.mocked(previewSource).mockReset()
  vi.mocked(discardPreview).mockReset()
  vi.mocked(createBook).mockReset()
  vi.mocked(notifyLoreUpdated).mockReset()
  beforeCreate.mockReset().mockResolvedValue(true)
  onCreated.mockReset().mockResolvedValue(undefined)
})

it('only adopts reviewed fields and protects existing book defaults', async () => {
  const user = userEvent.setup()
  vi.mocked(fetchProjectSettings).mockResolvedValue({ workspace: { game_creation_defaults: { image_preset_id: 'mine' } }, effective: {}, user: {} } as LayeredSettings)
  vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Chosen package' } } })
  const candidate = { ...preview.candidates[1], game_defaults: { image_preset_id: 'image', default_background: { resource_id: 'lore', item_id: 'world', asset_path: 'background.png' } } }
  render(<ImportDialog preview={{ ...preview, candidates: [candidate] }} projectID="target" onClose={vi.fn()} onInstalled={vi.fn()} />)
  const adopt = await screen.findByRole('checkbox', { name: '用作本书的新故事默认配置' })
  await waitFor(() => expect(adopt).toBeEnabled())
  expect(adopt).not.toBeChecked()
  await user.click(adopt)
  expect(screen.getByRole('checkbox', { name: '插图方案 · Selected image' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: '默认背景 · Not selected' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: '生成安装计划' }))
  await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({ project_id: 'target', game_defaults_fields: ['default_background'] })))
})

it('preselects available recommendations only when creating a new book', async () => {
  const user = userEvent.setup()
  const candidate = { ...preview.candidates[1], game_defaults: { image_preset_id: 'image', default_background: { resource_id: 'lore', item_id: 'world', asset_path: 'background.png' } } }
  render(<ImportDialog preview={{ ...preview, candidates: [candidate] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  expect(screen.getByRole('checkbox', { name: '默认背景 · Not selected' })).toBeChecked()
  expect(screen.getByRole('checkbox', { name: '插图方案 · Selected image' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: '清空选择' }))
  expect(screen.getByRole('checkbox', { name: '插图方案 · Selected image' })).toBeDisabled()
})
it('plans exactly the selected candidate and resources, while displaying its dependency', async () => {
  vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Chosen package' } } })
  render(<ImportDialog preview={{ ...preview, candidates: [preview.candidates[1]] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '清空选择' }))
  fireEvent.click(screen.getByRole('checkbox', { name: 'Selected image' }))
  expect(screen.getByText(/Selected image/)).toBeVisible()
  expect(screen.getByText(/Required style/)).toBeVisible()
  expect(screen.queryByRole('checkbox', { name: 'Not selected' })).not.toBeInTheDocument()
  expect(screen.queryByText('目标作品')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('来源链接')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '生成安装计划' }))
  await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({ preview_id: 'frozen', candidate_id: 'second', resources: ['image'] })))
})
it('releases the owned preview on cancel', () => {
  const onClose = vi.fn()
  render(<ImportDialog preview={{ ...preview, candidates: [preview.candidates[1]] }} onClose={onClose} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  expect(onClose).toHaveBeenCalledOnce()
  expect(discardPreview).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ preview_id: 'frozen' }))
  expect(exchange).not.toHaveBeenCalled()
})

it('submits a complete selected collection without adding another collection or openings', async () => {
  vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Story package' } } })
  const groupedPreview: Preview = { ...preview, candidates: [{
    candidate_id: 'story', package: { id: 'story', name: 'Story package' }, format: 'denova.resource-pack', resources: [
      { id: 'harbor', kind: 'lore.collection', name: 'Harbor', path: 'harbor.json', digest: '1' },
      { id: 'island', kind: 'lore.collection', name: 'Island', path: 'island.json', digest: '2' },
      { id: 'dawn', kind: 'game.openings', name: 'Dawn', path: 'dawn.json', digest: '3' },
      { id: 'dusk', kind: 'game.openings', name: 'Dusk', path: 'dusk.json', digest: '4' },
    ],
  }] }
  render(<ImportDialog preview={groupedPreview} projectID="target" onClose={vi.fn()} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '清空选择' }))
  fireEvent.change(screen.getByRole('textbox', { name: '搜索名称、描述或类型' }), { target: { value: 'Harbor' } })
  fireEvent.click(screen.getByRole('checkbox', { name: '全选资料的搜索结果' }))
  fireEvent.click(screen.getByRole('button', { name: '生成安装计划' }))
  await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({
    preview_id: 'frozen', candidate_id: 'story', resources: ['harbor'], project_id: 'target',
  })))
})

it('creates a book for lore and reuses it when planning fails', async () => {
  const user = userEvent.setup()
  vi.mocked(createBook).mockResolvedValue({ project_id: 'new-project', workspace: '/books/new', book_meta: { title: 'New world' } } as Awaited<ReturnType<typeof createBook>>)
  vi.mocked(exchange).mockRejectedValueOnce(new Error('Plan failed')).mockResolvedValueOnce({ plan_id: 'plan', items: [], installation: { package: { name: 'Chosen package' } } })
  render(<ImportDialog preview={{ ...preview, candidates: [{ ...preview.candidates[1], resources: [preview.candidates[1].resources[0]] }] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  expect(screen.queryByRole('combobox', { name: '目标作品' })).not.toBeInTheDocument()
  const review = screen.getByRole('button', { name: '创建书籍并生成计划' })
  expect(review).toBeDisabled()
  expect(createBook).not.toHaveBeenCalled()
  await user.type(screen.getByRole('textbox', { name: '新书名称' }), '  New world  ')
  await user.click(review)
  await screen.findByText('Plan failed')
  expect(createBook).toHaveBeenCalledWith('New world')
  expect(beforeCreate).toHaveBeenCalledOnce()
  expect(onCreated).toHaveBeenCalledExactlyOnceWith('/books/new')
  expect(beforeCreate.mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(createBook).mock.invocationCallOrder[0])
  expect(onCreated.mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(exchange).mock.invocationCallOrder[0])
  expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({ project_id: 'new-project', resources: ['lore'] }))
  await user.click(screen.getByRole('button', { name: '生成安装计划' }))
  await screen.findByText('确认安装计划')
  expect(createBook).toHaveBeenCalledTimes(1)
  expect(onCreated).toHaveBeenCalledTimes(1)
})

it('refreshes the opened book catalog only after its import succeeds', async () => {
  const user = userEvent.setup()
  vi.mocked(createBook).mockResolvedValue({ project_id: 'new-project', workspace: '/books/new', book_meta: { title: 'New world' } } as Awaited<ReturnType<typeof createBook>>)
  const installed = { project_id: 'new-project', bindings: [
    { local: { kind: 'lore.collection', project_id: 'new-project' } },
    { local: { kind: 'lore.collection', project_id: 'new-project' } },
  ] }
  vi.mocked(exchange)
    .mockResolvedValueOnce({ plan_id: 'plan', items: [], installation: { package: { name: 'Story package' } } })
    .mockRejectedValueOnce(new Error('Import failed'))
    .mockResolvedValueOnce(installed)
  const onInstalled = vi.fn()
  render(<ImportDialog preview={{ ...preview, candidates: [{ ...preview.candidates[1], resources: [preview.candidates[1].resources[0]] }] }} onClose={vi.fn()} onInstalled={onInstalled} />)
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  await user.type(screen.getByRole('textbox', { name: '新书名称' }), 'New world')
  await user.click(screen.getByRole('button', { name: '创建书籍并生成计划' }))
  await screen.findByText('确认安装计划')
  expect(onCreated).toHaveBeenCalledExactlyOnceWith('/books/new')
  expect(notifyLoreUpdated).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: '确认添加' }))
  await screen.findByText('Import failed')
  expect(notifyLoreUpdated).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: '确认添加' }))
  await waitFor(() => expect(onInstalled).toHaveBeenCalledWith(installed))
  expect(notifyLoreUpdated).toHaveBeenCalledExactlyOnceWith({ projectId: 'new-project', source: 'resource-import' })
})

it('does not create or plan a book when workspace drafts cannot be saved', async () => {
  const user = userEvent.setup()
  beforeCreate.mockResolvedValue(false)
  render(<ImportDialog preview={{ ...preview, candidates: [{ ...preview.candidates[1], resources: [preview.candidates[1].resources[0]] }] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  await user.type(screen.getByRole('textbox', { name: '新书名称' }), 'New world')
  await user.click(screen.getByRole('button', { name: '创建书籍并生成计划' }))
  expect(beforeCreate).toHaveBeenCalledOnce()
  expect(createBook).not.toHaveBeenCalled()
  expect(exchange).not.toHaveBeenCalled()
  expect(onCreated).not.toHaveBeenCalled()
})

it('keeps the new-book form after creation fails and never plans without a project', async () => {
  const user = userEvent.setup()
  vi.mocked(createBook).mockRejectedValueOnce(new Error('Book creation failed'))
  render(<ImportDialog preview={{ ...preview, candidates: [{ ...preview.candidates[1], resources: [preview.candidates[1].resources[0]] }] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  expect(screen.queryByRole('combobox', { name: '目标作品' })).not.toBeInTheDocument()
  await user.type(screen.getByRole('textbox', { name: '新书名称' }), 'New world')
  await user.click(screen.getByRole('button', { name: '创建书籍并生成计划' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Book creation failed')
  expect(screen.getByRole('textbox', { name: '新书名称' })).toHaveValue('New world')
  expect(exchange).not.toHaveBeenCalled()
})

it('asks for the import mode before showing the corresponding book fields', async () => {
  const user = userEvent.setup()
  render(<ImportDialog preview={{ ...preview, candidates: [{ ...preview.candidates[1], resources: [preview.candidates[1].resources[0]] }] }} onClose={vi.fn()} onInstalled={vi.fn()} />)
  const mode = screen.getByRole('radiogroup', { name: '导入方式' })
  expect(mode).toBeVisible()
  expect(screen.getByRole('radio', { name: '导入到已有书籍' })).toBeChecked()
  expect(screen.getByRole('combobox', { name: '目标作品' })).toBeVisible()
  expect(screen.queryByRole('textbox', { name: '新书名称' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  await user.type(screen.getByRole('textbox', { name: '新书名称' }), 'New world')
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: '导入到已有书籍' }))
  expect(screen.getByRole('combobox', { name: '目标作品' })).toBeVisible()
  expect(screen.queryByRole('textbox', { name: '新书名称' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '生成安装计划' })).toBeDisabled()
  await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
  expect(screen.getByRole('textbox', { name: '新书名称' })).toHaveValue('New world')
  expect(createBook).not.toHaveBeenCalled()
})

const downloaded: Preview = { ...preview, candidates: [preview.candidates[1]] }
it('downloads a supplied source once in StrictMode and releases it on close', async () => {
  vi.mocked(previewSource).mockResolvedValue(downloaded)
  const onClose = vi.fn()
  const view = render(<StrictMode><ImportDialog source={preview.source} onClose={onClose} onInstalled={vi.fn()} /></StrictMode>)
  expect(screen.getByRole('status')).toHaveTextContent('正在下载并检查资源包')
  expect(await screen.findByRole('checkbox', { name: 'Selected image' })).toBeChecked()
  expect(previewSource).toHaveBeenCalledExactlyOnceWith(preview.source)
  expect(screen.queryByLabelText('来源链接')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  expect(onClose).toHaveBeenCalledOnce()
  view.unmount()
  expect(discardPreview).toHaveBeenCalledExactlyOnceWith(downloaded)
})
it('allows retry after a source download fails', async () => {
  vi.mocked(previewSource).mockRejectedValueOnce(new Error('Download failed')).mockResolvedValueOnce(downloaded)
  render(<ImportDialog source={preview.source} onClose={vi.fn()} onInstalled={vi.fn()} />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Download failed')
  fireEvent.click(screen.getByRole('button', { name: '重新加载' }))
  expect(await screen.findByRole('checkbox', { name: 'Selected image' })).toBeChecked()
  expect(previewSource).toHaveBeenCalledTimes(2)
})
it('allows cancelling a download and releases its late result', async () => {
  let resolve!: (result: Preview) => void
  vi.mocked(previewSource).mockReturnValue(new Promise(done => { resolve = done }))
  const onClose = vi.fn()
  const view = render(<ImportDialog source={preview.source} onClose={onClose} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '取消' }))
  expect(onClose).toHaveBeenCalledOnce()
  view.unmount()
  await act(async () => resolve(downloaded))
  expect(discardPreview).toHaveBeenCalledExactlyOnceWith(downloaded)
})

const cardPreview: Preview = {
 ...preview,
 character: { name: 'Explorer', target_path: '', entry_count: 0, item_count: 1, item_ids: [], opening_preset_count: 0, user_placeholder_found: false, compatibility: { capabilities: [], sanitized_runtime: [], discarded_extensions: [], warnings: [], ignored_loading_rules: false }, message: '', resident_lore_bytes: 0, classification_mode: 'heuristic', classification_counts: {}, uncertain_type_count: 0 },
 candidates: [{ candidate_id: 'card', package: { id: 'card', name: 'Explorer' }, format: 'denova.resource-pack', resources: [
  { id: 'lore', kind: 'lore.collection', name: 'Explorer lore', path: 'lore.json', digest: '1' },
  { id: 'cover', kind: 'project.cover', name: 'Book portrait', path: 'cover.json', digest: '2' },
 ] }],
}
it('excludes the character book cover when importing into an existing book', async () => {
 vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Explorer' } } })
 render(<ImportDialog preview={cardPreview} projectID="existing" onClose={vi.fn()} onInstalled={vi.fn()} />)
 expect(screen.queryByText('Book portrait')).not.toBeInTheDocument()
 fireEvent.click(screen.getByRole('button', { name: '生成安装计划' }))
 await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({ project_id: 'existing', resources: ['lore'] })))
})
it('keeps the character book cover for a new book including a plan retry', async () => {
 const user = userEvent.setup()
 vi.mocked(createBook).mockResolvedValue({ project_id: 'new-card', workspace: '/books/card', book_meta: { title: 'Explorer' } } as Awaited<ReturnType<typeof createBook>>)
 vi.mocked(exchange).mockRejectedValueOnce(new Error('Retry card plan')).mockResolvedValueOnce({ plan_id: 'plan', items: [], installation: { package: { name: 'Explorer' } } })
 render(<ImportDialog preview={cardPreview} onClose={vi.fn()} onInstalled={vi.fn()} />)
 await user.click(screen.getByRole('radio', { name: '导入成新书籍' }))
 await user.type(screen.getByRole('textbox', { name: '新书名称' }), 'Explorer')
 await user.click(screen.getByRole('button', { name: '创建书籍并生成计划' }))
 await screen.findByText('Retry card plan')
 await user.click(screen.getByRole('button', { name: '生成安装计划' }))
 await screen.findByText('确认安装计划')
 expect(createBook).toHaveBeenCalledOnce()
 for (const [, request] of vi.mocked(exchange).mock.calls) expect(request).toEqual(expect.objectContaining({ project_id: 'new-card', resources: ['lore', 'cover'] }))
})

import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HomeView } from './HomeView'
import { createBook, downloadBookExport, exportBook, generateBookCover, getBookInfo, uploadBookCover } from '@/lib/api'
import { getImagePresets } from '@/features/interactive/api'
import { fetchSettings } from '@/features/settings/api'
import { toast } from 'sonner'

vi.mock('@/lib/api', () => ({
  bookCoverURL: (path: string, version?: string) => `/api/books/cover?path=${encodeURIComponent(path)}${version ? `&v=${encodeURIComponent(version)}` : ''}`,
  createBook: vi.fn(),
  downloadBookExport: vi.fn(),
  exportBook: vi.fn(),
  generateBookCover: vi.fn(),
  getBookInfo: vi.fn(),
  removeBook: vi.fn(),
  reorderBooks: vi.fn(),
  switchWorkspace: vi.fn(),
  updateBookInfo: vi.fn(),
  uploadBookCover: vi.fn(),
}))

vi.mock('@/features/interactive/api', () => ({
  getImagePresets: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

describe('HomeView book covers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getImagePresets).mockResolvedValue([
      { version: 2, id: 'realistic', name: '写实', description: '', custom: false },
    ])
    vi.mocked(fetchSettings).mockResolvedValue({ effective: { ide_image_preset_id: 'realistic' } } as any)
    vi.mocked(getBookInfo).mockResolvedValue({
      title: '星河边境',
      author: '',
      description: '舰队与边城。',
      created_at: '',
      updated_at: '',
    })
    vi.mocked(generateBookCover).mockResolvedValue({
      schema: 'book_cover.v1',
      cover_path: 'assets/image/cover.png',
      source_path: 'assets/image/covers/run/cover.png',
      meta_path: 'assets/image/covers/run/meta.json',
      cover_updated_at: 'new-version',
      profile_id: 'default',
      provider: 'openai',
      model: 'gpt-image-1',
    })
    vi.mocked(createBook).mockResolvedValue({
      workspace: '/books/new',
      book_meta: {
        title: '新书',
        author: '',
        description: '',
        created_at: '',
        updated_at: '',
      },
    })
    vi.mocked(uploadBookCover).mockResolvedValue({
      schema: 'book_cover.v1',
      cover_path: 'assets/image/cover.png',
      source_path: 'assets/image/covers/run/upload.png',
      meta_path: 'assets/image/covers/run/meta.json',
      cover_updated_at: 'upload-version',
      profile_id: 'manual',
      provider: 'user_upload',
      model: 'manual',
    })
    vi.mocked(exportBook).mockResolvedValue({
      filename: '星河边境.txt',
      blob: new Blob(['星河边境']),
    })
  })

  it('uses the fixed book cover endpoint with the cover version', async () => {
    renderHome()

    const covers = await screen.findAllByRole('img', { name: '星河边境' })
    expect(covers.some((img) => (img as HTMLImageElement).src.includes('/api/books/cover'))).toBe(true)
    expect(covers.some((img) => (img as HTMLImageElement).src.includes('v=old-version'))).toBe(true)
  })

  it('tries the fixed book cover endpoint even when the book list has no cover version', async () => {
    renderHome({ books: [{
      name: '星河边境',
      path: '/books/star',
      author: '',
      last_opened_at: '',
    }] })

    const covers = await screen.findAllByRole('img', { name: '星河边境' })
    expect(covers.some((img) => (img as HTMLImageElement).src.includes('/api/books/cover'))).toBe(true)
    expect(covers.every((img) => !(img as HTMLImageElement).src.includes('v='))).toBe(true)
  })

  it('generates a cover from the edit dialog and refreshes the local version', async () => {
    const user = userEvent.setup()
    const onBooksChange = vi.fn()
    renderHome({ onBooksChange })

    await user.click(await screen.findByRole('button', { name: '编辑信息' }))
    const dialog = await screen.findByRole('dialog', { name: '编辑信息' })
    const presetSelect = within(dialog).getByLabelText('封面图像方案')
    await waitFor(() => expect(presetSelect).toHaveValue('realistic'))
    await user.type(within(dialog).getByPlaceholderText('生成要求（选填）'), '冷色调')
    await user.click(within(dialog).getByRole('button', { name: '生成封面' }))

    await waitFor(() => {
      expect(generateBookCover).toHaveBeenCalledWith({
        path: '/books/star',
        imagePresetId: 'realistic',
        instruction: '冷色调',
      })
    })
    expect(onBooksChange).toHaveBeenCalled()
    await waitFor(() => {
      const covers = screen.getAllByRole('img', { name: '星河边境' })
      expect(covers.some((img) => (img as HTMLImageElement).src.includes('v=new-version'))).toBe(true)
    })
  })

  it('opens book creation in the shared dialog and uploads a selected cover after creating', async () => {
    const user = userEvent.setup()
    const onSwitch = vi.fn()
    const onBooksChange = vi.fn()
    renderHome({ onSwitch, onBooksChange })

    await user.click(screen.getByRole('button', { name: '新建书籍' }))
    const dialog = await screen.findByRole('dialog', { name: '新建书籍' })
    await user.type(within(dialog).getByPlaceholderText('书名（必填）'), '新书')
    const file = new File(['cover'], 'cover.png', { type: 'image/png' })
    await user.upload(within(dialog).getByLabelText('封面文件'), file)
    expect(within(dialog).getByText('封面已选择，创建书籍时保存')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: '创建' }))

    await waitFor(() => {
      expect(createBook).toHaveBeenCalledWith('新书', undefined, undefined)
      expect(uploadBookCover).toHaveBeenCalledWith('/books/new', file)
    })
    expect(onSwitch).toHaveBeenCalledWith('/books/new')
    expect(onBooksChange).toHaveBeenCalled()
  })

  it('can create a book directly from the cover generate action', async () => {
    const user = userEvent.setup()
    const onSwitch = vi.fn()
    const onBooksChange = vi.fn()
    renderHome({ onSwitch, onBooksChange })

    await user.click(screen.getByRole('button', { name: '新建书籍' }))
    const dialog = await screen.findByRole('dialog', { name: '新建书籍' })
    await user.type(within(dialog).getByPlaceholderText('书名（必填）'), '新书')
    await user.type(within(dialog).getByPlaceholderText('生成要求（选填）'), '冷色调')
    await user.click(within(dialog).getByRole('button', { name: '新建并生成封面' }))

    await waitFor(() => {
      expect(createBook).toHaveBeenCalledWith('新书', undefined, undefined)
      expect(generateBookCover).toHaveBeenCalledWith({
        path: '/books/new',
        imagePresetId: 'realistic',
        instruction: '冷色调',
      })
    })
    expect(onSwitch).toHaveBeenCalledWith('/books/new')
    expect(onBooksChange).toHaveBeenCalled()
  })

  it('exports a book as txt without switching workspaces', async () => {
    const user = userEvent.setup()
    const onSwitch = vi.fn()
    renderHome({ onSwitch })

    const exportButtons = await screen.findAllByRole('button', { name: '导出 TXT' })
    await user.click(exportButtons[0])

    await waitFor(() => {
      expect(exportBook).toHaveBeenCalledWith({ path: '/books/star', format: 'txt' })
      expect(downloadBookExport).toHaveBeenCalledWith({
        filename: '星河边境.txt',
        blob: expect.any(Blob),
      })
    })
    expect(onSwitch).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledWith('已开始下载 星河边境.txt')
  })

  it('shows an error when txt export fails', async () => {
    const user = userEvent.setup()
    vi.mocked(exportBook).mockRejectedValueOnce(new Error('没有可导出的非空章节'))
    renderHome()

    const exportButtons = await screen.findAllByRole('button', { name: '导出 TXT' })
    await user.click(exportButtons[0])

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('导出失败', {
        description: '没有可导出的非空章节',
      })
    })
    expect(downloadBookExport).not.toHaveBeenCalled()
  })
})

function renderHome(overrides: Partial<Parameters<typeof HomeView>[0]> = {}) {
  return render(
    <HomeView
      workspace="/books/star"
      novaDir="/nova"
      books={[{
        name: '星河边境',
        path: '/books/star',
        author: '',
        cover_updated_at: 'old-version',
        last_opened_at: '',
      }]}
      onSwitch={vi.fn()}
      onBooksChange={vi.fn()}
      {...overrides}
    />,
  )
}

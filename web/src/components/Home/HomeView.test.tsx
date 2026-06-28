import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HomeView } from './HomeView'
import { generateBookCover, getBookInfo } from '@/lib/api'
import { getImagePresets } from '@/features/interactive/api'
import { fetchSettings } from '@/features/settings/api'

vi.mock('@/lib/api', () => ({
  bookCoverURL: (path: string, version?: string) => `/api/books/cover?path=${encodeURIComponent(path)}${version ? `&v=${encodeURIComponent(version)}` : ''}`,
  createBook: vi.fn(),
  generateBookCover: vi.fn(),
  getBookInfo: vi.fn(),
  removeBook: vi.fn(),
  reorderBooks: vi.fn(),
  switchWorkspace: vi.fn(),
  updateBookInfo: vi.fn(),
}))

vi.mock('@/features/interactive/api', () => ({
  getImagePresets: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
}))

describe('HomeView book covers', () => {
  beforeEach(() => {
    vi.mocked(getImagePresets).mockResolvedValue([
      { version: 2, id: 'realistic', name: '写实', description: '', tags: [], custom: false },
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

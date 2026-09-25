import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { ImportDialog } from './ImportDialog'
import { exchange, type Preview } from './api'
vi.mock('@/lib/api', () => ({ getBooks: vi.fn().mockResolvedValue([]) }))
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), exchange: vi.fn() }))
const preview: Preview = {
  preview_id: 'frozen', source: { kind: 'github', url: 'https://github.com/test/package' }, expires_at: '2030-01-01',
  candidates: [{ candidate_id: 'first', package: { id: 'first', name: 'Other package' }, format: 'skill', resources: [] }, {
    candidate_id: 'second', package: { id: 'second', name: 'Chosen package' }, format: 'denova.resource-pack', resources: [
      { id: 'lore', kind: 'lore.item', name: 'Not selected', path: 'lore.json', digest: '1' },
      { id: 'image', kind: 'preset.image', name: 'Selected image', path: 'image.json', requires: ['style'], digest: '2' },
      { id: 'style', kind: 'style.reference', name: 'Required style', path: 'style.md', digest: '3' },
    ],
  }],
}
beforeEach(() => vi.mocked(exchange).mockReset())
it('plans exactly the selected candidate and resources, while displaying its dependency', async () => {
  vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Chosen package' } } })
  render(<ImportDialog preview={preview} selection={{ candidateID: 'second', resourceIDs: ['image'] }} previewOwner="caller" onClose={vi.fn()} onInstalled={vi.fn()} />)
  expect(screen.getByText(/Selected image/)).toBeVisible()
  expect(screen.getByText(/Required style/)).toBeVisible()
  expect(screen.queryByText('Not selected')).not.toBeInTheDocument()
  expect(screen.queryByText('目标作品')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('来源链接')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '生成安装计划' }))
  await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({ preview_id: 'frozen', candidate_id: 'second', resources: ['image'] })))
})
it('returns to the same borrowed preview without deleting it', () => {
  const onClose = vi.fn()
  render(<ImportDialog preview={preview} selection={{ candidateID: 'second', resourceIDs: ['image'] }} previewOwner="caller" onClose={onClose} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '返回修改选择' }))
  expect(onClose).toHaveBeenCalledOnce()
  expect(exchange).not.toHaveBeenCalled()
})

it('submits every resource in the selected lore group and omits the deselected openings group', async () => {
  vi.mocked(exchange).mockResolvedValue({ plan_id: 'plan', items: [], installation: { package: { name: 'Story package' } } })
  const groupedPreview: Preview = { ...preview, candidates: [{
    candidate_id: 'story', package: { id: 'story', name: 'Story package' }, format: 'denova.resource-pack', resources: [
      { id: 'harbor', kind: 'lore.item', name: 'Harbor', path: 'harbor.json', digest: '1' },
      { id: 'island', kind: 'lore.item', name: 'Island', path: 'island.json', digest: '2' },
      { id: 'dawn', kind: 'game.opening', name: 'Dawn', path: 'dawn.json', digest: '3' },
      { id: 'dusk', kind: 'game.opening', name: 'Dusk', path: 'dusk.json', digest: '4' },
    ],
  }] }
  render(<ImportDialog preview={groupedPreview} projectID="target" onClose={vi.fn()} onInstalled={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: '清空选择' }))
  fireEvent.change(screen.getByRole('textbox', { name: '搜索名称、描述或类型' }), { target: { value: 'Harbor' } })
  fireEvent.click(screen.getByRole('checkbox', { name: '全选资料' }))
  fireEvent.click(screen.getByRole('button', { name: '生成安装计划' }))
  await waitFor(() => expect(exchange).toHaveBeenCalledWith('/plans', expect.objectContaining({
    preview_id: 'frozen', candidate_id: 'story', resources: ['harbor', 'island'], project_id: 'target',
  })))
})

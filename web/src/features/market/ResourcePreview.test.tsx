import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { ResourcePreview } from './ResourcePreview'
import { exchange } from './api'
vi.mock('./api', async (original) => ({ ...await original<typeof import('./api')>(), exchange: vi.fn() }))
it('opens a single file automatically and renders lore prose separately from JSON', async () => {
  const files = [{ path: 'lore.json', bytes: 200 }]
  vi.mocked(exchange).mockResolvedValueOnce({ files, binary: false, truncated: false }).mockResolvedValueOnce({ files, path: 'lore.json', content: JSON.stringify({ enabled: true, content: '# Harbor\nA quiet harbor.' }), binary: false, truncated: false })
  render(<ResourcePreview previewID="p" candidateID="c" resourceID="r" />)
  expect(await screen.findByRole('heading', { name: 'Harbor' })).toBeVisible()
  expect(screen.getByText('A quiet harbor.')).toBeVisible()
  expect(screen.getByText('查看源文件')).toBeVisible()
})

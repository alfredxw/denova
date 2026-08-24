import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { WorkspaceSearchResult } from '@/lib/api'
import { SearchPanel } from './SearchPanel'

const api = vi.hoisted(() => ({
  searchWorkspace: vi.fn(),
  replaceWorkspace: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api')>(),
  ...api,
}))

describe('SearchPanel with GroupedVirtuoso', () => {
  beforeEach(() => vi.clearAllMocks())

  it('keeps every result under its own file group', async () => {
    const user = userEvent.setup()
    const onSelectResult = vi.fn()
    api.searchWorkspace.mockResolvedValue([
      result('interactive/story/first.idx.json', 700, 'alpha from first file'),
      result('interactive/story/second.idx.json', 700, 'alpha from second file'),
    ])

    render(
      <VirtuosoMockContext.Provider value={{ itemHeight: 48, viewportHeight: 320 }}>
        <div style={{ height: 320 }}>
          <SearchPanel projectId="book-a" onSelectResult={onSelectResult} />
        </div>
      </VirtuosoMockContext.Provider>,
    )

    await user.type(screen.getByPlaceholderText('搜索当前书籍...'), 'alpha')
    const firstResult = await screen.findByText(exactElementText('alpha from first file'))
    const firstButton = firstResult.closest('button')
    expect(firstButton).not.toBeNull()
    if (!firstButton) throw new Error('Expected the first grouped result to be clickable')
    await user.click(firstButton)

    expect(onSelectResult).toHaveBeenCalledWith(
      expect.objectContaining({ path: 'interactive/story/first.idx.json', line: 700 }),
      'alpha',
    )
  })
})

function result(path: string, line: number, preview: string): WorkspaceSearchResult {
  return { path, line, column: 1, preview, match_text: 'alpha' }
}

function exactElementText(text: string) {
  return (_content: string, element: Element | null) => element?.tagName === 'P' && element.textContent === text
}
